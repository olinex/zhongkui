package views

import (
	// 标准库

	"encoding/json"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	// 第三方
	"github.com/gin-gonic/gin"
	. "github.com/smartystreets/goconvey/convey"
)

// catalogRenderTestOption 是嵌套类型展开与 JSON 示例生成的测试用嵌套结构体。
type catalogRenderTestOption struct {
	Name  string `json:"name" binding:"required"`
	Value int    `json:"value" default:"7"`
}

// catalogRenderTestItem 是嵌套类型展开测试用顶层结构体（含切片嵌套与时间字段）。
type catalogRenderTestItem struct {
	Keyword   string                    `json:"keyword" binding:"required"`
	Options   []catalogRenderTestOption `json:"options"`
	CreatedAt time.Time                 `json:"createdAt"`
}

// catalogRenderTestRecursive 是自引用类型，用于验证示例生成与嵌套展开的防环能力。
type catalogRenderTestRecursive struct {
	Name string                      `json:"name"`
	Next *catalogRenderTestRecursive `json:"next"`
}

// catalogRenderTestCredential 用于验证凭证字段使用占位符而非普通字符串示例。
type catalogRenderTestCredential struct {
	Password string `json:"password"`
	Total    int64  `json:"total"`
	Context  string `json:"context"`
}

// catalogRenderTestOut 是示例生成测试用出参结构体。
type catalogRenderTestOut struct {
	Total int64                   `json:"total"`
	Items []catalogRenderTestItem `json:"items"`
}

// TestRenderApiKnowledgeDocIdempotent 校验 renderApiKnowledgeDoc 为纯函数：
// 同一 descriptor 连续渲染两次，Title 与 Content 逐字节相等。
func TestRenderApiKnowledgeDocIdempotent(t *testing.T) {
	Convey("同一 ApiDescriptor 连续渲染两次应逐字节相等", t, func() {
		descriptor := ApiDescriptor{
			Method:      http.MethodPost,
			Path:        "/api/v1/catalog/:id/render",
			Description: "渲染幂等性测试接口",
			RagEnabled:  true,
			Meta: &MethodMeta{
				BodyInput: &StructMeta{
					Name: "CatalogRenderTestIn",
					Fields: []*FieldMeta{
						{Name: "name", Type: "string", Binding: "required", Kind: KindString, Required: true},
						{Name: "format", Type: "string", Kind: KindString, Enum: true, EnumType: "documentFormat"},
					},
				},
			},
		}

		first := renderApiKnowledgeDoc(descriptor)
		second := renderApiKnowledgeDoc(descriptor)

		So(second.Title, ShouldEqual, first.Title)
		So(second.Content, ShouldEqual, first.Content)
		So(second, ShouldResemble, first)
	})
}

// TestRenderApiKnowledgeDocPathParams 校验含路径占位符的 descriptor 渲染出的正文
// 包含用途、方法、完整路径以及「路径参数」小节与参数名。
func TestRenderApiKnowledgeDocPathParams(t *testing.T) {
	Convey("含 :id 路径参数的接口应渲染出用途/方法/路径/路径参数", t, func() {
		descriptor := ApiDescriptor{
			Method:      http.MethodPost,
			Path:        "/api/v1/catalog/:id/detail",
			Description: "按 id 查询目录详情",
			RagEnabled:  true,
		}

		doc := renderApiKnowledgeDoc(descriptor)

		So(doc.Content, ShouldContainSubstring, descriptor.Description)
		So(doc.Content, ShouldContainSubstring, "## 用途")
		So(doc.Content, ShouldContainSubstring, "## 请求")
		So(doc.Content, ShouldContainSubstring, "`"+http.MethodPost+"`")
		So(doc.Content, ShouldContainSubstring, descriptor.Path)
		So(doc.Content, ShouldContainSubstring, "路径参数")
		So(doc.Content, ShouldContainSubstring, "`id`")
		So(doc.Title, ShouldEqual, "原生接口 "+http.MethodPost+" "+descriptor.Path)
	})
}

