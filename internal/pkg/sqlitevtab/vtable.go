// Package sqlitevtab 提供"通用 sqlite vtable 外部数据源接入"设施。
// 第三方数据源只需实现本包的 Datasource 标准接口（Open 返回行游标 Rows），
// 即可在任意时机经 *SQLitePool 动态注册为 sqlite 虚拟表（vtable），并通过
// SQL 语句查询；vtable 的 schema 声明、下推约束（BestIndex/xFilter）与
// 结果列输出（xColumn）均由本包代理，数据源实现无需关心 sqlite 细节。
// 数据源实例与 sqlite 连接的生命周期统一由 internal/pkg/resourcepool 管理：
// sqlite 连接按固定 Key 池化复用（每个连接独占一个内存库），数据源按
// 数据源名池化复用；vtable 回调在查询 goroutine 上同步执行，查询上下文
// 经连接对象透传给数据源。
//
// 包内错误统一使用 internal/pkg/errors 创建；查询为只读语义（连接级
// authorizer 拒绝一切写操作）；单次查询默认最多返回 1000 行，超出即报错。
//
// Package sqlitevtab provides a "generic sqlite vtable external datasource
// access" facility. A third-party datasource only needs to implement the
// package's Datasource standard interface (Open returns a Rows cursor) to be
// dynamically registered as an sqlite virtual table via *SQLitePool at any
// time and queried through SQL. Schema declaration, pushdown constraints
// (BestIndex/xFilter) and result column output (xColumn) are all proxied by
// this package, so datasource implementations need not care about sqlite
// details. The lifecycles of datasource instances and sqlite connections are
// uniformly managed by internal/pkg/resourcepool: sqlite connections are
// pooled by a fixed key (each connection owns a private in-memory database),
// and datasources are pooled per datasource name. VTable callbacks run
// synchronously on the query goroutine, and the query context is passed to
// datasources through the connection object.
//
// Errors in this package are created with internal/pkg/errors; queries are
// strictly read-only (a connection-level authorizer denies all writes); a
// single query returns at most 1000 rows by default and errors out beyond.
package sqlitevtab

import (
	// stdlib
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	// third-party
	sqlite3 "github.com/ncruces/go-sqlite3"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/resourcepool"
)

// dataVTab 是 sqlite vtable 适配实例：持有所属池、数据源名、schema、
// 注册版本号、所在连接与注销标记。version 记录该模块注册时的注册表版本，
// 供游标归还数据源实例时校验注册表是否已变更（覆盖/注销后旧实例不得回池）。
// 方法全部使用值接收者，以保证 connect 闭包返回的值类型实例可直接作为
// sqlite3.VTab 使用。
// dataVTab is the sqlite vtable adapter instance: it holds the owning pool,
// the datasource name, the schema, the registry version at registration time,
// the owning connection and the tombstone flag. version records the registry
// version when the module was registered, so cursors can verify on return
// whether the registry changed (stale instances after overwrite/unregister must
// not return to the pool). All methods use value receivers so that the value
// instance returned by the connect closure can be used directly as a
// sqlite3.VTab.
type dataVTab struct {
	pool      *SQLitePool
	name      string
	schema    Schema
	sc        *sqliteConn
	version   uint64
	tombstone bool
}

