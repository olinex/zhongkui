package sqlitevtab

// sqliteKeywords 是 SQLite 保留关键字集合（小写）。数据源名作为 vtable
// 表名直接嵌入 SQL，保留关键字需加引号才能使用，因此在注册期直接拒绝，
// 避免 rego 作者踩坑。
var sqliteKeywords = map[string]struct{}{
	"abort": {}, "action": {}, "add": {}, "after": {}, "all": {}, "alter": {},
	"always": {}, "analyze": {}, "and": {}, "as": {}, "asc": {}, "attach": {},
	"autoincrement": {}, "before": {}, "begin": {}, "between": {}, "by": {},
	"cascade": {}, "case": {}, "cast": {}, "check": {}, "collate": {},
	"column": {}, "commit": {}, "conflict": {}, "constraint": {}, "create": {},
	"cross": {}, "current": {}, "current_date": {}, "current_time": {},
	"current_timestamp": {}, "database": {}, "default": {}, "deferrable": {},
	"deferred": {}, "delete": {}, "desc": {}, "detach": {}, "distinct": {},
	"do": {}, "drop": {}, "each": {}, "else": {}, "end": {}, "escape": {},
	"except": {}, "exclude": {}, "exclusive": {}, "exists": {}, "explain": {},
	"fail": {}, "filter": {}, "first": {}, "following": {}, "for": {},
	"foreign": {}, "from": {}, "full": {}, "generated": {}, "glob": {},
	"group": {}, "groups": {}, "having": {}, "if": {}, "ignore": {},
	"immediate": {}, "in": {}, "index": {}, "indexed": {}, "initially": {},
	"inner": {}, "insert": {}, "instead": {}, "intersect": {}, "into": {},
	"is": {}, "isnull": {}, "join": {}, "key": {}, "last": {}, "left": {},
	"like": {}, "limit": {}, "match": {}, "materialized": {}, "natural": {},
	"no": {}, "not": {}, "nothing": {}, "notnull": {}, "null": {}, "nulls": {},
	"of": {}, "offset": {}, "on": {}, "or": {}, "order": {}, "others": {},
	"outer": {}, "over": {}, "partition": {}, "plan": {}, "pragma": {},
	"preceding": {}, "primary": {}, "query": {}, "raise": {}, "range": {},
	"recursive": {}, "references": {}, "regexp": {}, "reindex": {},
	"release": {}, "rename": {}, "replace": {}, "restrict": {},
	"returning": {}, "right": {}, "rollback": {}, "row": {}, "rows": {},
	"savepoint": {}, "select": {}, "set": {}, "table": {}, "temp": {},
	"temporary": {}, "then": {}, "ties": {}, "to": {}, "transaction": {},
	"trigger": {}, "unbounded": {}, "union": {}, "unique": {}, "update": {},
	"using": {}, "vacuum": {}, "values": {}, "view": {}, "virtual": {},
	"when": {}, "where": {}, "window": {}, "with": {}, "without": {},
}
