// Package f 演示正文里提到生成标记的文件仍参与检查。
package f

// Marker 生成标记文本（正文提及不算生成文件）。
const Marker = "DO NOT EDIT"

func BadFunc() {} // want "导出函数 BadFunc 缺少注释"
