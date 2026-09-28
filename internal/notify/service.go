package notify

import (
	// 标准库
	"context"
	"encoding/json"
	"strings"
	"text/template"
	"time"

	// 第三方
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	// 项目内部
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/enum"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/message"
	"zhongkui.com/zhongkui/internal/pkg/translate"
)

// ErrDeliveryPending 通知记录仍处于 pending 占位（其他发送者正在投递，或崩溃遗留的占位
// 租约尚未过期），本次调用未获得发送槽位，作为瞬时错误交还任务重试，待槽位释放或被回收。
var ErrDeliveryPending = errors.New("notify: delivery slot is pending, retry later")

// claimLease 通知记录占位租约时长：发送者在占位后崩溃或超时，遗留的 pending 记录超过该
// 时长即视为失效，后续重试可条件更新回收该槽位并接管发送，避免槽位被永久占死。
// 取值需明显大于一次渠道发送的耗时（秒级），以平衡"并发重复发送"与"崩溃遗留回收"。
const claimLease = 5 * time.Minute

// Service 通用通知服务：将一次通知请求（事件引用 + 通知组 + 消息）解析为具体目标，
// 经渠道基座发送，并维护 notification_records 发送记录。
// 通知组未绑定目标、目标无可用渠道等场景静默跳过（记录日志），不视为错误。
type Service struct {
	coreDB *database.DB
	logger *logging.Logger
	pool   *message.Pool
}

// NewService 创建通用通知服务。
func NewService(coreDB *database.DB, logger *logging.Logger, pool *message.Pool) *Service {
	return &Service{coreDB: coreDB, logger: logger, pool: pool}
}

// sendEvent 通知事件引用与消息的公共载体，供组发送（SendGroupInput）与单目标直投
// （SendTargetInput）复用：sendOne / claim / finalize 仅依赖该公共子集，故统一入参为
// *sendEvent。SendGroupInput / SendTargetInput 各自保留独立的 JSON 字段（不内嵌此私有类型），
// 以便保持既有复合字面量写法并允许外部包构造入参。
type sendEvent struct {
	Namespace string
	EventType string
	EventID   uint
	Message   message.Message
}

// SendGroupInput 一次向通知组发送的请求，为 notify:send:group 任务载荷的持久化形态。
type SendGroupInput struct {
	// Namespace 命名空间，通知组与目标均限定在该命名空间内（必填）
	Namespace string `json:"namespace" binding:"required"`
	// GroupID 通知组 ID；nil 表示使用命名空间默认通知组（无默认组则跳过）
	GroupID *uint `json:"groupID"`
	// EventType 事件类型键（如 tag_event），与 EventID、TargetID 组合唯一定位通知记录（必填）
	EventType string `json:"eventType" binding:"required"`
	// EventID 触发通知的事件 ID（必填，须为非零值；0 会使同类型事件折叠到同一条记录，破坏幂等去重）
	EventID uint `json:"eventID" binding:"required"`
	// Message 通用消息内容（必填，模板键另须非空）
	Message message.Message `json:"message" binding:"required"`
}

// SendTargetInput 一次不经通知组、直接投递单个目标的通知发送请求，为 notify:send:target
// 任务载荷的持久化形态。TargetID 唯一指定目标（限定在命名空间内）。
type SendTargetInput struct {
	// Namespace 命名空间，目标限定在该命名空间内（必填）
	Namespace string `json:"namespace" binding:"required"`
	// EventType 事件类型键（如 tag_event），与 EventID、TargetID 组合唯一定位通知记录（必填）
	EventType string `json:"eventType" binding:"required"`
	// EventID 触发通知的事件 ID（必填，须为非零值；0 会使同类型事件折叠到同一条记录，破坏幂等去重）
	EventID uint `json:"eventID" binding:"required"`
	// Message 通用消息内容（必填，模板键另须非空）
	Message message.Message `json:"message" binding:"required"`
	// TargetID 直接投递的目标 ID（必填，须为已存在且启用、所属渠道启用的目标）
	TargetID uint `json:"targetID" binding:"required"`
}

