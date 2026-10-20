// Package tools 提供 AI 助手内置工具（builtin tools）的实现注册表：按工具名解析为 eino
// tool.BaseTool 并注入 DB / 日志等运行期依赖；工具元信息与出厂种子在别处声明，本包只承载实现。
//
// 本文件是三个无状态文本类内置工具的薄适配器：regex（正则测试与替换）、json_diff（JSON 差异比较）与
// text_stats（文本统计）。三者都不持有任何依赖，构造器忽略 Deps，全部信息均来自模型入参。纯算法实现
// 已下沉到 internal/pkg/utils（RunRegex / DiffJSON / StatsText），本文件只保留入参结构体、Info（含中文
// Desc/schema）、init 期登记与 InvokableRun 骨架：解析入参 JSON、调用 utils 导出函数、出错时补上工具名
// 前缀并以 JSON 序列化结果。工具元信息（internal/ai 的 ToolSpecs）与出厂行（002_fixtures.sql）由另外
// 两处提供。

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
// regex：正则测试与替换
// ============================================================================

// regexArgs 是 regex 的 JSON 入参形态：operation / pattern / text 必填，replacement（replace 时
// 必填）与 global 可选。
type regexArgs struct {
	// Operation 操作类型（test / replace，必填）
	Operation string `json:"operation"`
	// Pattern 正则表达式（Go RE2 语法，必填）
	Pattern string `json:"pattern"`
	// Text 待处理的文本（必填）
	Text string `json:"text"`
	// Replacement 替换表达式（operation 为 replace 时必填）
	Replacement string `json:"replacement"`
	// Global 是否替换全部匹配（仅 replace 使用，缺省 false 表示仅替换首个）
	Global bool `json:"global"`
}

// regexTool 是内置工具 regex 的无状态实现：不持有依赖，全部参数来自模型入参。
type regexTool struct{}

// Info 返回 regex 的工具元信息与参数 JSON schema，供模型决定何时调用。参数经 eino 的 ParameterInfo
// 形式描述：operation 带枚举取值，pattern / text 必填，replacement 与 global 仅在 replace 时使用。
func (t *regexTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "regex",
		Desc: "对文本执行正则测试或替换。采用 Go 正则（RE2）语法，不支持反向引用与环视；" +
			"operation 为 test 时返回是否匹配与全部命中，为 replace 时返回替换后的文本与替换次数。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"operation": {
				Type:     schema.String,
				Desc:     "操作类型：test 仅测试匹配，replace 执行替换。",
				Enum:     utils.RegexOperations,
				Required: true,
			},
			"pattern": {
				Type:     schema.String,
				Desc:     "正则表达式，采用 Go 正则（RE2）语法，不支持反向引用与环视。",
				Required: true,
			},
			"text": {
				Type:     schema.String,
				Desc:     "待处理的文本。",
				Required: true,
			},
			"replacement": {
				Type: schema.String,
				Desc: "替换表达式，operation 为 replace 时必填，可用 $1 等引用捕获组。",
			},
			"global": {
				Type: schema.Boolean,
				Desc: "是否替换全部匹配，仅 operation 为 replace 时生效；为 false（缺省）时仅替换首个匹配。",
			},
		}),
	}, nil
}

// InvokableRun 执行 regex：解析 JSON 入参，调用 utils.RunRegex 校验并执行，返回形如
// {"operation":"test","matched":true,...} 或 {"operation":"replace","result":"...","count":2} 的 JSON
// 字符串。任何校验失败或正则编译失败均返回英文错误（工具名前缀在此补齐）。
func (t *regexTool) InvokableRun(_ context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var args regexArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", errors.WithStack(err)
	}
	result, err := utils.RunRegex(args.Operation, args.Pattern, args.Text, args.Replacement, args.Global)
	if err != nil {
		return "", errors.Newf("regex: %s", err.Error())
	}
	return textEncodeResult(result)
}

// ============================================================================
// json_diff：JSON 差异比较
// ============================================================================

// jsonDiffArgs 是 json_diff 的 JSON 入参形态：left / right 均为 JSON 文本，必填。
type jsonDiffArgs struct {
	// Left 左侧 JSON 文本（必填）
	Left string `json:"left"`
	// Right 右侧 JSON 文本（必填）
	Right string `json:"right"`
}

