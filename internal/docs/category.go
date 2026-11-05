// Package docs 提供基于 Diátaxis 分类的文档注册表与导出框架：
// 定义文档分类、登记各文档条目，并借助 gomarkdoc 将功能包的注释导出为 Markdown。
package docs

import "slices"

// Category 表示 Diátaxis 文档分类。
type Category string

const (
	// CategoryTutorial 教程分类（学习导向），页面位于策展目录的教程分区。
	CategoryTutorial Category = "tutorial"
	// CategoryHowTo 操作指南分类（任务导向），页面位于策展目录的操作指南分区。
	CategoryHowTo Category = "how-to"
	// CategoryReference 参考分类（信息导向），页面位于生成目录的参考分区。
	CategoryReference Category = "reference"
	// CategoryExplanation 解释分类（理解导向），生成页与策展页均可使用。
	CategoryExplanation Category = "explanation"
)

// Categories 返回全部文档分类，是分类集合的唯一注册点。
func Categories() []Category {
	return []Category{
		CategoryTutorial,
		CategoryHowTo,
		CategoryReference,
		CategoryExplanation,
	}
}

// Valid 报告该分类是否为已注册的合法分类。
func (c Category) Valid() bool {
	return slices.Contains(Categories(), c)
}

// Label 返回该分类的中文标签。
func (c Category) Label() string {
	switch c {
	case CategoryTutorial:
		return "教程"
	case CategoryHowTo:
		return "操作指南"
	case CategoryReference:
		return "参考"
	case CategoryExplanation:
		return "解释"
	default:
		return string(c)
	}
}