// ResolveGroupTargets 解析通知组（显式指定或命名空间默认）并将组内容量分解为逐目标直投
// 入参列表，供上层编排器为每个目标投递一条 notify:send:target 任务。
// 流程：解析通知组 -> 加载组内启用且所属渠道启用的目标；无组或无目标时记录日志并返回
// 空列表（不视为错误）。每个目标映射为一个 SendTargetInput（透传命名空间、事件引用与
// 消息，仅补充 TargetID）。
func (s *Service) ResolveGroupTargets(ctx context.Context, in SendGroupInput) ([]SendTargetInput, error) {
	group, err := s.resolveGroup(ctx, in.Namespace, in.GroupID)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if group == nil {
		s.logger.Ctx(ctx).Infof("notify: no group (namespace=%s groupID=%v), skip", in.Namespace, in.GroupID)
		return nil, nil
	}
	targets, err := s.loadTargets(ctx, in.Namespace, group.ID)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if len(targets) == 0 {
		s.logger.Ctx(ctx).Infof("notify: group %q has no enabled targets, skip", group.Name)
		return nil, nil
	}
	inputs := make([]SendTargetInput, 0, len(targets))
	for i := range targets {
		inputs = append(inputs, SendTargetInput{
			Namespace: in.Namespace,
			EventType: in.EventType,
			EventID:   in.EventID,
			Message:   in.Message,
			TargetID:  targets[i].ID,
		})
	}
	return inputs, nil
}

// resolveGroup 解析通知组：GroupID 非 nil 时加载指定组（校验命名空间），
// 否则加载命名空间默认组；组不存在时返回 (nil, nil)。
func (s *Service) resolveGroup(ctx context.Context, namespace string, groupID *uint) (*core.NotificationGroup, error) {
	query := s.coreDB.WithContext(ctx).Where("namespace_name = ?", namespace)
	if groupID != nil {
		query = query.Where("id = ?", *groupID)
	} else {
		query = query.Where("is_default = ?", true)
	}
	var group core.NotificationGroup
	if err := query.First(&group).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, errors.WithStack(err)
	}
	return &group, nil
}

// loadTargets 加载通知组内启用且所属渠道启用的目标（预载所属渠道）。
func (s *Service) loadTargets(ctx context.Context, namespace string, groupID uint) ([]core.NotificationTarget, error) {
	var targets []core.NotificationTarget
	if err := s.coreDB.WithContext(ctx).
		Joins("JOIN notification_group_targets AS g ON g.target_id = notification_targets.id").
		Where("g.group_id = ?", groupID).
		Where("notification_targets.namespace_name = ?", namespace).
		Where("notification_targets.enabled = ?", true).
		Preload("Channel").
		Find(&targets).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	// 过滤所属渠道缺失或禁用的目标
	kept := targets[:0]
	for i := range targets {
		if targets[i].Channel.ID != 0 && targets[i].Channel.Enabled {
			kept = append(kept, targets[i])
		}
	}
	return kept, nil
}

// sendOne 对单个目标执行发送：以唯一索引 (event_type, event_id, target_id) 为互斥锁原子
// 抢占发送槽位（claim），只有占位成功者负责本次投递，其余并发方跳过，杜绝并发重复发送；
// 已成功投递的目标跳过（幂等），失败目标在后续重试中可重新占位。占位成功后再按目标语言
// 渲染消息、经渠道发送，最终将占位记录更新为成功/失败终态（finalize）。
func (s *Service) sendOne(ctx context.Context, ev *sendEvent, group *core.NotificationGroup, target *core.NotificationTarget) error {
	// 1) 原子占位发送槽位：唯一索引冲突即说明已有记录，占位失败者跳过
	won, err := s.claim(ctx, ev, group, target)
	if err != nil {
		return errors.WithStack(err)
	}
	if !won {
		return nil // 槽位已被他人占用（发送中）或已成功投递，幂等跳过
	}
	// 2) 按目标语言与目标所属渠道类型渲染消息（模板键缺失为编程错误，记录失败但不阻断）
	channelType := target.Channel.Type
	if channelType == "" {
		channelType = target.Type
	}
	rendered, err := s.renderMessage(ctx, target, channelType, &ev.Message)
	if err != nil {
		return s.finalize(ctx, ev, group, target, nil, "", err)
	}
	// 3) 经统一资源池借出/归还渠道实例发送并落终态（成功/失败）；
	//    未注册类型/配置非法或借出/发送失败记录失败但不阻断
	msgID, sendErr := s.pool.Send(ctx, target.Channel.Type, json.RawMessage(target.Channel.ConfigData()), &message.Target{Name: target.Name, Language: target.Language, Config: json.RawMessage(target.ConfigData())}, rendered)
	return s.finalize(ctx, ev, group, target, rendered, msgID, sendErr)
}

