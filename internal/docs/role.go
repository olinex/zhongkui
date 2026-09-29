package docs

import "slices"

// Role 表示文档建议阅读的角色。
type Role string

const (
	// RoleAdmin 管理员：负责平台治理——用户、角色权限、命名空间、功能开关的开启与配置。
	RoleAdmin Role = "admin"
	// RoleSecOps 安全运营：日常使用告警与标签能力——查看风险、编写/调整 rego 规则、
	// 解读标签窗口统计、接收通知。
	RoleSecOps Role = "secops"
	// RoleOps 运维：负责部署与运行——安装配置、数据库迁移、数据源接入、通知渠道配置、
	// 异步任务与队列、采集运行。
	RoleOps Role = "ops"
	// RoleDev 研发：扩展系统能力——开发数据源插件、新增通知渠道类型、新增枚举类、
	// 扩展 AI Agent 工具、新增管理端页面、依赖注入与生命周期、流水线调度。
	RoleDev Role = "dev"
)

// Roles 返回全部建议阅读角色，是角色集合的唯一注册点。
func Roles() []Role {
	return []Role{
		RoleAdmin,
		RoleSecOps,
		RoleOps,
		RoleDev,
	}
}

// Valid 报告该角色是否为已注册的合法角色。
func (r Role) Valid() bool {
	return slices.Contains(Roles(), r)
}

// Label 返回该角色的中文标签。
func (r Role) Label() string {
	switch r {
	case RoleAdmin:
		return "管理员"
	case RoleSecOps:
		return "安全运营"
	case RoleOps:
		return "运维"
	case RoleDev:
		return "研发"
	default:
		return string(r)
	}
}
