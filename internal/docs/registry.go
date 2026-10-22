package docs

// Root 是文档根目录，相对于项目根。
const Root = "docs"

// GeneratedRoot 是生成文档根目录，相对 docs/：仅生成器写入，禁止人工修改。
const GeneratedRoot = "generated"

// CuratedRoot 是策展文档根目录，相对 docs/：仅人工（AI 起草 + 审校）写入，生成器禁止写入。
const CuratedRoot = "curated"

// Section 是文档的导航分区：描述读者旅程与领域，与文件系统目录、Diátaxis 分类相互独立。
type Section string

const (
	// SectionGettingStarted 是「开始使用」分区。
	SectionGettingStarted Section = "getting-started"
	// SectionConcepts 是「核心概念」分区。
	SectionConcepts Section = "concepts"
	// SectionGuides 是「操作指南」分区。
	SectionGuides Section = "guides"
	// SectionReference 是「参考」分区。
	SectionReference Section = "reference"
	// SectionAI 是「AI 能力」分区。
	SectionAI Section = "ai"
	// SectionContribute 是「贡献与治理」分区。
	SectionContribute Section = "contribute"
)

// Sections 返回全部导航分区，顺序即站点与目录的展示顺序。
func Sections() []Section {
	return []Section{
		SectionGettingStarted,
		SectionConcepts,
		SectionGuides,
		SectionReference,
		SectionAI,
		SectionContribute,
	}
}

// Valid 报告分区取值是否合法。
func (s Section) Valid() bool {
	switch s {
	case SectionGettingStarted, SectionConcepts, SectionGuides, SectionReference, SectionAI, SectionContribute:
		return true
	default:
		return false
	}
}

// Label 返回分区中文展示名。
func (s Section) Label() string {
	switch s {
	case SectionGettingStarted:
		return "开始使用"
	case SectionConcepts:
		return "核心概念"
	case SectionGuides:
		return "操作指南"
	case SectionReference:
		return "参考"
	case SectionAI:
		return "AI 能力"
	case SectionContribute:
		return "贡献与治理"
	default:
		return string(s)
	}
}

// Source 是文档的来源，决定它落在哪个根目录、由谁写入。
type Source string

const (
	// SourceComment 表示由代码声明（包注释、接口元信息、模型、枚举、配置、CLI）派生，可随时重算。
	SourceComment Source = "comment"
	// SourceCurated 表示由人策展（AI 起草 + 人工审校），包含判断与取舍，不可被生成器覆盖。
	SourceCurated Source = "curated"
)

// Valid 报告来源取值是否合法。
func (s Source) Valid() bool {
	return s == SourceComment || s == SourceCurated
}

// Root 返回来源对应的根目录（相对 docs/）。
func (s Source) Root() string {
	switch s {
	case SourceComment:
		return GeneratedRoot
	case SourceCurated:
		return CuratedRoot
	default:
		return ""
	}
}

// AiIngest 是文档进入 RAG 时的分区取值；空串表示不进入 RAG。
type AiIngest string

const (
	// IngestGenerated 是包注释派生文档的分区。
	IngestGenerated AiIngest = "generated"
	// IngestCurated 是策展文档分区。
	IngestCurated AiIngest = "curated"
	// IngestNativeAPI 是原生接口知识分区。
	IngestNativeAPI AiIngest = "native_api"
	// IngestSchema 是数据表知识分区。
	IngestSchema AiIngest = "schema"
	// IngestCapability 是能力剧本分区。
	IngestCapability AiIngest = "capability"
)

// Valid 报告 AI 摄入分区是否合法（空串合法，表示不摄入）。
func (i AiIngest) Valid() bool {
	switch i {
	case "", IngestGenerated, IngestCurated, IngestNativeAPI, IngestSchema, IngestCapability:
		return true
	default:
		return false
	}
}