// claim 原子抢占 (event_type, event_id, target_id) 发送槽位，返回是否占位成功（won）。
// 唯一索引充当互斥锁：
//   - 首次占位：INSERT ... ON CONFLICT DO NOTHING 插入 status=pending 的占位记录，
//     插入成功（RowsAffected==1）即获得槽位，本发送者负责投递；
//   - 冲突（记录已存在）：读取现有状态——
//     success：已成功投递，不占位（跳过）；
//     pending：他人正在发送；占位未超过租约返回 ErrDeliveryPending（交还任务重试），
//     超过租约（发送者崩溃/超时遗留）落入回收；
//     failed：上次发送失败，落入回收；
//   - 回收：条件 UPDATE 仅将 status 从 pending/failed 置回 pending，且 pending 需满足
//     updated_at 早于租约截止（避免两个回收方同时成功），仅一个并发方
//     RowsAffected==1 获得槽位，其余返回 ErrDeliveryPending 交还重试。
func (s *Service) claim(ctx context.Context, ev *sendEvent, group *core.NotificationGroup, target *core.NotificationTarget) (won bool, err error) {
	now := time.Now()
	record := &core.NotificationRecord{
		NamespaceName: ev.Namespace,
		EventType:     ev.EventType,
		EventID:       ev.EventID,
		GroupID:       groupID(group),
		TargetID:      target.ID,
		ChannelID:     target.ChannelID,
		ChannelType:   target.Channel.Type,
		Status:        core.RecordStatusPending,
	}
	// 首次占位：唯一索引冲突（记录已存在）时静默忽略，不视为错误
	result := s.coreDB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(record)
	if result.Error != nil {
		return false, errors.WithStack(result.Error)
	}
	if result.RowsAffected == 1 {
		return true, nil // 占位成功，本发送者负责投递
	}
	// 冲突：读取现有记录状态决定是否可回收
	var existing core.NotificationRecord
	if err := s.coreDB.WithContext(ctx).
		Select("status", "updated_at").
		Where("event_type = ? AND event_id = ? AND target_id = ?", ev.EventType, ev.EventID, target.ID).
		First(&existing).Error; err != nil {
		return false, errors.WithStack(err)
	}
	switch existing.Status {
	case core.RecordStatusSuccess:
		return false, nil // 已成功投递，跳过
	case core.RecordStatusPending:
		if now.Sub(existing.UpdatedAt) < claimLease {
			// 他人正在发送（租约未过期），交还任务重试，避免并发重复投递
			return false, ErrDeliveryPending
		}
		// 占位已过期（发送者崩溃/超时遗留），落入下方回收
	case core.RecordStatusFailed:
		// 上次发送失败，可回收重试
	default:
		return false, nil
	}
	// 回收：failed 恒可回收；pending 仅当 updated_at 早于租约截止才可回收。
	// 条件更新使同一时刻至多一个回收方 RowsAffected==1，其余匹配 0 行交还重试。
	reclaim := s.coreDB.WithContext(ctx).
		Model(&core.NotificationRecord{}).
		Where("event_type = ? AND event_id = ? AND target_id = ?", ev.EventType, ev.EventID, target.ID).
		Where(
			"(status = ? OR (status = ? AND updated_at < ?))",
			core.RecordStatusFailed,
			core.RecordStatusPending,
			now.Add(-claimLease),
		).
		Update("status", core.RecordStatusPending)
	if reclaim.Error != nil {
		return false, errors.WithStack(reclaim.Error)
	}
	return reclaim.RowsAffected == 1, nil
}

