package base

// NamespaceScope 返回一次跨命名空间操作应覆盖的命名空间集合：调用方限定的命名空间（如非空）
// 加上内置系统命名空间（承载由代码声明的公开共享数据，对所有租户可见）。
//
// 命名空间为空时不追加系统作用域，保持既有「空命名空间不命中任何数据」的语义；命名空间本身
// 为系统命名空间时不重复。结果按「调用方命名空间在前、系统命名空间在后」排列，可直接用于
// SQL 的 `IN` 条件（如 `namespace_name IN ?`）。
//
// 注意与视图层「可见命名空间」的区别：后者（user.go 的 visibleNamespaces）是从展示列表中
// 剔除系统命名空间，与本函数方向相反。
func NamespaceScope(namespaceName string) []string {
	names := []string{namespaceName}
	if namespaceName != "" && namespaceName != SystemNamespace {
		names = append(names, SystemNamespace)
	}
	return names
}
