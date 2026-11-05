package docs

import (
	// 标准库
	"sort"
	"strings"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/enum"
	"zhongkuisec.com/zhongkui/internal/pkg/translate"
)

// enumReferenceProjection 生成枚举参考页：每个已注册枚举类一节，列出 key、存储值与展示名。
func enumReferenceProjection() Projection {
	types := enum.List()
	sort.Slice(types, func(i, j int) bool { return types[i].Name < types[j].Name })
	return Projection{
		Title:       "枚举参考",
		Summary:     "列出全部已注册枚举类的 key、数据库存储值与中英文展示名。",
		Roles:       []Role{RoleDev, RoleSecOps},
		Scene:       "需要确认枚举取值、数据库存储值或前后端展示文案时阅读。",
		Output:      GeneratedRoot + "/reference/enums.md",
		SourceLabel: "internal/pkg/enum",
		Content:     renderEnums(types),
	}
}

// enumLabel 返回枚举条目的展示名；缺失或未翻译时回退为短横线。
func enumLabel(typ *enum.Type, entry enum.Entry) string {
	key := typ.LabelKey(entry.Key)
	label := translate.T(key, nil)
	if strings.TrimSpace(label) == "" || label == key {
		return "-"
	}
	return label
}

// renderEnums 生成枚举参考正文。
func renderEnums(types []*enum.Type) string {
	var b strings.Builder
	b.WriteString("## 登记约定\n\n")
	b.WriteString("每个枚举类在取值空间的归属包内唯一注册；`Key` 用于 i18n 定位，`存储值` 是数据库中的实际取值。")
	b.WriteString("同一个类内 key 与 value 各自唯一，富枚举场景下两者可以不同。\n")
	for _, typ := range types {
		b.WriteString("\n## ")
		b.WriteString(typ.Name)
		b.WriteString("\n\n| Key | 存储值 | 展示名 |\n| --- | --- | --- |\n")
		for _, entry := range typ.Entries {
			b.WriteString("| `")
			b.WriteString(mdCell(entry.Key))
			b.WriteString("` | `")
			b.WriteString(mdCell(entry.Value))
			b.WriteString("` | ")
			b.WriteString(mdCell(enumLabel(typ, entry)))
			b.WriteString(" |\n")
		}
	}
	return b.String()
}
