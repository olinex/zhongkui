package core

import (
	"zhongkuisec.com/zhongkui/internal/pkg/base"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// RbacPolicy 表示基于角色的访问控制策略
// 该结构体遵循Casbin RBAC模型，支持策略(policy)和组(group)两种类型
// 数据库表名默认为"rbac_policies"，通过uniqueIndex:uq_rbac_policies_ptype_v0_v1_v2确保唯一性
type RbacPolicy struct {
	database.BaseModel

	// Ptype 策略类型，只能是'p'(policy)或'g'(group)
	// 'p'表示权限策略，'g'表示用户组关系
	Ptype string `gorm:"type:CHAR(1)"`

	// V0 策略的第一个参数
	// 当Ptype='p'时：表示组名(group name)
	// 当Ptype='g'时：表示用户联合ID(user union id)
	V0 string `gorm:"type:VARCHAR(36)"`

	// V1 策略的第二个参数
	// 当Ptype='p'时：表示目标对象名(target object name)
	// 当Ptype='g'时：表示组名(group name)
	V1 string `gorm:"type:VARCHAR(512)"`

	// V2 策略的第三个参数
	// 当Ptype='p'时：表示操作名(action name)
	// 当Ptype='g'时：表示命名空间(namespace)
	V2 string `gorm:"type:VARCHAR(32)"`

	// V3 策略的第四个参数
	// 当Ptype='p'时：表示效果(effect)，只能是'allow'或'deny'
	// 当Ptype='g'时：留空
	V3 string `gorm:"type:VARCHAR(64)"`

	// V4 策略的第五个参数
	// 为 gorm-adapter 的 CasbinRule 结构预留（INSERT 会携带 v4 列），当前模型不使用，留空
	V4 string `gorm:"type:VARCHAR(64)"`

	// V5 策略的第六个参数
	// 为 gorm-adapter 的 CasbinRule 结构预留（INSERT 会携带 v5 列），当前模型不使用，留空
	V5 string `gorm:"type:VARCHAR(64)"`
}

// NewGroupPolicy 创建新的组策略实例
// 组策略定义了一个组对特定目标对象的操作权限
//
// 参数:
//   - group: 组名，表示拥有权限的组
//   - target: 目标对象名，表示被访问的资源
//   - action: 操作名，表示对资源的操作类型
//   - effect: 效果，只能是'allow'(允许)或'deny'(拒绝)
//
// 返回:
//   - *RbacPolicy: 新创建的组策略实例
func NewGroupPolicy(group, target, action, effect string) *RbacPolicy {
	return &RbacPolicy{
		Ptype: base.PolicyLabel,
		V0:    group,
		V1:    target,
		V2:    action,
		V3:    effect,
	}
}

// NewUserPolicy 创建新的用户策略实例
// 用户策略定义了用户与组在特定命名空间下的关系
//
// 参数:
//   - unionID: 用户联合ID，唯一标识用户
//   - group: 组名，表示用户所属的组
//   - namespace: 命名空间，用于隔离不同环境或租户的策略
//
// 返回:
//   - *RbacPolicy: 新创建的用户策略实例
func NewUserPolicy(unionID serializer.HexUUID, group, namespace string) *RbacPolicy {
	return &RbacPolicy{
		Ptype: base.GroupLabel,
		V0:    unionID.String(),
		V1:    group,
		V2:    namespace,
	}
}

// RbacPolicyTemplate 表示RBAC策略模板
// 用于预定义可用的目标对象和操作组合，确保策略的一致性
// 数据库表名默认为"rbac_policy_templates"，通过uniqueIndex:uq_rbac_policy_templates_target_action确保唯一性
type RbacPolicyTemplate struct {
	database.BaseModel

	// Target 目标对象名，表示被访问的资源
	// 最大长度512字符，与Action字段共同构成唯一索引
	Target string `gorm:"type:VARCHAR(512)"`

	// Action 操作名，表示对资源的操作类型
	// 最大长度64字符，与Target字段共同构成唯一索引
	Action string `gorm:"type:VARCHAR(64)"`
}

// NewRbacPolicyTemplate 创建新的RBAC策略模板实例
//
// 参数:
//   - target: 目标对象名，表示被访问的资源
//   - action: 操作名，表示对资源的操作类型
//
// 返回:
//   - *RbacPolicyTemplate: 新创建的策略模板实例
func NewRbacPolicyTemplate(target, action string) *RbacPolicyTemplate {
	return &RbacPolicyTemplate{
		Target: target,
		Action: action,
	}
}
