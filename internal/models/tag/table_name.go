package tag

// 本文件集中声明模型对应的数据库表名，供 GORM 与文档生成统一读取。

// TableName 返回 TagEventCondition 对应的数据库表名。
func (m *TagEventCondition) TableName() string {
	return "tag_event_conditions"
}

// TableName 返回 TagEventRecord 对应的数据库表名。
func (m *TagEventRecord) TableName() string {
	return "tag_event_records"
}

// TableName 返回 TagEventRecordHit 对应的数据库表名。
func (m *TagEventRecordHit) TableName() string {
	return "tag_event_record_hits"
}

// TableName 返回 TagEventRule 对应的数据库表名。
func (m *TagEventRule) TableName() string {
	return "tag_event_rules"
}

// TableName 返回 TagEventRuleVersion 对应的数据库表名。
func (m *TagEventRuleVersion) TableName() string {
	return "tag_event_rule_versions"
}

// TableName 返回 TagRule 对应的数据库表名。
func (m *TagRule) TableName() string {
	return "tag_rules"
}

// TableName 返回 TagRuleSample 对应的数据库表名。
func (m *TagRuleSample) TableName() string {
	return "tag_rule_samples"
}

// TableName 返回 TagRuleVersion 对应的数据库表名。
func (m *TagRuleVersion) TableName() string {
	return "tag_rule_versions"
}

// TableName 返回 TagSumming 对应的数据库表名。
func (m *TagSumming) TableName() string {
	return "tag_summings"
}
