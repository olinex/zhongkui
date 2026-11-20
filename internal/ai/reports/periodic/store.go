package periodic

import (
	// 标准库
	"context"
	"encoding/json"
	"strings"
	"time"

	// 第三方
	"github.com/robfig/cron/v3"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	// 项目内部
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
	"zhongkuisec.com/zhongkui/internal/pkg/validator"
)

// Store 提供报告计划、运行记录与关注关系的持久化读写，全部操作基于 GORM（database.DB）。
// 所有计划与运行记录读写都以命名空间为隔离维度，跨命名空间一律按不存在处理。
type Store struct {
	// db core 库连接（periodic_report_plans / periodic_report_runs / periodic_report_plan_watches 读写）。
	db *database.DB
}

// NewStore 构造 Store；db 为空时返回英文错误。
func NewStore(db *database.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("periodic-report store: nil database")
	}
	return &Store{db: db}, nil
}

// EligibleSkills 返回给定命名空间下可作为周期性报告计划来源的技能：仅自定义、已启用、标记
// 可周期执行、固定参数能解析出合法 timeOffset/timeWindow 且输入字段恰好含一个 nowTime 字段
// 的技能，按 created_at 降序。单行数据解析失败时跳过该行，不让一行坏数据拖垮整个选项列表。
func (s *Store) EligibleSkills(ctx context.Context, namespace string) ([]*modelsai.AgentSkill, error) {
	var rows []*modelsai.AgentSkill
	if err := s.db.WithContext(ctx).
		Where("namespace_name = ? AND source = ? AND enabled = ? AND periodic_capable = ?",
			strings.TrimSpace(namespace), modelsai.SkillSourceCustom, true, true).
		Order("created_at DESC").
		Find(&rows).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	eligible := make([]*modelsai.AgentSkill, 0, len(rows))
	for _, row := range rows {
		if skillEligible(row) {
			eligible = append(eligible, row)
		}
	}
	return eligible, nil
}

// ValidatePlanInput 校验报告计划输入并返回其使用的技能行：先校验 cron 表达式（失败
// ErrScheduleInvalid），再校验技能资格（同 EligibleSkills 条件，不合格 ErrSkillNotEligible），
// 再校验必填输入（跳过运行时注入的 nowTime，缺失 ErrInputMissing），最后做偏置键与类型校验
// （不合法 ErrInputInvalid）。保留天数的默认值与范围由视图层或命令层的结构体标签统一校验后传入。
func (s *Store) ValidatePlanInput(ctx context.Context, namespace string, in PlanInput) (*modelsai.AgentSkill, error) {
	if _, err := cron.ParseStandard(strings.TrimSpace(in.CronSpec)); err != nil {
		return nil, ErrScheduleInvalid
	}
	var skill modelsai.AgentSkill
	err := s.db.WithContext(ctx).
		Where("id = ? AND namespace_name = ? AND source = ? AND enabled = ? AND periodic_capable = ?",
			in.SkillID, strings.TrimSpace(namespace), modelsai.SkillSourceCustom, true, true).
		First(&skill).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrSkillNotEligible
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if !skillEligible(&skill) {
		return nil, ErrSkillNotEligible
	}
	fields, err := modelsai.ParseSkillInputFields(skill.InputFields)
	if err != nil {
		return nil, ErrSkillNotEligible
	}
	for _, field := range fields {
		if field.Name == modelsai.SkillInputFieldNameNowTime {
			continue
		}
		if field.Required && !requiredInputPresent(field, in.InputValues) {
			return nil, ErrInputMissing
		}
	}
	if err := ValidatePlanBias(&skill, in.InputValues); err != nil {
		return nil, err
	}
	return &skill, nil
}

