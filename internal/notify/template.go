// Package notify 提供通知事件模板注册表、消息模板校验与渲染所需的事件元信息，以及通用通知
// 服务（Service）——把一次通知请求解析为具体目标、按目标语言渲染模板、经渠道基座投递并维护
// `notification_records` 发送记录。
//
// 本文件定义消息事件模板注册表（event template registry）。注册表仅声明每个事件
// （Event）支持的模板键（Template）、渠道类型（Channels）与语言（Languages）等元数据，
// 不承载具体标题/正文内容——各语言默认模板标题/正文已迁至翻译包 i18n 键
// `notify.{template}.title` / `notify.{template}.content`（见 internal/pkg/translate/locales）。
// 发送消息渲染时，若命名空间下无对应持久化模板行，回退到该 i18n 默认模板。
//
// 包内文件分工：template.go（事件模板注册表与元信息）、service.go（通知服务、渲染与发送去重）、
// enum.go（把通知事件注册为枚举类 notificationEvent）。
//
// 本指南覆盖两类扩展：新增一种「渠道类型」（email / webhook 之外的传输方式），以及为事件
// 新增或维护「多语言模板」。
//
// ## 一、渠道类型：消息基座
//
// 渠道的类型与实现都在 internal/pkg/message（与 GORM/asynq 解耦）：
//   - 渠道类型键常量：message.TypeEmail / message.TypeWebhook（定义在 message 而非 models，
//     避免 import cycle）；
//   - 渠道契约 Channel：Type() string 与 `Send(ctx, *Target, *RenderedMessage) (string, error)`；
//   - 工厂注册表：message.RegisterChannel(typ, factory, channelConfigType, targetConfigType)
//     ——同时注册渠道级/目标级配置结构体类型，供 /notification/channel/types 反射表单元信息；
//   - 统一发送入口：message.Pool.Send(ctx, typ, config, target, rendered)
//     ——按「类型 + 规范化渠道级配置」维护独立连接池，复用底层连接、失效重建、闲置回收；
//   - 渠道类型枚举：message/enum.go 经 enum.RegisterSource("channelType", ...) 从 Types() 实时派生。
//
// 渠道级配置承载传输凭据（如 SMTP 账号密码），目标级配置承载具体接收者（如收件邮箱、Webhook
// URL），二者结构相互独立，见 webhook.go 的 WebhookChannelConfig / WebhookTarget。
//
// ## 二、分步清单：新增一个渠道
//
//  1. 在 internal/pkg/message/ 新增 `{type}.go`，定义渠道级配置结构体与目标级配置结构体
//     （字段带 json/binding/form 标签）。
//  2. 实现 Channel：Type() 返回新类型键；Send 解析 target.Config、校验并投递，失败返回非 nil
//     错误（触发任务重试）。
//  3. 在文件底部 init() 调用 `message.RegisterChannel(TypeXxx, factory,
//     reflect.TypeFor[ChannelConfig](), reflect.TypeFor[TargetConfig]())`；重复键会 panic。
//  4. 新增渠道类型键常量（如 message.TypeDingTalk）。
//  5. 补 i18n：zh-Hans.toml / en.toml 各补 `[enum.channelType.{key}]`，否则
//     `internal/manager/views/enum_i18n_test.go` 的 TestEnumClassesHaveBilingualLabels 失败。
//  6. 若渠道需长期连接（如 SMTP），按需实现 resourcepool 的 Lifespan；否则按纯对象复用
//     （HTTP 类渠道的连接复用在 net/http Transport 层完成，见 webhook.go）。
//  7. 验证：go test ./internal/pkg/message/... ；POST /api/v1/notification/channel/types
//     应出现新类型及其配置 schema。
//
// ## 三、事件模板注册表
//
// 事件模板注册表定义在 notify/template.go（唯一事实来源）：
//
//	var eventTemplateSpecs = map[string]EventTemplateSpec{
//		core.EventTypeTagEventRecord: {
//			Template:  TemplateTagEventAlert,
//			Channels:  []string{"*"},
//			Languages: []base.Language{base.LangSimplifiedChinese, base.LangEnglish},
//			SampleParams: map[string]any{ /* 样例渲染参数，键为大驼峰占位符名 */ },
//		},
//	}
//
// 注册表只声明元数据（模板键、支持的渠道、语言、样例参数），不承载标题/正文。相关读取接口：
// TemplateSpecs / EventSpecOf / TemplateSpecOf / SupportedChannels / SupportedLanguages /
// EventFields / EventSampleParams / RenderDefaultMessage。
//
// ## 四、分步清单：新增一个事件模板
//
//  1. 在 template.go 的模板键常量区新增 TemplateXxx 常量，并在 eventTemplateSpecs 中为该事件
//     （core.EventType*）登记规格：Template / Channels / Languages / SampleParams。
//
//  2. 在 locales 的 zh-Hans.toml 与 en.toml 中新增：
//
//     `[notify.{template}.title]`
//     `other = "标题模板"`
//     `[notify.{template}.content]`
//     `other = "正文模板（大段正文可用 @notify/xxx.{lang}.txt 外置引用）"`
//
//     占位符用 {{.Key}}（大驼峰，与 SampleParams 的键一致）。
//
//  3. 若该事件同时是枚举类 notificationEvent 的一部分：notify/enum.go 经 enum.RegisterSource
//     从 TemplateSpecs() 实时派生，无需另加代码；但需补 `enum.notificationEvent.{event}` 中英 label。
//
//  4. 验证：go test ./internal/notify/... ；通知模板事件元信息接口
//     POST /api/v1/notification/template/events/meta 可见新事件，可用样例参数试跑预览。
//
// ## 五、渲染流程与语法约定
//
// Service.renderMessage 按目标语言与渠道类型渲染消息：
//  1. 以 msg.Template 查注册表（TemplateSpecOf），命中得事件与规格；
//  2. 先按 (命名空间, 事件, 渠道, 语言) 解析持久化模板行（resolveTemplate：渠道/语言的精确
//     匹配优先于 "*" 通配），命中则用其 Title/Content；
//  3. 无持久化行时回退 i18n 默认模板（renderI18nDefault，键 `notify.{template}.title/.content`）。
//
// 两者都不可用则报错，由 sendOne 记录失败。
//
// 模板语法为 Go text/template，占位符写作 {{.X}}。定界符约定见 AGENTS.md：go-i18n 数据占位
// 统一用 [[.X]]，而 notify.* 模板正文用 {{ }}（其源串不含 [[，go-i18n 原样透出后再渲染）。
// 模板内可调用 renderFuncs 注入的函数：T "i18n键"、TenumKey "类名" "key"、
// TenumValue "类名" "value"（枚举 label 经 `enum.{类名}.{key}` 翻译）。保存前可用
// notify.ValidateTemplateSyntax 做语法校验。
//
// ## 六、去重与抑制（发送记录）
//
// Service 以 `notification_records` 的唯一索引 (`event_type`, `event_id`, `target_id`) 作为互斥锁：
//   - claim：INSERT ... ON CONFLICT DO NOTHING 抢占 status=pending 占位，仅占位成功者负责投递；
//     status=success 直接跳过（幂等）；status=pending 且未超 claimLease(5m) 返回
//     ErrDeliveryPending 交还任务重试；超租约或 failed 则条件 UPDATE 回收槽位；
//   - finalize：投递后把占位更新为 success/failed 终态，失败透传错误触发重试。
//
// 通知组未绑定目标、目标或所属渠道禁用等场景静默跳过（不视为错误）。异步投递的 asynq 任务
// notify:send:group（扇出为逐目标 notify:send:target）见 internal/worker/handlers/notify。
//
// ## 七、模板解析与持久化约定
//
// 持久化模板行 core.NotificationTemplate 以 (namespace_name, event, channel_type, language)
// 复合唯一，`channel_type` 与 `language` 支持 "*" 通配，`source` 区分内置默认（default）与
// 用户覆盖（override）。内置默认模板不再物化落库：`notification_templates` 仅承载命名空间
// 维度的覆盖行，"恢复默认"即删除该事件行，渲染自动回退 i18n 默认。
//
// `resolveTemplate` 的解析优先级由高到低：(事件, 渠道, 语言) → (事件, 渠道, "*") →
// (事件, "*", 语言) → (事件, "*", "*")；总分并列时优先精确渠道而非精确语言，仍平局按
// 模板行 ID 升序，保证确定性。
//
// 模板统一经 message.RenderTemplateWithFuncs 渲染，底层 text/template 开启 missingkey=zero：
// 模板引用而 params 缺失的键渲染为空/零值而非报错。i18n 键中不含语言（语言维度由各语言
// 文件承载），较长正文经 "@相对路径" 外置文件（如 locales/notify/*.txt）。
package notify

