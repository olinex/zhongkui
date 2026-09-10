package utils

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// FilterByOptionItem 定义了枚举过滤条件的可选值结构
// Title 为展示名的 i18n 键，由前端渲染时翻译
type FilterByOptionItem struct {
	Title string `json:"title"` // 展示名 i18n 键
	Value string `json:"value"` // 枚举值（与数据库存储值一致）
}

// FilterByOption 定义了过滤选项的配置结构
// 用于描述前端可以使用的过滤条件
type FilterByOption struct {
	Name    string                `json:"name"`              // 过滤条件名称，用于显示
	Type    string                `json:"type"`              // 数据类型，如 string、number、date、enum 等
	Actions []string              `json:"actions"`           // 支持的操作列表，如 eq、neq、ctn 等
	Options []*FilterByOptionItem `json:"options,omitempty"` // 枚举字段的下拉可选值列表（仅 Type 为 enum 时提供）
}

// SearchOptions 定义了搜索选项的配置结构
// 用于描述前端可以使用的排序和过滤配置
type SearchOptions struct {
	OrderBy  []string          `json:"orderBy"`  // 可排序的字段列表
	FilterBy []*FilterByOption `json:"filterBy"` // 可过滤的字段配置列表
	Fields   []string          `json:"fields"`   // 可投影（字段选择）的字段列表
}

// OrderByItem 定义了排序项的接口
// 实现此接口的类型可以提供排序所需的字段信息
type OrderByItem interface {
	GetName() string   // 获取排序项名称
	GetColumn() string // 获取数据库列名
	GetOrder() string  // 获取排序方向（ASC/DESC）
}

// FilterByItem 定义了过滤项的接口
// 实现此接口的类型可以提供过滤所需的字段信息
type FilterByItem interface {
	GetName() string   // 获取过滤项名称
	GetColumn() string // 获取数据库列名
	GetAction() string // 获取过滤操作类型（如 eq、neq、ctn 等）
	GetValue() any     // 获取过滤值
}

// SearcherIn 定义了搜索输入参数的接口
// 实现此接口的类型可以提供搜索所需的所有参数
type SearcherIn interface {
	GetGreedy() bool                      // 获取是否使用贪婪模式（OR 连接过滤条件）
	GetPage() uint                        // 获取页码（从1开始）
	GetPageSize() uint                    // 获取每页大小
	GetOrderBy() []OrderByItem            // 获取排序项列表
	GetFilterBy() map[string]FilterByItem // 获取过滤项映射表
	GetOptions() *SearchOptions           // 获取搜索选项配置
}

// FieldProjector 可选接口：列表请求支持字段投影
// 实现此接口的类型可指定查询返回的字段列表
type FieldProjector interface {
	GetFields() []string            // 获取请求的投影字段（camelCase JSON 键，可为空）
	ResolveSelectColumns() []string // 解析为数据库列（白名单过滤、去重；fields 为空时返回 nil）
}

// ProjectorSearcher 字段投影搜索入参
// 组合 SearcherIn 与 FieldProjector，为 ProjectSearchItems 的入参契约：
// 列表接口入参必须同时具备搜索参数与字段投影能力。
type ProjectorSearcher interface {
	SearcherIn
	FieldProjector
}

