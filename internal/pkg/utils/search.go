package utils

import (
	"fmt"
	"time"

	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// FilterByOption 定义了过滤选项的配置结构
// 用于描述前端可以使用的过滤条件
type FilterByOption struct {
	Name    string   `json:"name"`    // 过滤条件名称，用于显示
	Type    string   `json:"type"`    // 数据类型，如 string、number、date 等
	Actions []string `json:"actions"` // 支持的操作列表，如 eq、neq、ctn 等
}

// SearchOptions 定义了搜索选项的配置结构
// 用于描述前端可以使用的排序和过滤配置
type SearchOptions struct {
	OrderBy  []string          `json:"orderBy"`  // 可排序的字段列表
	FilterBy []*FilterByOption `json:"filterBy"` // 可过滤的字段配置列表
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

// BuildSearchQuery 构建搜索查询
// db: 数据库连接对象
// searcher: 搜索输入参数接口
// 返回值：构建的数据库查询对象和可能的错误
func BuildSearchQuery(db *database.DB, searcher SearcherIn) (builder *database.DB, err error) {
	// 添加总计数查询
	builder = db.Select("*, COUNT(*) OVER() AS total")
	filterBy := searcher.GetFilterBy()

	// 根据贪婪模式选择不同的过滤条件注入方式
	if searcher.GetGreedy() {
		builder, err = injectMultiGreedyFilterClauses(builder, filterBy)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	} else {
		builder, err = injectMultiModerateFilterClauses(builder, filterBy)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}

	// 注入排序条件
	builder = injectOrderClauses(builder, searcher.GetOrderBy())

	// 添加分页限制
	page := searcher.GetPage()
	pageSize := searcher.GetPageSize()
	return builder.Offset(int((page - 1) * pageSize)).Limit(int(pageSize)), nil
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