import (
	// 标准库
	"fmt"
	"slices"
	"text/template"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/models/core"
	"zhongkuisec.com/zhongkui/internal/pkg/base"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// 消息模板键常量（业务相关）。模板键由各事件源在通知层固化，随 message.Message
// 携带并在渲染时按 `notify.<template>.title` / `notify.<template>.content` 在 locales
// 翻译文件中查找对应语言的文本模板。此处隶属业务层，故定义在 notify 而非 message 基座。
const (
	// TemplateTagEventAlert 标签事件告警消息模板（tag_event 事件源）
	TemplateTagEventAlert = "tag_event.alert"
)

// eventTemplateSpecs 事件模板注册表，键为事件类型键（Event key），是注册表的唯一事实来源。
// 每条规格声明该事件支持的模板键（Template）、渠道类型（Channels）、语言（Languages）与
// 样例渲染参数（SampleParams）。标题/正文默认内容不在此：已迁至 i18n 语言文件
// notify.<template>.title / notify.<template>.content，发送渲染时持久化模板行缺位回退到该键。
// SampleParams 用于前端试运行（以内置样例值渲染编辑器内标题/正文）并作为智能补全字段清单
// （EventFields）的数据源。
var eventTemplateSpecs = map[string]EventTemplateSpec{
	core.EventTypeTagEventRecord: {
		Template:  TemplateTagEventAlert,
		Channels:  []string{"*"},
		Languages: []base.Language{base.LangSimplifiedChinese, base.LangEnglish},
		SampleParams: map[string]any{
			"Namespace":     "示例命名空间",
			"RuleName":      "示例规则名",
			"DetectedAt":    "2026-01-01T00:00:00Z",
			"DetectedAtFmt": "2026-01-01 00:00:00 UTC",
			"Hits": []any{
				map[string]any{"ConditionName": "条件A", "Dimension": "fpTcpClientSig", "Item": "10.0.0.1", "Count": 5, "Threshold": 3, "Model": "zhongkuisec.com/zhongkui/internal/models/network.HttpTransport", "Window": "5m0s", "RecordCount": 5},
				map[string]any{"ConditionName": "条件B", "Dimension": "fpTcpServerSig", "Item": "443", "Count": 128, "Threshold": 50, "Model": "zhongkuisec.com/zhongkui/internal/models/network.HttpTransport", "Window": "10m0s", "RecordCount": 128},
			},
		},
	},
}

// EventTemplateSpec 描述一个事件（Event）的默认通知模板元信息规格。
// 标题/正文默认内容不再挂在此处：已迁至 i18n 语言文件（键 notify.<template>.title/.content，
// 见 internal/pkg/translate），发送渲染时若持久化模板行缺位，回退按该键查找。
// 事件类型键（Event key）由注册表 map 的键承载，故本结构体不重复声明 Event 字段。
type EventTemplateSpec struct {
	// Template 消息模板键，取值定义在 notify.TemplateXxx 常量（如 notify.TemplateTagEventAlert）
	Template string
	// Channels 渠道类型键的并集（支持的类型），"*" 表示对所有渠道生效
	Channels []string
	// Languages 该事件支持的语言（base.Language 强类型），供前端枚举该事件的可用语言。
	// 注意：与 Channels 不同，语言不支持 "*" 通配——"对所有语言生效"由
	// SupportedLanguages 在并集时统一追加 * 表示。
	Languages []base.Language
	// SampleParams 该事件的样例渲染参数：供前端试运行预览，并作为智能补全字段
	// 清单（EventFields）的数据源；键为模板占位符（大驼峰）。
	SampleParams map[string]any
}

// TitleKey 返回该事件模板标题在 i18n 语言文件中的键（notify.<template>.title）。
func (spec *EventTemplateSpec) TitleKey() string {
	return fmt.Sprintf(`notify.%s.title`, spec.Template)
}

// ContentKey 返回该事件模板正文在 i18n 语言文件中的键（notify.<template>.content）。
func (spec *EventTemplateSpec) ContentKey() string {
	return fmt.Sprintf(`notify.%s.content`, spec.Template)
}

// validateTemplateSyntax 校验单条模板字符串的 Go text/template 语法。
func validateTemplateSyntax(kind, text string) error {
	if _, err := template.New("default").Option("missingkey=zero").Parse(text); err != nil {
		return errors.Newf("invalid %s template: %w", kind, err)
	}
	return nil
}

// ValidateTemplateSyntax 导出校验单条模板字符串的 Go text/template 语法，
// 供语法校验接口在保存前对用户模板做权威校验。
func ValidateTemplateSyntax(kind, text string) error {
	return validateTemplateSyntax(kind, text)
}

// EventSampleParams 返回某事件（Event key）的样例渲染参数；未知事件返回 (nil, false)。
// 未注册的事件无样例参数时，试运行接口可回退为空 map 照常渲染。
func EventSampleParams(event string) (map[string]any, bool) {
	spec, ok := eventTemplateSpecs[event]
	if !ok {
		return nil, false
	}
	return spec.SampleParams, true
}

// EventFields 返回某事件的可用模板变量名清单：顶层键 + 数组元素对象的嵌套键（以供
// {{range .Hits}} 内部如 .ConditionName 等引用补全），去重后升序返回。
func EventFields(event string) []string {
	spec, ok := eventTemplateSpecs[event]
	if !ok {
		return nil
	}
	set := make(map[string]bool, len(spec.SampleParams))
	for k, v := range spec.SampleParams {
		set[k] = true
		if arr, ok := v.([]any); ok {
			for _, item := range arr {
				if m, ok := item.(map[string]any); ok {
					for k := range m {
						set[k] = true
					}
				}
			}
		}
	}
	fields := make([]string, 0, len(set))
	for f := range set {
		fields = append(fields, f)
	}
	slices.Sort(fields)
	return fields
}

// TemplateSpecs 返回事件模板注册表的安全副本（键为事件类型键），调用方可安全修改
// 而不会污染注册表。事件类型键取决于注册表定义，暴露顺序不作保证：set 语义下顺序不构成
// 契约，展示顺序由调用方/前端按自身展示键决定。
func TemplateSpecs() map[string]EventTemplateSpec {
	specs := make(map[string]EventTemplateSpec, len(eventTemplateSpecs))
	for event, s := range eventTemplateSpecs {
		spec := EventTemplateSpec{
			Template:     s.Template,
			Channels:     slices.Clone(s.Channels),
			Languages:    slices.Clone(s.Languages),
			SampleParams: cloneParams(s.SampleParams),
		}
		specs[event] = spec
	}
	return specs
}

// cloneParams 深拷贝样例渲染参数（map[string]any），内部嵌套的 []any 元素 map 同样拷贝，
// 保证调用方经 TemplateSpecs 取得的副本可安全修改而不污染注册表。
func cloneParams(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		switch tv := v.(type) {
		case []any:
			cloned := make([]any, len(tv))
			for i, item := range tv {
				if m, ok := item.(map[string]any); ok {
					cloned[i] = cloneParams(m)
				} else {
					cloned[i] = item
				}
			}
			out[k] = cloned
		default:
			out[k] = tv
		}
	}
	return out
}