// convertFilterClause 将过滤项转换为数据库查询条件
// item: 过滤项接口
// f: 数据库查询构建函数（Where 或 Or）
// 返回值：构建的数据库查询对象和可能的错误
func convertFilterClause(item FilterByItem, f func(query any, args ...any) (tx *database.DB)) (*database.DB, error) {
	column := item.GetColumn()
	action := item.GetAction()
	value := item.GetValue()

	switch action {
	case "eq": // 等于
		return f(column+" = ?", value), nil
	case "neq": // 不等于
		return f(column+" != ?", value), nil

	// 前缀匹配相关操作
	case "pfx", "npfx", "ipfx", "inpfx":
		switch v := value.(type) {
		case string:
			var clause string
			switch action {
			case "pfx": // 前缀匹配（区分大小写）
				clause = " LIKE ?"
			case "npfx": // 非前缀匹配（区分大小写）
				clause = " NOT LIKE ?"
			case "ipfx": // 前缀匹配（不区分大小写）
				clause = " ILIKE ?"
			case "inpfx": // 非前缀匹配（不区分大小写）
				clause = " NOT ILIKE ?"
			}
			return f(column+clause, v+"%"), nil
		default:
			return nil, errors.Newf("Invalid value type: %T for action: %s", value, action)
		}

	// 后缀匹配相关操作
	case "sfx", "nsfx", "isfx", "insfx":
		switch v := value.(type) {
		case string:
			var clause string
			switch action {
			case "sfx": // 后缀匹配（区分大小写）
				clause = " LIKE ?"
			case "nsfx": // 非后缀匹配（区分大小写）
				clause = " NOT LIKE ?"
			case "isfx": // 后缀匹配（不区分大小写）
				clause = " ILIKE ?"
			case "insfx": // 非后缀匹配（不区分大小写）
				clause = " NOT ILIKE ?"
			}
			return f(column+clause, "%"+v), nil
		default:
			return nil, errors.Newf("Invalid value type: %T for action: %s", value, action)
		}

	// 包含匹配相关操作
	case "ctn", "nctn", "ictn", "inctn":
		switch v := value.(type) {
		case string:
			var clause string
			switch action {
			case "ctn": // 包含匹配（区分大小写）
				clause = " LIKE ?"
			case "nctn": // 不包含匹配（区分大小写）
				clause = " NOT LIKE ?"
			case "ictn": // 包含匹配（不区分大小写）
				clause = " ILIKE ?"
			case "inctn": // 不包含匹配（不区分大小写）
				clause = " NOT ILIKE ?"
			}
			return f(column+clause, "%"+v+"%"), nil
		default:
			return nil, errors.Newf("Invalid value type: %T for action: %s", value, action)
		}

	case "nil": // 为空
		return f(column + " IS NULL"), nil
	case "nnil": // 不为空
		return f(column + " IS NOT NULL"), nil
	case "lt": // 小于
		return f(column+" < ?", value), nil
	case "lte": // 小于等于
		return f(column+" <= ?", value), nil
	case "gt": // 大于
		return f(column+" > ?", value), nil
	case "gte": // 大于等于
		return f(column+" >= ?", value), nil
	default:
		return nil, errors.Newf("Invalid action: %s", action)
	}
}

// injectMultiModerateFilterClauses 注入多个过滤条件，使用 AND 连接（保守模式）
// builder: 数据库查询构建器
// items: 过滤项映射表
// 返回值：构建的数据库查询对象和可能的错误
func injectMultiModerateFilterClauses(builder *database.DB, items map[string]FilterByItem) (*database.DB, error) {
	for _, item := range items {
		temp, err := convertFilterClause(item, builder.Where)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		builder = temp
	}
	return builder, nil
}

// injectMultiGreedyFilterClauses 注入多个过滤条件，使用 OR 连接（贪婪模式）
// builder: 数据库查询构建器
// items: 过滤项映射表
// 返回值：构建的数据库查询对象和可能的错误
func injectMultiGreedyFilterClauses(builder *database.DB, items map[string]FilterByItem) (*database.DB, error) {
	for _, item := range items {
		temp, err := convertFilterClause(item, builder.Or)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		builder = temp
	}
	return builder, nil
}

// injectOrderClauses 注入排序条件
// db: 数据库查询构建器
// items: 排序项列表
// 返回值：构建的数据库查询对象
func injectOrderClauses(db *database.DB, items []OrderByItem) *database.DB {
	temp := db
	for _, item := range items {
		column := item.GetColumn()
		order := item.GetOrder()
		temp = temp.Order(fmt.Sprintf("%s %s", column, order))
	}
	return temp
}

// resolveProjectionColumns 解析字段投影列
// searcher 未实现 FieldProjector 或投影字段为空时返回 nil（表示不执行投影）
func resolveProjectionColumns(searcher SearcherIn) []string {
	projector, ok := searcher.(FieldProjector)
	if !ok {
		return nil
	}
	return projector.ResolveSelectColumns()
}

