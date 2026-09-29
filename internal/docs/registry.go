package docs

// Root 是文档根目录，相对于项目根。
const Root = "docs"

// Entry 表示一条 Diátaxis 分类的文档条目。
type Entry struct {
	// Category 是条目的 Diátaxis 分类。
	Category Category
	// Title 是条目标题。
	Title string
	// Summary 是条目摘要，用于生成索引。
	Summary string
	// Roles 是建议阅读角色，可为多个；为空表示未标注（守卫测试将拦截）。
	Roles []Role
	// Scene 是适用场景说明，一句话描述读者在什么情境下读这篇文档。
	Scene string
	// Package 是内容来源的 Go 导入路径；为空表示手写文档，不参与生成。
	Package string
	// Output 是相对于 docs/ 的输出路径，例如 explanation/capture-architecture.md。
	Output string
	// NarrativeOnly 为 true 时按叙述档渲染（仅包注释）；为 false 时按参考档渲染。
	NarrativeOnly bool
	// Overview 为 true 表示条目为手写概述文档，不参与生成。
	Overview bool
}

// Registry 返回全部文档条目，是文档体系的唯一注册点。
// 每个包至多对应一个叙述档条目，避免相同知识内容被重复索引。
func Registry() []Entry {
	return []Entry{
		// 教程（学习导向）
		{
			Category:      CategoryTutorial,
			Title:         "快速开始：从零运行 zhongkui",
			Summary:       "从安装、配置到启动服务，完成一次最小可运行的部署。",
			Roles:         []Role{RoleOps},
			Scene:         "首次部署 zhongkui、验证采集到展示全链路是否打通时阅读。",
			Package:       "zhongkui.com/zhongkui/internal/cmd",
			Output:        "tutorials/quick-start.md",
			NarrativeOnly: true,
		},
		{
			Category:      CategoryTutorial,
			Title:         "教程：看到你的第一条风险告警",
			Summary:       "编写一条 rego 规则、触发标签窗口统计，收到第一封风险告警。",
			Roles:         []Role{RoleSecOps},
			Scene:         "初次上手标签与告警、确认自编 rego 规则生效并收到第一封风险告警时阅读。",
			Package:       "zhongkui.com/zhongkui/internal/tagengine",
			Output:        "tutorials/first-alert.md",
			NarrativeOnly: true,
		},

		// 操作指南（任务导向）
		{
			Category:      CategoryHowTo,
			Title:         "如何接入新的数据源插件",
			Summary:       "实现数据源接口并注册一个新的数据源插件。",
			Roles:         []Role{RoleOps, RoleDev},
			Scene:         "需要把一个新的采集数据源接入平台时阅读。",
			Package:       "zhongkui.com/zhongkui/internal/datasource",
			Output:        "how-to/add-datasource-plugin.md",
			NarrativeOnly: true,
		},
		{
			Category:      CategoryHowTo,
			Title:         "如何新增通知渠道与模板",
			Summary:       "扩展通知渠道类型并维护对应的多语言模板。",
			Roles:         []Role{RoleOps, RoleDev},
			Scene:         "需要为告警扩展新的触达渠道及其多语言模板时阅读。",
			Package:       "zhongkui.com/zhongkui/internal/notify",
			Output:        "how-to/add-notify-channel.md",
			NarrativeOnly: true,
		},
		{
			Category:      CategoryHowTo,
			Title:         "如何编写一次数据库迁移",
			Summary:       "新增一次向前/向后兼容的数据库迁移脚本。",
			Roles:         []Role{RoleOps, RoleDev},
			Scene:         "需要在 schema 演进中编写一次数据库迁移时阅读。",
			Package:       "zhongkui.com/zhongkui/internal/pkg/migrate",
			Output:        "how-to/write-migration.md",
			NarrativeOnly: true,
		},
		{
			Category:      CategoryHowTo,
			Title:         "如何新增枚举类并生成绑定",
			Summary:       "在枚举注册表中新增枚举类并生成前后端绑定。",
			Roles:         []Role{RoleDev},
			Scene:         "需要在枚举注册表中扩展取值并生成前后端绑定时阅读。",
			Package:       "zhongkui.com/zhongkui/internal/pkg/enum",
			Output:        "how-to/add-enum-class.md",
			NarrativeOnly: true,
		},

		// 解释（理解导向）
		{
			Category:      CategoryExplanation,
			Title:         "采集架构：wire/XDP、快照与上报",
			Summary:       "说明采集侧的 wire/XDP 抓包、快照生成与上报链路。",
			Roles:         []Role{RoleOps, RoleDev},
			Scene:         "需要理解采集链路构成、或在采集异常时排查故障时阅读。",
			Package:       "zhongkui.com/zhongkui/internal/capture",
			Output:        "explanation/capture-architecture.md",
			NarrativeOnly: true,
		},
		{
			Category:      CategoryExplanation,
			Title:         "网络指纹与 TCP 检测原理",
			Summary:       "解释网络指纹的计算方式与 TCP 流检测原理。",
			Roles:         []Role{RoleSecOps, RoleDev},
			Scene:         "需要理解检测依据、并据此校准规则阈值时阅读。",
			Package:       "zhongkui.com/zhongkui/internal/network/detectors",
			Output:        "explanation/network-detection.md",
			NarrativeOnly: true,
		},
		{
			Category:      CategoryExplanation,
			Title:         "LLM Agent + RAG 设计",
			Summary:       "说明 LLM Agent 编排与 RAG 检索的整体设计。",
			Roles:         []Role{RoleDev},
			Scene:         "需要理解智能问答与检索的整体设计时阅读。",
			Package:       "zhongkui.com/zhongkui/internal/ai",
			Output:        "explanation/agent-rag-design.md",
			NarrativeOnly: true,
		},
		{
			Category:      CategoryExplanation,
			Title:         "Agent 工具契约",
			Summary:       "约定 Agent 可调用工具的入参、出参与错误契约。",
			Roles:         []Role{RoleDev},
			Scene:         "需要扩展或调用 Agent 工具时阅读。",
			Package:       "zhongkui.com/zhongkui/internal/ai/tools",
			Output:        "explanation/agent-tools.md",
			NarrativeOnly: true,
		},
		{
			Category:      CategoryExplanation,
			Title:         "数据源插件体系与 sqlite vtab",
			Summary:       "说明数据源插件体系与 sqlite 虚拟表机制的配合方式。",
			Roles:         []Role{RoleDev},
			Scene:         "需要理解数据源插件体系内部机制时阅读。",
			Package:       "zhongkui.com/zhongkui/internal/pkg/sqlitevtab/vtab",
			Output:        "explanation/datasource-plugin.md",
			NarrativeOnly: true,
		},
		{
			Category:      CategoryExplanation,
			Title:         "异步任务与队列",
			Summary:       "说明基于队列的异步任务调度与执行模型。",
			Roles:         []Role{RoleOps, RoleDev},
			Scene:         "遇到异步任务不执行或队列堆积、需要排查时阅读。",
			Package:       "zhongkui.com/zhongkui/internal/worker",
			Output:        "explanation/async-worker.md",
			NarrativeOnly: true,
		},
		{
			Category:      CategoryExplanation,
			Title:         "流水线与调度",
			Summary:       "说明流水线的阶段划分与调度协作方式。",
			Roles:         []Role{RoleDev},
			Scene:         "需要理解流水线的阶段调度与协作方式时阅读。",
			Package:       "zhongkui.com/zhongkui/internal/pkg/pipeline",
			Output:        "explanation/pipeline.md",
			NarrativeOnly: true,
		},
		{
			Category:      CategoryExplanation,
			Title:         "管理端分层与视图 In/Out 解耦",
			Summary:       "说明管理端分层结构以及视图层 In/Out 解耦约束。",
			Roles:         []Role{RoleDev},
			Scene:         "新增后端接口、需要遵循管理端分层约束时阅读。",
			Package:       "zhongkui.com/zhongkui/internal/manager",
			Output:        "explanation/manager-layering.md",
			NarrativeOnly: true,
		},
		{
			Category:      CategoryExplanation,
			Title:         "依赖注入与生命周期",
			Summary:       "说明 samber/do 依赖注入三层模式与资源生命周期管理。",
			Roles:         []Role{RoleDev},
			Scene:         "需要理解资源装配与生命周期管理时阅读。",
			Package:       "zhongkui.com/zhongkui/internal/provider",
			Output:        "explanation/dependency-injection.md",
			NarrativeOnly: true,
		},

		// 手写条目（不参与生成）
		{
			Category:      CategoryExplanation,
			Title:         "前端设计体系分层",
			Summary:       "说明前端样式体系的分层与设计令牌组织方式。",
			Roles:         []Role{RoleDev},
			Scene:         "进行前端样式与组件开发时阅读。",
			Output:        "explanation/frontend-design-system.md",
			NarrativeOnly: true,
			Overview:      true,
		},
		{
			Category:      CategoryExplanation,
			Title:         "前端功能开关与不可用态",
			Summary:       "说明前端功能开关的接入方式与不可用态呈现约定。",
			Roles:         []Role{RoleAdmin, RoleDev},
			Scene:         "接入前端功能开关、并理解能力不可用时的呈现约定时阅读。",
			Output:        "explanation/frontend-feature-flags.md",
			NarrativeOnly: true,
			Overview:      true,
		},
		{
			Category:      CategoryHowTo,
			Title:         "如何新增一个管理端页面",
			Summary:       "说明新增管理端页面时的路由、视图与 API 接入步骤。",
			Roles:         []Role{RoleDev},
			Scene:         "需要新增一个管理端功能页面时阅读。",
			Output:        "how-to/frontend-add-page.md",
			NarrativeOnly: true,
			Overview:      true,
		},
		{
			Category:      CategoryExplanation,
			Title:         "XDP/BPF 设计说明",
			Summary:       "说明内核侧 XDP/BPF 程序的设计与数据交互方式。",
			Roles:         []Role{RoleDev},
			Scene:         "需要理解内核侧 XDP/BPF 程序的设计与交互时阅读。",
			Output:        "kernel/xdp_bpf_design.md",
			NarrativeOnly: true,
			Overview:      true,
		},
	}
}

// Generated 返回所有需从代码注释生成的条目（Package 非空）。
func Generated() []Entry {
	entries := Registry()
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if e.Package != "" {
			out = append(out, e)
		}
	}
	return out
}

// Handwritten 返回所有手写条目（Package 为空）。
func Handwritten() []Entry {
	entries := Registry()
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if e.Package == "" {
			out = append(out, e)
		}
	}
	return out
}
