// Package skill 提供 AI 技能（AgentSkill）的持久化存储：技能按命名空间隔离，
// 支持创建、详情、分页列表、更新、批量删除与启停。技能正文经 crypto 序列化器
// 加密落库，列表与详情按需取用，跨命名空间一律不可见。
//
// 除命名空间级自定义技能外，本包另承载内置技能：内置技能是 agent_skills 中 source='builtin'
// 的注册行（固定落在系统命名空间，名称 / 明文描述 / 启停由 migrate fixture 注入，正文为空），
// 技能正文由代码注册表 BuiltinSkills 按名提供（见 builtin.go），与内置工具的组织方式同构。
// 内置技能跨命名空间恒可用、不落租户表。两者都由视图层统一以 agent_skills 行 ID 选择，
// 不再区分引用前缀。
package skill

import (
	// 标准库
	"context"
	"encoding/json"
	"strings"
	"time"

	// 第三方
	"gorm.io/gorm"

	// 项目内部
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// 包级错误哨兵：技能存储的业务错误，供上层 errors.Is 判断。
var (
	// ErrSkillNotFound 技能不存在或不属于给定命名空间
	ErrSkillNotFound = errors.New("skill: skill not found")
	// ErrSkillNameExists 同一命名空间下已存在同名的未删除技能
	ErrSkillNameExists = errors.New("skill: skill name already exists in the namespace")
	// ErrEmptySkillIDs 批量删除时未提供任何技能 ID
	ErrEmptySkillIDs = errors.New("skill: empty skill ids")
)

// CreateInput 是创建技能时的输入内容：名称、描述、正文与用户声明的周期能力、输入字段、
// 固定参数；字段的必填性、长度、空白与周期 nowTime 契约由视图层统一校验，本类型只承载已校验的值。
type CreateInput struct {
	// Name 技能名称
	Name string
	// Description 技能描述
	Description string
	// Content 技能正文
	Content string
	// PeriodicCapable 技能是否可周期执行（由用户声明）
	PeriodicCapable bool
	// InputFields 技能输入字段（由用户填写）
	InputFields []modelsai.SkillInputField
	// FixedFields 技能固定参数（由用户填写），值为原始 JSON
	FixedFields map[string]json.RawMessage
}

// UpdateInput 是更新技能时的输入内容：名称、描述、正文与用户声明的周期能力、输入字段、
// 固定参数整体覆盖；字段校验同样由视图层负责，本类型只承载已校验的值。
type UpdateInput struct {
	// Name 技能名称
	Name string
	// Description 技能描述
	Description string
	// Content 技能正文
	Content string
	// PeriodicCapable 技能是否可周期执行（由用户声明）
	PeriodicCapable bool
	// InputFields 技能输入字段（由用户填写）
	InputFields []modelsai.SkillInputField
	// FixedFields 技能固定参数（由用户填写），值为原始 JSON
	FixedFields map[string]json.RawMessage
}

// Store 提供 AI 技能（AgentSkill）的持久化 CRUD，全部操作基于 GORM（database.DB）。
// 所有读写都以命名空间为隔离维度，正文仅在详情与创建/更新结果中返回；列表查询由视图层
// 统一经 utils.BuildSearchCountQuery 完成，存储层不承担搜索与分页。
type Store struct {
	// db core 库连接（agent_skills 读写）。
	db *database.DB
}

// NewStore 构造 Store；db 为空时返回英文错误。
func NewStore(db *database.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("skill store: nil database")
	}
	return &Store{db: db}, nil
}

// Create 在给定命名空间下创建一条启用的技能；同命名空间下已存在同名未删除技能时返回
// ErrSkillNameExists。输入字段的必填、长度、空白与周期 nowTime 契约由视图层校验完成，
// 本方法只做落库。
func (s *Store) Create(ctx context.Context, namespace string, in CreateInput) (*modelsai.AgentSkill, error) {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return nil, errors.New("skill: empty namespace name")
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&modelsai.AgentSkill{}).
		Where("namespace_name = ? AND name = ?", namespace, in.Name).
		Count(&count).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	if count > 0 {
		return nil, ErrSkillNameExists
	}
	inputFields, err := modelsai.SkillInputFieldsJSON(in.InputFields)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	fixedFields, err := modelsai.SkillFixedFieldsJSON(in.FixedFields)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	skill := &modelsai.AgentSkill{
		NamespaceName:   namespace,
		Source:          modelsai.SkillSourceCustom,
		Name:            in.Name,
		Description:     in.Description,
		Content:         serializer.NewCrypto([]byte(in.Content)),
		Enabled:         true,
		PeriodicCapable: in.PeriodicCapable,
		InputFields:     inputFields,
		FixedFields:     fixedFields,
	}
	if err := s.db.WithContext(ctx).Create(skill).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return skill, nil
}

// Get 按 ID 查询给定命名空间下的一条自定义技能（含解密后的正文）；未找到时返回 ErrSkillNotFound。
// 内置技能不属于任何租户命名空间，故一律按不存在处理（其读取走内置专用方法）。
func (s *Store) Get(ctx context.Context, namespace string, id uint) (*modelsai.AgentSkill, error) {
	var skill modelsai.AgentSkill
	err := s.db.WithContext(ctx).
		Where("id = ? AND namespace_name = ? AND source = ?", id, strings.TrimSpace(namespace), modelsai.SkillSourceCustom).
		First(&skill).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrSkillNotFound
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &skill, nil
}