// groupID 从通知组指针提取 ID；组为 nil（单目标直投不经通知组）时返回 0，
// 使直投记录写入 GroupID=0。
func groupID(group *core.NotificationGroup) uint {
	if group == nil {
		return 0
	}
	return group.ID
}

// resolveTemplate 依据三要素（事件 Event、渠道类型 ChannelType、语言 Language）在命名
// 空间下解析持久化通知模板：查询 (namespace_name, event) 匹配且 channel_type ∈
// {channelType, "*"}、language ∈ {lang, "*"} 的行，并按最低特异性选出最佳行：
// 渠道类型/语言精确匹配得分 0、"*" 通配得分 1，组合总分越低越优先；总分相同时优先
// 精确渠道而非精确语言，仍平局时按 ID 升序确定（确定性）。无匹配行时返回 (nil, nil)。
// 软删除的行由 GORM 默认过滤。resolveTemplate 返回模板行；查询失败返回错误。
func (s *Service) resolveTemplate(ctx context.Context, namespace string, event, channelType, lang string) (*core.NotificationTemplate, error) {
	var rows []core.NotificationTemplate
	if err := s.coreDB.WithContext(ctx).
		Where("namespace_name = ? AND event = ? AND channel_type IN ? AND language IN ?",
			namespace, event, []string{channelType, "*"}, []string{lang, "*"}).
		Find(&rows).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	channelScore := func(v string) int {
		if v == "*" {
			return 1
		}
		return 0
	}
	languageScore := func(v string) int {
		if v == "*" {
			return 1
		}
		return 0
	}
	best := &rows[0]
	bestTotal := channelScore(best.ChannelType) + languageScore(best.Language)
	for i := 1; i < len(rows); i++ {
		cand := &rows[i]
		cs, ls := channelScore(cand.ChannelType), languageScore(cand.Language)
		total := cs + ls
		bestCS, bestLS := channelScore(best.ChannelType), languageScore(best.Language)
		better := total < bestTotal ||
			(total == bestTotal && cs < bestCS) ||
			(total == bestTotal && cs == bestCS && ls < bestLS) ||
			(total == bestTotal && cs == bestCS && ls == bestLS && cand.ID < best.ID)
		if better {
			best = cand
			bestTotal = total
		}
	}
	return best, nil
}

// renderFuncs 构造按目标语言渲染通知模板的自定义函数映射，注入三个语义互斥的模板函数，
// 避免原先被过度重载的单函数 t 造成的歧义：
//   - T "<i18n键>"：对完整 i18n 键直接翻译（translate.Localize），缺失键回退为键本身；
//   - TenumKey "<类名>" "<key>"：按枚举 key 经 enum.<类名>.<key> 翻译 label，未注册类
//     或未命中 key 时回退 key 本身；
//   - TenumValue "<类名>" "<value>"：先按 value 命中条目（enum 注册表 Resolve 语义，
//     value 优先、key 其次），再经 enum.<类名>.<key> 翻译 label，未注册类或未命中 value
//     时回退原 value。
//
// 三者类名均兼容 "enum." 前缀写法。lang 为 nil 时由 translate 层回退默认语言。
func renderFuncs(lang *base.Language) template.FuncMap {
	return template.FuncMap{
		"T": func(key string) string {
			return translate.Localize(lang, key, nil)
		},
		"TenumKey": func(className, key string) string {
			name := strings.TrimPrefix(className, "enum.")
			return translate.EnumLabel(lang, name, key)
		},
		"TenumValue": func(className, val string) string {
			name := strings.TrimPrefix(className, "enum.")
			if typ, exists := enum.Get(name); exists {
				if entry, ok := typ.Resolve(val); ok {
					return translate.EnumLabel(lang, name, entry.Key)
				}
			}
			return val
		},
	}
}

