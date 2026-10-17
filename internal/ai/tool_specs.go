package ai

// ToolSpec 是单个内置工具的元信息，由本包注册表唯一声明：本包只持有工具名与分组，既不
// 含任何实现（实现在 internal/ai/tools，按名解析为 eino tool.BaseTool），也不含任何出厂值
// （工具出厂行由 migrate fixture 注入 llm_tools，source='builtin'），更不含任何面向用户的
// 文案（展示名与说明一律经后端 i18n 提供：tool.<Name>.label / tool.<Name>.comment，Name
// 全局唯一因而直接复用为文案键的一部分），故本包内不出现任何可翻译文案。
type ToolSpec struct {
	// Name 工具名（llm_tools.name，全局唯一），复用为 i18n 文案键的一部分
	Name string
	// Group 前端展示分组（如 builtin 等），与参数注册表的 Group 同义
	Group string
}

// ToolSpecs 是全部内置工具的元信息注册表，声明顺序即前端展示顺序。新增内置工具必须同时补齐
// 三处，缺一处都会被守卫拦下：在此登记（缺则实现注册表无对应键，启动期按名解析即暴露缺陷）、
// 在 internal/ai/tools 注册实现（缺则 Build 返回「编程缺陷」错误）、在 migrate fixture 中
// 种子化出厂行（缺则由守卫测试 internal/ai/tools_fixture_test.go 暴露）。三条链路（元信息
// 注册表 / 实现注册表 / fixture 种子化 + 守卫测试）必须齐备，使后续新增内置工具无需再改配置面
// 代码。
var ToolSpecs = []ToolSpec{
	// ---- builtin：平台自有内置工具 ----
	{
		Name:  "memory_write",
		Group: "builtin",
	},
	{
		Name:  "knowledge_search",
		Group: "builtin",
	},
	// ---- client：客户端工具 ----
	// client 分组的工具由前端在用户自身凭据下执行，后端只并入其 schema（见 internal/ai 的
	// client_tools.go 注册表），不构建本地执行器、不拨号任何远端；模型调用后由后端把待执行调用
	// 经 SSE 下发给前端并等待用户审批与执行结果。该分组当前登记 ask_user（问卷类只读交互能力）。
	{
		Name:  askUserToolName,
		Group: "client",
	},
}

// LookupTool 按名查询内置工具元信息，未声明时返回 ok=false（同名函数 Lookup 已被参数注册表
// 占用，故此处以 LookupTool 区分）。
func LookupTool(name string) (ToolSpec, bool) {
	for _, spec := range ToolSpecs {
		if spec.Name == name {
			return spec, true
		}
	}
	return ToolSpec{}, false
}

// init 校验工具注册表自身一致性（fail fast）：名称非空且唯一、分组非空。任一项不满足即
// panic，避免非法注册表在生产中以残缺形态运行；出厂行的合法性由守卫测试
// internal/ai/tools_fixture_test.go 与启动期装配共同保证。
func init() {
	seen := make(map[string]struct{}, len(ToolSpecs))
	for _, spec := range ToolSpecs {
		if spec.Name == "" {
			panic("ai: tool spec with an empty name")
		}
		if spec.Group == "" {
			panic("ai: tool " + spec.Name + " has an empty group")
		}
		if _, dup := seen[spec.Name]; dup {
			panic("ai: duplicate tool name: " + spec.Name)
		}
		seen[spec.Name] = struct{}{}
	}
}