// paginate 应用分页限制
// builder: 数据库查询构建器；searcher: 搜索输入参数接口
// 返回值：带分页限制的数据库查询构建器
func paginate(builder *database.DB, searcher SearcherIn) *database.DB {
	page := searcher.GetPage()
	pageSize := searcher.GetPageSize()
	return builder.Offset(int((page - 1) * pageSize)).Limit(int(pageSize))
}

// BuildSearchCondQuery 构建仅含过滤条件的查询前缀（无排序/分页/总数列）。
// 作为 BuildSearchQuery（合并流程）与 BuildSearchDataQuery（拆分流程）共用的共享步骤，
// 供 ClickHouse 等大宽表列表接口使用：先以 count(*) 轻量统计总数，
// 避免 COUNT(*) OVER() 窗口函数在 LIMIT 前强制物化全部命中行的所有列。
//
// 以独立 Session 隔离构建链：GORM 的 clone:0 实例上 Where 会原地追加条件，
// 若调用方复用同一 db 实例先后构建 count 与 data 两条链，条件会被重复注入。
func BuildSearchCondQuery(db *database.DB, searcher SearcherIn) (*database.DB, error) {
	builder := db.Session(&gorm.Session{})
	filterBy := searcher.GetFilterBy()
	var err error

	// 根据贪婪模式选择不同的过滤条件注入方式
	if searcher.GetGreedy() {
		builder, err = injectMultiGreedyFilterClauses(builder, filterBy)
	} else {
		builder, err = injectMultiModerateFilterClauses(builder, filterBy)
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return builder, nil
}

// BuildSearchCountQuery 构建合并搜索查询（过滤 + 字段投影 + 排序 + 分页）。
// 单查询同时返回行数据与总数列（SELECT 恒含 COUNT(*) OVER() AS total），
// 供 PostgreSQL 等无窗口函数物化问题的列表接口使用。
func BuildSearchCountQuery(db *database.DB, searcher SearcherIn) (*database.DB, error) {
	builder, err := BuildSearchCondQuery(db, searcher)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// 先注入 total 窗口列：无投影时直接返回 * + total
	builder = builder.Select("*, COUNT(*) OVER() AS total")
	// 字段投影：投影命中时替换为 投影列 + total，保持 total 契约不变（投影仅解析一次）
	if columns := resolveProjectionColumns(searcher); len(columns) > 0 {
		builder = builder.Select(append(columns, "COUNT(*) OVER() AS total"))
	}
	// 注入排序条件
	builder = injectOrderClauses(builder, searcher.GetOrderBy())
	// 添加分页限制
	return paginate(builder, searcher), nil
}

// BuildSearchDataQuery 构建数据查询（过滤 + 字段投影 + 排序 + 分页，无总数列）。
// 与 BuildSearchCountQuery 配套（拆分流程）：ORDER BY 命中 ClickHouse 排序键时可倒序读 + LIMIT 早停，
// 仅物化当前页行数据；总数由配套 count 查询提供，避免窗口函数全量物化。
func BuildSearchDataQuery(db *database.DB, searcher SearcherIn) (*database.DB, error) {
	builder, err := BuildSearchCondQuery(db, searcher)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// 字段投影：投影命中时仅查询投影列（不含 total，总数由配套 count 查询提供）
	if columns := resolveProjectionColumns(searcher); len(columns) > 0 {
		builder = builder.Select(columns)
	}
	// 注入排序条件
	builder = injectOrderClauses(builder, searcher.GetOrderBy())
	// 添加分页限制
	return paginate(builder, searcher), nil
}

// TotalOut 定义了获取总数的接口
// 实现此接口的类型可以提供查询结果的总数
type TotalOut interface {
	GetTotal() uint // 获取总数
}

// BaseItemOut 定义了基础输出项结构
// 包含所有实体共有的字段
type BaseItemOut struct {
	ID        uint      `json:"id"`        // 主键ID
	CreatedAt time.Time `json:"createdAt"` // 创建时间
	UpdatedAt time.Time `json:"updatedAt"` // 更新时间
}

// BaseSoftDeleteItemOut 定义了软删除基础输出项结构
// 在 BaseItemOut 基础上添加了删除时间字段
type BaseSoftDeleteItemOut struct {
	BaseItemOut
	DeletedAt *time.Time `json:"deletedAt"` // 删除时间（软删除时使用）
}

// BaseTotalOut 定义了基础总数输出结构
// 用于嵌入到其他输出结构中
type BaseTotalOut struct {
	Total uint `json:"-"` // 总数（不序列化到JSON）
}

// GetTotal 获取总数
// 返回值：总数
func (o *BaseTotalOut) GetTotal() uint {
	if o == nil {
		return 0
	}
	return o.Total
}

// SearchOut 定义了搜索输出结构
// M: 实现了 TotalOut 接口的类型
type SearchOut[M TotalOut] struct {
	Total uint `json:"total"` // 总记录数
	Items []M  `json:"items"` // 当前页的记录列表
}

// NewSearchOut 创建新的搜索输出对象
// items: 查询结果列表
// 返回值：搜索输出对象指针
func NewSearchOut[M TotalOut](items []M) *SearchOut[M] {
	var total uint
	if len(items) == 0 {
		total = 0
	} else {
		// 从第一个元素获取总数
		total = items[0].GetTotal()
	}
	return &SearchOut[M]{
		Total: total,
		Items: items,
	}
}

// fieldIndexCache 缓存结构体类型到 JSON 键字段索引链的映射（reflect.Type → map[string][]int）
var fieldIndexCache sync.Map

// fieldIndexMap 获取或构建结构体类型的字段索引缓存
// 首次构建后经 sync.Map 缓存，避免重复遍历结构体字段
func fieldIndexMap(t reflect.Type) map[string][]int {
	if cached, ok := fieldIndexCache.Load(t); ok {
		return cached.(map[string][]int)
	}
	index := make(map[string][]int)
	buildFieldIndex(t, nil, index)
	actual, _ := fieldIndexCache.LoadOrStore(t, index)
	return actual.(map[string][]int)
}

// buildFieldIndex 递归构建 JSON 键到字段索引链的映射
// 匿名嵌入的 struct/指针 struct 字段递归展开；外层键优先，重复键仅保留先注册的浅层路径
// 无 json tag 的字段使用字段名（与 encoding/json 默认行为一致），json:"-" 字段跳过
func buildFieldIndex(t reflect.Type, prefix []int, index map[string][]int) {
	// 第一遍：注册直接字段（最浅层优先，外层键覆盖嵌入层）
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if isEmbeddedStruct(field) {
			continue // 匿名嵌入的 struct 字段留待第二遍展开
		}
		registerFieldKey(field, i, prefix, index)
	}
	// 第二遍：递归展开匿名嵌入的 struct/指针 struct 字段（不覆盖已注册的键）
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !isEmbeddedStruct(field) {
			continue
		}
		path := make([]int, 0, len(prefix)+1)
		path = append(path, prefix...)
		path = append(path, i)
		ft := field.Type
		if ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		buildFieldIndex(ft, path, index)
	}
}