// renderI18nDefault 依据语言在翻译包（internal/pkg/translate）中按
// notify.<template>.title / notify.<template>.content 键取回默认标题/正文，作为持久化模板
// 解析缺位时的回退。notify.* 默认模板因含 {{...}} 函数调用一律经 text/template 渲染：translate 的
// 全局数据定界符为 [[ / ]]（dataDelimParser），这些源串不含 [[，go-i18n fast-path 原样透出
// 未渲染原文，故此处经 Localize(lang, key, nil) 拿到的即原始模板串（保留 {{...}} 占位符），
// 再经 message.RenderTemplateWithFuncs 以 renderFuncs 注入的模板函数（T/TenumKey/TenumValue）
// 渲染枚举 label。键缺失时
// Localize 回退返回键本身，据此判定默认模板是否可用。渲染得到的结构体仍为
// *message.RenderedMessage{Title, Content, ContentType}。
func renderI18nDefault(lang *base.Language, spec *EventTemplateSpec, params map[string]any, contentType string) (*message.RenderedMessage, error) {
	titleKey := spec.TitleKey()
	contentKey := spec.ContentKey()
	rawTitle := translate.Localize(lang, titleKey, nil)
	rawContent := translate.Localize(lang, contentKey, nil)
	if rawTitle == titleKey || rawContent == contentKey || rawTitle == "" || rawContent == "" {
		return nil, errors.Newf("notify: no i18n default template for message %q (lang %q)", spec.Template, lang)
	}
	fn := renderFuncs(lang)
	title, tErr := message.RenderTemplateWithFuncs(rawTitle, params, fn)
	if tErr != nil {
		return nil, errors.Newf("notify: render i18n default title for message %q (lang %q): %v", spec.Template, lang, tErr)
	}
	content, cErr := message.RenderTemplateWithFuncs(rawContent, params, fn)
	if cErr != nil {
		return nil, errors.Newf("notify: render i18n default content for message %q (lang %q): %v", spec.Template, lang, cErr)
	}
	return &message.RenderedMessage{Title: title, Content: content, ContentType: contentType}, nil
}

// RenderDefaultMessage 依据模板键与语言渲染 notify 内置默认模板（标题+正文），供管理端
// 事件枚举等场景用事件样例参数展示事件的默认模板能力。模板内容同样经模板函数 t 渲染枚举
// label（与发送路径一致）。模板键未注册到事件注册表时返回错误。
func RenderDefaultMessage(lang *base.Language, templateKey string, params map[string]any, contentType string) (*message.RenderedMessage, error) {
	_, spec, ok := TemplateSpecOf(templateKey)
	if !ok {
		return nil, errors.Newf("notify: no notification template spec for %q", templateKey)
	}
	return renderI18nDefault(lang, spec, params, contentType)
}

