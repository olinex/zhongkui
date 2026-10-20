// Package tools 提供 AI 助手内置工具（builtin tools）的实现注册表：按工具名解析为 eino
// tool.BaseTool 并注入 DB / 日志等运行期依赖；工具元信息与出厂种子在别处声明，本包只承载实现。
//
// 本文件是三个无状态数据类内置工具的薄适配器：json（校验 / 格式化 / 压缩 / 路径查询）、csv_parse
// （CSV 文本解析）与 url_parse（URL 拆解）。三者都不持有任何依赖，构造器忽略 Deps，全部信息均来自
// 模型入参。纯算法实现已下沉到 internal/pkg/utils（ProcessJSON / ParseCSV / ParseURL），本文件只保留
// 入参结构体、Info（含中文 Desc/schema）、init 期登记与 InvokableRun 骨架：解析入参 JSON、调用 utils
// 导出函数、出错时补上工具名前缀并以 JSON 序列化结果。工具元信息（internal/ai 的 ToolSpecs）与出厂行
// （002_fixtures.sql）由另外两处提供。

package tools

import (
	// 标准库
	"context"
	"encoding/json"

	// 第三方
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

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
	Operation string `json:"operation"`
	// JSON 待处理的 JSON 文本（必填）
	JSON string `json:"json"`
	// Path 查询路径（可选，仅 query 需要）
	Path string `json:"path"`
	// Indent 格式化缩进空格数（可选，仅 format 需要，缺省 2，范围 1..8）
	Indent int `json:"indent"`
}

// jsonTool 是内置工具 json 的无状态实现：不持有依赖，全部输入取自调用入参。
type jsonTool struct{}

// Info 返回 json 的工具元信息与参数 JSON schema，供模型决定何时调用。参数经 eino 的
// ParameterInfo 形式描述（operation 与 json 必填，operation 带枚举取值；path 与 indent 可选）。
func (t *jsonTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "json",
		Desc: "校验、格式化、压缩 JSON 文本，或按路径查询 JSON 中的一个值。" +
			"用于判断文本是否为合法 JSON、整理缩进或压缩体积、以及从嵌套结构中取值。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"operation": {
				Type:     schema.String,
				Desc:     "操作类型，必填，可选 validate 校验合法性、format 格式化缩进、minify 压缩为紧凑文本或 query 按路径查询。",
				Required: true,
				Enum:     utils.JSONOperations,
			},
			"json": {
				Type:     schema.String,
				Desc:     "待处理的 JSON 文本，必填。",
				Required: true,
			},
			"path": {
				Type: schema.String,
				Desc: "查询路径，仅 query 需要；对象键用点号、数组下标用方括号，如 a.b[0].c，根也可以是 [0] 或 a。",
			},
			"indent": {
				Type: schema.Integer,
				Desc: "格式化缩进空格数，仅 format 需要，可选，取值范围 1 到 8，缺省为 2。",
			},
		}),
	}, nil
}

// InvokableRun 执行 json：解析 JSON 入参，调用 utils.ProcessJSON 按 operation 分派，返回紧凑 JSON
// 字符串。validate 对非法文本正常返回 {"valid":false,"error":"..."}（不视为工具失败）；format /
// minify / query 遇非法 JSON 或非法路径时返回英文错误（工具名前缀在此补齐）。
func (t *jsonTool) InvokableRun(_ context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var args jsonArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", errors.WithStack(err)
	}
	result, err := utils.ProcessJSON(args.Operation, args.JSON, args.Path, args.Indent)
	if err != nil {
		return "", errors.Newf("json: %s", err.Error())
	}
	return dataEncodeResult(result)
}

// ============================================================================
// csv_parse：CSV 文本解析
// ============================================================================

// csvParseArgs 是 csv_parse 的 JSON 入参形态：text 必填，delimiter 与 has_header 可选（缺省分别为
// "," 与 true）。HasHeader 用指针以便区分「未提供」（缺省 true）与「显式 false」。
type csvParseArgs struct {
	// Text 待解析的 CSV 文本（必填）
	Text string `json:"text"`
	// Delimiter 字段分隔符（可选，单个字符，缺省 ","）
	Delimiter string `json:"delimiter"`
	// HasHeader 首行是否为表头（可选，缺省 true）
	HasHeader *bool `json:"has_header"`
}

// csvParseTool 是内置工具 csv_parse 的无状态实现：不持有依赖，全部输入取自调用入参。
type csvParseTool struct{}

