// 本文件定义消息事件模板注册表（event template registry），作为命名空间级默认通知
// 模板（NotificationTemplate，source=default）的种子来源。每个事件（Event）声明其
// 支持的消息模板（Template）、渠道类型（Channels）、语言（Languages）以及各语言下
// 的标题/正文 Go text/template 模板（TitleByLang/ContentByLang）。种子回填逻辑即按
// 此注册表为各命名空间写入默认模板行（Task 3 实现），此处仅声明规格并给出枚举辅助
// 函数。
//
// This file defines the event template registry, the seed source of the default
// namespace-scoped notification templates (NotificationTemplate, source=default).
// Each event declares its message template key (Template), supported channel types
// (Channels), languages (Languages) and the per-language Go text/template title and
// content templates (TitleByLang/ContentByLang). The seeding/backfill logic that
// writes default template rows for each namespace from this registry is implemented
// elsewhere (Task 3); this file only declares the specs and enumeration helpers.
package notify

import (
	// stdlib
	"maps"
	_ "embed"
	"fmt"
	"slices"
	"text/template"

	// third-party
	"github.com/pelletier/go-toml/v2"

	// internal
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// defaultTemplatesFile 内嵌的默认通知模板注册表文件（TOML），启动时解析为
// defaultTemplateSpecs。默认模板内容与代码分离，便于独立维护。
//
// defaultTemplatesFile is the embedded default notification template registry file
// (TOML), parsed into defaultTemplateSpecs at init. Default template content is kept
// separate from code for easier maintenance.
//
//go:embed default_templates.toml
var defaultTemplatesFile []byte

// sampleParamsByEvent 各事件（Event key）的样例渲染参数，键为事件类型键。这些参数
// 用于前端试运行：用内置样例值渲染编辑器内的标题/正文，让用户无需手填参数即可预览
// 效果；其键也作为智能补全的可选变量清单（EventFields）的数据源。
//
// sampleParamsByEvent holds the sample render params per event (keyed by event type key).
// They back the frontend dry-run: the in-editor title/content is rendered against these
// built-in sample values so users can preview without filling params manually; their keys
// also seed the smart-completion field list (EventFields).
var sampleParamsByEvent = map[string]map[string]any{
	core.EventTypeTagEventRecord: {
		"Namespace":  "示例命名空间",
		"RuleName":   "示例规则名",
		"DetectedAt": "2026-01-01T00:00:00Z",
		"Hits": []any{
			map[string]any{"ConditionName": "条件A", "Dimension": "src_ip", "Item": "10.0.0.1", "Count": 5, "Threshold": 3},
			map[string]any{"ConditionName": "条件B", "Dimension": "dst_port", "Item": "443", "Count": 128, "Threshold": 50},
		},
	},
}

// defaultTemplateSpecs 内置默认事件模板规格注册表，由内嵌的 default_templates.toml
// 在初始化时加载生成；种子回填（Task 3）据此为各命名空间写入 source=default 的
// 默认通知模板行。文件解析或模板语法非法时 panic（启动即失败，快速暴露）。
// defaultTemplateSpecs is the built-in default event template spec registry, loaded
// from the embedded default_templates.toml at init; the seed backfill (Task 3) writes
// source=default notification template rows per namespace from it. A parse failure or
// invalid template syntax panics at init to surface errors early.
var defaultTemplateSpecs = mustLoadEventTemplateSpecs()

// eventTemplateFile 默认通知模板注册表文件的 TOML 结构，仅用于解析，与
// EventTemplateSpec 分离（前者是磁盘形态，后者是内存规格）。
//
// eventTemplateFile is the TOML structure of the default template registry file, used
// only for parsing; it is decoupled from EventTemplateSpec (the former is the on-disk
// shape, the latter the in-memory spec).
type eventTemplateFile struct {
	// Events 事件模板条目数组
	// Events the array of event template entries
	Events []eventTemplateFileEvent `toml:"event"`
}

// eventTemplateFileEvent 单条事件模板的 TOML 结构。
// eventTemplateFileEvent is the TOML structure of a single event template entry.
type eventTemplateFileEvent struct {
	// Event 事件类型键
	// Event the event type key
	Event string `toml:"event"`
	// Template 消息模板键
	// Template the message template key
	Template string `toml:"template"`
	// Channels 渠道类型键并集
	// Channels the union of channel type keys
	Channels []string `toml:"channels"`
	// Languages 语言键并集
	// Languages the union of language keys
	Languages []string `toml:"languages"`
	// Title 各语言下的标题模板，键为语言
	// Title the per-language title templates, keyed by language
	Title map[string]string `toml:"title"`
	// Content 各语言下的正文模板，键为语言
	// Content the per-language content templates, keyed by language
	Content map[string]string `toml:"content"`
}

// EventTemplateSpec 描述一个事件（Event）的默认通知模板规格。
// EventTemplateSpec describes the default notification template spec of an event.
type EventTemplateSpec struct {
	// Event 事件类型键，取值定义在 core.EventTypeXxx 常量（如 core.EventTypeTagEventRecord）
	// Event the event type key, e.g. one of the core.EventTypeXxx constants
	// (core.EventTypeTagEventRecord)
	Event string
	// Template 消息模板键，取值定义在 notify.TemplateXxx 常量（如 notify.TemplateTagEventAlert）
	// Template the message template key, e.g. one of the notify.TemplateXxx constants
	// (notify.TemplateTagEventAlert)
	Template string
	// Channels 渠道类型键的并集（支持的类型），"*" 表示对所有渠道生效
	// Channels the supported channel type keys, "*" means all channels
	Channels []string
	// Languages 语言键的并集（支持的语言），"*" 表示对所有语言生效
	// Languages the supported language keys, "*" means all languages
	Languages []string
	// TitleByLang 各语言下的标题模板（Go text/template），键为语言（如 zh-Hans、en）
	// TitleByLang the per-language title template (Go text/template), keyed by language
	TitleByLang map[string]string
	// ContentByLang 各语言下的正文模板（Go text/template），键为语言（如 zh-Hans、en）
	// ContentByLang the per-language content template (Go text/template), keyed by language
	ContentByLang map[string]string
}

// mustLoadEventTemplateSpecs 解析内嵌的 default_templates.toml 并转换为
// EventTemplateSpec 切片，同时校验每条标题/正文模板语法合法。任一失败即 panic。
//
// mustLoadEventTemplateSpecs parses the embedded default_templates.toml into a slice of
// EventTemplateSpec, validating that every title/content template has valid syntax.
// Any failure panics.
func mustLoadEventTemplateSpecs() []EventTemplateSpec {
	var file eventTemplateFile
	if err := toml.Unmarshal(defaultTemplatesFile, &file); err != nil {
		panic("notify: failed to parse embedded default_templates.toml: " + err.Error())
	}

	specs := make([]EventTemplateSpec, 0, len(file.Events))
	for _, e := range file.Events {
		spec := EventTemplateSpec{
			Event:         e.Event,
			Template:      e.Template,
			Channels:      slices.Clone(e.Channels),
			Languages:     slices.Clone(e.Languages),
			TitleByLang:   e.Title,
			ContentByLang: e.Content,
		}
		if err := validateEventTemplateSpec(&spec); err != nil {
			panic(fmt.Sprintf("notify: invalid default template for event %q: %v", e.Event, err))
		}
		specs = append(specs, spec)
	}
	return specs
}

// validateEventTemplateSpec 校验单条事件规格的所有标题/正文模板语法合法；
// Event 与 Template 键缺失也视为非法。lang 为出错语言（用于报错定位）。
//
// validateEventTemplateSpec checks that every title/content template of a spec has valid
// Go text/template syntax; a missing Event or Template key is also invalid.
func validateEventTemplateSpec(spec *EventTemplateSpec) error {
	if spec.Event == "" {
		return errors.New("event key is empty")
	}
	if spec.Template == "" {
		return errors.New("template key is empty")
	}
	for _, lang := range spec.Languages {
		if _, ok := spec.TitleByLang[lang]; ok {
			if err := validateTemplateSyntax("title", spec.TitleByLang[lang]); err != nil {
				return errors.WithStack(fmt.Errorf("lang %q title: %w", lang, err))
			}
		}
		if _, ok := spec.ContentByLang[lang]; ok {
			if err := validateTemplateSyntax("content", spec.ContentByLang[lang]); err != nil {
				return errors.WithStack(fmt.Errorf("lang %q content: %w", lang, err))
			}
		}
	}
	return nil
}

// validateTemplateSyntax 校验单条模板字符串的 Go text/template 语法。
// validateTemplateSyntax checks the Go text/template syntax of a single template string.
func validateTemplateSyntax(kind, text string) error {
	if _, err := template.New("default").Option("missingkey=zero").Parse(text); err != nil {
		return errors.WithStack(fmt.Errorf("invalid %s template: %w", kind, err))
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
	params, ok := sampleParamsByEvent[event]
	if !ok {
		return nil, false
	}
	return params, true
}

// EventFields 返回某事件的可用模板变量名清单：顶层键 + 数组元素对象的嵌套键（以供
// {{range .Hits}} 内部如 .ConditionName 等引用补全），去重后升序返回。
// EventFields returns the available template variable names of an event: top-level keys
// plus the nested keys of array element objects (for completing references inside
// {{range .Hits}} such as .ConditionName), deduplicated and sorted ascending.
func EventFields(event string) []string {
	params, ok := sampleParamsByEvent[event]
	if !ok {
		return nil
	}
	set := make(map[string]bool, len(params))
	for k := range params {
		set[k] = true
	}
	for _, v := range params {
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

// TemplateSpecs 返回事件模板注册表的规格副本，调用方可安全修改而不会污染注册表。
// TemplateSpecs returns a copy of the event template spec registry so callers can
// mutate it safely without affecting the registry.
func TemplateSpecs() []EventTemplateSpec {
	specs := make([]EventTemplateSpec, 0, len(defaultTemplateSpecs))
	for _, s := range defaultTemplateSpecs {
		spec := EventTemplateSpec{
			Event:         s.Event,
			Template:      s.Template,
			Channels:      slices.Clone(s.Channels),
			Languages:     slices.Clone(s.Languages),
			TitleByLang:   make(map[string]string, len(s.TitleByLang)),
			ContentByLang: make(map[string]string, len(s.ContentByLang)),
		}
		maps.Copy(spec.TitleByLang, s.TitleByLang)
		maps.Copy(spec.ContentByLang, s.ContentByLang)
		specs = append(specs, spec)
	}
	return specs
}

// TemplateSpecOf 依据消息模板键在注册表中查找并返回对应规格；未找到时返回 (nil, false)。
// TemplateSpecOf returns the spec whose Template key matches templateKey, or (nil, false)
// when no spec matches.
func TemplateSpecOf(templateKey string) (*EventTemplateSpec, bool) {
	for _, s := range defaultTemplateSpecs {
		if s.Template == templateKey {
			return &s, true
		}
	}
	return nil, false
}

// EventSpecOf 依据事件类型键在注册表中查找并返回对应规格；未找到时返回 (nil, false)。
// EventSpecOf returns the spec whose Event key matches event, or (nil, false) when no
// spec matches.
func EventSpecOf(event string) (*EventTemplateSpec, bool) {
	for _, s := range defaultTemplateSpecs {
		if s.Event == event {
			return &s, true
		}
	}
	return nil, false
}

// EventTypes 返回注册表中出现的去重事件类型键（升序），供管理接口枚举可选事件类型。
// EventTypes returns the distinct event type keys present in the registry in ascending
// order, for management APIs to enumerate available event types.
func EventTypes() []string {
	seen := make(map[string]bool, len(defaultTemplateSpecs))
	types := make([]string, 0, len(defaultTemplateSpecs))
	for _, s := range defaultTemplateSpecs {
		if !seen[s.Event] {
			seen[s.Event] = true
			types = append(types, s.Event)
		}
	}
	slices.Sort(types)
	return types
}

// SupportedChannels 返回注册表支持的渠道类型键的并集（升序），并保证包含 "*"，
// 供管理接口枚举可选渠道类型。
// SupportedChannels returns the union of the channel type keys supported by the
// registry in ascending order, always including "*", for management APIs to enumerate.
func SupportedChannels() []string {
	seen := map[string]bool{"*": true}
	for _, s := range defaultTemplateSpecs {
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
	for _, s := range defaultTemplateSpecs {
		for _, l := range s.Languages {
			seen[l] = true
		}
	}
	languages := make([]string, 0, len(seen))
	for l := range seen {
		languages = append(languages, l)
	}
	slices.Sort(languages)
	return languages
}