// ValidatePlanBias 校验计划偏置输入：键必须在技能固定参数或输入字段内，禁止覆盖运行时的
// nowTime，值须与原字段的 JSON 类型一致（输入字段按字段类型，固定字段按现有值的 JSON 类型），
// timeOffset/timeWindow 覆盖后仍须能解析为合法时长且 timeWindow > 0。违规返回包装后的
// ErrInputInvalid，供创建/更新计划与运行期复用。
func ValidatePlanBias(skill *modelsai.AgentSkill, values map[string]json.RawMessage) error {
	if skill == nil {
		return ErrInputInvalid
	}
	fixedFields, err := modelsai.ParseSkillFixedFields(skill.FixedFields)
	if err != nil {
		return ErrInputInvalid
	}
	inputFields, err := modelsai.ParseSkillInputFields(skill.InputFields)
	if err != nil {
		return ErrInputInvalid
	}
	inputByName := make(map[string]modelsai.SkillInputField, len(inputFields))
	for _, field := range inputFields {
		inputByName[field.Name] = field
	}
	for key, value := range values {
		if key == modelsai.SkillInputFieldNameNowTime {
			return errors.Wrapf(ErrInputInvalid, "input key %q is injected at runtime and cannot be overridden", key)
		}
		if field, ok := inputByName[key]; ok {
			if !modelsai.InputFieldAcceptsJSON(field.Type, value) {
				kind, kindErr := validator.JSONValueKind(value)
				if kindErr != nil {
					kind = validator.JSONKindInvalid
				}
				return errors.Wrapf(ErrInputInvalid,
					"input key %q expects a %s value, got %s", key, field.Type, kind)
			}
			continue
		}
		existing, ok := fixedFields[key]
		if !ok {
			return errors.Wrapf(ErrInputInvalid, "input key %q is neither a fixed field nor an input field of the skill", key)
		}
		kind, err := validator.JSONValueKind(value)
		if err != nil {
			return errors.Wrapf(ErrInputInvalid, "input key %q has an invalid JSON value", key)
		}
		existingKind, err := validator.JSONValueKind(existing)
		if err != nil {
			return ErrInputInvalid
		}
		if kind != existingKind {
			return errors.Wrapf(ErrInputInvalid,
				"input key %q expects a %s value, got %s", key, existingKind, kind)
		}
	}
	merged := modelsai.MergeParameters(fixedFields, values)
	if _, err := modelsai.ParsePeriodicWindowFromFields(merged); err != nil {
		return errors.Wrapf(ErrInputInvalid, "merged periodic window is invalid: %v", err)
	}
	return nil
}

// CreatePlan 在给定命名空间下创建一条启用的报告计划：校验通过后落库，输入实参编码为 JSON
// 对象（空实参存空对象）；技能名称不落库，展示时按 skill_id 关联读取。
func (s *Store) CreatePlan(ctx context.Context, namespace string, userID uint, in PlanInput) (*modelsai.PeriodicReportPlan, error) {
	namespace = strings.TrimSpace(namespace)
	if userID == 0 {
		// user_id 为 users(id) 外键，0 不是合法属主。
		return nil, errors.New("periodic-report: user id is required")
	}
	if _, err := s.ValidatePlanInput(ctx, namespace, in); err != nil {
		return nil, err
	}
	values, err := encodeInputValues(in.InputValues)
	if err != nil {
		return nil, err
	}
	plan := &modelsai.PeriodicReportPlan{
		NamespaceName: namespace,
		Name:          in.Name,
		Description:   in.Description,
		SkillID:       in.SkillID,
		CronSpec:      strings.TrimSpace(in.CronSpec),
		InputValues:   values,
		Enabled:       true,
		UserID:        userID,
		RetentionDays: in.RetentionDays,
		ExpiresAt:     in.ExpiresAt,
	}
	if err := s.db.WithContext(ctx).Create(plan).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return plan, nil
}

// UpdatePlan 更新给定命名空间下的一条报告计划并返回更新后的计划：先按命名空间确认计划存在
// （不存在 ErrPlanNotFound），校验通过后整体覆盖可编辑字段，保留启用状态、会话 ID 与属主。
// 显式刷新 updated_at，保证同微秒内的连续更新也能体现。
func (s *Store) UpdatePlan(ctx context.Context, namespace string, id uint, in PlanInput) (*modelsai.PeriodicReportPlan, error) {
	namespace = strings.TrimSpace(namespace)
	plan, err := s.GetPlan(ctx, namespace, id)
	if err != nil {
		return nil, err
	}
	if _, err := s.ValidatePlanInput(ctx, namespace, in); err != nil {
		return nil, err
	}
	values, err := encodeInputValues(in.InputValues)
	if err != nil {
		return nil, err
	}
	plan.Name = in.Name
	plan.Description = in.Description
	plan.SkillID = in.SkillID
	plan.CronSpec = strings.TrimSpace(in.CronSpec)
	plan.InputValues = values
	plan.RetentionDays = in.RetentionDays
	plan.ExpiresAt = in.ExpiresAt
	// cron 可能已被修改，清空下次执行时间让分发器按新表达式重新计算
	plan.NextRunAt = nil
	updatedAt := time.Now()
	if !updatedAt.After(plan.UpdatedAt) {
		updatedAt = plan.UpdatedAt.Add(time.Millisecond)
	}
	plan.UpdatedAt = updatedAt
	if err := s.db.WithContext(ctx).Save(plan).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return plan, nil
}

