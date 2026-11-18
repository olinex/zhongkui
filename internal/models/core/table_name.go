package core

// 本文件集中声明模型对应的数据库表名，供 GORM 与文档生成统一读取。

// TableName 返回 AccessKey 对应的数据库表名。
func (m *AccessKey) TableName() string {
	return "access_keys"
}

// TableName 返回 Capturer 对应的数据库表名。
func (m *Capturer) TableName() string {
	return "capturers"
}

// TableName 返回 Datasource 对应的数据库表名。
func (m *Datasource) TableName() string {
	return "datasources"
}

// TableName 返回 Group 对应的数据库表名。
func (m *Group) TableName() string {
	return "groups"
}

// TableName 返回 Namespace 对应的数据库表名。
func (m *Namespace) TableName() string {
	return "namespaces"
}

// TableName 返回 NotificationChannel 对应的数据库表名。
func (m *NotificationChannel) TableName() string {
	return "notification_channels"
}

// TableName 返回 NotificationGroup 对应的数据库表名。
func (m *NotificationGroup) TableName() string {
	return "notification_groups"
}

// TableName 返回 NotificationGroupTarget 对应的数据库表名。
func (m *NotificationGroupTarget) TableName() string {
	return "notification_group_targets"
}

// TableName 返回 NotificationRecord 对应的数据库表名。
func (m *NotificationRecord) TableName() string {
	return "notification_records"
}

// TableName 返回 NotificationTarget 对应的数据库表名。
func (m *NotificationTarget) TableName() string {
	return "notification_targets"
}

// TableName 返回 NotificationTemplate 对应的数据库表名。
func (m *NotificationTemplate) TableName() string {
	return "notification_templates"
}

// TableName 返回 RbacPolicy 对应的数据库表名。
func (m *RbacPolicy) TableName() string {
	return "rbac_policies"
}

// TableName 返回 RbacPolicyTemplate 对应的数据库表名。
func (m *RbacPolicyTemplate) TableName() string {
	return "rbac_policy_templates"
}

// TableName 返回 User 对应的数据库表名。
func (m *User) TableName() string {
	return "users"
}

// TableName 返回 UserPasswordReset 对应的数据库表名。
func (m *UserPasswordReset) TableName() string {
	return "user_password_resets"
}
