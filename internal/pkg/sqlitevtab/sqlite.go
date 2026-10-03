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
package sqlitevtab

import (
	// 标准库
	"context"
	"maps"
	"sort"
	"strings"
	"sync"
	"time"

	// 第三方

	"github.com/creasty/defaults"
	sqlite3 "github.com/ncruces/go-sqlite3"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/resourcepool"
)

// connPoolKey 是 sqlite 连接池的固定 Key。
const connPoolKey resourcepool.Key = "sqlitevtab:conn"

// sqlitePoolOptions 是 NewSQLitePool 的函数式选项承载结构：各字段默认值由 default 标签
// 声明，先经 defaults.Set 填充，再由调用方传入的选项覆盖。
type sqlitePoolOptions struct {
	// Connections sqlite 连接池默认上限。作为 sqlitevtab 自有选项默认值，不与 resourcepool
	// 的默认配置（NewDefaultConfig.MaxTotal）隐式绑定。
	Connections int `default:"8"`
	// MaxRows 单次查询最大返回行数。
	MaxRows int `default:"1000"`
	// QueryTimeout 查询默认超时（仅在调用方 ctx 无截止时间时生效）。
	QueryTimeout time.Duration `default:"5s"`
}

// Option 配置 SQLitePool 的函数式选项。
type Option func(*sqlitePoolOptions)

// WithConnections 设置 sqlite 连接池上限（默认 8）。
func WithConnections(n int) Option {
	return func(o *sqlitePoolOptions) { o.Connections = n }
}

// WithMaxRows 设置单次查询最大返回行数（默认 1000）。
func WithMaxRows(n int) Option {
	return func(o *sqlitePoolOptions) { o.MaxRows = n }
}

// WithQueryTimeout 设置查询默认超时（默认 5s；仅在调用方 ctx 无截止时间时生效）。
func WithQueryTimeout(d time.Duration) Option {
	return func(o *sqlitePoolOptions) { o.QueryTimeout = d }
}

// registryEntry 是注册表条目：注册后不可变，覆盖时整体替换指针。
type registryEntry struct {
	schema    Schema
	make      resourcepool.Make[Datasource]
	version   uint64
	tombstone bool
}

// SQLitePool 是 sqlite vtable 外部数据源接入的总入口：维护 sqlite 连接池、
// 数据源实例池与数据源注册表，提供注册/注销/查询/关闭能力。进程级共享，
// 可并发使用。
type SQLitePool struct {
	// connPool sqlite 连接池，按固定 Key 池化复用。
	connPool *resourcepool.Manager[*sqliteConn]
	// dsPool 数据源实例池，按数据源名池化复用。
	dsPool *resourcepool.Manager[Datasource]

	// mu 保护 registry 与 version 的读写锁。
	mu sync.RWMutex
	// registry 数据源注册表，按数据源名索引。
	registry map[string]*registryEntry
	// version 注册表版本号，每次变更递增。
	version uint64

	// maxRows 单次查询最大返回行数。
	maxRows int
	// queryTimeout 查询默认超时。
	queryTimeout time.Duration
}

// NewSQLitePool 创建 SQLitePool。连接在首次借出时经 Lifespan.Open 惰性建立，
// 数据源实例在首次被 vtable 引用时惰性构建。
func NewSQLitePool(opts ...Option) *SQLitePool {
	var options sqlitePoolOptions
	// 默认值由 default 标签声明（标签为编译期字面量，失败即为致命不变量）
	defaults.MustSet(&options)
	for _, opt := range opts {
		opt(&options)
	}
	p := &SQLitePool{
		registry:     make(map[string]*registryEntry),
		maxRows:      options.MaxRows,
		queryTimeout: options.QueryTimeout,
	}
	// 连接池与数据源池均从默认资源池配置起步：连接池仅将 MaxTotal 覆盖为
	// 连接数上限，数据源池保持默认配置（按数据源名各自独立实例池）。
	connCfg := *resourcepool.NewDefaultConfig()
	connCfg.MaxTotal = options.Connections
	p.connPool = resourcepool.NewManager(func(resourcepool.Key) (*sqliteConn, error) {
		return &sqliteConn{pool: p, modules: make(map[string]uint64)}, nil
	}, &connCfg)
	p.dsPool = resourcepool.NewManager(p.dsMake, resourcepool.NewDefaultConfig())
	return p
}

