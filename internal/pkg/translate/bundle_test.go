package translate

import (
	"bytes"
	"strings"
	"testing"
	"text/template"

	. "github.com/smartystreets/goconvey/convey"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/enum"
)

// TestSupportLanguages 测试应用支持的语言列表
func TestSupportLanguages(t *testing.T) {
	Convey("Test SupportLanguages", t, func() {
		Convey("should support zh-Hans and en", func() {
			So(len(base.SupportLanguages), ShouldEqual, 2)
			So(base.SupportLanguages[0].StdTag().String(), ShouldEqual, "zh-Hans")
			So(base.SupportLanguages[1].StdTag().String(), ShouldEqual, "en")
		})
	})
}

// TestBundle 测试消息包的初始化
func TestBundle(t *testing.T) {
	Convey("Test Bundle", t, func() {
		Convey("Bundle should be initialized at package init", func() {
			So(Bundle, ShouldNotBeNil)
		})

		Convey("newBundle should return a non-nil bundle", func() {
			So(newBundle(), ShouldNotBeNil)
		})
	})
}

// TestLocalize 测试本地化函数
func TestLocalize(t *testing.T) {
	Convey("Test Localize", t, func() {
		zhLang, err := base.ParseLanguage("zh-Hans")
		So(err, ShouldBeNil)

		Convey("should translate an existing key", func() {
			// locales/zh-Hans.toml 中 [code] 200 = "成功"
			So(Localize(zhLang, "code.200", nil), ShouldEqual, "成功")
		})

		Convey("should return the key itself for a missing key", func() {
			// 未命中 key 时回退为 key 本身，由调用方（如 cliT）处理中文 fallback
			So(Localize(zhLang, "missing.key", nil), ShouldEqual, "missing.key")
		})

		Convey("external @file reference is expanded and passed through verbatim under the [[ ]] data delimiter", func() {
			// notify.password.reset.content 声明为 "@emails/password-reset-content.zh-Hans.html"，
			// 全局数据定界符为 [[ / ]]（dataDelimParser），该源串含 {{.ResetURL}} 不含 [[，go-i18n
			// 原样透出"未渲染的原始 HTML 模板"；通知默认模板统一经 text/template 渲染，而非 go-i18n
			// 数据渲染。
			raw := Localize(zhLang, "notify.password.reset.content", map[string]any{"ResetURL": "https://example.com/reset?token=abc"})
			So(raw, ShouldNotEqual, "@emails/password-reset-content.zh-Hans.html")
			So(raw, ShouldContainSubstring, "{{.ResetURL}}")
			So(raw, ShouldContainSubstring, "<html")
		})
	})
}

// TestLocalizer 测试创建本地化器并翻译英文消息
func TestLocalizer(t *testing.T) {
	Convey("Test Localizer", t, func() {
		enLang, err := base.ParseLanguage("en")
		So(err, ShouldBeNil)

		Convey("should create a localizer for a supported language", func() {
			So(Localizer(enLang), ShouldNotBeNil)
		})

		Convey("should translate an existing English key", func() {
			// locales/en.toml 中 [code] 200 = "Ok"
			So(Localize(enLang, "code.200", nil), ShouldEqual, "Ok")
		})
	})
}

// ensureTestEnumClasses 幂等注册本测试用枚举类（translate 测试二进制不链接各枚举类的归属包，
// 其 init() 才注册该类；此处按同一契约注册 tagRuleModel / channelType，已存在则跳过）。
func ensureTestEnumClasses() {
	for _, t := range []enum.Type{
		{
			Name: "channelType",
			Entries: []enum.Entry{
				{Key: "email", Value: "email"},
				{Key: "webhook", Value: "webhook"},
			},
		},
		{
			Name: "tagRuleModel",
			Entries: []enum.Entry{
				{Key: "httpTransport", Value: "zhongkui.com/zhongkui/internal/models/network.HttpTransport"},
			},
		},
		{
			Name: "tagEventDimension",
			Entries: []enum.Entry{
				{Key: "fpTcpClientSig", Value: "fpTcpClientSig"},
				{Key: "fpTcpServerSig", Value: "fpTcpServerSig"},
			},
		},
	} {
		if !enum.Exists(t.Name) {
			enum.Register(t)
		}
	}
}