// renderMessage 依据目标偏好的语言与所属渠道类型将结构化消息渲染为具体文本
// （message.RenderedMessage）。模板内容以 NotificationTemplate 表为唯一事实来源：先按消息模板键
// 在注册表中查找事件规格（TemplateSpecOf），命中则调用 resolveTemplate 按
// (事件, 渠道, 语言) 解析持久化模板并渲染；无持久化行时回退翻译包 i18n 内置默认模板
// （renderI18nDefault，键 notify.<template>.title/.content）。两者皆不可用时返回错误，由
// sendOne 记录失败。模板必须先注册到事件注册表：未注册的模板键无持久化行也无可预知的内容，
// 后端文件/语言包无法预知前端会配置什么模板键，故直接返回错误，不经 locales notify.* 兜底。
func (s *Service) renderMessage(ctx context.Context, target *core.NotificationTarget, channelType string, msg *message.Message) (*message.RenderedMessage, error) {
	if msg.Template == "" {
		return nil, errors.New("notify: message template is required")
	}
	lang := base.DefaultLanguage
	if target.Language != "" {
		if parsed, err := base.ParseLanguage(target.Language); err != nil {
			s.logger.Ctx(ctx).Warnf("notify: invalid target language %q, fallback to default: %v", target.Language, err)
		} else {
			lang = parsed
		}
	}
	// 事件模板：优先持久化模板，其次翻译包 i18n 内置默认；不经 locales notify.* 兜底
	if event, spec, ok := TemplateSpecOf(msg.Template); ok {
		if tmpl, err := s.resolveTemplate(ctx, target.NamespaceName, event, channelType, lang.String()); err != nil {
			s.logger.Ctx(ctx).Warnf("notify: resolve persisted template for event %q channel %q lang %q: %v", event, channelType, lang, err)
		} else if tmpl != nil {
			title, tErr := message.RenderTemplateWithFuncs(tmpl.Title, msg.Params, renderFuncs(lang))
			content, cErr := message.RenderTemplateWithFuncs(tmpl.Content, msg.Params, renderFuncs(lang))
			if tErr == nil && cErr == nil {
				return &message.RenderedMessage{Title: title, Content: content, ContentType: msg.ContentType}, nil
			}
			if tErr != nil {
				s.logger.Ctx(ctx).Warnf("notify: render persisted title template for event %q: %v", event, tErr)
			}
			if cErr != nil {
				s.logger.Ctx(ctx).Warnf("notify: render persisted content template for event %q: %v", event, cErr)
			}
		}
		// 无可用持久化模板时，回退翻译包（i18n）内置默认模板：统一以注册表解析出的
		// spec.Template 作为 i18n key 前缀（notify.<template>.title/.content）查找并渲染。
		if rendered, rErr := renderI18nDefault(lang, spec, msg.Params, msg.ContentType); rErr == nil {
			return rendered, nil
		} else {
			s.logger.Ctx(ctx).Warnf("notify: render i18n default template for message %q lang %q: %v", msg.Template, lang, rErr)
		}
		return nil, errors.Newf("notify: no usable template for message %q (lang %q)", msg.Template, lang)
	}
	// 未注册到事件注册表的模板键：无持久化行可解析、也无注册表默认内容可渲染，
	// 且后端无法预知前端会配置的模板键，故不经 locales notify.* 兜底，直接报错。
	return nil, errors.Newf("notify: no notification template spec for message %q", msg.Template)
}

// RenderPreview 按事件在指定命名空间下渲染模板预览（供管理端 render-test 使用），
// 直接以事件类型键驱动，不经过目标/消息结构。事件无对应规格时返回描述性错误；存在
// 持久化模板行时经 renderTemplateText 渲染并回传错误详情（管理端据此返回 400）；无
// 持久化行时回退到规格默认（renderSpecDefault，注册表内置默认）。lang 空或非法时
// 归一化到平台默认语言。
func (s *Service) RenderPreview(ctx context.Context, namespace, event string, channelType, lang string, params map[string]any) (*message.RenderedMessage, error) {
	spec, ok := EventSpecOf(event)
	if !ok {
		return nil, errors.Newf("notify: no notification template spec for event %q", event)
	}
	tmpl, err := s.resolveTemplate(ctx, namespace, event, channelType, lang)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	parsed := base.DefaultLanguage
	if lang != "" {
		if p, perr := base.ParseLanguage(lang); perr == nil {
			parsed = p
		}
	}
	parsed = parsed.Normalize()
	if tmpl != nil {
		title, tErr := message.RenderTemplateWithFuncs(tmpl.Title, params, renderFuncs(parsed))
		if tErr != nil {
			return nil, errors.Newf("notify: render preview title for event %q: %v", event, tErr)
		}
		content, cErr := message.RenderTemplateWithFuncs(tmpl.Content, params, renderFuncs(parsed))
		if cErr != nil {
			return nil, errors.Newf("notify: render preview content for event %q: %v", event, cErr)
		}
		return &message.RenderedMessage{Title: title, Content: content}, nil
	}
	return renderI18nDefault(parsed, spec, params, "")
}