// sqliteConn 是池化的 sqlite 连接，实现 resourcepool.Lifespan：惰性建立
// 内存库、设置只读 authorizer、按需同步注册 vtable 模块，并透传查询上下文。
type sqliteConn struct {
	conn     *sqlite3.Conn
	pool     *SQLitePool
	modules  map[string]uint64
	queryCtx context.Context
}

// Open 实现 resourcepool.Lifespan.Open：打开内存库、设置只读 authorizer、
// 注册注册表中的全部模块；任一失败即关闭连接并返回错误。
func (sc *sqliteConn) Open(ctx context.Context) error {
	conn, err := sqlite3.OpenContext(ctx, ":memory:")
	if err != nil {
		return errors.WithStack(err)
	}
	if err := conn.SetAuthorizer(readOnlyAuthorizer); err != nil {
		_ = conn.Close()
		return errors.WithStack(err)
	}
	sc.conn = conn
	if err := sc.syncModules(sc.pool.registrySnapshot()); err != nil {
		_ = conn.Close()
		sc.conn = nil
		return err
	}
	return nil
}

// Validate 实现 resourcepool.Lifespan.Validate：连接句柄存在即视为可用。
func (sc *sqliteConn) Validate(ctx context.Context) bool {
	return sc.conn != nil
}