// TestRenderApiKnowledgeDocFieldTable 校验入参小节字段表的表头、必填列与枚举列渲染。
func TestRenderApiKnowledgeDocFieldTable(t *testing.T) {
	Convey("字段表应覆盖必填与枚举", t, func() {
		descriptor := ApiDescriptor{
			Method:      http.MethodPost,
			Path:        "/api/v1/catalog/field-table",
			Description: "字段表渲染测试接口",
			RagEnabled:  true,
			Meta: &MethodMeta{
				BodyInput: &StructMeta{
					Name: "CatalogRenderTestIn",
					Fields: []*FieldMeta{
						{
							Name:     "name",
							Type:     "string",
							Binding:  "required",
							Kind:     KindString,
							Required: true,
						},
						{
							Name:     "format",
							Type:     "string",
							Kind:     KindString,
							Enum:     true,
							EnumType: "documentFormat",
						},
					},
				},
			},
		}

		doc := renderApiKnowledgeDoc(descriptor)

		So(doc.Content, ShouldContainSubstring, "## 入参")
		So(doc.Content, ShouldContainSubstring, "| 字段 | 类型 | 必填 | 校验 | 枚举 | 默认值 |")
		// 必填字段渲染为「是」
		So(doc.Content, ShouldContainSubstring, "| name | string | 是 | required | - | - |")
		// 非必填字段的必填/校验/默认值列渲染为「-」
		So(doc.Content, ShouldContainSubstring, "| format | string | - | - | documentFormat")
		// 枚举类已注册（views 依赖 pkg/docconv）：枚举列内联该类全部取值
		So(doc.Content, ShouldContainSubstring, "documentFormat：markdown")
	})
}

// TestRenderApiKnowledgeDocNoEmptySection 校验无入参/无出参时不产出空表标题。
func TestRenderApiKnowledgeDocNoEmptySection(t *testing.T) {
	emptyHeadings := []string{"## 入参", "## 出参", "## 请求头入参", "## 响应头出参"}

	Convey("Meta 为 nil 时不产出任何结构体小节标题", t, func() {
		descriptor := ApiDescriptor{
			Method:      http.MethodGet,
			Path:        "/api/v1/catalog/nil-meta",
			Description: "无元信息接口",
			RagEnabled:  true,
			Meta:        nil,
		}

		doc := renderApiKnowledgeDoc(descriptor)

		for _, heading := range emptyHeadings {
			So(doc.Content, ShouldNotContainSubstring, heading)
		}
		So(doc.Content, ShouldContainSubstring, "## 用途")
		So(doc.Content, ShouldContainSubstring, "## 请求")
		So(doc.Content, ShouldContainSubstring, descriptor.Description)
	})

	Convey("Meta 非 nil 但四类结构体全为 nil 时不产出空表标题", t, func() {
		descriptor := ApiDescriptor{
			Method:      http.MethodGet,
			Path:        "/api/v1/catalog/empty-meta",
			Description: "空元信息接口",
			RagEnabled:  true,
			Meta:        &MethodMeta{},
		}

		doc := renderApiKnowledgeDoc(descriptor)

		for _, heading := range emptyHeadings {
			So(doc.Content, ShouldNotContainSubstring, heading)
		}
		So(doc.Content, ShouldContainSubstring, "## 用途")
		So(doc.Content, ShouldContainSubstring, "## 请求")
		So(doc.Content, ShouldContainSubstring, descriptor.Description)
	})
}

// TestApiCatalogRagFilter 校验 ApiKnowledgeDocs 按 RagEnabled 过滤：
// 显式 WithRagEnabled(false) 的接口不产出文档，缺省纳入的接口产出文档。
func TestApiCatalogRagFilter(t *testing.T) {
	Convey("RAG 开关应过滤知识文档", t, func() {
		gin.SetMode(gin.TestMode)
		group := gin.New().Group("/api/v1")

		RegisterApi(group, "/__rag_test_disabled__", "测试用途-退出 RAG",
			WithMethod(http.MethodPost), WithRagEnabled(false))
		RegisterApi(group, "/__rag_test_enabled__", "测试用途-纳入 RAG",
			WithMethod(http.MethodPost))

		disabledTitle := "原生接口 " + http.MethodPost + " /api/v1/__rag_test_disabled__"
		enabledTitle := "原生接口 " + http.MethodPost + " /api/v1/__rag_test_enabled__"

		docs := ApiKnowledgeDocs()
		titles := make(map[string]bool, len(docs))
		for _, doc := range docs {
			titles[doc.Title] = true
		}

		So(titles[disabledTitle], ShouldBeFalse)
		So(titles[enabledTitle], ShouldBeTrue)
	})
}