// Update 更新给定命名空间下的一条技能并返回更新后的技能（含解密后的正文）：先按 ID 确保
// 技能存在且归属该命名空间；改为同命名空间下已存在的其他技能名称时返回 ErrSkillNameExists。
// 名称、描述、正文与周期能力、输入字段、固定参数按整体覆盖落库，显式刷新 updated_at。
// 输入字段的必填、长度、空白与周期 nowTime 契约由视图层校验完成。
func (s *Store) Update(ctx context.Context, namespace string, id uint, in UpdateInput) (*modelsai.AgentSkill, error) {
	namespace = strings.TrimSpace(namespace)
	skill, err := s.Get(ctx, namespace, id)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&modelsai.AgentSkill{}).
		Where("namespace_name = ? AND name = ? AND id <> ?", namespace, in.Name, id).
		Count(&count).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	if count > 0 {
		return nil, ErrSkillNameExists
	}
	inputFields, err := modelsai.SkillInputFieldsJSON(in.InputFields)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	fixedFields, err := modelsai.SkillFixedFieldsJSON(in.FixedFields)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	skill.Name = in.Name
	skill.Description = in.Description
	skill.Content = serializer.NewCrypto([]byte(in.Content))
	skill.PeriodicCapable = in.PeriodicCapable
	skill.InputFields = inputFields
	skill.FixedFields = fixedFields
	// 显式赋值，保证 updated_at 每次更新都会变化（同微秒内的连续更新也能体现）。
	updatedAt := time.Now()
	if !updatedAt.After(skill.UpdatedAt) {
		updatedAt = skill.UpdatedAt.Add(time.Millisecond)
	}
	skill.UpdatedAt = updatedAt
	// 用「结构体 + Select」整体覆盖用户可写列：密文列 content 经 Select 更新以触发序列化器。
	columns := []string{"name", "description", "content", "periodic_capable", "input_fields", "fixed_fields", "updated_at"}
	if err := s.db.WithContext(ctx).Model(skill).Select(columns).Updates(skill).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return skill, nil
}

// Delete 软删除给定命名空间下的若干技能；ids 为空时返回 ErrEmptySkillIDs。
func (s *Store) Delete(ctx context.Context, namespace string, ids []uint) error {
	if len(ids) == 0 {
		return ErrEmptySkillIDs
	}
	if err := s.db.WithContext(ctx).
		Where("id IN ? AND namespace_name = ? AND source = ?", ids, strings.TrimSpace(namespace), modelsai.SkillSourceCustom).
		Delete(&modelsai.AgentSkill{}).Error; err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// SetEnabled 设置一条技能的启停状态：内置技能可按 ID 全局启停，自定义技能须归属给定命名空间；
// 技能不存在或自定义技能跨命名空间时返回 ErrSkillNotFound。
func (s *Store) SetEnabled(ctx context.Context, namespace string, id uint, enabled bool) error {
	var skill modelsai.AgentSkill
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&skill).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrSkillNotFound
	}
	if err != nil {
		return errors.WithStack(err)
	}
	if skill.Source != modelsai.SkillSourceBuiltin && skill.NamespaceName != strings.TrimSpace(namespace) {
		return ErrSkillNotFound
	}
	if err := s.db.WithContext(ctx).Model(&modelsai.AgentSkill{}).
		Where("id = ?", id).Update("enabled", enabled).Error; err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// ListEnabledBuiltinSkills 返回全部已启用的内置技能注册行（按 id 升序，即 fixture 注入顺序），
// 供选择器与运行时装配使用；行只含名称、明文描述与启停状态，技能正文由代码注册表按名提供
// （内置行 Content 为 nil）。
func (s *Store) ListEnabledBuiltinSkills(ctx context.Context) ([]modelsai.AgentSkill, error) {
	var rows []modelsai.AgentSkill
	if err := s.db.WithContext(ctx).
		Where("source = ? AND enabled = ?", modelsai.SkillSourceBuiltin, true).
		Order("id").
		Find(&rows).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return rows, nil
}

// GetByID 按 ID 查询一条技能（含解密后的正文，内置行 Content 为 nil）；未找到时返回
// ErrSkillNotFound。内置与自定义技能都以该 ID 作为选择标识。
func (s *Store) GetByID(ctx context.Context, id uint) (*modelsai.AgentSkill, error) {
	var skill modelsai.AgentSkill
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&skill).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrSkillNotFound
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &skill, nil
}

// IsBuiltinSkill 报告给定 ID 是否为内置技能；不存在时返回 false（由调用方按不存在处理）。
func (s *Store) IsBuiltinSkill(ctx context.Context, id uint) (bool, error) {
	var skill modelsai.AgentSkill
	err := s.db.WithContext(ctx).Select("id", "source").Where("id = ?", id).First(&skill).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, errors.WithStack(err)
	}
	return skill.Source == modelsai.SkillSourceBuiltin, nil
}
