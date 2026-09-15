// 本文件定义消息事件模板注册表（event template registry）。注册表仅声明每个事件
// （Event）支持的模板键（Template）、渠道类型（Channels）与语言（Languages）等元数据，
// 不承载具体标题/正文内容——各语言默认模板标题/正文已迁至翻译包 i18n 键
// notify.<template>.title / notify.<template>.content（见 internal/pkg/translate/locales）。
// 发送消息渲染时，若命名空间下无对应持久化模板行，回退到该 i18n 默认模板。
//
// This file defines the event template registry metadata. The registry only declares
// per-event template key (Template), supported channel types (Channels) and languages
// (Languages); the concrete default title/content templates live in the i18n locale
// keys notify.<template>.title / notify.<template>.content (see
// internal/pkg/translate/locales). When rendering a message for a namespace without a
// matching persisted template row, the renderer falls back to that i18n default.
package notify

import (
	// stdlib
	"fmt"
	"slices"
	"text/template"

	// internal
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// 消息模板键常量（业务相关）。模板键由各事件源在通知层固化，随 message.Message
// 携带并在渲染时按 `notify.<template>.title` / `notify.<template>.content` 在 locales
// 翻译文件中查找对应语言的文本模板。此处隶属业务层，故定义在 notify 而非 message 基座。
// Message template keys (business-scoped). They are pinned by event sources in the
// notify layer and carried in message.Message; rendering looks up
// `notify.<template>.title` / `notify.<template>.content` in the locale files. They
// live in the business notify layer rather than the message base package.
const (
	// TemplateTagEventAlert 标签事件告警消息模板（tag_event 事件源）
	// TemplateTagEventAlert the tag event alert message template (tag_event source)
	TemplateTagEventAlert = "tag_event.alert"
)

// eventTemplateSpecs 事件模板注册表，键为事件类型键（Event key），是注册表的唯一事实来源。
// 每条规格声明该事件支持的模板键（Template）、渠道类型（Channels）、语言（Languages）与
// 样例渲染参数（SampleParams）。标题/正文默认内容不在此：已迁至 i18n 语言文件
// notify.<template>.title / notify.<template>.content，发送渲染时持久化模板行缺位回退到该键。
// SampleParams 用于前端试运行（以内置样例值渲染编辑器内标题/正文）并作为智能补全字段清单
// （EventFields）的数据源。
//
// eventTemplateSpecs is the event template registry keyed by event type key and the single
// source of truth. Each spec declares the event's template key (Template), supported channel
// types (Channels), languages (Languages) and sample render params (SampleParams). The default
// title/content no longer live here (moved to the i18n keys notify.<template>.title/.content);
// Send-time render falls back to those keys when no persisted template row exists. SampleParams
// back the frontend dry-run and seed the smart-completion field list (EventFields).
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
				map[string]any{"ConditionName": "条件A", "Dimension": "fpTcpClientSig", "Item": "10.0.0.1", "Count": 5, "Threshold": 3, "Model": "zhongkui.com/zhongkui/internal/models/network.HttpTransport", "Window": "5m0s", "RecordCount": 5},
				map[string]any{"ConditionName": "条件B", "Dimension": "fpTcpServerSig", "Item": "443", "Count": 128, "Threshold": 50, "Model": "zhongkui.com/zhongkui/internal/models/network.HttpTransport", "Window": "10m0s", "RecordCount": 128},
			},
		},
	},
}

// EventTemplateSpec 描述一个事件（Event）的默认通知模板元信息规格。
// 标题/正文默认内容不再挂在此处：已迁至 i18n 语言文件（键 notify.<template>.title/.content，
// 见 internal/pkg/translate），发送渲染时若持久化模板行缺位，回退按该键查找。
// 事件类型键（Event key）由注册表 map 的键承载，故本结构体不重复声明 Event 字段。
// EventTemplateSpec describes the metadata spec of an event's notification template.
// The default title/content no longer live here: they moved to the i18n locale files
// (keys notify.<template>.title/.content, see internal/pkg/translate); when no persisted
// template row exists the send-time render falls back to those keys. The event type key is
// carried by the registry map key, so the struct does not duplicate an Event field.
type EventTemplateSpec struct {
	// Template 消息模板键，取值定义在 notify.TemplateXxx 常量（如 notify.TemplateTagEventAlert）
	// Template the message template key, e.g. one of the notify.TemplateXxx constants
	// (notify.TemplateTagEventAlert)
	Template string
	// Channels 渠道类型键的并集（支持的类型），"*" 表示对所有渠道生效
	// Channels the supported channel type keys, "*" means all channels
	Channels []string
	// Languages 该事件支持的语言（base.Language 强类型），供前端枚举该事件的可用语言。
	// 注意：与 Channels 不同，语言不支持 "*" 通配——"对所有语言生效"由
	// SupportedLanguages 在并集时统一追加 * 表示。
	// Languages the languages supported by this event, typed as base.Language so callers
	// get a validated BCP-47 tag. Unlike Channels, "*" wildcard is not allowed here; the
	// "available in all languages" notion is expressed by SupportedLanguages appending "*".
	Languages []base.Language
	// SampleParams 该事件的样例渲染参数：供前端试运行预览，并作为智能补全字段
	// 清单（EventFields）的数据源；键为模板占位符（大驼峰）。
	// SampleParams the sample render params of this event, backing the frontend dry-run
	// and seeding the smart-completion field list (EventFields); keys match the template
	// placeholders in UpperCamelCase.
	SampleParams map[string]any
}