// renderNotifyRaw 以目标语言渲染 notify 默认模板的“原始模板串”：注入与 notify 层
// renderFuncs 一致的三个模板函数（T 直接翻译 i18n 键，TenumKey/TenumValue 按枚举
// key/value 经 enum 注册表解析 label），供本包测试验证带函数调用的默认模板渲染路径。
func renderNotifyRaw(lang *base.Language, raw string, params map[string]any) string {
	tmpl, err := template.New("render").Option("missingkey=zero").Funcs(template.FuncMap{
		"T": func(key string) string {
			return Localize(lang, key, nil)
		},
		"TenumKey": func(className, key string) string {
			name := strings.TrimPrefix(className, "enum.")
			return EnumLabel(lang, name, key)
		},
		"TenumValue": func(className, val string) string {
			name := strings.TrimPrefix(className, "enum.")
			if typ, exists := enum.Get(name); exists {
				if entry, ok := typ.Resolve(val); ok {
					return EnumLabel(lang, name, entry.Key)
				}
			}
			return val
		},
	}).Parse(raw)
	if err != nil {
		return ""
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return ""
	}
	return buf.String()
}

// TestRenderNotifyTemplateWithFuncs 验证 notify 默认模板经“原始模板串 + 模板函数”的渲染路径：
// 模板内 TenumValue 调用按目标语言解析枚举 label（目标模型 httpTransport、标签维度 fpTcp*Sig 等），
// 缺失键回退为 key 本身。对应 notify 层 renderFuncs/renderI18nDefault 的实现。
func TestRenderNotifyTemplateWithFuncs(t *testing.T) {
	Convey("Test Render Notify Template With Funcs", t, func() {
		ensureTestEnumClasses()
		zhLang, err := base.ParseLanguage("zh-Hans")
		So(err, ShouldBeNil)
		enLang, err := base.ParseLanguage("en")
		So(err, ShouldBeNil)

		Convey("标题经原始串按参数渲染出最终文案（不含 t 调用，与旧路径一致）", func() {
			raw := Localize(zhLang, "notify.tag_event.alert.title", nil)
			So(raw, ShouldNotEqual, "notify.tag_event.alert.title")
			So(renderNotifyRaw(zhLang, raw, map[string]any{"RuleName": "告警规则"}), ShouldEqual, "标签事件告警: 告警规则")
		})

		Convey("正文经 TenumValue 把目标模型与维度 value 解析为可读 label", func() {
			raw := Localize(zhLang, "notify.tag_event.alert.content", nil)
			So(raw, ShouldNotEqual, "notify.tag_event.alert.content")
			rendered := renderNotifyRaw(zhLang, raw, map[string]any{
				"Namespace":  "default",
				"RuleName":   "告警规则",
				"DetectedAt": "2026-01-01T00:00:00Z",
				"Hits": []map[string]any{
					{"ConditionName": "C1", "Dimension": "fpTcpClientSig", "Item": "10.0.0.1", "Count": 12, "Threshold": 10, "Model": "zhongkui.com/zhongkui/internal/models/network.HttpTransport", "Window": "5m0s", "RecordCount": 5},
					{"ConditionName": "C2", "Dimension": "fpTcpServerSig", "Item": "443", "Count": 8, "Threshold": 5, "Model": "zhongkui.com/zhongkui/internal/models/network.HttpTransport"},
				},
			})
			So(rendered, ShouldContainSubstring, "命名空间 : default")
			So(rendered, ShouldContainSubstring, "告警规则")
			So(rendered, ShouldContainSubstring, "条件「C1」")
			So(rendered, ShouldContainSubstring, "维度/项 : 客户端指纹签名 / 10.0.0.1")
			So(rendered, ShouldContainSubstring, "目标模型 : HTTP 传输")
			So(rendered, ShouldContainSubstring, "维度/项 : 服务端指纹签名 / 443")
			// 缺失 Model 字段条件分支时跳过目标模型行，不渲染空行
			So(rendered, ShouldContainSubstring, "涉及记录 : 5 条")
		})

		Convey("英语经 TenumValue 渲染出英文 label", func() {
			raw := Localize(enLang, "notify.tag_event.alert.content", nil)
			So(raw, ShouldNotEqual, "notify.tag_event.alert.content")
			rendered := renderNotifyRaw(enLang, raw, map[string]any{
				"Namespace":  "default",
				"RuleName":   "rule",
				"DetectedAt": "2026-01-01T00:00:00Z",
				"Hits": []map[string]any{
					{"ConditionName": "C1", "Dimension": "fpTcpServerSig", "Item": "443", "Count": 8, "Threshold": 5, "Model": "zhongkui.com/zhongkui/internal/models/network.HttpTransport", "Window": "10m0s"},
				},
			})
			So(rendered, ShouldContainSubstring, "Namespace   : default")
			So(rendered, ShouldContainSubstring, "Dimension/Item : Server Fingerprint Signature / 443")
			So(rendered, ShouldContainSubstring, "Target model   : HTTP Transport")
		})

		Convey("缺失键回退为 key 本身", func() {
			// T 对未配置的 i18n 键回退为键本身，符合回退契约
			rendered := renderNotifyRaw(zhLang, `{{ T "model.no.such.key" }}`, nil)
			So(rendered, ShouldEqual, "model.no.such.key")
		})
	})
}