// Close 实现 resourcepool.Lifespan.Close：关闭底层 sqlite 连接（幂等）。
func (sc *sqliteConn) Close() error {
	if sc.conn == nil {
		return nil
	}
	err := sc.conn.Close()
	sc.conn = nil
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// syncModules 将注册表条目同步到本连接：仅对版本不一致的模块重新注册，
// 使运行期动态注册/覆盖/注销在连接借出时惰性生效。
func (sc *sqliteConn) syncModules(entries map[string]*registryEntry) error {
	for name, e := range entries {
		if sc.modules[name] != e.version {
			if err := registerModule(name, e, sc); err != nil {
				return errors.WithStack(err)
			}
			sc.modules[name] = e.version
		}
	}
	return nil
}

// readOnlyAuthorizer 只读授权回调：允许 SELECT/READ/FUNCTION/RECURSIVE/
// TRANSACTION，其余（INSERT/UPDATE/DELETE/CREATE/DROP/ATTACH/DETACH/
// PRAGMA/ALTER 等）一律拒绝。
func readOnlyAuthorizer(action sqlite3.AuthorizerActionCode, name3rd, name4th, schema, inner string) sqlite3.AuthorizerReturnCode {
	switch action {
	case sqlite3.AUTH_SELECT, sqlite3.AUTH_READ, sqlite3.AUTH_FUNCTION, sqlite3.AUTH_RECURSIVE, sqlite3.AUTH_TRANSACTION:
		return sqlite3.AUTH_OK
	default:
		return sqlite3.AUTH_DENY
	}
}

// RegisterDatasource 注册（或覆盖）一个数据源为可查询的 vtable。
// 先校验名称与 schema，再加写锁写入注册表；不触碰任何在途连接，变更经
// 连接借出时的模块同步惰性生效。覆盖已存在的数据源时，同步清理该数据源
// 名下的闲置实例（借出中的实例由游标归还时的版本校验失效），保证新工厂
// 在下次查询立即生效。
func (p *SQLitePool) RegisterDatasource(ctx context.Context, name string, schema Schema, make resourcepool.Make[Datasource]) error {
	if err := validateDatasourceName(name); err != nil {
		return errors.WithStack(err)
	}
	if err := validateSchema(schema); err != nil {
		return errors.WithStack(err)
	}
	p.mu.Lock()
	_, overwrite := p.registry[name]
	p.version++
	p.registry[name] = &registryEntry{
		schema:    schema,
		make:      make,
		version:   p.version,
		tombstone: false,
	}
	p.mu.Unlock()
	if overwrite {
		// 覆盖：销毁闲置实例，确保后续借出经新工厂重建
		p.dsPool.Clear(ctx, resourcepool.Key(name))
	}
	return nil
}

// UnregisterDatasource 注销一个数据源：条目标记为 tombstone 并升版本
// （保留原 schema 供占位模块声明），已注册但尚未注销时返回
// ErrDatasourceNotRegistered。
func (p *SQLitePool) UnregisterDatasource(ctx context.Context, name string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	e, ok := p.registry[name]
	if !ok {
		return errors.Newf("%w: %q", ErrDatasourceNotRegistered, name)
	}
	p.version++
	p.registry[name] = &registryEntry{
		schema:    e.schema,
		make:      e.make,
		version:   p.version,
		tombstone: true,
	}
	return nil
}

// Datasources 返回当前已注册（未注销）的数据源名，按字典序排序。
func (p *SQLitePool) Datasources() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	names := make([]string, 0, len(p.registry))
	for name, e := range p.registry {
		if !e.tombstone {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// registrySnapshot 返回注册表的读锁浅拷贝，供连接打开/查询时同步模块。
func (p *SQLitePool) registrySnapshot() map[string]*registryEntry {
	p.mu.RLock()
	defer p.mu.RUnlock()
	snap := make(map[string]*registryEntry, len(p.registry))
	maps.Copy(snap, p.registry)
	return snap
}

// isCurrentVersion 判断注册表中指定数据源的当前版本号是否与给定版本一致。
// 游标归还数据源实例时据此决定 Release（版本未变）还是 Invalidate
// （已被覆盖/注销/重注册，旧实例不得回池）。
func (p *SQLitePool) isCurrentVersion(name string, version uint64) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	e, ok := p.registry[name]
	return ok && e.version == version
}

// dsMake 是数据源实例池的 Make：查注册表并调用条目的构造器；
// 不存在或已注销时返回 ErrDatasourceNotRegistered。
func (p *SQLitePool) dsMake(key resourcepool.Key) (Datasource, error) {
	p.mu.RLock()
	e, ok := p.registry[string(key)]
	p.mu.RUnlock()
	if !ok || e.tombstone {
		return nil, errors.Newf("%w: %q", ErrDatasourceNotRegistered, key)
	}
	return e.make(key)
}

// Query 执行一条只读 SQL，返回列名与行值。；
// ctx 无截止时间时套用连接级默认超时。vtable 回调期间查询上下文经连接
// 对象透传给数据源。
func (p *SQLitePool) Query(ctx context.Context, sql string, args ...any) ([]string, [][]any, error) {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.queryTimeout)
		defer cancel()
	}
	sc, err := p.connPool.Acquire(ctx, connPoolKey)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	defer p.connPool.Release(ctx, connPoolKey, sc)

	sc.queryCtx = ctx
	defer func() { sc.queryCtx = nil }()

	if err = sc.syncModules(p.registrySnapshot()); err != nil {
		return nil, nil, err
	}

	stmt, _, err := sc.conn.Prepare(sql)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	defer stmt.Close()

	for i, arg := range args {
		if err := bindStmtValue(stmt, i+1, arg); err != nil {
			return nil, nil, err
		}
	}

	n := stmt.ColumnCount()
	cols := make([]string, n)
	seen := make(map[string]struct{}, n)
	for i := range n {
		name := stmt.ColumnName(i)
		if _, dup := seen[name]; dup {
			return nil, nil, errors.Newf("%w: %q", ErrDuplicateResultColumn, name)
		}
		seen[name] = struct{}{}
		cols[i] = name
	}

	var rows [][]any
	for stmt.Step() {
		if len(rows) >= p.maxRows {
			return nil, nil, errors.Newf("%w: row limit %d", ErrTooManyRows, p.maxRows)
		}
		row := make([]any, n)
		for i := range n {
			switch stmt.ColumnType(i) {
			case sqlite3.INTEGER:
				row[i] = stmt.ColumnInt64(i)
			case sqlite3.FLOAT:
				row[i] = stmt.ColumnFloat(i)
			case sqlite3.TEXT:
				row[i] = stmt.ColumnText(i)
			case sqlite3.BLOB:
				row[i] = append([]byte(nil), stmt.ColumnRawBlob(i)...)
			default:
				row[i] = nil
			}
		}
		rows = append(rows, row)
	}
	if err := stmt.Err(); err != nil {
		return nil, nil, rewrapVTabError(err)
	}
	return cols, rows, nil
}

