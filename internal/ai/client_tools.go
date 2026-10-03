// 本文件承载 client 来源工具的 schema 注册表：client 工具由前端在用户自身凭据下执行，后端不
// 构建本地执行器、不拨号任何远端，只把这里的 *schema.ToolInfo 并入工具装配的 schema 清单（见
// store_tools.go 的 appendClientTool），供模型选择调用。与「元信息注册表（internal/ai 的
// ToolSpecs）+ 出厂行（002_fixtures.sql）」共同构成 client 工具的完整链路：元信息与出厂行决定
// 工具是否存在，本注册表决定工具的入参契约。它是 builtin 工具实现注册表（internal/ai/tools 的
// Register）在 client 侧的等价物——故守卫测试 internal/ai/tools_fixture_test.go 要求 source=client
// 的工具必须在此有对应条目。当前注册表为空，是保留的扩展点。

package ai

import (
	// 第三方
	"github.com/cloudwego/eino/schema"
)

// clientToolInfos 是 client 工具名到 eino schema 的注册表，仅供本包注册与解析使用。
var clientToolInfos = map[string]*schema.ToolInfo{}

// registerClientToolInfo 登记一个 client 工具的 schema，应由工具声明所在的 init 期调用。工具名
// 为空、schema 为 nil、schema 名与工具名不一致或同名重复登记均视为编程缺陷，直接 panic（fail
// fast），避免非法注册表在生产中以残缺形态运行（风格与 internal/ai/tools 的 Register 一致）。
func registerClientToolInfo(name string, info *schema.ToolInfo) {
	if name == "" {
		panic("ai: register a client tool with an empty name")
	}
	if info == nil {
		panic("ai: register client tool " + name + " with a nil schema")
	}
	if info.Name != name {
		panic("ai: register client tool " + name + " with mismatched schema name: " + info.Name)
	}
	if _, dup := clientToolInfos[name]; dup {
		panic("ai: duplicate client tool registration: " + name)
	}
	clientToolInfos[name] = info
}

// clientToolInfo 按名取回 client 工具的 schema，未登记时返回 ok=false（装配层据此记英文告警并跳过）。
func clientToolInfo(name string) (*schema.ToolInfo, bool) {
	info, ok := clientToolInfos[name]
	return info, ok
}

// isClientTool 判断工具名是否为已登记的 client 工具：是则本次调用须挂起等待前端审批与执行
// （实现不落在本地工具索引中），否则按本地工具派发。
func isClientTool(name string) bool {
	_, ok := clientToolInfos[name]
	return ok
}