// GetPlan 按 ID 查询给定命名空间下的一条报告计划；不存在或跨命名空间时返回 ErrPlanNotFound。
func (s *Store) GetPlan(ctx context.Context, namespace string, id uint) (*modelsai.PeriodicReportPlan, error) {
	var plan modelsai.PeriodicReportPlan
	err := s.db.WithContext(ctx).
		Where("id = ? AND namespace_name = ?", id, strings.TrimSpace(namespace)).
		First(&plan).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrPlanNotFound
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &plan, nil
}

// GetPlanByID 按 ID 查询一条未删除的报告计划，供报告生成器内部使用（调用方已由调度配置
// 携带的 ID 定位计划，无需再传命名空间）；不存在时返回 ErrPlanNotFound。
func (s *Store) GetPlanByID(ctx context.Context, id uint) (*modelsai.PeriodicReportPlan, error) {
	var plan modelsai.PeriodicReportPlan
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&plan).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrPlanNotFound
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &plan, nil
}

// SkillNamesByIDs 批量返回技能 ID 到名称的映射，供计划列表展示关联技能名。查询包含已软删除的
// 技能行，保证历史计划在技能下线后仍能显示原名称；ids 为空时返回空映射。
func (s *Store) SkillNamesByIDs(ctx context.Context, ids []uint) (map[uint]string, error) {
	names := make(map[uint]string, len(ids))
	if len(ids) == 0 {
		return names, nil
	}
	var rows []struct {
		// ID 技能 ID
		ID uint
		// Name 技能名称
		Name string
	}
	if err := s.db.WithContext(ctx).Unscoped().Model(&modelsai.AgentSkill{}).
		Select("id", "name").Where("id IN ?", ids).Scan(&rows).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	for _, row := range rows {
		names[row.ID] = row.Name
	}
	return names, nil
}

// PlanNamesByIDs 批量返回计划 ID 到名称的映射，供信息流第二趟回填计划名称；计划已软删除时
// 映射中缺失该 ID（调用方以空名称展示）。ids 为空时返回空映射。
func (s *Store) PlanNamesByIDs(ctx context.Context, namespace string, ids []uint) (map[uint]string, error) {
	names := make(map[uint]string, len(ids))
	if len(ids) == 0 {
		return names, nil
	}
	var rows []struct {
		// ID 计划 ID
		ID uint
		// Name 计划名称
		Name string
	}
	if err := s.db.WithContext(ctx).Model(&modelsai.PeriodicReportPlan{}).
		Select("id", "name").
		Where("namespace_name = ? AND id IN ?", strings.TrimSpace(namespace), ids).
		Scan(&rows).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	for _, row := range rows {
		names[row.ID] = row.Name
	}
	return names, nil
}

// SetPlanEnabled 设置给定命名空间下一条计划的启停状态并返回最新行；不存在或跨命名空间时
// 返回 ErrPlanNotFound。启用时清空 next_run_at，交给分发器按当前 cron 重新计算；停用时保留原值。
func (s *Store) SetPlanEnabled(ctx context.Context, namespace string, id uint, enabled bool) (*modelsai.PeriodicReportPlan, error) {
	namespace = strings.TrimSpace(namespace)
	updates := map[string]any{"enabled": enabled}
	if enabled {
		updates["next_run_at"] = nil
	}
	if err := s.db.WithContext(ctx).Model(&modelsai.PeriodicReportPlan{}).
		Where("id = ? AND namespace_name = ?", id, namespace).
		Updates(updates).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return s.GetPlan(ctx, namespace, id)
}

// DeletePlan 软删除给定命名空间下的一条计划并在删除前返回该计划；
// 不存在或跨命名空间时返回 ErrPlanNotFound。
func (s *Store) DeletePlan(ctx context.Context, namespace string, id uint) (*modelsai.PeriodicReportPlan, error) {
	namespace = strings.TrimSpace(namespace)
	plan, err := s.GetPlan(ctx, namespace, id)
	if err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).
		Where("id = ? AND namespace_name = ?", id, namespace).
		Delete(&modelsai.PeriodicReportPlan{}).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return plan, nil
}

