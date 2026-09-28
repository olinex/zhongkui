// Package tools 是内置工具的实现注册表：把「工具名」解析为 eino 的 tool.BaseTool，并按需
// 向实现注入 DB / 日志等运行期依赖。元信息（键与分组）声明在 internal/pkg/llm 的 ToolSpecs，
// 出厂行由 migrate fixture 种子化；本包只承载实现，三者共同构成内置工具的完整链路。默认
// 清单为空，新增内置工具时在本包内 init 期调用 Register 登记实现即可。
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