// rewrapVTabError 修复 vtable 回调错误的错误链丢失：回调（Filter/Next/Close）
// 返回的错误经 sqlite C 边界会被压平为纯字符串（驱动仅保留消息，Sentinel
// 链随错误对象消亡），此处按已知哨兵的消息前缀重建包装，使 errors.Is 在
// Query 返回的错误上仍可匹配（如 ErrDatasourceNotRegistered）。
func rewrapVTabError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, ErrDatasourceNotRegistered.Error()):
		return errors.WithStack(errors.Newf("%w%s", ErrDatasourceNotRegistered,
			strings.TrimPrefix(msg, ErrDatasourceNotRegistered.Error())))
	default:
		return errors.WithStack(err)
	}
}

// bindStmtValue 将查询参数绑定到预编译语句，按 Go 类型分派；
// 不支持的类型返回 ErrUnsupportedBindValue。
func bindStmtValue(stmt *sqlite3.Stmt, param int, value any) error {
	var err error
	switch v := value.(type) {
	case nil:
		err = stmt.BindNull(param)
	case bool:
		err = stmt.BindBool(param, v)
	case int:
		err = stmt.BindInt64(param, int64(v))
	case int64:
		err = stmt.BindInt64(param, v)
	case float64:
		err = stmt.BindFloat(param, v)
	case string:
		err = stmt.BindText(param, v)
	case []byte:
		err = stmt.BindBlob(param, v)
	default:
		return errors.Newf("%w: %T", ErrUnsupportedBindValue, value)
	}
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// Close 关闭连接池与数据源池，销毁全部闲置资源（幂等，可重复调用）。
func (p *SQLitePool) Close(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	p.connPool.CloseAll(ctx)
	p.dsPool.CloseAll(ctx)
}

// Shutdown 实现 do.ShutdownerWithContextAndError，作为 DI 生命周期钩子：
// 转发至 Close 关闭连接池与数据源池（幂等）。*SQLitePool 由此自身即关闭器，
// provider 层无需再包一层 shutdowner 结构体，提升内聚性。
func (p *SQLitePool) Shutdown(ctx context.Context) error {
	p.Close(ctx)
	return nil
}

// DatasourceSchema 描述一个已注册数据源的 vtable schema 元信息。
type DatasourceSchema struct {
	// Name 数据源名（rego 规则中 sqlite.query 引用的裸名表名）。
	Name string
	// Columns 列定义（名称 + 值类型）。
	Columns []Column
}

// DatasourceSchemas 返回当前已注册（未注销）数据源的 schema 元信息，按名排序。
// 供前端 Rego 编辑器的 SQL 补全（表名/列名）与文档展示使用；schema 为注册时
// 的静态声明，不含实例状态。
func (p *SQLitePool) DatasourceSchemas() []DatasourceSchema {
	p.mu.RLock()
	defer p.mu.RUnlock()
	names := make([]string, 0, len(p.registry))
	for name, e := range p.registry {
		if !e.tombstone {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	out := make([]DatasourceSchema, 0, len(names))
	for _, name := range names {
		e := p.registry[name]
		out = append(out, DatasourceSchema{Name: name, Columns: e.schema.Columns})
	}
	return out
}