// jsonDiffTool 是内置工具 json_diff 的无状态实现：不持有依赖，全部参数来自模型入参。
type jsonDiffTool struct{}

// Info 返回 json_diff 的工具元信息与参数 JSON schema，供模型决定何时调用。left / right 均为 JSON
// 文本而非对象，以便直接比较两侧原始取值的差异。
func (t *jsonDiffTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "json_diff",
		Desc: "比较两段 JSON 文本的结构化差异：对象按键比较（缺失 / 新增 / 值变），数组按下标比较" +
			"（长度差视为新增 / 删除），标量直接比较。返回差异路径、类型与两侧取值，可用于核对配置、" +
			"接口响应或数据快照的前后变化。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"left": {
				Type:     schema.String,
				Desc:     "左侧 JSON 文本，必须为合法 JSON。",
				Required: true,
			},
			"right": {
				Type:     schema.String,
				Desc:     "右侧 JSON 文本，必须为合法 JSON。",
				Required: true,
			},
		}),
	}, nil
}

// InvokableRun 执行 json_diff：解析入参，调用 utils.DiffJSON 递归比较两侧，返回形如
// {"equal":false,"count":2,"differences":[...]} 的 JSON 字符串。任一侧非法 JSON 均返回英文错误
// （工具名前缀在此补齐）。
func (t *jsonDiffTool) InvokableRun(_ context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var args jsonDiffArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", errors.WithStack(err)
	}
	result, err := utils.DiffJSON(args.Left, args.Right)
	if err != nil {
		return "", errors.Newf("json_diff: %s", err.Error())
	}
	return textEncodeResult(result)
}

// ============================================================================
// text_stats：文本统计
// ============================================================================

// textStatsArgs 是 text_stats 的 JSON 入参形态：text 必填（允许为空串，空文本的统计结果全为 0）。
type textStatsArgs struct {
	// Text 待统计的文本（必填）
	Text string `json:"text"`
}

// textStatsTool 是内置工具 text_stats 的无状态实现：不持有依赖，全部参数来自模型入参。
type textStatsTool struct{}

// Info 返回 text_stats 的工具元信息与参数 JSON schema，供模型决定何时调用。唯一键为 text（必填），
// 用于估算长文本的体量、字数与结构。
func (t *textStatsTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "text_stats",
		Desc: "统计一段文本的字符数、去空白字符数、UTF-8 字节数、词数、行数与段落数。" +
			"字符数按 Unicode 码点（rune）计，行数按 \\n 计，段落按连续空行分隔的非空块计。" +
			"用于估算文本体量、检查截断长度是否符合预期。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"text": {
				Type:     schema.String,
				Desc:     "待统计的文本，可为空串（空文本各项统计均为 0）。",
				Required: true,
			},
		}),
	}, nil
}

// InvokableRun 执行 text_stats：解析 JSON 入参，调用 utils.StatsText，返回形如
// {"characters":11,"charactersNoWhitespace":9,"bytes":15,"words":3,"lines":2,"paragraphs":1} 的
// JSON 字符串（工具名前缀在此补齐）。
func (t *textStatsTool) InvokableRun(_ context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var args textStatsArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", errors.WithStack(err)
	}
	result, err := utils.StatsText(args.Text)
	if err != nil {
		return "", errors.Newf("text_stats: %s", err.Error())
	}
	return textEncodeResult(result)
}

// textEncodeResult 把 utils 返回的结果结构体序列化为紧凑 JSON 字符串；失败时返回带堆栈错误。
func textEncodeResult(v any) (string, error) {
	out, err := json.Marshal(v)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return string(out), nil
}

// init 向内置工具实现注册表登记 regex / json_diff / text_stats；三者均无状态，不消费 Deps。
func init() {
	Register("regex", func(_ context.Context, _ Deps) (tool.BaseTool, error) {
		return &regexTool{}, nil
	})
	Register("json_diff", func(_ context.Context, _ Deps) (tool.BaseTool, error) {
		return &jsonDiffTool{}, nil
	})
	Register("text_stats", func(_ context.Context, _ Deps) (tool.BaseTool, error) {
		return &textStatsTool{}, nil
	})
}
