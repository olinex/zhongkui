package docs

import "strings"

// mdCell 转义 Markdown 表格单元格中的竖线与换行，空值以短横线占位。
func mdCell(value string) string {
	if value == "" {
		return "-"
	}
	value = strings.ReplaceAll(value, "|", "\\|")
	return strings.ReplaceAll(value, "\n", " ")
}