// BestIndex 实现 sqlite3.VTab.BestIndex：消费 Filterable 列上可映射的
// 可用约束，生成 "column:op" 计划（IdxStr）并分配 ArgvIndex。
// Omit 恒为 false，让 sqlite 复查每个返回行以兜底正确性。
// sqlite 对常量 LIKE/GLOB 模式会额外生成一组合成范围约束（如 name >= 'GO'
// 且 name < 'gp'），与 LIKE/GLOB 约束本身冗余并泄露 sqlite 内部细节；
// 这类约束在此跳过，保证下推契约只包含用户书写的谓词。
// BestIndex implements sqlite3.VTab.BestIndex: it consumes usable constraints
// on Filterable columns, producing a "column:op" plan (IdxStr) with ArgvIndex
// assigned. Omit stays false so sqlite re-checks every returned row for
// correctness. For constant LIKE/GLOB patterns sqlite additionally generates a
// set of synthetic range constraints (e.g. name >= 'GO' and name < 'gp') that
// are redundant with the LIKE/GLOB constraint itself and leak sqlite internals;
// such constraints are skipped here so the pushdown contract only carries
// user-authored predicates.
func (v dataVTab) BestIndex(idx *sqlite3.IndexInfo) error {
	filterable := make(map[int]bool, len(v.schema.Filterable))
	for _, col := range v.schema.Filterable {
		filterable[col] = true
	}
	// 先扫描：标记存在 LIKE/GLOB 约束的可过滤列
	likeCols := make(map[int]bool)
	for _, cst := range idx.Constraint {
		if !cst.Usable || !filterable[cst.Column] {
			continue
		}
		if cst.Op == sqlite3.INDEX_CONSTRAINT_LIKE || cst.Op == sqlite3.INDEX_CONSTRAINT_GLOB {
			likeCols[cst.Column] = true
		}
	}
	var plan []string
	argv := 1
	for i, cst := range idx.Constraint {
		if !cst.Usable || !filterable[cst.Column] {
			continue
		}
		op, ok := sqliteOpToOp(cst.Op)
		if !ok {
			continue
		}
		if likeCols[cst.Column] && (cst.Op == sqlite3.INDEX_CONSTRAINT_GT || cst.Op == sqlite3.INDEX_CONSTRAINT_GE ||
			cst.Op == sqlite3.INDEX_CONSTRAINT_LT || cst.Op == sqlite3.INDEX_CONSTRAINT_LE) {
			// LIKE/GLOB 优化产生的合成范围约束：跳过，sqlite 自行复查
			continue
		}
		idx.ConstraintUsage[i] = sqlite3.IndexConstraintUsage{ArgvIndex: argv, Omit: false}
		plan = append(plan, fmt.Sprintf("%d:%d", cst.Column, op))
		argv++
	}
	// IdxStr 将下推计划序列化为逗号分隔的 "column:op" 字符串，随 xFilter 的
	// idxStr 参数原样回传，由 decodePlan 解码还原为约束列表。
	// IdxStr serializes the pushdown plan into a comma-separated "column:op"
	// string passed verbatim to xFilter via idxStr and decoded back by decodePlan.
	idx.IdxStr = strings.Join(plan, ",")
	// IdxNum 是随计划回传的整型编号；本实现计划细节全部承载在 IdxStr 中，
	// IdxNum 恒为 1，仅标记存在有效计划（xFilter 不再据此分派）。
	// IdxNum is the integer id passed back with the plan; all plan details live
	// in IdxStr here, so IdxNum stays 1 merely marking a valid plan (xFilter
	// does not dispatch on it).
	idx.IdxNum = 1
	// EstimatedCost 为计划估算成本，成本越低 sqlite 越倾向选择该访问路径。
	// 本模块仅提供单一访问方式（Filterable 下推 + sqlite 复查），故恒为
	// 固定粗估值，规划器选择不受影响。
	// EstimatedCost is the estimated plan cost; lower costs make sqlite prefer
	// this access path. The module offers a single access method (Filterable
	// pushdown + sqlite re-check), so a fixed rough estimate is used and the
	// planner's choice is unaffected.
	idx.EstimatedCost = 1e6
	// EstimatedRows 为计划估算返回行数，仅供 sqlite 规划器成本参考，与实际
	// 结果行数无关；同 EstimatedCost，恒为固定粗估值。
	// EstimatedRows is the estimated number of returned rows, used only by the
	// sqlite planner for cost reference and unrelated to the actual row count;
	// like EstimatedCost, it is a fixed rough estimate.
	idx.EstimatedRows = 10000
	return nil
}

// Open 实现 sqlite3.VTab.Open：返回一个绑定到本 vtab 的行游标。
// Open implements sqlite3.VTab.Open: returns a row cursor bound to this vtab.
func (v dataVTab) Open() (sqlite3.VTabCursor, error) {
	return &dataCursor{vtab: &v}, nil
}

