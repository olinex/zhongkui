package llm

// ToolSpec 是单个内置工具的元信息，由本包注册表唯一声明：本包只持有工具名与分组，既不
// 含任何实现（实现在 internal/ai/tools，按名解析为 eino tool.BaseTool），也不含任何出厂值
// （工具出厂行由 migrate fixture 注入 llm_tools，source='builtin'），更不含任何面向用户的
// 文案（展示名与说明一律经后端 i18n 提供：tool.<Name>.label / tool.<Name>.comment，Name
// 全局唯一因而直接复用为文案键的一部分），故本包内不出现任何可翻译文案。
// ToolSpec is the metadata of a single builtin tool, declared solely by the registry of this
// package: the package holds only the tool name and the group — never any implementation (which
// lives in internal/ai/tools and resolves a name to an eino tool.BaseTool), never any factory
// value (builtin tool rows are seeded by the migrate fixtures into llm_tools with
// source='builtin'), and never any user-facing copy (the display name and description always come
// from the backend i18n bundle: tool.<Name>.label / tool.<Name>.comment — Name is globally unique
// and therefore reused as part of the copy key), so no translatable copy ever appears inside this
// package.
type ToolSpec struct {
	// Name 工具名（llm_tools.name，全局唯一），复用为 i18n 文案键的一部分
	// Name the tool name (llm_tools.name, globally unique), reused as part of the i18n copy key
	Name string
	// Group 前端展示分组（如 builtin 等），与参数注册表的 Group 同义
	// Group the frontend display group (e.g. builtin), with the same meaning as the Group of the
	// parameter registry
	Group string
}

// ToolSpecs 是全部内置工具的元信息注册表，声明顺序即前端展示顺序。新增内置工具必须同时补齐
// 三处，缺一处都会被守卫拦下：在此登记（缺则实现注册表无对应键，启动期按名解析即暴露缺陷）、
// 在 internal/ai/tools 注册实现（缺则 Build 返回「编程缺陷」错误）、在 migrate fixture 中
// 种子化出厂行（缺则由守卫测试 internal/ai/tools_fixture_test.go 暴露）。三条链路（元信息
// 注册表 / 实现注册表 / fixture 种子化 + 守卫测试）必须齐备，使后续新增内置工具无需再改配置面
// 代码。
// ToolSpecs is the registry of every builtin tool; the declaration order is the frontend display
// order. A new builtin tool must be completed in three places at once, each guarded on its own:
// registered here (a miss leaves the implementation registry without the key and surfaces as soon
// as the runtime resolves the name at startup), given an implementation in internal/ai/tools (a
// miss makes Build return a "programming defect" error), and seeded as a factory row in the
// migrate fixtures (a miss is surfaced by the guard test internal/ai/tools_fixture_test.go). All
// three links (metadata registry / implementation registry / fixture seeding plus the guard test)
// must be in place so that adding a builtin tool later needs no further edit of the config surface.
var ToolSpecs = []ToolSpec{
	// ---- builtin：平台自有内置工具 ----
	// ---- builtin: the platform's own builtin tools ----
	{
		Name:  "memory_write",
		Group: "builtin",
	},
}

// LookupTool 按名查询内置工具元信息，未声明时返回 ok=false（同名函数 Lookup 已被参数注册表
// 占用，故此处以 LookupTool 区分）。
// LookupTool returns the metadata of the builtin tool; ok is false when it is not declared (the
// homonymous Lookup is already taken by the parameter registry, hence LookupTool here).
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
// init validates the internal consistency of the tool registry (fail fast): names are non-empty
// and unique, and groups are non-empty. Any violation panics so an invalid registry can never run
// in a crippled form in production; the validity of factory rows is guaranteed by the guard test
// internal/ai/tools_fixture_test.go together with the startup assembly.
func init() {
	seen := make(map[string]struct{}, len(ToolSpecs))
	for _, spec := range ToolSpecs {
		if spec.Name == "" {
			panic("llm: tool spec with an empty name")
		}
		if spec.Group == "" {
			panic("llm: tool " + spec.Name + " has an empty group")
		}
		if _, dup := seen[spec.Name]; dup {
			panic("llm: duplicate tool name: " + spec.Name)
		}
		seen[spec.Name] = struct{}{}
	}
}