// TemplateSpecOf 依据消息模板键在注册表中查找并返回对应的事件类型键（key）与规格；
// 未找到时返回 ("", nil, false)。
func TemplateSpecOf(templateKey string) (string, *EventTemplateSpec, bool) {
	for event, s := range eventTemplateSpecs {
		if s.Template == templateKey {
			return event, &s, true
		}
	}
	return "", nil, false
}

// EventSpecOf 依据事件类型键在注册表中查找并返回对应规格；未找到时返回 (nil, false)。
func EventSpecOf(event string) (*EventTemplateSpec, bool) {
	spec, ok := eventTemplateSpecs[event]
	if ok {
		return &spec, true
	}
	return nil, false
}

// SupportedChannels 返回注册表支持的渠道类型键的并集（升序），并保证包含 "*"，
// 供管理接口枚举可选渠道类型。
func SupportedChannels() []string {
	seen := map[string]bool{"*": true}
	for _, s := range eventTemplateSpecs {
		for _, c := range s.Channels {
			seen[c] = true
		}
	}
	channels := make([]string, 0, len(seen))
	for c := range seen {
		channels = append(channels, c)
	}
	slices.Sort(channels)
	return channels
}

// SupportedLanguages 返回注册表支持的语言键的并集（升序），并保证包含 "*" 与平台
// 支持的全部语言（base.SupportLanguages），供管理接口枚举可选语言。
func SupportedLanguages() []string {
	seen := map[string]bool{"*": true}
	for _, l := range base.SupportLanguages {
		seen[l.String()] = true
	}
	for _, s := range eventTemplateSpecs {
		for _, l := range s.Languages {
			seen[l.String()] = true
		}
	}
	languages := make([]string, 0, len(seen))
	for l := range seen {
		languages = append(languages, l)
	}
	slices.Sort(languages)
	return languages
}