// sqliteOpToOp 将 sqlite IndexConstraintOp 映射为本包 Op；无法映射的
// 操作符返回 false（不消费该约束）。
// sqliteOpToOp maps an sqlite IndexConstraintOp to this package's Op;
// unmappable operators return false (the constraint is not consumed).
func sqliteOpToOp(op sqlite3.IndexConstraintOp) (Op, bool) {
	switch op {
	case sqlite3.INDEX_CONSTRAINT_EQ:
		return OpEQ, true
	case sqlite3.INDEX_CONSTRAINT_NE:
		return OpNE, true
	case sqlite3.INDEX_CONSTRAINT_GT:
		return OpGT, true
	case sqlite3.INDEX_CONSTRAINT_GE:
		return OpGE, true
	case sqlite3.INDEX_CONSTRAINT_LT:
		return OpLT, true
	case sqlite3.INDEX_CONSTRAINT_LE:
		return OpLE, true
	case sqlite3.INDEX_CONSTRAINT_LIKE:
		return OpLIKE, true
	case sqlite3.INDEX_CONSTRAINT_GLOB:
		return OpGLOB, true
	case sqlite3.INDEX_CONSTRAINT_MATCH:
		return OpMATCH, true
	case sqlite3.INDEX_CONSTRAINT_ISNULL:
		return OpISNULL, true
	case sqlite3.INDEX_CONSTRAINT_ISNOTNULL:
		return OpISNOTNULL, true
	default:
		return 0, false
	}
}

// dataCursor 是 vtable 行游标：借出数据源实例并逐行推进。
// dataCursor is the vtable row cursor: it borrows a datasource instance and
// advances row by row.
type dataCursor struct {
	vtab   *dataVTab
	ds     Datasource
	rows   Rows
	row    []any
	rowID  int64
	eof    bool
	closed bool
}

// Filter 实现 sqlite3.VTabCursor.Filter：解码下推计划、借出数据源、
// 打开行游标并预读第一行。
// Filter implements sqlite3.VTabCursor.Filter: it decodes the pushdown plan,
// borrows the datasource, opens the row cursor and prefetches the first row.
func (c *dataCursor) Filter(idxNum int, idxStr string, arg ...sqlite3.Value) error {
	v := c.vtab
	if v.tombstone {
		return errors.Newf("%w: %q", ErrDatasourceNotRegistered, v.name)
	}
	pushdown := decodePlan(idxStr, arg, v.schema)
	ctx := v.sc.queryCtx
	if ctx == nil {
		ctx = context.Background()
	}
	key := resourcepool.Key(v.name)
	ds, err := v.pool.dsPool.Acquire(ctx, key)
	if err != nil {
		return errors.WithStack(err)
	}
	rows, err := ds.Open(ctx, pushdown)
	if err != nil {
		v.pool.dsPool.Release(ctx, key, ds)
		return errors.WithStack(err)
	}
	c.ds = ds
	c.rows = rows
	c.rowID = 0
	c.eof = false
	return c.advance()
}

// Next 实现 sqlite3.VTabCursor.Next：推进到下一行。
// Next implements sqlite3.VTabCursor.Next: advances to the next row.
func (c *dataCursor) Next() error {
	return c.advance()
}

// advance 预读下一行：io.EOF 置 eof 与 nil 行，其余错误上抛，成功则
// rowID 递增并暂存该行。
// advance prefetches the next row: io.EOF sets eof and a nil row, other
// errors are returned, and on success the rowID is incremented and the row
// is stashed.
func (c *dataCursor) advance() error {
	row, err := c.rows.Next()
	if err != nil {
		if errors.Is(err, io.EOF) {
			c.eof = true
			c.row = nil
			return nil
		}
		return errors.WithStack(err)
	}
	c.rowID++
	c.row = row
	c.eof = false
	return nil
}

// EOF 实现 sqlite3.VTabCursor.EOF。
// EOF implements sqlite3.VTabCursor.EOF.
func (c *dataCursor) EOF() bool {
	return c.eof
}