// CreateRunIfAbsent 幂等创建一条报告运行记录：同一计划同一触发点重复投递时命中
// (plan_id, trigger_at) 唯一约束，插入被忽略并返回 false。
func (s *Store) CreateRunIfAbsent(ctx context.Context, run *modelsai.PeriodicReportRun) (bool, error) {
	result := s.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(run)
	if result.Error != nil {
		return false, errors.WithStack(result.Error)
	}
	return result.RowsAffected > 0, nil
}

// MarkRunRunning 把一条运行记录标记为运行中并记录开始时间。
func (s *Store) MarkRunRunning(ctx context.Context, id uint, startedAt time.Time) error {
	if err := s.db.WithContext(ctx).Model(&modelsai.PeriodicReportRun{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":     modelsai.PeriodicReportRunStatusRunning,
			"started_at": startedAt,
		}).Error; err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// MarkRunSuccess 把一条运行记录标记为成功并写入产出对象键、标题、大小与结束时间；返回值表示
// 是否命中未删除的记录。生成期间记录被用户删除时返回 false，调用方据此清理已写入的报告对象，
// 避免「先检查存在性、后写入」之间的竞态留下无主对象。
func (s *Store) MarkRunSuccess(ctx context.Context, id uint, objectKey, title string, size int64, finishedAt time.Time) (bool, error) {
	// 结构体 + Select 更新以触发 s3crypto 序列化器（map 更新不会走序列化器）；
	// Select 显式包含零值列（空 error），避免 Updates 跳过零值。
	result := s.db.WithContext(ctx).Model(&modelsai.PeriodicReportRun{}).
		Where("id = ?", id).
		Select("status", "object_key", "title", "size_bytes", "error", "finished_at").
		Updates(&modelsai.PeriodicReportRun{
			Status:     modelsai.PeriodicReportRunStatusSuccess,
			ObjectKey:  serializer.NewS3Crypto(objectKey),
			Title:      title,
			SizeBytes:  size,
			Error:      "",
			FinishedAt: &finishedAt,
		})
	if result.Error != nil {
		return false, errors.WithStack(result.Error)
	}
	return result.RowsAffected > 0, nil
}

// MarkRunFailed 把一条运行记录标记为失败并写入失败原因（截断到 2048 字节）与结束时间。
func (s *Store) MarkRunFailed(ctx context.Context, id uint, reason string, finishedAt time.Time) error {
	if len(reason) > 2048 {
		reason = reason[:2048]
	}
	if err := s.db.WithContext(ctx).Model(&modelsai.PeriodicReportRun{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":      modelsai.PeriodicReportRunStatusFailed,
			"error":       reason,
			"finished_at": finishedAt,
		}).Error; err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// GetRun 按 ID 查询给定命名空间下的一条运行记录；不存在或跨命名空间时返回 ErrRunNotFound。
func (s *Store) GetRun(ctx context.Context, namespace string, id uint) (*modelsai.PeriodicReportRun, error) {
	var run modelsai.PeriodicReportRun
	err := s.db.WithContext(ctx).
		Where("id = ? AND namespace_name = ?", id, strings.TrimSpace(namespace)).
		First(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrRunNotFound
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &run, nil
}

// ListWatchedPlanIDs 返回给定用户在该命名空间下关注的计划 ID 列表。
func (s *Store) ListWatchedPlanIDs(ctx context.Context, namespace string, userID uint) ([]uint, error) {
	ids := make([]uint, 0)
	if err := s.db.WithContext(ctx).Model(&modelsai.PeriodicReportPlanWatch{}).
		Where("namespace_name = ? AND user_id = ?", strings.TrimSpace(namespace), userID).
		Order("created_at DESC").
		Pluck("plan_id", &ids).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return ids, nil
}

// ListWatchedPlans 返回给定用户在该命名空间下关注的计划，按关注记录创建时间降序。
func (s *Store) ListWatchedPlans(ctx context.Context, namespace string, userID uint) ([]*modelsai.PeriodicReportPlan, error) {
	plans := make([]*modelsai.PeriodicReportPlan, 0)
	if err := s.db.WithContext(ctx).Model(&modelsai.PeriodicReportPlan{}).
		Joins("JOIN periodic_report_plan_watches ON periodic_report_plan_watches.plan_id = periodic_report_plans.id "+
			"AND periodic_report_plan_watches.namespace_name = periodic_report_plans.namespace_name "+
			"AND periodic_report_plan_watches.deleted_at IS NULL").
		Where("periodic_report_plans.namespace_name = ? AND periodic_report_plan_watches.user_id = ?",
			strings.TrimSpace(namespace), userID).
		Order("periodic_report_plan_watches.created_at DESC").
		Find(&plans).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return plans, nil
}

// SetWatch 幂等设置用户对计划的关系：watched 为 true 时先确认计划属于该命名空间（否则
// ErrPlanNotFound）再插入关注记录（已关注时不重复插入）；为 false 时软删除对应关注记录。
func (s *Store) SetWatch(ctx context.Context, namespace string, userID, planID uint, watched bool) error {
	namespace = strings.TrimSpace(namespace)
	watch := &modelsai.PeriodicReportPlanWatch{
		UserID:        userID,
		NamespaceName: namespace,
		PlanID:        planID,
	}
	if watched {
		if _, err := s.GetPlan(ctx, namespace, planID); err != nil {
			return err
		}
		if err := s.db.WithContext(ctx).
			Clauses(clause.OnConflict{DoNothing: true}).
			Create(watch).Error; err != nil {
			return errors.WithStack(err)
		}
		return nil
	}
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND namespace_name = ? AND plan_id = ?", userID, namespace, planID).
		Delete(&modelsai.PeriodicReportPlanWatch{}).Error; err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// ListAllPlans 返回全部命名空间下的报告计划，包含已软删除的计划，供清理流程使用。
func (s *Store) ListAllPlans(ctx context.Context) ([]*modelsai.PeriodicReportPlan, error) {
	plans := make([]*modelsai.PeriodicReportPlan, 0)
	if err := s.db.WithContext(ctx).Unscoped().Order("id ASC").Find(&plans).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return plans, nil
}

// ListExpiredPlans 返回失效时间不晚于 now 且仍启用的报告计划，供清理流程自动停用。
func (s *Store) ListExpiredPlans(ctx context.Context, now time.Time) ([]*modelsai.PeriodicReportPlan, error) {
	plans := make([]*modelsai.PeriodicReportPlan, 0)
	if err := s.db.WithContext(ctx).
		Where("expires_at IS NOT NULL AND expires_at <= ? AND enabled = ?", now, true).
		Find(&plans).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return plans, nil
}

// ListDispatchablePlans 返回当前应由分发器处理的计划：已启用、未到期，且 next_run_at 为 NULL
// （待计算）或已到 now（到期需投递）。按 next_run_at 升序（NULL 在前）；limit 大于 0 时限制条数。
func (s *Store) ListDispatchablePlans(ctx context.Context, now time.Time, limit int) ([]*modelsai.PeriodicReportPlan, error) {
	tx := s.db.WithContext(ctx).
		Where("enabled = ? AND (expires_at IS NULL OR expires_at > ?) AND (next_run_at IS NULL OR next_run_at <= ?)",
			true, now, now).
		Order("next_run_at ASC NULLS FIRST")
	if limit > 0 {
		tx = tx.Limit(limit)
	}
	plans := make([]*modelsai.PeriodicReportPlan, 0)
	if err := tx.Find(&plans).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return plans, nil
}

// SetRunSession 仅按运行记录 ID 更新本次运行使用的会话 ID，供报告生成器内部使用；
// sessionID 为 0 时写入 NULL（尚未创建或会话已删除）。
func (s *Store) SetRunSession(ctx context.Context, runID, sessionID uint) error {
	var value any
	if sessionID != 0 {
		value = sessionID
	}
	if err := s.db.WithContext(ctx).Model(&modelsai.PeriodicReportRun{}).
		Where("id = ?", runID).
		Update("session_id", value).Error; err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// SetNextRunAtIfNull 仅在计划 next_run_at 仍为 NULL 时写入下次执行时间，避免覆盖其它实例
// 已计算的触发点。
func (s *Store) SetNextRunAtIfNull(ctx context.Context, planID uint, next time.Time) error {
	if err := s.db.WithContext(ctx).Model(&modelsai.PeriodicReportPlan{}).
		Where("id = ? AND next_run_at IS NULL", planID).
		Update("next_run_at", next).Error; err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// SetNextRunAtIfUnchanged 仅在计划 next_run_at 仍等于 old 时推进到 next，返回是否命中。
// 未命中说明其它实例已推进该计划的触发点，调用方据此跳过重复投递。
func (s *Store) SetNextRunAtIfUnchanged(ctx context.Context, planID uint, old, next time.Time) (bool, error) {
	result := s.db.WithContext(ctx).Model(&modelsai.PeriodicReportPlan{}).
		Where("id = ? AND next_run_at = ?", planID, old).
		Update("next_run_at", next)
	if result.Error != nil {
		return false, errors.WithStack(result.Error)
	}
	return result.RowsAffected > 0, nil
}

// ListExpiredRuns 返回指定计划在 before 之前创建且已结束的运行记录（不含运行中记录），按
// created_at 升序；limit 大于 0 时限制返回条数。
func (s *Store) ListExpiredRuns(ctx context.Context, planID uint, before time.Time, limit int) ([]*modelsai.PeriodicReportRun, error) {
	tx := s.db.WithContext(ctx).Model(&modelsai.PeriodicReportRun{}).
		Where("plan_id = ? AND created_at < ? AND status <> ?", planID, before, modelsai.PeriodicReportRunStatusRunning).
		Order("created_at ASC")
	if limit > 0 {
		tx = tx.Limit(limit)
	}
	runs := make([]*modelsai.PeriodicReportRun, 0)
	if err := tx.Find(&runs).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return runs, nil
}

// DeleteRun 软删除一条运行记录。
func (s *Store) DeleteRun(ctx context.Context, id uint) error {
	if err := s.db.WithContext(ctx).Where("id = ?", id).Delete(&modelsai.PeriodicReportRun{}).Error; err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// skillEligible 判断技能是否满足周期报告资格：固定参数能解析出合法的 timeOffset/timeWindow，
// 且输入字段恰好含一个 name=nowTime、type=date 的字段。解析失败视为不合格。
func skillEligible(skill *modelsai.AgentSkill) bool {
	fixedFields, err := modelsai.ParseSkillFixedFields(skill.FixedFields)
	if err != nil {
		return false
	}
	if _, err := modelsai.ParsePeriodicWindowFromFields(fixedFields); err != nil {
		return false
	}
	inputFields, err := modelsai.ParseSkillInputFields(skill.InputFields)
	if err != nil {
		return false
	}
	count := 0
	for _, field := range inputFields {
		if field.Name != modelsai.SkillInputFieldNameNowTime {
			continue
		}
		if field.Type != modelsai.SkillInputFieldTypeDate {
			return false
		}
		count++
	}
	return count == 1
}

// requiredInputPresent 判断必填输入是否已提供：缺失或 JSON null 视为未提供；string 类型还要求
// 非空字符串（非字符串值交由偏置类型校验拒绝），其余类型只要存在且非 null 即视为已提供。
func requiredInputPresent(field modelsai.SkillInputField, values map[string]json.RawMessage) bool {
	raw, ok := values[field.Name]
	if !ok {
		return false
	}
	kind, err := validator.JSONValueKind(raw)
	if err != nil || kind == validator.JSONKindNull {
		return false
	}
	if field.Type == modelsai.SkillInputFieldTypeString {
		var text string
		if err := json.Unmarshal(raw, &text); err == nil {
			return strings.TrimSpace(text) != ""
		}
	}
	return true
}

// DecodeInputValues 把报告计划 input_values 列的 JSON 解码为带类型的键值映射：空值（nil 或空串）
// 与 JSON null 返回非 nil 的空 map；非 JSON 对象或非法 JSON 返回英文错误。值以 json.RawMessage
// 承载，保留数字、布尔、字符串、对象与数组的原始类型。
func DecodeInputValues(raw datatypes.JSON) (map[string]json.RawMessage, error) {
	values := make(map[string]json.RawMessage)
	if _, err := validator.DecodeOptionalObject(string(raw), &values); err != nil {
		return nil, errors.Newf("periodic-report: input values must be a JSON object: %v", err)
	}
	if values == nil {
		values = make(map[string]json.RawMessage)
	}
	return values, nil
}

// encodeInputValues 把技能输入实参编码为 JSON 对象；nil 或空映射编码为空对象。
func encodeInputValues(values map[string]json.RawMessage) (datatypes.JSON, error) {
	if len(values) == 0 {
		return datatypes.JSON([]byte("{}")), nil
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return datatypes.JSON(raw), nil
}