// Info 返回 csv_parse 的工具元信息与参数 JSON schema，供模型决定何时调用。参数经 eino 的
// ParameterInfo 形式描述（text 必填，delimiter 与 has_header 可选）。
func (t *csvParseTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "csv_parse",
		Desc: "把一段 CSV 文本解析为表头与数据行。用于读取用户粘贴的表格数据、把逗号分隔内容整理成" +
			"结构化数组，支持自定义分隔符与是否含表头。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"text": {
				Type:     schema.String,
				Desc:     "待解析的 CSV 文本，必填。",
				Required: true,
			},
			"delimiter": {
				Type: schema.String,
				Desc: "字段分隔符，可选，必须是单个字符，缺省为英文逗号 \",\"。",
			},
			"has_header": {
				Type: schema.Boolean,
				Desc: "首行是否为表头，可选，缺省为 true；为 false 时首行也作为数据行返回。",
			},
		}),
	}, nil
}

// InvokableRun 执行 csv_parse：解析 JSON 入参，用 utils.DefaultCSVHasHeader 解析可选缺省后调用
// utils.ParseCSV 解析文本，返回形如
// {"delimiter":",","hasHeader":true,"header":[...],"rows":[...],"rowCount":2,"columnCount":2} 的
// JSON 字符串。分隔符非单字符或 CSV 语法非法（如引号不闭合）时返回英文错误（工具名前缀在此补齐）。
func (t *csvParseTool) InvokableRun(_ context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var args csvParseArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", errors.WithStack(err)
	}
	hasHeader := utils.DefaultCSVHasHeader
	if args.HasHeader != nil {
		hasHeader = *args.HasHeader
	}
	result, err := utils.ParseCSV(args.Text, args.Delimiter, hasHeader)
	if err != nil {
		return "", errors.Newf("csv_parse: %s", err.Error())
	}
	return dataEncodeResult(result)
}

// ============================================================================
// url_parse：URL 拆解
// ============================================================================

// urlParseArgs 是 url_parse 的 JSON 入参形态：url 必填。
type urlParseArgs struct {
	// URL 待解析的 URL 文本（必填）
	URL string `json:"url"`
}

// urlParseTool 是内置工具 url_parse 的无状态实现：不持有依赖，全部输入取自调用入参。
type urlParseTool struct{}

// Info 返回 url_parse 的工具元信息与参数 JSON schema，供模型决定何时调用。参数经 eino 的
// ParameterInfo 形式描述（url 必填）。
func (t *urlParseTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "url_parse",
		Desc: "把一段 URL 拆解为结构化字段：协议、用户信息、主机、端口、路径、片段与查询参数。" +
			"用于分析链接构成、提取域名与查询参数，或判断 URL 是否为绝对地址。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"url": {
				Type:     schema.String,
				Desc:     "待解析的 URL 文本，必填。",
				Required: true,
			},
		}),
	}, nil
}

// InvokableRun 执行 url_parse：解析 JSON 入参，调用 utils.ParseURL 拆解 URL，返回形如
// {"scheme":"https","user":"u","host":"example.com","port":"8443","path":"/a/b","fragment":"frag",
// "query":{"x":["1","2"]},"queryString":"x=1&x=2","isAbsolute":true} 的 JSON 字符串。URL 为空或非法
// 时返回英文错误（工具名前缀在此补齐）。
func (t *urlParseTool) InvokableRun(_ context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var args urlParseArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", errors.WithStack(err)
	}
	result, err := utils.ParseURL(args.URL)
	if err != nil {
		return "", errors.Newf("url_parse: %s", err.Error())
	}
	return dataEncodeResult(result)
}

// dataEncodeResult 把 utils 返回的结果结构体序列化为紧凑 JSON 字符串；失败时返回带堆栈错误。
func dataEncodeResult(v any) (string, error) {
	out, err := json.Marshal(v)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return string(out), nil
}

// init 向内置工具实现注册表登记 json、csv_parse 与 url_parse；三者均无状态，不消费 Deps。
func init() {
	Register("json", func(_ context.Context, _ Deps) (tool.BaseTool, error) {
		return &jsonTool{}, nil
	})
	Register("csv_parse", func(_ context.Context, _ Deps) (tool.BaseTool, error) {
		return &csvParseTool{}, nil
	})
	Register("url_parse", func(_ context.Context, _ Deps) (tool.BaseTool, error) {
		return &urlParseTool{}, nil
	})
}