// Column 实现 sqlite3.VTabCursor.Column：按列 ValueType 将当前行值输出为
// sqlite 结果；nil 一律输出 NULL，类型不匹配按 NULL 处理（避免 panic）。
// Column implements sqlite3.VTabCursor.Column: it outputs the current row
// value as an sqlite result per the column ValueType; nil always outputs
// NULL, and type mismatches output NULL as well (avoiding panics).
func (c *dataCursor) Column(ctx sqlite3.Context, n int) error {
	var value any
	if n >= 0 && n < len(c.row) {
		value = c.row[n]
	}
	if value == nil {
		ctx.ResultNull()
		return nil
	}
	if n < 0 || n >= len(c.vtab.schema.Columns) {
		ctx.ResultNull()
		return nil
	}
	switch c.vtab.schema.Columns[n].ValueType {
	case ValueTypeString:
		if s, ok := value.(string); ok {
			ctx.ResultText(s)
		} else {
			ctx.ResultNull()
		}
	case ValueTypeInt64:
		if i, ok := value.(int64); ok {
			ctx.ResultInt64(i)
		} else if i, ok := value.(int); ok {
			ctx.ResultInt64(int64(i))
		} else {
			ctx.ResultNull()
		}
	case ValueTypeFloat64:
		if f, ok := value.(float64); ok {
			ctx.ResultFloat(f)
		} else {
			ctx.ResultNull()
		}
	case ValueTypeBool:
		if b, ok := value.(bool); ok {
			ctx.ResultBool(b)
		} else {
			ctx.ResultNull()
		}
	case ValueTypeBlob:
		if b, ok := value.([]byte); ok {
			ctx.ResultBlob(b)
		} else {
			ctx.ResultNull()
		}
	default:
		switch v := value.(type) {
		case string:
			ctx.ResultText(v)
		case int64:
			ctx.ResultInt64(v)
		case int:
			ctx.ResultInt(v)
		case float64:
			ctx.ResultFloat(v)
		case bool:
			ctx.ResultBool(v)
		case []byte:
			ctx.ResultBlob(v)
		default:
			ctx.ResultNull()
		}
	}
	return nil
}

// RowID 实现 sqlite3.VTabCursor.RowID。
// RowID implements sqlite3.VTabCursor.RowID.
func (c *dataCursor) RowID() (int64, error) {
	return c.rowID, nil
}

// Close 实现 io.Closer（幂等）：先关闭行游标，再归还借出的数据源实例；
// 归还前校验注册表版本，注册表已变更（覆盖/注销/重注册）时旧实例直接
// 失效销毁，避免过期实例回池后被后续查询复用。由 sqlite 销毁游标时自动调用。
// Close implements io.Closer (idempotent): it closes the row cursor first, then
// returns the borrowed datasource instance. Before returning, it verifies the
// registry version: after the registry changed (overwrite/unregister/
// re-register) the stale instance is invalidated and destroyed instead of being
// pooled for reuse. It is invoked automatically by sqlite when the cursor is
// destroyed.
func (c *dataCursor) Close() error {
	if c.closed {
		return nil
	}
	c.closed = true
	var closeErr error
	if c.rows != nil {
		closeErr = c.rows.Close()
	}
	if c.ds != nil {
		ctx := c.vtab.sc.queryCtx
		if ctx == nil {
			ctx = context.Background()
		}
		key := resourcepool.Key(c.vtab.name)
		if c.vtab.pool.isCurrentVersion(c.vtab.name, c.vtab.version) {
			c.vtab.pool.dsPool.Release(ctx, key, c.ds)
		} else {
			c.vtab.pool.dsPool.Invalidate(ctx, key, c.ds)
		}
		c.ds = nil
	}
	return closeErr
}

// decodePlan 将 BestIndex 生成的 "column:op" 计划解码为下推约束列表。
// 计划项即 ArgvIndex 顺序（逐项递增），sqlite 按 ArgvIndex-1 定位参数，
// 故第 k 个计划项对应 arg[k]；ISNULL/ISNOTNULL 无右操作数，不读取参数
// （其参数槽位为 NULL）。
// decodePlan decodes the "column:op" plan produced by BestIndex into a list
// of pushdown constraints. The plan items are in ArgvIndex order (incrementing
// one by one) and sqlite places arguments at ArgvIndex-1, so the k-th plan
// item corresponds to arg[k]; ISNULL/ISNOTNULL have no right-hand operand and
// read no argument (their argument slot is NULL).
func decodePlan(idxStr string, arg []sqlite3.Value, schema Schema) []Constraint {
	if idxStr == "" {
		return nil
	}
	pushdown := make([]Constraint, 0, 4)
	for k, item := range strings.Split(idxStr, ",") {
		colStr, opStr, ok := strings.Cut(item, ":")
		if !ok {
			continue
		}
		col, err1 := strconv.Atoi(colStr)
		opVal, err2 := strconv.Atoi(opStr)
		if err1 != nil || err2 != nil {
			continue
		}
		op := Op(opVal)
		cst := Constraint{Column: col, Op: op}
		if op != OpISNULL && op != OpISNOTNULL && k < len(arg) {
			cst.Value = bindValueToGo(arg[k], columnValueType(schema, col))
		}
		pushdown = append(pushdown, cst)
	}
	return pushdown
}

