// Package tools 是内置工具的实现注册表：把「工具名」解析为 eino 的 tool.BaseTool，并按需
// 向实现注入 DB / 日志等运行期依赖。元信息（键与分组）声明在 internal/ai 的 ToolSpecs，
// 出厂行由 migrate fixture 种子化；本包只承载实现，三者共同构成内置工具的完整链路。默认
// 清单为空，新增内置工具时在本包内 init 期调用 Register 登记实现即可。
//
// ## 三层链路与单一事实来源
//
// 一个内置工具由三处分别负责、共同构成完整链路：internal/ai 的 ToolSpecs 声明工具键与
// 分组（元信息），`002_fixtures.sql` 种子化出厂行，本包在 init 期以 Register 登记实现。三者以
// 工具名对齐：Build 按名解析实现，若元信息注册表与实现注册表漂移（键无实现）即视为编程缺陷并
// 返回英文错误，交由调用方决定告警或中止，本包只负责快速暴露缺陷。Register 对空名、nil
// 构造器与同名重复登记一律 panic（fail fast），避免残缺注册表在生产中以看似正常的形态运行。
//
// ## 依赖注入与解析
//
// Deps 承载构造工具时可注入的运行期依赖（core 库句柄与日志记录器），由运行时装配持有并复用；
// Builder 是「按名取实现 + 注入依赖」的构造器，返回的 eino tool.BaseTool 的 Info 提供含 JSON
// schema 的 `*schema.ToolInfo`。Build 串联解析与注入，Names 返回已注册实现名（字典序），供守卫
// 测试与诊断使用。
//
// ## 工具契约
//
// 工具遵循 eino 的统一契约，因而内置工具与远端 MCP 工具对消费侧（Agent 的生成节点）不可区分：
// Info 声明名称、描述与 JSON schema（Agent 用它向模型描述可用工具）；InvokableRun 接收模型给出
// 的入参 JSON 并返回字符串结果与错误。入参解析失败、依赖缺失等情形以英文错误表达，错误信息面向
// 排障而非终端用户；工具结果会由 Agent 按 agent.maxToolResultBytes 截断后再回灌对话，故实现
// 无需自行限制体量。工具应尽量无状态——需要运行期信息的，经 ctx 注入而非在构造期固化。
//
// ## 叶层约束与 memory_write
//
// 本包是「工具」这一能力的叶层：internal/ai 单向依赖本包，故凡需被 internal/ai 适配实现、又需
// 被工具实现引用的接口与入参，必须声明在本包内以避免导入环。`memory_write` 即典型：它让 Agent 在
// 工具循环中自主把一条值得在后续轮次复用、且仅在本会话内可见的长期记忆交给写入器落库。工具自身
// 无状态——写入器接口 MemoryWriter 与入参 MemoryWriteInput 声明于本包，由 internal/ai 的适配器
// 实现向量化、精确哈希去重与语义合并；本次运行的归属信息（命名空间 / Agent 类型 / 来源会话）与
// 写入器一同经 WithMemoryRuntime 注入 ctx，由 Agent 在派发工具前写入，取不到时工具返回英文错误。
// MemoryWriteInput.SessionID 必须非 0：记忆按会话隔离，无会话归属的记忆不被接受。
//
// ## MCP 工具
//
// 远端工具由子包 internal/ai/tools/mcp 适配为同一契约：ConnConfig 描述一个远程 MCP 服务器的
// 传输方式（`streamable_http` / sse）、端点与凭据，Dial 完成 initialize 握手并返回客户端句柄
// （调用方负责 Close，SSE 为有状态长连接），LoadTools 按工具名取回并以 tool.BaseTool 暴露。
// 它是 MCP SDK 在本仓库内的唯一落点；握手时向远端声明的客户端身份取自源码级常量
// （base.GlobalPrefix 与 version.Short()），避免本地字面量随改名或发版而漂移。凭据仅经内存传递，
// 不落日志。运行时装配把内置工具与各 MCP 服务器的工具汇总为 ToolSet 供 Agent 消费。
//
// ## 设计取舍
//
// 采用「注册表 + 按名解析」而非在 Agent 内硬编码工具清单，有三点考虑。其一，工具是可选能力：
// 是否需要、需要哪些由数据库中的工具配置（models/ai.ToolSource 决定内置 / MCP 来源）驱动，注册表让
// 「有哪些实现」与「本次启用哪些」分离。其二，扩展成本最低：新增一个内置工具只需在子包内实现
// 并 Register，Agent 的生成节点无需改动。其三，fail fast：空名、nil 构造器与重复登记在 init 期
// panic，把配置缺陷挡在启动阶段，而不是在用户触发时以「工具不存在」的模糊错误暴露。相应地，
// 「元信息有键、实现无键」的漂移由 Build 返回英文错误而非 panic——它可能是运行时配置的产物，
// 允许调用方降级处理。
//
// ## 扩展方式
//
// 新增内置工具的标准动作是：在 internal/ai 的 ToolSpecs 中补充键与分组，在 fixture 中
// 种子化出厂行（如需默认启用），并在本包内新增一个实现文件于 init 期调用 Register 登记构造器。
// 三处同源即可，无需改动 Agent、视图层或迁移脚本之外的部分。
package tools

import (
	// 标准库
	"context"
	"sort"

	// 第三方
	"github.com/cloudwego/eino/components/tool"

	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

// Deps 是构建内置工具时可注入的运行期依赖：数据库句柄与日志记录器。构造由调用方（运行时
// 装配）持有并复用，实现不得修改其内容。
type Deps struct {
	// DB core 库句柄
	DB *database.DB
	// Logger 日志记录器
	Logger *logging.Logger
}

// Builder 是内置工具的构造器：按名取其实现，注入依赖后返回可被节点 4 消费的 eino
// tool.BaseTool（其 Info 提供含 JSON schema 的 *schema.ToolInfo）。
type Builder func(ctx context.Context, deps Deps) (tool.BaseTool, error)

// builders 是工具名到构造器的实现注册表，仅供本包注册与解析使用。
var builders = map[string]Builder{}

// Register 登记一个内置工具的实现，应由实现所在的 init 期调用。名称空、构造器为 nil 或
// 同名重复登记均视为编程缺陷，直接 panic（fail fast），避免非法注册表在生产中以残缺形态
// 运行。
func Register(name string, builder Builder) {
	if name == "" {
		panic("ai/tools: register a tool with an empty name")
	}
	if builder == nil {
		panic("ai/tools: register tool " + name + " with a nil builder")
	}
	if _, dup := builders[name]; dup {
		panic("ai/tools: duplicate tool registration: " + name)
	}
	builders[name] = builder
}

// Build 按名解析内置工具的实现并注入依赖，返回可被节点 4 消费的 eino tool.BaseTool。键未
// 注册实现即视为编程缺陷（元信息注册表与实现注册表漂移），返回英文错误；是否告警还是中止
// 由调用方决定，本包只负责快速暴露缺陷。
func Build(ctx context.Context, name string, deps Deps) (tool.BaseTool, error) {
	builder, ok := builders[name]
	if !ok {
		return nil, errors.Newf("ai/tools: builtin tool %q has no registered implementation, the metadata registry and the implementation registry have drifted apart", name)
	}
	built, err := builder(ctx, deps)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return built, nil
}

// Names 返回已注册实现的内置工具名（字典序），供守卫测试与诊断使用。
func Names() []string {
	names := make([]string, 0, len(builders))
	for name := range builders {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
