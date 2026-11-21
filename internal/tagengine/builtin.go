package tagengine

import (
	// 标准库
	"context"
	"strconv"

	// 第三方
	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/types"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/sqlitevtab"
)

const (
	// sqliteQueryName 自定义 OPA 内置函数名：sqlite.query(sql, params)。
	// 在 rego 规则中通过 SQL 语句直接查询注册为 sqlite vtable 的第三方数据源，
	// 返回行对象数组（键为列名，值为该行值）。
	sqliteQueryName = "sqlite.query"
)

// ErrSQLitePoolNotActivated 表示 sqlite 连接池尚未被绑定到求值上下文，
// sqlite.query 内置函数不可用。
var ErrSQLitePoolNotActivated = errors.New("tagengine: sqlite pool not activated")

// poolCtxKey 是 sqlite 连接池在 context 中存储键的私有类型，避免与其他库的
// context.WithValue 键冲突。池按命名空间解析后由引擎在求值前经 WithDatasourcePool
// 绑定到 ctx，builtin 每次调用时读取该 ctx 值；并发求值各命名空间用各自 ctx 隔离。
type poolCtxKey struct{}

// WithDatasourcePool 返回一个绑定了 sqlite 连接池的 ctx，供求值路径（引擎 Recognize、
// 视图 dry-run/样例校验）在调用 sqlite.query 前关联正确的命名空间池。
func WithDatasourcePool(ctx context.Context, pool *sqlitevtab.SQLitePool) context.Context {
	return context.WithValue(ctx, poolCtxKey{}, pool)
}

// poolFromContext 从 ctx 提取 sqlite 连接池；未绑定时返回 nil。
func poolFromContext(ctx context.Context) *sqlitevtab.SQLitePool {
	if pool, ok := ctx.Value(poolCtxKey{}).(*sqlitevtab.SQLitePool); ok {
		return pool
	}
	return nil
}

// sqliteQueryDecl 是 sqlite.query 内置函数的声明：
// 签名 (string, array[any]) → array[object]，Memoize 开启使单次评估内
// 相同参数复用结果。
var sqliteQueryDecl = &rego.Function{
	Name:        sqliteQueryName,
	Description: "query a third-party datasource registered as an sqlite vtable",
	Decl: types.NewFunction(
		types.Args(types.S, types.NewArray(nil, types.A)),
		types.NewArray(nil, types.NewObject(nil, types.NewDynamicProperty(types.S, types.A))),
	),
	Memoize: true,
}

// sqliteQuery 是 sqlite.query 内置函数的实现：从求值上下文取出绑定的 sqlite 池
// （经 WithDatasourcePool 注入），提取 SQL 与绑定参数，执行只读查询，并将结果编码
// 为行对象数组。未绑池时返回 ErrSQLitePoolNotActivated。
// 返回错误时规则体不满足（安全闭合），StrictBuiltinErrors 场景下错误浮出。
func sqliteQuery(bctx rego.BuiltinContext, sqlTerm, paramsTerm *ast.Term) (*ast.Term, error) {
	sqlStr, ok := sqlTerm.Value.(ast.String)
	if !ok {
		return nil, errors.Newf("tagengine: %s: first argument must be a string, got %T", sqliteQueryName, sqlTerm.Value)
	}
	arr, ok := paramsTerm.Value.(*ast.Array)
	if !ok {
		return nil, errors.Newf("tagengine: %s: second argument must be an array, got %T", sqliteQueryName, paramsTerm.Value)
	}
	args, err := regoArrayToBindArgs(arr)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	pool := poolFromContext(bctx.Context)
	if pool == nil {
		return nil, ErrSQLitePoolNotActivated
	}
	cols, rows, err := pool.Query(bctx.Context, string(sqlStr), args...)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return rowsToTerm(cols, rows)
}

// regoArrayToBindArgs 将 rego 数组参数转换为按顺序绑定到 ? 占位符的 Go 值。
func regoArrayToBindArgs(arr *ast.Array) ([]any, error) {
	var args []any
	if err := arr.Iter(func(term *ast.Term) error {
		v, err := regoValueToBind(term.Value)
		if err != nil {
			return errors.WithStack(err)
		}
		args = append(args, v)
		return nil
	}); err != nil {
		return nil, errors.WithStack(err)
	}
	return args, nil
}

