// Package tools 提供 AI 助手内置工具（builtin tools）的实现注册表：按工具名解析为 eino
// tool.BaseTool 并注入 DB / 日志等运行期依赖；工具元信息与出厂种子在别处声明，本包只承载实现。
//
// 本文件是三个无状态数据类内置工具的薄适配器：json（校验 / 格式化 / 压缩 / 路径查询）、csv_parse
// （CSV 文本解析）与 url_parse（URL 拆解）。三者都不持有任何依赖，构造器忽略 Deps，全部信息均来自
// 模型入参。纯算法实现已下沉到 internal/pkg/utils（ProcessJSON / ParseCSV / ParseURL），工具契约
// 由 eino 的 tool/utils.InferTool 按入参结构体生成参数 schema，并自动完成入参解析与结果序列化；
// 本文件只保留入参结构体、工具描述与调用 utils 的薄函数。工具元信息（internal/ai 的 ToolSpecs）与
// 出厂行（002_fixtures.sql）由另外两处提供。

package tools

import (
	// 标准库
	"context"

	// 第三方
	"github.com/cloudwego/eino/components/tool"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/utils"
)

// ============================================================================
// json：JSON 处理
// ============================================================================

// jsonArgs 是 json 的 JSON 入参形态：operation 与 json 必填，path 仅 query 需要，indent 仅 format
// 需要（可选，缺省 2）。
type jsonArgs struct {
	// Operation 操作类型（必填，取值见 utils.JSONOperations）
	Operation string `json:"operation" jsonschema:"enum=validate,enum=format,enum=minify,enum=query" jsonschema_description:"操作类型，必填，可选 validate 校验合法性、format 格式化缩进、minify 压缩为紧凑文本或 query 按路径查询。"`
	// JSON 待处理的 JSON 文本（必填）
	JSON string `json:"json" jsonschema_description:"待处理的 JSON 文本，必填。"`
	// Path 查询路径（可选，仅 query 需要）
	Path string `json:"path,omitempty" jsonschema_description:"查询路径，仅 query 需要；对象键用点号、数组下标用方括号，如 a.b[0].c，根也可以是 [0] 或 a。"`
	// Indent 格式化缩进空格数（可选，仅 format 需要，缺省 2，范围 1..8）
	Indent int `json:"indent,omitempty" jsonschema_description:"格式化缩进空格数，仅 format 需要，可选，取值范围 1 到 8，缺省为 2。"`
}

// jsonTool 是内置工具 json 的 eino 实现：不持有依赖，参数 schema 由入参结构体推导。
var jsonTool = mustInferTool(
	"json",
	"校验、格式化、压缩 JSON 文本，或按路径查询 JSON 中的一个值。"+
		"用于判断文本是否为合法 JSON、整理缩进或压缩体积、以及从嵌套结构中取值。",
	func(_ context.Context, in jsonArgs) (any, error) {
		result, err := utils.ProcessJSON(in.Operation, in.JSON, in.Path, in.Indent)
		if err != nil {
			return nil, errors.Newf("json: %s", err.Error())
		}
		return result, nil
	},
)

// ============================================================================
// csv_parse：CSV 文本解析
// ============================================================================

// csvParseArgs 是 csv_parse 的 JSON 入参形态：text 必填，delimiter 与 has_header 可选（缺省分别为
// "," 与 true）。HasHeader 用指针以便区分「未提供」（缺省 true）与「显式 false」。
type csvParseArgs struct {
	// Text 待解析的 CSV 文本（必填）
	Text string `json:"text" jsonschema_description:"待解析的 CSV 文本，必填。"`
	// Delimiter 字段分隔符（可选，单个字符，缺省 ","）
	Delimiter string `json:"delimiter,omitempty" jsonschema_description:"字段分隔符，可选，必须是单个字符，缺省为英文逗号 \",\"。"`
	// HasHeader 首行是否为表头（可选，缺省 true）
	HasHeader *bool `json:"has_header,omitempty" jsonschema_description:"首行是否为表头，可选，缺省为 true；为 false 时首行也作为数据行返回。"`
}

// csvParseTool 是内置工具 csv_parse 的 eino 实现：不持有依赖，参数 schema 由入参结构体推导。
var csvParseTool = mustInferTool(
	"csv_parse",
	"把一段 CSV 文本解析为表头与数据行。用于读取用户粘贴的表格数据、把逗号分隔内容整理成"+
		"结构化数组，支持自定义分隔符与是否含表头。",
	func(_ context.Context, in csvParseArgs) (utils.CSVParseResult, error) {
		hasHeader := utils.DefaultCSVHasHeader
		if in.HasHeader != nil {
			hasHeader = *in.HasHeader
		}
		result, err := utils.ParseCSV(in.Text, in.Delimiter, hasHeader)
		if err != nil {
			return utils.CSVParseResult{}, errors.Newf("csv_parse: %s", err.Error())
		}
		return result, nil
	},
)

// ============================================================================
// url_parse：URL 拆解
// ============================================================================

// urlParseArgs 是 url_parse 的 JSON 入参形态：url 必填。
type urlParseArgs struct {
	// URL 待解析的 URL 文本（必填）
	URL string `json:"url" jsonschema_description:"待解析的 URL 文本，必填。"`
}

// urlParseTool 是内置工具 url_parse 的 eino 实现：不持有依赖，参数 schema 由入参结构体推导。
var urlParseTool = mustInferTool(
	"url_parse",
	"把一段 URL 拆解为结构化字段：协议、用户信息、主机、端口、路径、片段与查询参数。"+
		"用于分析链接构成、提取域名与查询参数，或判断 URL 是否为绝对地址。",
	func(_ context.Context, in urlParseArgs) (utils.URLParseResult, error) {
		result, err := utils.ParseURL(in.URL)
		if err != nil {
			return utils.URLParseResult{}, errors.Newf("url_parse: %s", err.Error())
		}
		return result, nil
	},
)

// init 向内置工具实现注册表登记 json、csv_parse 与 url_parse；三者均无状态，不消费 Deps。
func init() {
	Register("json", func(_ context.Context, _ Deps) (tool.BaseTool, error) {
		return jsonTool, nil
	})
	Register("csv_parse", func(_ context.Context, _ Deps) (tool.BaseTool, error) {
		return csvParseTool, nil
	})
	Register("url_parse", func(_ context.Context, _ Deps) (tool.BaseTool, error) {
		return urlParseTool, nil
	})
}
