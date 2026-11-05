// 本文件承载「原生接口知识目录」：把各视图域在路由注册时声明的接口描述与入参/出参元信息，
// 收敛为一个进程内注册表，供接口知识入 RAG（zki sync apis）与守卫测试消费。
//
// 声明方式：全部原生接口经 RegisterApi / RegisterListApi 声明，二者在向 gin 注册 OPTIONS 元信息
// 路由的同时，把「完整路径 + 各 HTTP 方法元信息 + 单行用途 + RAG 开关」记入目录；禁止在 helper
// 之外直接调用 StructMetaGinHandler / ListMetaGinHandler（由守卫测试拦截）。
//
// 与 agent 工具调用链的关系：目录是「Agent 能获知有哪些原生接口、各自用途与入参出参」的唯一来源；
// 后续 api_invoke 工具的白名单与参数构造均复用本目录，故本次先落地声明与入 RAG 能力。
package views

import (
	// 标准库
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	// 第三方
	"github.com/gin-gonic/gin"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/enum"
	"zhongkuisec.com/zhongkui/internal/pkg/utils"
)

// ApiDescriptor 描述一个原生接口，以 (HTTP 方法, 完整路径) 为稳定键。
type ApiDescriptor struct {
	// Method HTTP 方法名（如 POST）。
	Method string
	// Path 完整路径，含 /api/v1 前缀与 :param 占位符。
	Path string
	// Description 单行中文用途说明。
	Description string
	// RagEnabled 是否纳入 RAG 接口知识目录。
	RagEnabled bool
	// Meta 该方法的入参/出参结构体元信息。
	Meta *MethodMeta
	// Capability AI 能力元数据；未声明时为 nil，该接口不进入能力地图与 api_invoke 白名单。
	Capability *CapabilityMeta
}

// ApiKnowledgeDoc 是接口知识文档的渲染结果，可直接映射为知识库生成文档。
type ApiKnowledgeDoc struct {
	// Title 文档标题，同时作为 RAG 入库与去重的稳定键。
	Title string
	// Content Markdown 正文。
	Content string
}

// apiRegistry 是接口目录的进程内注册表，键为 method + "\x00" + 完整路径。
// 重复登记以最后一次为准（幂等）；保护锁避免并发测试下的读写竞态。
var apiRegistry = map[string]ApiDescriptor{}

var apiRegistryMu sync.Mutex

// RegisterApi 声明一个原生接口：向 gin 注册其 OPTIONS 元信息路由，并同步记入接口目录。
// description 为单行中文用途说明，须非空（空即 panic，避免无用途条目进入 RAG）；
// opts 以 WithMethod 声明各 HTTP 方法的入参/出参结构体，以 WithRagEnabled 控制是否纳入 RAG。
//
// 典型用法：
//
//	views.RegisterApi(tagRuleGroup, "/delete", "批量删除标签规则及其全部历史版本",
//		views.WithMethod(http.MethodPost, views.WithBodyInput[TagRuleBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
//	)
func RegisterApi(group *gin.RouterGroup, path, description string, opts ...MetadataOption) {
	options := &metadataOptions{}
	for _, opt := range opts {
		opt(options)
	}
	options.description = description
	fullPath := joinApiPath(group.BasePath(), path)
	assertApiDescription(fullPath, description)
	group.OPTIONS(path, structMetaHandler(options))
	recordApi(fullPath, options)
}

// RegisterListApi 声明一个列表接口，语义与 RegisterListApi 相同，但按列表接口自省语义补齐
// POST 方法的入参/出参（搜索输入 M 与列表输出 N），与既有 ListMetaGinHandler 行为一致。
//
// 典型用法：
//
//	views.RegisterListApi[*views.TagRuleListGetSearchIn, *TagRuleListOut](tagRuleGroup, "/list", "标签规则列表查询")
func RegisterListApi[M utils.SearcherIn, N utils.TotalOut](group *gin.RouterGroup, path, description string, opts ...MetadataOption) {
	options := listMetaOptions[M, N](opts...)
	options.description = description
	fullPath := joinApiPath(group.BasePath(), path)
	assertApiDescription(fullPath, description)
	group.OPTIONS(path, listMetaHandler[M, N](options))
	recordApi(fullPath, options)
}

// assertApiDescription 校验接口用途非空（去空白后），为空即 panic（fail fast）。
func assertApiDescription(fullPath, description string) {
	if strings.TrimSpace(description) == "" {
		panic("views: register api " + fullPath + " with an empty description")
	}
}