// regoValueToBind 将单个 rego 值转换为 sqlite 绑定值：
// null→nil、boolean→bool、number→int64（整型）/float64（非整型）、
// string→string；其余类型（对象/数组/集合）不受支持。
func regoValueToBind(v ast.Value) (any, error) {
	switch x := v.(type) {
	case ast.Null:
		return nil, nil
	case ast.Boolean:
		return bool(x), nil
	case ast.Number:
		if i, ok := x.Int64(); ok {
			return i, nil
		} else if f, ok := x.Float64(); ok {
			return f, nil
		} else {
			return nil, errors.Newf("tagengine: %s: unsupported number %v", sqliteQueryName, x)
		}
	case ast.String:
		return string(x), nil
	default:
		return nil, errors.Newf("tagengine: %s: unsupported parameter type %T", sqliteQueryName, v)
	}
}

// rowsToTerm 将查询结果（列名 + 行值）编码为行对象数组的 rego 值。
func rowsToTerm(cols []string, rows [][]any) (*ast.Term, error) {
	arr := ast.NewArray()
	for _, row := range rows {
		pairs := make([][2]*ast.Term, len(cols))
		for i, col := range cols {
			val, err := goValueToTerm(row[i])
			if err != nil {
				return nil, errors.WithStack(err)
			}
			pairs[i] = [2]*ast.Term{ast.StringTerm(col), val}
		}
		arr = arr.Append(ast.ObjectTerm(pairs...))
	}
	return ast.NewTerm(arr), nil
}

// goValueToTerm 将查询行值编码为 rego 值：nil→null、bool→boolean、
// string→string、[]byte→string（原始字节）、int/int64→number、
// float64→number；其余类型不受支持。
func goValueToTerm(v any) (*ast.Term, error) {
	switch x := v.(type) {
	case nil:
		return ast.NullTerm(), nil
	case bool:
		return ast.BooleanTerm(x), nil
	case string:
		return ast.StringTerm(x), nil
	case []byte:
		return ast.StringTerm(string(x)), nil
	case int:
		return ast.NewTerm(ast.Number(strconv.FormatInt(int64(x), 10))), nil
	case int64:
		return ast.NewTerm(ast.Number(strconv.FormatInt(x, 10))), nil
	case float64:
		return ast.NewTerm(ast.Number(strconv.FormatFloat(x, 'g', -1, 64))), nil
	default:
		return nil, errors.Newf("tagengine: %s: unsupported result value type %T", sqliteQueryName, v)
	}
}

// CustomFunction 描述一个 rego 规则可调用的自定义 OPA 内置函数元信息。
// Description 为 i18n 消息键，由接口层按请求语言翻译后输出。
type CustomFunction struct {
	// Name 函数全名（如 sqlite.query）。
	Name string
	// Description 函数说明的 i18n 消息键（tagengine.custom-functions.<name>.description）。
	Description string
	// Signature 人类可读的调用签名（如 sqlite.query(sql, params)）。
	Signature string
	// Example 用法示例文本，可为空。
	Example string
}

// CustomFunctions 返回 rego 规则可调用的自定义 OPA 内置函数元信息列表。
// 供前端 Rego 编辑器实现自定义函数的关键词高亮、悬停签名提示与自动补全。
func CustomFunctions() []CustomFunction {
	return []CustomFunction{
		{
			Name:        sqliteQueryName,
			Description: "tagengine.custom-functions.sqlite.query.description",
			Signature:   "sqlite.query(sql, params)",
			Example: `rows := sqlite.query("SELECT value FROM buildin_redis WHERE key = ?", ["test_key"])
tags contains "redis-match" if {
	count(rows) > 0
	rows[0].value == "yes"
}`,
		},
		{
			Name:        revealName,
			Description: "tagengine.custom-functions.crypto.reveal.description",
			Signature:   "crypto.reveal(field)",
			Example: `tags contains "bearer-auth" if {
	startswith(crypto.reveal("reqHdrAuthorization"), "Bearer ")
}`,
		},
		{
			Name:        codecParseName,
			Description: "tagengine.custom-functions.codec.parse.description",
			Signature:   "codec.parse(contentType, text)",
			Example: `body := codec.parse(input.reqHdrContentType, crypto.reveal("reqBody"))
tags contains "admin-login" if {
	body.username == "admin"
}`,
		},
	}
}

// init 全局注册 sqlite.query：rego.RegisterBuiltin2 同时注册 ast 声明
// （编译/类型检查可见，校验与 dry-run 不误报 "undefined function"）与
// topdown 实现（评估时执行）。
func init() {
	rego.RegisterBuiltin2(sqliteQueryDecl, sqliteQuery)
}