// TestRenderApiKnowledgeDocNestedSections 校验具名嵌套类型被递归展开为子节：切片字段路径带 []、
// 子节标题含字段路径与类型全名、子表含子结构体字段；标量结构体（time.Time）不产出子节；
// 同一类型在同一小节内只定义一次。
func TestRenderApiKnowledgeDocNestedSections(t *testing.T) {
	Convey("入参引用的具名嵌套结构体应展开为自包含子节", t, func() {
		descriptor := ApiDescriptor{
			Method:      http.MethodPost,
			Path:        "/api/v1/catalog/nested",
			Description: "嵌套类型展开测试接口",
			RagEnabled:  true,
			Meta:        &MethodMeta{BodyInput: getStructMeta(reflect.TypeFor[catalogRenderTestItem]())},
		}

		doc := renderApiKnowledgeDoc(descriptor)

		So(doc.Content, ShouldContainSubstring, "## 入参")
		// 切片字段路径带 []，标题含类型的 Go 全名（含包前缀）
		So(doc.Content, ShouldContainSubstring, "### options[]（[]views.catalogRenderTestOption）")
		// 子结构体字段行：必填与默认值列均生效
		So(doc.Content, ShouldContainSubstring, "| name | string | 是 | required | - | - |")
		So(doc.Content, ShouldContainSubstring, "| value | int | - | - | - | 7 |")
		// time.Time 按标量呈现，不展开为子节
		So(doc.Content, ShouldNotContainSubstring, "### createdAt")
		// 同一类型在同一小节内只定义一次
		So(strings.Count(doc.Content, "### options[]"), ShouldEqual, 1)
	})
}

// TestBuildJSONExample 校验 JSON 示例生成器：输出为合法 JSON、同类型连续调用逐字节相等、
// 枚举字段取枚举类首个取值、time.Time 取固定字面量、default 标签生效、自引用类型不无限展开。
func TestBuildJSONExample(t *testing.T) {
	Convey("示例应为合法 JSON 且逐字节确定", t, func() {
		itemType := reflect.TypeFor[catalogRenderTestItem]()

		first := BuildJSONExample(itemType)
		second := BuildJSONExample(itemType)

		So(json.Valid([]byte(first)), ShouldBeTrue)
		So(second, ShouldEqual, first)
		So(first, ShouldContainSubstring, `"keyword": "string"`)
		So(first, ShouldContainSubstring, `"createdAt": "1970-01-01T00:00:00Z"`)
		So(first, ShouldContainSubstring, `"value": 7`)
	})

	Convey("自引用类型示例应在防环后收敛", t, func() {
		example := BuildJSONExample(reflect.TypeFor[catalogRenderTestRecursive]())

		So(json.Valid([]byte(example)), ShouldBeTrue)
		So(len(example), ShouldBeLessThan, 200)
		So(example, ShouldContainSubstring, `"next": {}`)
	})

	Convey("枚举字段示例应取枚举类首个取值", t, func() {
		// 登记 keyword → documentFormat 绑定后生成示例；用例结束恢复注册表
		registry := enumBindings
		enumBindings = map[string]map[string]string{}
		defer func() { enumBindings = registry }()
		AddEnumBinding("catalogRenderTestItem", "keyword", "documentFormat")

		example := BuildJSONExample(reflect.TypeFor[catalogRenderTestItem]())

		So(json.Valid([]byte(example)), ShouldBeTrue)
		So(example, ShouldContainSubstring, `"keyword": "markdown"`)
	})

	Convey("凭证字段示例应使用占位符而非普通字符串", t, func() {
		example := BuildJSONExample(reflect.TypeFor[catalogRenderTestCredential]())

		So(json.Valid([]byte(example)), ShouldBeTrue)
		So(example, ShouldContainSubstring, `"password": "<password>"`)
		// 非凭证字段与数值字段保持原占位方式
		So(example, ShouldContainSubstring, `"context": "string"`)
		So(example, ShouldContainSubstring, `"total": 0`)
	})

	Convey("凭证字段判定不应误伤计数类字段", t, func() {
		So(isCredentialFieldName("password"), ShouldBeTrue)
		So(isCredentialFieldName("newPassword"), ShouldBeTrue)
		So(isCredentialFieldName("accessKeySecret"), ShouldBeTrue)
		So(isCredentialFieldName("apiKey"), ShouldBeTrue)
		So(isCredentialFieldName("maxTokens"), ShouldBeFalse)
		So(isCredentialFieldName("contextTokens"), ShouldBeFalse)
	})

	Convey("非结构体类型返回空示例", t, func() {
		So(BuildJSONExample(reflect.TypeFor[string]()), ShouldEqual, "")
		So(BuildJSONExample(nil), ShouldEqual, "")
	})
}