func (spec *EventTemplateSpec) TitleKey() string {
	return fmt.Sprintf(`notify.%s.title`, spec.Template)
}

func (spec *EventTemplateSpec) ContentKey() string {
	return fmt.Sprintf(`notify.%s.content`, spec.Template)
}

// validateTemplateSyntax 校验单条模板字符串的 Go text/template 语法。
// validateTemplateSyntax checks the Go text/template syntax of a single template string.
func validateTemplateSyntax(kind, text string) error {
	if _, err := template.New("default").Option("missingkey=zero").Parse(text); err != nil {
		return errors.Newf("invalid %s template: %w", kind, err)
	}
	return nil
}

// ValidateTemplateSyntax 导出校验单条模板字符串的 Go text/template 语法，
// 供语法校验接口在保存前对用户模板做权威校验。
// ValidateTemplateSyntax exported wrapper that validates the Go text/template syntax of
// a single template string, used by the syntax validation API as the authoritative check
// before persisting user templates.
func ValidateTemplateSyntax(kind, text string) error {
	return validateTemplateSyntax(kind, text)
}

// EventSampleParams 返回某事件（Event key）的样例渲染参数；未知事件返回 (nil, false)。
// 未注册的事件无样例参数时，试运行接口可回退为空 map 照常渲染。
// EventSampleParams returns the sample render params for an event (Event key), or
// (nil, false) for unknown events; the dry-run API may fall back to an empty map and
// still render when no sample params are registered.
func EventSampleParams(event string) (map[string]any, bool) {
	spec, ok := eventTemplateSpecs[event]
	if !ok {
		return nil, false
	}
	return spec.SampleParams, true
}

// EventFields 返回某事件的可用模板变量名清单：顶层键 + 数组元素对象的嵌套键（以供
// {{range .Hits}} 内部如 .ConditionName 等引用补全），去重后升序返回。
// EventFields returns the available template variable names of an event: top-level keys
// plus the nested keys of array element objects (for completing references inside
// {{range .Hits}} such as .ConditionName), deduplicated and sorted ascending.
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
// TemplateSpecs returns a safe copy of the event template spec registry keyed by the event
// type key, so callers can mutate it without affecting the registry. The set has no
// meaningful order: the order of iteration is not part of the contract, and presentation
// ordering is left to callers/the frontend according to their own display key.
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
// cloneParams deep-copies the sample render params map, recursing into []any of maps, so
// callers can safely mutate copies obtained from TemplateSpecs without affecting the registry.
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
// TemplateSpecOf returns the event type key and spec whose Template key matches templateKey,
// or ("", nil, false) when no spec matches.
func TemplateSpecOf(templateKey string) (string, *EventTemplateSpec, bool) {
	for event, s := range eventTemplateSpecs {
		if s.Template == templateKey {
			return event, &s, true
		}
	}
	return "", nil, false
}

// EventSpecOf 依据事件类型键在注册表中查找并返回对应规格；未找到时返回 (nil, false)。
// EventSpecOf returns the spec whose Event key matches event, or (nil, false) when no
// spec matches.
func EventSpecOf(event string) (*EventTemplateSpec, bool) {
	spec, ok := eventTemplateSpecs[event]
	if ok {
		return &spec, true
	}
	return nil, false
}

// SupportedChannels 返回注册表支持的渠道类型键的并集（升序），并保证包含 "*"，
// 供管理接口枚举可选渠道类型。
// SupportedChannels returns the union of the channel type keys supported by the
// registry in ascending order, always including "*", for management APIs to enumerate.
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
// SupportedLanguages returns the union of the language keys supported by the registry
// in ascending order, always including "*" and every platform language
// (base.SupportLanguages), for management APIs to enumerate.
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
