// 本文件把应用支持语言、RBAC 策略动作与 MIME 媒体类型注册为枚举类
// language / policyEffect / mediaType。取值空间在本包已有唯一来源——语言取自
// SupportLanguages、策略动作取自 EffectAllow / EffectDeny、媒体类型取自 MediaType* 常量——
// 故此处只按该来源派生条目并在包内 init 注册，不重复声明取值常量。类名须与 search.toml
// 字段绑定的引用一致。
package base

import (
	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/enum"
)

// languageValues 是 language 枚举类的取值空间，按 SupportLanguages 的声明顺序派生（该顺序即枚举
// 类条目顺序与前端下拉的展示顺序），取值为语言标签字符串。
var languageValues = func() []string {
	values := make([]string, 0, len(SupportLanguages))
	for i := range SupportLanguages {
		values = append(values, SupportLanguages[i].String())
	}
	return values
}()

// policyEffectValues 是 policyEffect 枚举类的取值空间，取自 RBAC 策略动作常量
// （EffectAllow / EffectDeny），声明顺序即枚举类条目顺序与前端下拉的展示顺序。
var policyEffectValues = []string{EffectAllow, EffectDeny}

// mediaTypeValues 是 mediaType 枚举类的取值空间，取自 MIME 媒体类型常量，声明顺序即
// 枚举类条目顺序与前端下拉的展示顺序。结构化语法后缀（如 +json / +xml）不是完整媒体类型，
// 不纳入枚举条目。
var mediaTypeValues = []string{
	MediaTypeJSON,
	MediaTypeTextJSON,
	MediaTypeForm,
	MediaTypeHTML,
	MediaTypeXHTML,
	MediaTypeXML,
	MediaTypeTextXML,
	MediaTypeTextPlain,
}

// init 注册枚举类 language、policyEffect 与 mediaType（取值空间固定，条目在注册时快照）。
func init() {
	enum.Register(enum.Type{Name: "language", Entries: enum.EntriesOf(languageValues)})
	enum.Register(enum.Type{Name: "policyEffect", Entries: enum.EntriesOf(policyEffectValues)})
	enum.Register(enum.Type{Name: "mediaType", Entries: enum.EntriesOf(mediaTypeValues)})
}