// finalize 将已占位（status=pending）的通知记录更新为发送终态：rendered 为实际发送的
// 消息快照（可为 nil），sendErr 非 nil 时记录为失败并携带错误信息，notified_at 仅在
// 成功时写入。占位成功后行必然存在且由 (event_type, event_id, target_id) 唯一定位，
// 故以条件 UPDATE 落终态（updated_at 同时刷新，作为下一次占位租约的基准）。
func (s *Service) finalize(ctx context.Context, ev *sendEvent, group *core.NotificationGroup, target *core.NotificationTarget, rendered *message.RenderedMessage, msgID string, sendErr error) error {
	payload := []byte("{}")
	if rendered != nil {
		var err error
		if payload, err = json.Marshal(rendered); err != nil {
			return errors.WithStack(err)
		}
	}
	now := time.Now()
	record := &core.NotificationRecord{
		NamespaceName: ev.Namespace,
		EventType:     ev.EventType,
		EventID:       ev.EventID,
		GroupID:       groupID(group),
		TargetID:      target.ID,
		ChannelID:     target.ChannelID,
		ChannelType:   target.Channel.Type,
		Payload:       string(payload),
		Status:        core.RecordStatusSuccess,
		MessageID:     msgID,
		NotifiedAt:    &now,
	}
	if sendErr != nil {
		record.Status = core.RecordStatusFailed
		record.Error = sendErr.Error()
		record.NotifiedAt = nil
	}
	assignments := map[string]any{
		"payload":     record.Payload,
		"status":      record.Status,
		"error":       record.Error,
		"message_id":  record.MessageID,
		"notified_at": record.NotifiedAt,
		"updated_at":  now,
	}
	if err := s.coreDB.WithContext(ctx).
		Model(&core.NotificationRecord{}).
		Where("event_type = ? AND event_id = ? AND target_id = ?", ev.EventType, ev.EventID, target.ID).
		Updates(assignments).Error; err != nil {
		return errors.WithStack(err)
	}
	if sendErr != nil {
		// 发送失败透传错误，触发任务重试；错误已由渠道以项目 errors 包构造，无需重复包裹
		return sendErr
	}
	return nil
}

// SendToTarget 执行一次不经通知组的单目标直投：加载目标（含过滤：缺失/禁用渠道、禁用目标
// 均视为不可用跳过），不可用时记录日志并返回 nil；否则复用 sendOne 的唯一索引幂等占位与
// 投递逻辑，写入 GroupID=0 的发送记录。
func (s *Service) SendToTarget(ctx context.Context, in SendTargetInput) error {
	target, err := s.loadDirectTarget(ctx, in.Namespace, in.TargetID)
	if err != nil {
		return errors.WithStack(err)
	}
	if target == nil {
		s.logger.Ctx(ctx).Infof("notify: target unavailable (namespace=%s targetID=%d), skip", in.Namespace, in.TargetID)
		return nil
	}
	ev := &sendEvent{Namespace: in.Namespace, EventType: in.EventType, EventID: in.EventID, Message: in.Message}
	return s.sendOne(ctx, ev, nil, target)
}

// loadDirectTarget 按 (namespace_name, id) 加载指定目标并预载其渠道；记录不存在返回
// (nil, nil)。随后过滤：所属渠道缺失（Channel.ID==0）或禁用、或目标自身禁用时返回
// (nil, nil)，否则返回目标。
func (s *Service) loadDirectTarget(ctx context.Context, namespace string, targetID uint) (*core.NotificationTarget, error) {
	var target core.NotificationTarget
	if err := s.coreDB.WithContext(ctx).
		Where("namespace_name = ? AND id = ?", namespace, targetID).
		Preload("Channel").
		First(&target).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, errors.WithStack(err)
	}
	if target.Channel.ID == 0 || !target.Channel.Enabled || !target.Enabled {
		return nil, nil
	}
	return &target, nil
}