// joinApiPath 拼接路由组基础路径与相对路径为完整路径。
func joinApiPath(basePath, relPath string) string {
	if relPath == "" || relPath == "/" {
		return basePath
	}
	if !strings.HasPrefix(relPath, "/") {
		relPath = "/" + relPath
	}
	return basePath + relPath
}

// recordApi 把已应用选项的接口配置按 (method, path) 写入目录；重复登记以最后一次为准。
func recordApi(fullPath string, options *metadataOptions) {
	enabled := !options.ragDisabled
	apiRegistryMu.Lock()
	defer apiRegistryMu.Unlock()
	for method, meta := range buildApiMeta(options) {
		apiRegistry[method+"\x00"+fullPath] = ApiDescriptor{
			Method:      method,
			Path:        fullPath,
			Description: strings.TrimSpace(options.description),
			RagEnabled:  enabled,
			Meta:        meta,
			Capability:  options.capability,
		}
	}
}

// ApiCatalog 返回接口目录的快照，按 (路径, 方法) 确定性排序。
func ApiCatalog() []ApiDescriptor {
	apiRegistryMu.Lock()
	defer apiRegistryMu.Unlock()
	out := make([]ApiDescriptor, 0, len(apiRegistry))
	for _, descriptor := range apiRegistry {
		out = append(out, descriptor)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Method < out[j].Method
	})
	return out
}

// ApiKnowledgeDocs 将目录中纳入 RAG 的接口渲染为知识文档（每条 (method, path) 一篇，
// 标题为稳定去重键）。未纳入 RAG 的接口不出现，从而在同步时被幂等删除。
func ApiKnowledgeDocs() []ApiKnowledgeDoc {
	catalog := ApiCatalog()
	docs := make([]ApiKnowledgeDoc, 0, len(catalog))
	for _, descriptor := range catalog {
		if !descriptor.RagEnabled {
			continue
		}
		docs = append(docs, renderApiKnowledgeDoc(descriptor))
	}
	return docs
}

// RenderApiDoc 把单个接口渲染为文档投影使用的标题与正文。
// 正文与 RAG 知识文档同源，保证人类参考页与模型上下文逐字一致。
func RenderApiDoc(descriptor ApiDescriptor) ApiKnowledgeDoc {
	return renderApiKnowledgeDoc(descriptor)
}