// Entry 表示一条已登记文档条目。
type Entry struct {
	// Category 是条目的 Diátaxis 分类。
	Category Category
	// Section 是条目的导航分区。
	Section Section
	// Order 是分区内的展示顺序，同一分区内唯一。
	Order int
	// Title 是条目标题。
	Title string
	// Summary 是条目摘要，用于生成索引。
	Summary string
	// Roles 是建议阅读角色，可为多个；为空表示未标注（守卫测试将拦截）。
	Roles []Role
	// Scene 是适用场景说明，一句话描述读者在什么情境下读这篇文档。
	Scene string
	// Owner 是文档责任人（账号），用于 CODEOWNERS 与元信息头校验。
	Owner string
	// Source 是文档来源，决定写入根目录。
	Source Source
	// AiIngest 是进入 RAG 的分区；为空表示不进入 RAG。
	AiIngest AiIngest
	// Package 是内容来源的 Go 导入路径；仅 SourceComment 条目使用。
	Package string
	// Output 是相对于 docs/ 的输出路径，例如 curated/tutorials/quick-start.md。
	Output string
	// NarrativeOnly 为 true 时按叙述档渲染（仅包注释）；为 false 时按参考档渲染。
	NarrativeOnly bool
}

// Registry 返回全部文档条目，是文档体系的唯一注册点。
// 每个包至多对应一个叙述档条目，避免相同知识内容被重复索引。
func Registry() []Entry {
	return []Entry{
		// ---------- 开始使用 ----------
		{
			Category:      CategoryTutorial,
			Section:       SectionGettingStarted,
			Order:         10,
			Title:         "快速开始：从零运行 zhongkui",
			Summary:       "从安装、配置到启动服务，完成一次最小可运行的部署。",
			Roles:         []Role{RoleOps},
			Scene:         "首次部署 zhongkui、验证采集到展示全链路是否打通时阅读。",
			Owner:         "@olinex",
			Source:        SourceCurated,
			AiIngest:      IngestCurated,
			Output:        "curated/tutorials/quick-start.md",
			NarrativeOnly: true,
		},
		{
			Category:      CategoryTutorial,
			Section:       SectionGettingStarted,
			Order:         20,
			Title:         "教程：看到你的第一条风险告警",
			Summary:       "编写一条 rego 规则、触发标签窗口统计，收到第一封风险告警。",
			Roles:         []Role{RoleSecOps},
			Scene:         "初次上手标签与告警、确认自编 rego 规则生效并收到第一封风险告警时阅读。",
			Owner:         "@olinex",
			Source:        SourceCurated,
			AiIngest:      IngestCurated,
			Output:        "curated/tutorials/first-alert.md",
			NarrativeOnly: true,
		},

		// ---------- 核心概念 ----------
		{
			Category:      CategoryExplanation,
			Section:       SectionConcepts,
			Order:         10,
			Title:         "系统总览：组件、数据流与部署拓扑",
			Summary:       "从全局视角说明运行组件、数据流、部署拓扑与关键边界。",
			Roles:         []Role{RoleOps, RoleDev, RoleSecOps},
			Scene:         "第一次接触项目、或需要判断某项能力由哪个组件负责时阅读。",
			Owner:         "@olinex",
			Source:        SourceCurated,
			AiIngest:      IngestCurated,
			Output:        "curated/explanation/system-overview.md",
			NarrativeOnly: true,
		},
		{
			Category:      CategoryExplanation,
			Section:       SectionConcepts,
			Order:         20,
			Title:         "领域模型：实体、关系与生命周期",
			Summary:       "说明多租户下的核心实体、实体关系、生命周期与存储归属。",
			Roles:         []Role{RoleDev, RoleSecOps},
			Scene:         "需要理解数据如何组织、某个字段属于哪个实体、写入后如何流转时阅读。",
			Owner:         "@olinex",
			Source:        SourceCurated,
			AiIngest:      IngestCurated,
			Output:        "curated/explanation/domain-model.md",
			NarrativeOnly: true,
		},
		{
			Category:      CategoryExplanation,
			Section:       SectionConcepts,
			Order:         30,
			Title:         "术语表",
			Summary:       "统一项目术语、英文名、代码位置与常见误解。",
			Roles:         []Role{RoleOps, RoleSecOps, RoleDev, RoleAdmin},
			Scene:         "阅读文档或排查问题时遇到不熟悉的术语时查阅。",
			Owner:         "@olinex",
			Source:        SourceCurated,
			AiIngest:      IngestCurated,
			Output:        "curated/explanation/glossary.md",
			NarrativeOnly: true,
		},
		{
			Category:      CategoryExplanation,
			Section:       SectionConcepts,
			Order:         40,
			Title:         "采集架构：wire/XDP、快照与上报",
			Summary:       "说明采集侧的 wire/XDP 抓包、快照生成与上报链路。",
			Roles:         []Role{RoleOps, RoleDev},
			Scene:         "需要理解采集链路构成、或在采集异常时排查故障时阅读。",
			Owner:         "@olinex",
			Source:        SourceComment,
			AiIngest:      IngestGenerated,
			Package:       "zhongkuisec.com/zhongkui/internal/capture",
			Output:        "generated/explanation/capture-architecture.md",
			NarrativeOnly: true,
		},
		{
			Category:      CategoryExplanation,
			Section:       SectionConcepts,
			Order:         50,
			Title:         "网络指纹与 TCP 检测原理",
			Summary:       "解释网络指纹的计算方式与 TCP 流检测原理。",
			Roles:         []Role{RoleSecOps, RoleDev},
			Scene:         "需要理解检测依据、并据此校准规则阈值时阅读。",
			Owner:         "@olinex",
			Source:        SourceComment,
			AiIngest:      IngestGenerated,
			Package:       "zhongkuisec.com/zhongkui/internal/network/detectors",
			Output:        "generated/explanation/network-detection.md",
			NarrativeOnly: true,
		},
		{
			Category:      CategoryExplanation,
			Section:       SectionConcepts,
			Order:         60,
			Title:         "流水线与调度",
			Summary:       "说明流水线的阶段划分与调度协作方式。",
			Roles:         []Role{RoleDev},
			Scene:         "需要理解流水线的阶段调度与协作方式时阅读。",
			Owner:         "@olinex",
			Source:        SourceComment,
			AiIngest:      IngestGenerated,
			Package:       "zhongkuisec.com/zhongkui/internal/pkg/pipeline",
			Output:        "generated/explanation/pipeline.md",
			NarrativeOnly: true,
		},
		{
			Category:      CategoryExplanation,
			Section:       SectionConcepts,
			Order:         70,
			Title:         "异步任务与队列",
			Summary:       "说明基于队列的异步任务调度与执行模型。",
			Roles:         []Role{RoleOps, RoleDev},
			Scene:         "遇到异步任务不执行或队列堆积、需要排查时阅读。",
			Owner:         "@olinex",
			Source:        SourceComment,
			AiIngest:      IngestGenerated,
			Package:       "zhongkuisec.com/zhongkui/internal/worker",
			Output:        "generated/explanation/async-worker.md",
			NarrativeOnly: true,
		},
		{
			Category:      CategoryExplanation,
			Section:       SectionConcepts,
			Order:         80,
			Title:         "数据源插件体系与 sqlite vtab",
			Summary:       "说明数据源插件体系与 sqlite 虚拟表机制的配合方式。",
			Roles:         []Role{RoleDev},
			Scene:         "需要理解数据源插件体系内部机制时阅读。",
			Owner:         "@olinex",
			Source:        SourceComment,
			AiIngest:      IngestGenerated,
			Package:       "zhongkuisec.com/zhongkui/internal/pkg/sqlitevtab/vtab",
			Output:        "generated/explanation/datasource-plugin.md",
			NarrativeOnly: true,
		},
		{
			Category:      CategoryExplanation,
			Section:       SectionConcepts,
			Order:         90,
			Title:         "AI Agent 功能架构详解",
			Summary:       "用白话讲解 AI 助手的数据结构、数据流转、流程图与抽象设计关系，面向初级研发。",
			Roles:         []Role{RoleDev},
			Scene:         "初次接触 AI 助手功能，或需要系统理解其数据结构与数据流转时阅读。",
			Owner:         "@olinex",
			Source:        SourceCurated,
			AiIngest:      IngestCurated,
			Output:        "curated/explanation/agent-architecture.md",
			NarrativeOnly: true,
		},
		{
			Category:      CategoryExplanation,
			Section:       SectionConcepts,
			Order:         100,
			Title:         "LLM Agent + RAG 设计",
			Summary:       "说明 LLM Agent 编排与 RAG 检索的整体设计。",
			Roles:         []Role{RoleDev},
			Scene:         "需要理解智能问答与检索的整体设计时阅读。",
			Owner:         "@olinex",
			Source:        SourceComment,
			AiIngest:      IngestGenerated,
			Package:       "zhongkuisec.com/zhongkui/internal/ai",
			Output:        "generated/explanation/agent-rag-design.md",
			NarrativeOnly: true,
		},
		{
			Category:      CategoryExplanation,
			Section:       SectionConcepts,
			Order:         110,
			Title:         "Agent 工具契约",
			Summary:       "约定 Agent 可调用工具的入参、出参与错误契约。",
			Roles:         []Role{RoleDev},
			Scene:         "需要扩展或调用 Agent 工具时阅读。",
			Owner:         "@olinex",
			Source:        SourceComment,
			AiIngest:      IngestGenerated,
			Package:       "zhongkuisec.com/zhongkui/internal/ai/tools",
			Output:        "generated/explanation/agent-tools.md",
			NarrativeOnly: true,
		},
		{
			Category:      CategoryExplanation,
			Section:       SectionConcepts,
			Order:         120,
			Title:         "管理端分层与视图 In/Out 解耦",
			Summary:       "说明管理端分层结构以及视图层 In/Out 解耦约束。",
			Roles:         []Role{RoleDev},
			Scene:         "新增后端接口、需要遵循管理端分层约束时阅读。",
			Owner:         "@olinex",
			Source:        SourceComment,
			AiIngest:      IngestGenerated,
			Package:       "zhongkuisec.com/zhongkui/internal/manager",
			Output:        "generated/explanation/manager-layering.md",
			NarrativeOnly: true,
		},
		{
			Category:      CategoryExplanation,
			Section:       SectionConcepts,
			Order:         130,
			Title:         "依赖注入与生命周期",
			Summary:       "说明 samber/do 依赖注入三层模式与资源生命周期管理。",
			Roles:         []Role{RoleDev},
			Scene:         "需要理解资源装配与生命周期管理时阅读。",
			Owner:         "@olinex",
			Source:        SourceComment,
			AiIngest:      IngestGenerated,
			Package:       "zhongkuisec.com/zhongkui/internal/provider",
			Output:        "generated/explanation/dependency-injection.md",
			NarrativeOnly: true,
		},
		{
			Category:      CategoryExplanation,
			Section:       SectionConcepts,
			Order:         140,
			Title:         "前端设计体系分层",
			Summary:       "说明前端样式体系的分层与设计令牌组织方式。",
			Roles:         []Role{RoleDev},
			Scene:         "进行前端样式与组件开发时阅读。",
			Owner:         "@olinex",
			Source:        SourceCurated,
			AiIngest:      IngestCurated,
			Output:        "curated/explanation/frontend-design-system.md",
			NarrativeOnly: true,
		},
		{
			Category:      CategoryExplanation,
			Section:       SectionConcepts,
			Order:         150,
			Title:         "前端功能开关与不可用态",
			Summary:       "说明前端功能开关的接入方式与不可用态呈现约定。",
			Roles:         []Role{RoleAdmin, RoleDev},
			Scene:         "接入前端功能开关、并理解能力不可用时的呈现约定时阅读。",
			Owner:         "@olinex",
			Source:        SourceCurated,
			AiIngest:      IngestCurated,
			Output:        "curated/explanation/frontend-feature-flags.md",
			NarrativeOnly: true,
		},
		{
			Category:      CategoryExplanation,
			Section:       SectionConcepts,
			Order:         160,
			Title:         "XDP/BPF 设计说明",
			Summary:       "说明内核侧 XDP/BPF 程序的设计与数据交互方式。",
			Roles:         []Role{RoleDev},
			Scene:         "需要理解内核侧 XDP/BPF 程序的设计与交互时阅读。",
			Owner:         "@olinex",
			Source:        SourceCurated,
			AiIngest:      IngestCurated,
			Output:        "curated/kernel/xdp_bpf_design.md",
			NarrativeOnly: true,
		},
		{
			Category:      CategoryExplanation,
			Section:       SectionConcepts,
			Order:         170,
			Title:         "文档体系架构与执行规划",
			Summary:       "说明文档大纲、生成与策展的来源隔离、事实投影、AI 上下文分层与开源治理。",
			Roles:         []Role{RoleDev},
			Scene:         "需要新增或调整文档、维护文档生成流水线、或为 AI 接入新的业务能力文档时阅读。",
			Owner:         "@olinex",
			Source:        SourceCurated,
			AiIngest:      IngestCurated,
			Output:        "curated/explanation/documentation-architecture.md",
			NarrativeOnly: true,
		},

		// ---------- 操作指南 ----------
		{
			Category:      CategoryHowTo,
			Section:       SectionGuides,
			Order:         10,
			Title:         "故障排查：启动、采集、标签、通知与检索",
			Summary:       "按症状定位常见故障，并给出验证与恢复步骤。",
			Roles:         []Role{RoleOps, RoleSecOps, RoleDev},
			Scene:         "服务无法启动、采集无数据、标签不触发、通知未送达或检索异常时阅读。",
			Owner:         "@olinex",
			Source:        SourceCurated,
			AiIngest:      IngestCurated,
			Output:        "curated/how-to/troubleshooting.md",
			NarrativeOnly: true,
		},
		{
			Category:      CategoryHowTo,
			Section:       SectionGuides,
			Order:         20,
			Title:         "如何接入新的数据源插件",
			Summary:       "实现数据源接口并注册一个新的数据源插件。",
			Roles:         []Role{RoleOps, RoleDev},
			Scene:         "需要把一个新的采集数据源接入平台时阅读。",
			Owner:         "@olinex",
			Source:        SourceCurated,
			AiIngest:      IngestCurated,
			Output:        "curated/how-to/add-datasource-plugin.md",
			NarrativeOnly: true,
		},
		{
			Category:      CategoryHowTo,
			Section:       SectionGuides,
			Order:         30,
			Title:         "如何新增通知渠道与模板",
			Summary:       "扩展通知渠道类型并维护对应的多语言模板。",
			Roles:         []Role{RoleOps, RoleDev},
			Scene:         "需要为告警扩展新的触达渠道及其多语言模板时阅读。",
			Owner:         "@olinex",
			Source:        SourceCurated,
			AiIngest:      IngestCurated,
			Output:        "curated/how-to/add-notify-channel.md",
			NarrativeOnly: true,
		},
		{
			Category:      CategoryHowTo,
			Section:       SectionGuides,
			Order:         40,
			Title:         "如何编写一次数据库迁移",
			Summary:       "新增一次向前/向后兼容的数据库迁移脚本。",
			Roles:         []Role{RoleOps, RoleDev},
			Scene:         "需要在 schema 演进中编写一次数据库迁移时阅读。",
			Owner:         "@olinex",
			Source:        SourceCurated,
			AiIngest:      IngestCurated,
			Output:        "curated/how-to/write-migration.md",
			NarrativeOnly: true,
		},
		{
			Category:      CategoryHowTo,
			Section:       SectionGuides,
			Order:         50,
			Title:         "如何新增枚举类并生成绑定",
			Summary:       "在枚举注册表中新增枚举类并生成前后端绑定。",
			Roles:         []Role{RoleDev},
			Scene:         "需要在枚举注册表中扩展取值并生成前后端绑定时阅读。",
			Owner:         "@olinex",
			Source:        SourceCurated,
			AiIngest:      IngestCurated,
			Output:        "curated/how-to/add-enum-class.md",
			NarrativeOnly: true,
		},
		{
			Category:      CategoryHowTo,
			Section:       SectionGuides,
			Order:         60,
			Title:         "如何新增一个管理端页面",
			Summary:       "说明新增管理端页面时的路由、视图与 API 接入步骤。",
			Roles:         []Role{RoleDev},
			Scene:         "需要新增一个管理端功能页面时阅读。",
			Owner:         "@olinex",
			Source:        SourceCurated,
			AiIngest:      IngestCurated,
			Output:        "curated/how-to/frontend-add-page.md",
			NarrativeOnly: true,
		},
	}
}

// Generated 返回所有由代码声明派生的条目（SourceComment）。
func Generated() []Entry {
	entries := Registry()
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if e.Source == SourceComment {
			out = append(out, e)
		}
	}
	return out
}

// Curated 返回所有策展条目（SourceCurated）。
func Curated() []Entry {
	entries := Registry()
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if e.Source == SourceCurated {
			out = append(out, e)
		}
	}
	return out
}
