// Package notify 提供通知事件模板注册表、消息模板校验与渲染所需的事件元信息，以及通用通知
// 服务（Service）：把一次通知请求解析为具体目标、按目标语言渲染模板、经渠道基座投递并维护
// notification_records 发送记录。
//
// 新增通知渠道与模板的分步清单见 docs/curated/how-to/add-notify-channel.md；本包注释只描述
// 注册表、渲染与发送去重的职责边界。
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
// range 循环内部如 .ConditionName 等引用补全），去重后升序返回。
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