// renderApiKnowledgeDoc 把单个接口渲染为 Markdown 知识文档（纯函数，逐字节幂等）。
// 段落顺序固定：用途 / 请求 / 请求示例 / 请求头入参 / 入参 / 响应头出参 / 出参 / 响应示例。
// 四类结构体节均按结构体粒度拆节：顶层字段表 + 具名嵌套类型的子节（见 writeStructSections），
// 使每个分块小而自包含（表格是分块器的原子单元，整张大表会成为单个超长块而在检索时被截断）。
func renderApiKnowledgeDoc(descriptor ApiDescriptor) ApiKnowledgeDoc {
	var b strings.Builder
	b.WriteString("## 用途\n\n")
	b.WriteString(descriptor.Description + "\n\n")
	b.WriteString("## 请求\n\n")
	b.WriteString("- 方法：`" + descriptor.Method + "`\n")
	b.WriteString("- 路径：`" + descriptor.Path + "`\n")
	if params := pathParamsOf(descriptor.Path); len(params) > 0 {
		b.WriteString("- 路径参数（按路径段顺序，均为必填字符串）：")
		for i, name := range params {
			if i > 0 {
				b.WriteString("、")
			}
			b.WriteString("`" + name + "`")
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	if descriptor.Meta != nil {
		writeExampleSection(&b, "请求示例", descriptor.Meta.BodyInput)
		writeStructSections(&b, "请求头入参", descriptor.Meta.HeaderInput)
		writeStructSections(&b, "入参", descriptor.Meta.BodyInput)
		writeStructSections(&b, "响应头出参", descriptor.Meta.HeaderOutput)
		writeStructSections(&b, "出参", descriptor.Meta.BodyOutput)
		writeExampleSection(&b, "响应示例", descriptor.Meta.BodyOutput)
	}
	return ApiKnowledgeDoc{
		Title:   "原生接口 " + descriptor.Method + " " + descriptor.Path,
		Content: b.String(),
	}
}

// exampleBlockMaxRunes 是单个 JSON 示例代码块的字符数上限。与字段表同理：整段示例是原子代码块，
// 超长时会被检索侧按 snippet 截断而丢掉后半字段。超过上限时按顶层字段边界拆分为多个完整代码块
// （每块自身是合法 JSON 对象），以空行分隔，使每块独立成块且可被完整读取。
const exampleBlockMaxRunes = 480

// writeExampleSection 写出一个 JSON 示例小节；结构体类型未知（reflectType 为 nil，如手工构造的
// StructMeta）或示例为空时不产出空节。示例超长时拆分为连续多个完整代码块。
func writeExampleSection(b *strings.Builder, heading string, meta *StructMeta) {
	if meta == nil || meta.reflectType == nil {
		return
	}
	example := BuildJSONExample(meta.reflectType)
	if example == "" {
		return
	}
	b.WriteString("## " + heading + "\n\n")
	for _, block := range splitJSONExampleBlocks(example) {
		b.WriteString("```json\n" + block + "\n```\n\n")
	}
}

// splitJSONExampleBlocks 把 JSON 示例拆分为若干合法 JSON 对象文本：不超过 exampleBlockMaxRunes 时
// 原样返回单块；否则按顶层字段边界分组，每组用花括号收口为一个独立对象。顶层字段行以两空格缩进加
// 引号起始（嵌套内容缩进更深），故分组边界可稳定识别；单条顶层字段自身超限时独占一块（不再细分）。
// 输出对同一输入逐字节确定。
func splitJSONExampleBlocks(example string) []string {
	if utf8.RuneCountInString(example) <= exampleBlockMaxRunes {
		return []string{example}
	}
	lines := strings.Split(example, "\n")
	if len(lines) < 3 {
		return []string{example}
	}
	var entries [][]string
	for _, line := range lines[1 : len(lines)-1] {
		if len(entries) == 0 || strings.HasPrefix(line, `  "`) {
			entries = append(entries, []string{line})
			continue
		}
		entries[len(entries)-1] = append(entries[len(entries)-1], line)
	}
	// 每块自身的括号开销（"{" 与 "}" 各占一行）
	const wrapRunes = 2
	budget := exampleBlockMaxRunes - wrapRunes
	var (
		blocks []string
		cur    []string
		curLen = wrapRunes
		flush  = func() {
			if len(cur) == 0 {
				return
			}
			blocks = append(blocks, "{\n"+strings.Join(cur, ",\n")+"\n}")
			cur = nil
			curLen = wrapRunes
		}
	)
	for _, entry := range entries {
		// 条目尾部逗号由块内连接符统一补上，故先去掉，保证每块是合法 JSON
		text := strings.TrimSuffix(strings.Join(entry, "\n"), ",")
		size := utf8.RuneCountInString(text) + len(",\n")
		if len(cur) > 0 && curLen+size > budget {
			flush()
		}
		cur = append(cur, text)
		curLen += size
	}
	flush()
	if len(blocks) == 0 {
		return []string{example}
	}
	return blocks
}

// writeStructSections 写出一个结构体小节：顶层字段表 + 各字段引用的具名嵌套类型子节。
// 嵌套类型在同一小节内只定义一次（seen 逐节独立，故入参与出参各自自包含，不依赖检索侧的
// 整篇展开也能读到本节的类型定义）；结构体为空时不产出标题与空表。
func writeStructSections(b *strings.Builder, heading string, meta *StructMeta) {
	if meta == nil || len(meta.Fields) == 0 {
		return
	}
	b.WriteString("## " + heading + "\n\n")
	writeFieldTable(b, meta.Fields)
	seen := make(map[string]bool, len(meta.Fields))
	for _, field := range meta.Fields {
		writeNestedSections(b, fieldPathSegment(field), field, 0, seen)
	}
}

// nestedSectionMaxDepth 是嵌套类型子节的递归展开深度上限（达到上限时停止下钻）。
const nestedSectionMaxDepth = 6

// writeNestedSections 为字段引用的具名嵌套结构体写出「### 字段路径（类型）」子节并递归其字段。
// path 为从顶层字段累积而来的字段路径（如 layerStack[].tcpOptions），写入标题后经检索前置的
// headingPath 携带，使子节分块自包含。类型已在本节定义、达到深度上限或结构体无可展示字段时
// 不再产出；类型名集合用于防环。
func writeNestedSections(b *strings.Builder, path string, field *FieldMeta, depth int, seen map[string]bool) {
	t, ok := NestedStructType(field)
	if !ok || depth >= nestedSectionMaxDepth {
		return
	}
	key := t.PkgPath() + "." + t.Name()
	if seen[key] {
		return
	}
	fields := GetStructFields(t)
	if len(fields) == 0 {
		return
	}
	seen[key] = true
	b.WriteString("### " + markdownHeading(path) + "（" + markdownHeading(field.Type) + "）\n\n")
	writeFieldTable(b, fields)
	for _, child := range fields {
		writeNestedSections(b, path+"."+fieldPathSegment(child), child, depth+1, seen)
	}
}

// fieldPathSegment 把字段渲染为嵌套子节标题中的路径段：切片/数组追加 []、map 追加 {}，
// 以在标题中体现多重性（如 orderBy[]、options{}）。字段无原始 Go 类型（手工构造的 FieldMeta，
// reflectType 为零值）时退化为字段名本身。
func fieldPathSegment(field *FieldMeta) string {
	if field == nil {
		return ""
	}
	t := derefType(field.reflectType)
	if t == nil {
		return field.Name
	}
	switch t.Kind() {
	case reflect.Slice, reflect.Array:
		return field.Name + "[]"
	case reflect.Map:
		return field.Name + "{}"
	default:
		return field.Name
	}
}

// fieldTableMaxRunes 是单张字段表的字符数上限。列表出参（如流量全字段）可达 50+ 行，整张表会成为
// 一个超长原子块：分块器把表格视为不可切分单元，检索侧只能按 snippet 长度截断，后半字段对模型不可见。
// 超过上限时按行拆分为连续的多张完整表（各自重复表头，以空行分隔），使每张表成为独立且自包含的分块，
// 从而在生成侧解决截断问题，检索侧无需任何特判。
const fieldTableMaxRunes = 480

// fieldTableHeader 是字段表的两行表头（列名与分隔行）。
const fieldTableHeader = "| 字段 | 类型 | 必填 | 校验 | 枚举 | 默认值 |\n| --- | --- | --- | --- | --- | --- |\n"

// writeFieldTable 写出「字段 | 类型 | 必填 | 校验 | 枚举 | 默认值」六列表格；超过 fieldTableMaxRunes
// 时按行拆分为多张完整表，每张表都以空行结尾（空行是分块器判定表格块结束的边界）。
func writeFieldTable(b *strings.Builder, fields []*FieldMeta) {
	rows := make([]string, 0, len(fields))
	for _, field := range fields {
		required := "-"
		if field.Required {
			required = "是"
		}
		rows = append(rows, "| "+markdownCell(field.Name)+" | "+markdownCell(field.Type)+" | "+required+
			" | "+markdownCell(field.Binding)+" | "+markdownCell(enumCell(field))+" | "+markdownCell(defaultValueText(field.DefaultValue))+" |\n")
	}
	// 预算扣除表头长度：拆出的每张表都带完整表头，列名与分隔行不计入行预算会溢出上限
	budget := fieldTableMaxRunes - utf8.RuneCountInString(fieldTableHeader)
	var (
		page    []string
		pageLen int
	)
	flush := func() {
		if len(page) == 0 {
			return
		}
		b.WriteString(fieldTableHeader)
		for _, row := range page {
			b.WriteString(row)
		}
		b.WriteString("\n")
		page = nil
		pageLen = 0
	}
	for _, row := range rows {
		size := utf8.RuneCountInString(row)
		// 单行超预算时也单独成表，避免出现空表
		if len(page) > 0 && pageLen+size > budget {
			flush()
		}
		page = append(page, row)
		pageLen += size
	}
	flush()
}

// enumCell 渲染字段的枚举列：枚举类已注册时内联其全部取值（数据库存储值以「、」连接），
// 未注册时退化为仅枚举类名（离线生成进程未触发归属包 init 注册时即此情形），非枚举字段返回空串。
func enumCell(field *FieldMeta) string {
	if field == nil || !field.Enum || field.EnumType == "" {
		return ""
	}
	t, ok := enum.Get(field.EnumType)
	if !ok || len(t.Entries) == 0 {
		return field.EnumType
	}
	values := make([]string, 0, len(t.Entries))
	for _, entry := range t.Entries {
		values = append(values, entry.Value)
	}
	return field.EnumType + "：" + strings.Join(values, "、")
}

// markdownHeading 清理标题文本中的竖线与换行，避免破坏 Markdown 结构。
func markdownHeading(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	return strings.ReplaceAll(value, "\n", " ")
}

// markdownCell 转义 Markdown 表格单元格中的竖线与换行，空值以 "-" 占位。
func markdownCell(value string) string {
	if value == "" {
		return "-"
	}
	value = strings.ReplaceAll(value, "|", "\\|")
	return strings.ReplaceAll(value, "\n", " ")
}

// defaultValueText 把默认值渲染为展示文本，nil 以 "-" 占位。
func defaultValueText(value any) string {
	if value == nil {
		return "-"
	}
	return fmt.Sprint(value)
}

// pathParamsOf 提取路径中的占位符参数名（形如 :id 的段），按出现顺序返回。
func pathParamsOf(path string) []string {
	var names []string
	for _, segment := range strings.Split(path, "/") {
		if strings.HasPrefix(segment, ":") && len(segment) > 1 {
			names = append(names, segment[1:])
		}
	}
	return names
}