// TestWriteFieldTableSplitsOversizedTable 校验超长字段表按行拆分为多张完整表：每张表都带表头、
// 每张表不超过 fieldTableMaxRunes、全部字段行恰好出现一次（不丢行、不重复）。
func TestWriteFieldTableSplitsOversizedTable(t *testing.T) {
	Convey("超长字段表应拆分为多张完整表且不丢行", t, func() {
		fields := make([]*FieldMeta, 0, 40)
		for i := range 40 {
			fields = append(fields, &FieldMeta{
				Name: "fieldWithLongName" + strconv.Itoa(i),
				Type: "serializer.SomeQuiteLongTypeName",
				Kind: KindString,
			})
		}

		var b strings.Builder
		writeFieldTable(&b, fields)

		tables := strings.Split(strings.TrimRight(b.String(), "\n"), "\n\n")
		So(len(tables), ShouldBeGreaterThan, 1)

		header := "| 字段 | 类型 | 必填 | 校验 | 枚举 | 默认值 |"
		seen := make([]string, 0, len(fields))
		for _, table := range tables {
			So(utf8.RuneCountInString(table), ShouldBeLessThanOrEqualTo, fieldTableMaxRunes)
			So(strings.HasPrefix(table, header), ShouldBeTrue)
			So(strings.Count(table, header), ShouldEqual, 1)
			for _, line := range strings.Split(table, "\n")[2:] {
				seen = append(seen, strings.Split(line, " | ")[0][2:])
			}
		}
		So(seen, ShouldResemble, fieldNamesOf(fields))
	})
}

// fieldNamesOf 返回字段名序列，供拆分完整性断言比对。
func fieldNamesOf(fields []*FieldMeta) []string {
	names := make([]string, 0, len(fields))
	for _, field := range fields {
		names = append(names, field.Name)
	}
	return names
}

// TestSplitJSONExampleBlocks 校验超长 JSON 示例按顶层字段边界拆分：每块自身为合法 JSON、
// 每块不超过 exampleBlockMaxRunes、键集合与原文一致、输出确定；未超限时原样返回单块。
func TestSplitJSONExampleBlocks(t *testing.T) {
	Convey("超长 JSON 示例应拆分为多个合法 JSON 块", t, func() {
		lines := make([]string, 0, 40)
		for i := range 40 {
			lines = append(lines, `  "fieldWithLongName`+strconv.Itoa(i)+`": "string"`)
		}
		example := "{\n" + strings.Join(lines, ",\n") + "\n}"
		So(utf8.RuneCountInString(example), ShouldBeGreaterThan, exampleBlockMaxRunes)

		blocks := splitJSONExampleBlocks(example)
		So(len(blocks), ShouldBeGreaterThan, 1)

		keyCount := 0
		for _, block := range blocks {
			So(json.Valid([]byte(block)), ShouldBeTrue)
			So(utf8.RuneCountInString(block), ShouldBeLessThanOrEqualTo, exampleBlockMaxRunes)
			var obj map[string]any
			So(json.Unmarshal([]byte(block), &obj), ShouldBeNil)
			keyCount += len(obj)
		}
		So(keyCount, ShouldEqual, 40)
		So(blocks, ShouldResemble, splitJSONExampleBlocks(example))

		// 未超限的示例原样返回单块
		short := "{\n  \"a\": 1\n}"
		So(splitJSONExampleBlocks(short), ShouldResemble, []string{short})
	})
}

// TestRenderApiKnowledgeDocRegisteredExample 校验经 RegisterApi 真实注册（走 getStructMeta 赋值
// reflectType 的路径）的接口文档含请求与响应示例段。
func TestRenderApiKnowledgeDocRegisteredExample(t *testing.T) {
	Convey("真实注册的接口应渲染出请求示例与响应示例", t, func() {
		gin.SetMode(gin.TestMode)
		group := gin.New().Group("/api/v1")

		RegisterApi(group, "/__example_test__", "示例段落渲染测试接口",
			WithMethod(http.MethodPost,
				WithBodyInput[catalogRenderTestItem](),
				WithBodyOutput[catalogRenderTestOut]()))

		title := "原生接口 " + http.MethodPost + " /api/v1/__example_test__"
		var content string
		for _, doc := range ApiKnowledgeDocs() {
			if doc.Title == title {
				content = doc.Content
			}
		}

		So(content, ShouldNotEqual, "")
		So(content, ShouldContainSubstring, "## 请求示例")
		So(content, ShouldContainSubstring, "## 响应示例")
		So(content, ShouldContainSubstring, "```json")
		// 入参（含枚举绑定 keyword）与出参（含嵌套 items[]）的示例各自自包含
		So(content, ShouldContainSubstring, "### items[]（[]views.catalogRenderTestItem）")
	})
}
