package api

// 本文件集中声明模型对应的数据库表名，供 GORM 与文档生成统一读取。

// TableName 返回 HttpApiMeta 对应的数据库表名。
func (m *HttpApiMeta) TableName() string {
	return "http_api_meta"
}

// TableName 返回 HttpApiNode 对应的数据库表名。
func (m *HttpApiNode) TableName() string {
	return "http_api_nodes"
}
