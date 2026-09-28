package utils

// EnumOption 定义可选枚举值项。
// 由元信息自省（meta.go）从声明的枚举注册表中读取，随 FieldMeta 的
// enumOptions 字段下发给前端：
//   - Key：字段内唯一标识，前端借此查找 i18n 键（model.<model>.<entity>.option.<key>）翻译展示
//   - Value：提交/存储的枚举值（多数与 Key 相同，个别富枚举允许不同）
type EnumOption struct {
	// Key 字段内唯一标识，前端借此查找 i18n 键翻译展示
	Key string `json:"key"`
	// Value 提交/存储的枚举值，多数与 Key 相同
	Value string `json:"value"`
}
