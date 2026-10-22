// Package capability 定义 AI 能力的数据模型与 Markdown 渲染；能力条目由视图层在接口声明处
// 提供（views.CapabilityItems），本包不持有业务注册表，也不导入任何业务包。
package capability

import (
	// 标准库
	"strings"
)

// Access 表示能力对业务数据的访问性质。
type Access string

const (
	// AccessRead 表示只读查询。
	AccessRead Access = "read"
	// AccessWrite 表示写入或变更。
	AccessWrite Access = "write"
)

// Valid 报告访问性质是否合法。
func (a Access) Valid() bool {
	return a == AccessRead || a == AccessWrite
}

// Capability 描述一条「用户意图 → 原生接口」的能力。
type Capability struct {
	// ID 是能力稳定标识，全局唯一。
	ID string
	// Domain 是业务域标识，用于地图分组。
	Domain string
	// DomainLabel 是业务域中文名，由能力来源按统一顺序提供。
	DomainLabel string
	// Intent 是一句话用户意图，面向模型与读者。
	Intent string
	// Method 是 HTTP 方法。
	Method string
	// Path 是完整接口路径。
	Path string
	// Entities 是该能力涉及的数据表。
	Entities []string
	// Access 是只读或写入。
	Access Access
	// Invokable 表示是否允许 api_invoke 直接调用。
	Invokable bool
}

// Lookup 按能力 ID 查找能力，未命中返回 false。
func Lookup(items []Capability, id string) (Capability, bool) {
	for _, item := range items {
		if item.ID == id {
			return item, true
		}
	}
	return Capability{}, false
}

// MapMarkdown 生成能力地图，用于人类文档投影与 RAG 能力分区的整体索引；不再常驻模型上下文。
// 调用方需按业务域分组并稳定排序（见 views.CapabilityItems），本函数只负责渲染；无能力时返回空串。
func MapMarkdown(items []Capability) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("# 平台能力地图（只读参考）\n\n")
	b.WriteString("以下按业务域列出平台已登记的原生接口。判断用户需求涉及哪个业务对象时以此为准；")
	b.WriteString("不要编造未列出的接口或字段，也不要假设接口已被执行。需要精确入参、出参与字段含义时，调用 knowledge_search 并传 format=native_api。\n")
	current := ""
	for _, item := range items {
		if item.Domain != current {
			current = item.Domain
			b.WriteString("\n## ")
			b.WriteString(domainLabel(item))
			b.WriteString("\n")
		}
		b.WriteString("- ")
		b.WriteString(item.Intent)
		b.WriteString(" → `")
		b.WriteString(item.Method)
		b.WriteString(" ")
		b.WriteString(item.Path)
		b.WriteString("`（")
		if item.Access == AccessRead {
			b.WriteString("只读")
		} else {
			b.WriteString("写入")
		}
		if item.Invokable {
			b.WriteString("；可调用：是")
		} else {
			b.WriteString("；仅参考")
		}
		b.WriteString("；实体：")
		b.WriteString(strings.Join(item.Entities, "、"))
		b.WriteString("）\n")
	}
	return b.String()
}

// domainLabel 返回能力的业务域展示名；能力来源未提供时回退为业务域标识。
func domainLabel(item Capability) string {
	if strings.TrimSpace(item.DomainLabel) != "" {
		return item.DomainLabel
	}
	return item.Domain
}

// KnowledgeDoc 是能力分区入 RAG 的一篇生成文档：Title 为同步与去重的稳定键，Content 为正文。
type KnowledgeDoc struct {
	// Title 文档标题，同时作为入库与同步判定的稳定键。
	Title string
	// Content Markdown 正文。
	Content string
}

// KnowledgeDocs 返回能力分区的生成文档集：一篇整体能力地图，以及每条能力一篇明细。
// 明细使检索能按「用户意图」直接命中具体能力，而不必先取回整张地图；文档集与入参同源，
// 内容对同一能力集逐字节确定，保证 zki sync capabilities 幂等。
func KnowledgeDocs(items []Capability) []KnowledgeDoc {
	docs := make([]KnowledgeDoc, 0, len(items)+1)
	docs = append(docs, KnowledgeDoc{Title: "平台能力地图", Content: MapMarkdown(items)})
	for _, item := range items {
		docs = append(docs, KnowledgeDoc{
			Title:   "平台能力：" + item.ID,
			Content: describeMarkdown(item),
		})
	}
	return docs
}

// describeMarkdown 生成单条能力的 Markdown 说明：意图、接口、读写属性、可调用性与关联实体。
func describeMarkdown(item Capability) string {
	var b strings.Builder
	b.WriteString("# 平台能力：")
	b.WriteString(item.ID)
	b.WriteString("\n\n")
	b.WriteString("- 用户意图：")
	b.WriteString(item.Intent)
	b.WriteString("\n- 接口：`")
	b.WriteString(item.Method)
	b.WriteString(" ")
	b.WriteString(item.Path)
	b.WriteString("`\n- 业务域：")
	b.WriteString(domainLabel(item))
	b.WriteString("\n- 访问性质：")
	if item.Access == AccessRead {
		b.WriteString("只读")
	} else {
		b.WriteString("写入")
	}
	b.WriteString("\n- 可调用：")
	if item.Invokable {
		b.WriteString("是")
	} else {
		b.WriteString("否")
	}
	b.WriteString("\n- 关联实体：")
	b.WriteString(strings.Join(item.Entities, "、"))
	b.WriteString("\n\n需要精确的入参、出参与字段含义时，调用 knowledge_search 并传 format=native_api，或查阅接口参考文档。\n")
	return b.String()
}