// isEmbeddedStruct 判断字段是否为匿名嵌入的 struct/指针 struct
func isEmbeddedStruct(field reflect.StructField) bool {
	if !field.Anonymous {
		return false
	}
	ft := field.Type
	if ft.Kind() == reflect.Pointer {
		ft = ft.Elem()
	}
	return ft.Kind() == reflect.Struct
}

// registerFieldKey 解析字段的 JSON 键名并注册字段索引链
// 不可导出字段与 json:"-" 字段跳过；无 json tag 时使用字段名（与 encoding/json 默认行为一致）
func registerFieldKey(field reflect.StructField, i int, prefix []int, index map[string][]int) {
	// 不可导出字段不参与投影
	if field.PkgPath != "" {
		return
	}
	path := make([]int, 0, len(prefix)+1)
	path = append(path, prefix...)
	path = append(path, i)

	// 解析 JSON 键名
	name := field.Name
	if jsonTag, ok := field.Tag.Lookup("json"); ok {
		if idx := strings.Index(jsonTag, ","); idx != -1 {
			jsonTag = jsonTag[:idx]
		}
		switch jsonTag {
		case "-": // 显式忽略
			return
		case "": // 空 tag 名使用字段名（与 encoding/json 一致）
		default:
			name = jsonTag
		}
	}
	if _, exists := index[name]; !exists {
		index[name] = path
	}
}

