// Package tools 提供 AI 助手内置工具（builtin tools）的实现注册表：按工具名解析为 eino
// tool.BaseTool 并注入 DB / 日志等运行期依赖；工具元信息与出厂种子在别处声明，本包只承载实现。
//
// 本文件是三个无状态文本类内置工具的薄适配器：regex（正则测试与替换）、json_diff（JSON 差异比较）与
// text_stats（文本统计）。三者都不持有任何依赖，构造器忽略 Deps，全部信息均来自模型入参。纯算法实现
// 已下沉到 internal/pkg/utils（RunRegex / DiffJSON / StatsText），工具契约由 eino 的
// tool/utils.InferTool 按入参结构体生成参数 schema，并自动完成入参解析与结果序列化；本文件只保留
// 入参结构体、工具描述与调用 utils 的薄函数。工具元信息（internal/ai 的 ToolSpecs）与出厂行
// （002_fixtures.sql）由另外两处提供。

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
// regex：正则测试与替换
// ============================================================================

// regexArgs 是 regex 的 JSON 入参形态：operation / pattern / text 必填，replacement 与 global 可选。
type regexArgs struct {
	// Operation 操作类型（test / replace，必填）
	Operation string `json:"operation" jsonschema:"enum=test,enum=replace" jsonschema_description:"操作类型：test 仅测试匹配，replace 执行替换。"`
	// Pattern 正则表达式（Go RE2 语法，必填）
	Pattern string `json:"pattern" jsonschema_description:"正则表达式，采用 Go 正则（RE2）语法，不支持反向引用与环视。"`
	// Text 待处理的文本（必填）
	Text string `json:"text" jsonschema_description:"待处理的文本。"`
	// Replacement 替换表达式（operation 为 replace 时必填）
	Replacement string `json:"replacement,omitempty" jsonschema_description:"替换表达式，operation 为 replace 时必填，可用 $1 等引用捕获组。"`
	// Global 是否替换全部匹配（仅 replace 使用，缺省 false 表示仅替换首个）
	Global bool `json:"global,omitempty" jsonschema_description:"是否替换全部匹配，仅 operation 为 replace 时生效；为 false（缺省）时仅替换首个匹配。"`
}

// regexTool 是内置工具 regex 的 eino 实现：不持有依赖，参数 schema 由入参结构体推导。
var regexTool = mustInferTool(
	"regex",
	"对文本执行正则测试或替换。采用 Go 正则（RE2）语法，不支持反向引用与环视；"+
		"operation 为 test 时返回是否匹配与全部命中，为 replace 时返回替换后的文本与替换次数。",
	func(_ context.Context, in regexArgs) (any, error) {
		result, err := utils.RunRegex(in.Operation, in.Pattern, in.Text, in.Replacement, in.Global)
		if err != nil {
			return nil, errors.Newf("regex: %s", err.Error())
		}
		return result, nil
	},
)

// ============================================================================
// json_diff：JSON 差异比较
// ============================================================================

// jsonDiffArgs 是 json_diff 的 JSON 入参形态：left / right 均为 JSON 文本，必填。
type jsonDiffArgs struct {
	// Left 左侧 JSON 文本（必填）
	Left string `json:"left" jsonschema_description:"左侧 JSON 文本，必须为合法 JSON。"`
	// Right 右侧 JSON 文本（必填）
	Right string `json:"right" jsonschema_description:"右侧 JSON 文本，必须为合法 JSON。"`
}

// jsonDiffTool 是内置工具 json_diff 的 eino 实现：不持有依赖，参数 schema 由入参结构体推导。
var jsonDiffTool = mustInferTool(
	"json_diff",
	"比较两段 JSON 文本的结构化差异：对象按键比较（缺失 / 新增 / 值变），数组按下标比较"+
		"（长度差视为新增 / 删除），标量直接比较。返回差异路径、类型与两侧取值，可用于核对配置、"+
		"接口响应或数据快照的前后变化。",
	func(_ context.Context, in jsonDiffArgs) (utils.JsonDiffResult, error) {
		result, err := utils.DiffJSON(in.Left, in.Right)
		if err != nil {
			return utils.JsonDiffResult{}, errors.Newf("json_diff: %s", err.Error())
		}
		return result, nil
	},
)

// ============================================================================
// text_stats：文本统计
// ============================================================================

// textStatsArgs 是 text_stats 的 JSON 入参形态：text 必填（允许为空串，空文本的统计结果全为 0）。
type textStatsArgs struct {
	// Text 待统计的文本（必填）
	Text string `json:"text" jsonschema_description:"待统计的文本，可为空串（空文本各项统计均为 0）。"`
}

// textStatsTool 是内置工具 text_stats 的 eino 实现：不持有依赖，参数 schema 由入参结构体推导。
var textStatsTool = mustInferTool(
	"text_stats",
	"统计一段文本的字符数、去空白字符数、UTF-8 字节数、词数、行数与段落数。"+
		"字符数按 Unicode 码点（rune）计，行数按 \\n 计，段落按连续空行分隔的非空块计。"+
		"用于估算文本体量、检查截断长度是否符合预期。",
	func(_ context.Context, in textStatsArgs) (utils.TextStatsResult, error) {
		result, err := utils.StatsText(in.Text)
		if err != nil {
			return utils.TextStatsResult{}, errors.Newf("text_stats: %s", err.Error())
		}
		return result, nil
	},
)

// init 向内置工具实现注册表登记 regex / json_diff / text_stats；三者均无状态，不消费 Deps。
func init() {
	Register("regex", func(_ context.Context, _ Deps) (tool.BaseTool, error) {
		return regexTool, nil
	})
	Register("json_diff", func(_ context.Context, _ Deps) (tool.BaseTool, error) {
		return jsonDiffTool, nil
	})
	Register("text_stats", func(_ context.Context, _ Deps) (tool.BaseTool, error) {
		return textStatsTool, nil
	})
}