// columnValueType 返回 schema 中指定列的类型；越界按 ValueTypeAny 处理。
// columnValueType returns the type of a column in the schema; out-of-range
// means ValueTypeAny.
func columnValueType(schema Schema, col int) ValueType {
	if col >= 0 && col < len(schema.Columns) {
		return schema.Columns[col].ValueType
	}
	return ValueTypeAny
}

// bindValueToGo 将 sqlite 绑定值按列 ValueType 转换为 Go 值；NULL 一律
// 转为 nil，Any 按 sqlite 实际类型分派到 int64/float64/string/[]byte/nil。
// bindValueToGo converts an sqlite bound value to a Go value per the column
// ValueType; NULL always becomes nil, and Any dispatches by the sqlite type
// to int64/float64/string/[]byte/nil.
func bindValueToGo(v sqlite3.Value, t ValueType) any {
	if v.Type() == sqlite3.NULL {
		return nil
	}
	switch t {
	case ValueTypeString:
		return v.Text()
	case ValueTypeInt64:
		return v.Int64()
	case ValueTypeFloat64:
		return v.Float()
	case ValueTypeBool:
		return v.Bool()
	case ValueTypeBlob:
		return v.Blob(nil)
	default:
		switch v.Type() {
		case sqlite3.INTEGER:
			return v.Int64()
		case sqlite3.FLOAT:
			return v.Float()
		case sqlite3.TEXT:
			return v.Text()
		case sqlite3.BLOB:
			return v.Blob(nil)
		default:
			return nil
		}
	}
}

// registerModule 在指定连接上注册（或覆盖）一个 vtable 模块。create 传 nil
// 即 eponymous vtable；connect 闭包捕获连接、schema 与注销标记，供
// Filter 读取查询上下文与数据源名。
// registerModule registers (or replaces) a vtable module on the given
// connection. A nil create makes the vtable eponymous; the connect closure
// captures the connection, schema and tombstone flag so Filter can read the
// query context and the datasource name.
func registerModule(name string, e *registryEntry, sc *sqliteConn) error {
	connect := func(db *sqlite3.Conn, module, schema, table string, arg ...string) (dataVTab, error) {
		if err := db.DeclareVTab(declareSQL(e.schema)); err != nil {
			return dataVTab{}, errors.WithStack(err)
		}
		return dataVTab{pool: sc.pool, name: module, schema: e.schema, sc: sc, version: e.version, tombstone: e.tombstone}, nil
	}
	if err := sqlite3.CreateModule(sc.conn, name, nil, connect); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// declareSQL 生成 vtable 的 schema 声明 SQL：列名 + 亲和性类型，Any 省略。
// declareSQL generates the vtable schema declaration SQL: column names plus
// affinity types, with Any omitted.
func declareSQL(schema Schema) string {
	var b strings.Builder
	b.WriteString("CREATE TABLE x(")
	for i, col := range schema.Columns {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(col.Name)
		if aff := affinityOf(col.ValueType); aff != "" {
			b.WriteString(" ")
			b.WriteString(aff)
		}
	}
	b.WriteString(")")
	return b.String()
}

// affinityOf 返回 ValueType 对应的 SQLite 类型亲和性；Any 返回空串。
// affinityOf returns the SQLite type affinity for a ValueType; Any yields "".
func affinityOf(t ValueType) string {
	switch t {
	case ValueTypeString:
		return "TEXT"
	case ValueTypeInt64:
		return "INTEGER"
	case ValueTypeFloat64:
		return "REAL"
	case ValueTypeBool:
		return "INTEGER"
	case ValueTypeBlob:
		return "BLOB"
	default:
		return ""
	}
}