// jsonMarshalerType json.Marshaler 接口类型
// 供 fieldByPath 末端解引用判断：指针接收器实现 MarshalJSON 的类型
// （如 serializer.Crypto）保留指针交由 encoding/json 触发自定义序列化，
// 避免解引用后退化为默认结构体序列化（unexported 字段被跳过，输出 {}）。
var jsonMarshalerType = reflect.TypeFor[json.Marshaler]()

// fieldByPath 按字段索引链从结构体取值
// 逐层解引用指针；中途或末端为 nil 指针时返回 nil；
// 末端指针类型实现 json.Marshaler 时保留指针原样返回。
func fieldByPath(v reflect.Value, path []int) any {
	for _, idx := range path {
		if v.Kind() == reflect.Pointer {
			if v.IsNil() {
				return nil
			}
			v = v.Elem()
		}
		v = v.Field(idx)
	}
	// 末端解引用
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		if v.Type().Implements(jsonMarshalerType) {
			return v.Interface()
		}
		v = v.Elem()
	}
	return v.Interface()
}

// ProjectItemFields 按 JSON 键从列表输出项结构体中提取字段值，返回投影 map
// - 仅提取 fields 命中的 JSON 键；不强制包含 "id" 字段（前端未请求时不输出）
// - 跳过 json:"-" 字段与未知键；支持匿名嵌入结构体字段展开
// - 反射字段索引按 reflect.Type 缓存（sync.Map），避免重复遍历
func ProjectItemFields(out any, fields []string) map[string]any {
	result := make(map[string]any)
	if out == nil {
		return result
	}
	v := reflect.ValueOf(out)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return result
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return result
	}
	index := fieldIndexMap(v.Type())

	// 遍历请求字段（去重），仅放入命中的 JSON 键，未知键静默跳过
	seen := make(map[string]struct{}, len(fields))
	for _, key := range fields {
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		if path, ok := index[key]; ok {
			result[key] = fieldByPath(v, path)
		}
	}
	return result
}

// ProjectSearchItems 将行列表按请求字段投影为 map 项列表
// rows: 查询行（模型行或 Out 结构体）；searcher: 搜索入参（提供请求字段与投影白名单）
// newOut: 全字段映射函数（行 → 列表 Out，可为恒等函数）
// 请求字段为空时回退到全部白名单字段（GetOptions().Fields），
// 保证前端未指定 fields 时默认返回白名单全集，视图层无需自行判断。
func ProjectSearchItems[T any](rows []T, searcher ProjectorSearcher, newOut func(*T) any) []map[string]any {
	fields := searcher.GetFields()
	if len(fields) == 0 {
		if options := searcher.GetOptions(); options != nil {
			fields = options.Fields
		}
	}
	items := make([]map[string]any, 0, len(rows))
	for i := range rows {
		out := newOut(&rows[i])
		items = append(items, ProjectItemFields(out, fields))
	}
	return items
}

// SearchProjectedOut 定义了投影搜索输出结构（items 为字段投影 map）
type SearchProjectedOut struct {
	Total uint             `json:"total"` // 总记录数
	Items []map[string]any `json:"items"` // 当前页的记录列表（字段投影项）
}

// NewSearchProjectedOut 创建投影搜索输出对象
// total: 总记录数；items: 字段投影项列表
// 返回值：投影搜索输出对象指针
func NewSearchProjectedOut(total uint, items []map[string]any) *SearchProjectedOut {
	return &SearchProjectedOut{
		Total: total,
		Items: items,
	}
}
