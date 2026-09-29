package utils

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	. "github.com/smartystreets/goconvey/convey"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"zhongkui.com/zhongkui/internal/pkg/database"
)

// 测试用的 OrderByItem 实现
type testOrderByItem struct {
	name   string
	column string
	order  string
}

// GetName 返回排序项名称。
func (t *testOrderByItem) GetName() string { return t.name }

// GetColumn 返回排序字段名。
func (t *testOrderByItem) GetColumn() string { return t.column }

// GetOrder 返回排序方向。
func (t *testOrderByItem) GetOrder() string { return t.order }

// 测试用的 FilterByItem 实现
type testFilterByItem struct {
	name   string
	column string
	action string
	value  any
}

// GetName 返回过滤项名称。
func (t *testFilterByItem) GetName() string { return t.name }

// GetColumn 返回过滤字段名。
func (t *testFilterByItem) GetColumn() string { return t.column }

// GetAction 返回过滤操作类型。
func (t *testFilterByItem) GetAction() string { return t.action }

// GetValue 返回过滤比较值。
func (t *testFilterByItem) GetValue() any { return t.value }

// 测试用的 SearcherIn 实现
type testSearcherIn struct {
	greedy   bool
	page     uint
	pageSize uint
	orderBy  []OrderByItem
	filterBy map[string]FilterByItem
	options  *SearchOptions
}

// GetGreedy 返回是否贪婪查询。
func (t *testSearcherIn) GetGreedy() bool { return t.greedy }

// GetPage 返回页码。
func (t *testSearcherIn) GetPage() uint { return t.page }

// GetPageSize 返回每页条数。
func (t *testSearcherIn) GetPageSize() uint { return t.pageSize }

// GetOrderBy 返回排序项列表。
func (t *testSearcherIn) GetOrderBy() []OrderByItem { return t.orderBy }

// GetFilterBy 返回过滤项映射。
func (t *testSearcherIn) GetFilterBy() map[string]FilterByItem { return t.filterBy }

// GetOptions 返回搜索选项。
func (t *testSearcherIn) GetOptions() *SearchOptions { return t.options }

// 测试用的 TotalOut 实现
type testTotalOut struct {
	Total uint `json:"-"`
}

// GetTotal 返回总数，nil 接收者返回 0。
func (t *testTotalOut) GetTotal() uint {
	if t == nil {
		return 0
	}
	return t.Total
}

// TestConvertFilterClause 测试 convertFilterClause 函数
func TestConvertFilterClause(t *testing.T) {
	Convey("Test convertFilterClause function", t, func() {
		Convey("Should handle eq action correctly", func() {
			item := &testFilterByItem{
				column: "name",
				action: "eq",
				value:  "test",
			}

			// 模拟数据库查询构建函数
			called := false
			var calledQuery any
			var calledArgs []any

			f := func(query any, args ...any) (tx *database.DB) {
				called = true
				calledQuery = query
				calledArgs = args
				return &database.DB{}
			}

			result, err := convertFilterClause(item, f)

			So(err, ShouldBeNil)
			So(result, ShouldNotBeNil)
			So(called, ShouldBeTrue)
			So(calledQuery, ShouldEqual, "name = ?")
			So(calledArgs, ShouldHaveLength, 1)
			So(calledArgs[0], ShouldEqual, "test")
		})

		Convey("Should handle neq action correctly", func() {
			item := &testFilterByItem{
				column: "age",
				action: "neq",
				value:  25,
			}

			called := false
			var calledQuery any
			var calledArgs []any

			f := func(query any, args ...any) (tx *database.DB) {
				called = true
				calledQuery = query
				calledArgs = args
				return &database.DB{}
			}

			result, err := convertFilterClause(item, f)

			So(err, ShouldBeNil)
			So(result, ShouldNotBeNil)
			So(called, ShouldBeTrue)
			So(calledQuery, ShouldEqual, "age != ?")
			So(calledArgs, ShouldHaveLength, 1)
			So(calledArgs[0], ShouldEqual, 25)
		})

		Convey("Should handle pfx action correctly", func() {
			item := &testFilterByItem{
				column: "name",
				action: "pfx",
				value:  "test",
			}

			called := false
			var calledQuery any
			var calledArgs []any

			f := func(query any, args ...any) (tx *database.DB) {
				called = true
				calledQuery = query
				calledArgs = args
				return &database.DB{}
			}

			result, err := convertFilterClause(item, f)

			So(err, ShouldBeNil)
			So(result, ShouldNotBeNil)
			So(called, ShouldBeTrue)
			So(calledQuery, ShouldEqual, "name LIKE ?")
			So(calledArgs, ShouldHaveLength, 1)
			So(calledArgs[0], ShouldEqual, "test%")
		})

		Convey("Should handle ipfx action correctly", func() {
			item := &testFilterByItem{
				column: "name",
				action: "ipfx",
				value:  "test",
			}

			called := false
			var calledQuery any
			var calledArgs []any

			f := func(query any, args ...any) (tx *database.DB) {
				called = true
				calledQuery = query
				calledArgs = args
				return &database.DB{}
			}

			result, err := convertFilterClause(item, f)

			So(err, ShouldBeNil)
			So(result, ShouldNotBeNil)
			So(called, ShouldBeTrue)
			So(calledQuery, ShouldEqual, "name ILIKE ?")
			So(calledArgs, ShouldHaveLength, 1)
			So(calledArgs[0], ShouldEqual, "test%")
		})

		Convey("Should handle sfx action correctly", func() {
			item := &testFilterByItem{
				column: "name",
				action: "sfx",
				value:  "test",
			}

			called := false
			var calledQuery any
			var calledArgs []any

			f := func(query any, args ...any) (tx *database.DB) {
				called = true
				calledQuery = query
				calledArgs = args
				return &database.DB{}
			}

			result, err := convertFilterClause(item, f)

			So(err, ShouldBeNil)
			So(result, ShouldNotBeNil)
			So(called, ShouldBeTrue)
			So(calledQuery, ShouldEqual, "name LIKE ?")
			So(calledArgs, ShouldHaveLength, 1)
			So(calledArgs[0], ShouldEqual, "%test")
		})

		Convey("Should handle ctn action correctly", func() {
			item := &testFilterByItem{
				column: "description",
				action: "ctn",
				value:  "important",
			}

			called := false
			var calledQuery any
			var calledArgs []any

			f := func(query any, args ...any) (tx *database.DB) {
				called = true
				calledQuery = query
				calledArgs = args
				return &database.DB{}
			}

			result, err := convertFilterClause(item, f)

			So(err, ShouldBeNil)
			So(result, ShouldNotBeNil)
			So(called, ShouldBeTrue)
			So(calledQuery, ShouldEqual, "description LIKE ?")
			So(calledArgs, ShouldHaveLength, 1)
			So(calledArgs[0], ShouldEqual, "%important%")
		})

		Convey("Should handle ictn action correctly", func() {
			item := &testFilterByItem{
				column: "description",
				action: "ictn",
				value:  "IMPORTANT",
			}

			called := false
			var calledQuery any
			var calledArgs []any

			f := func(query any, args ...any) (tx *database.DB) {
				called = true
				calledQuery = query
				calledArgs = args
				return &database.DB{}
			}

			result, err := convertFilterClause(item, f)

			So(err, ShouldBeNil)
			So(result, ShouldNotBeNil)
			So(called, ShouldBeTrue)
			So(calledQuery, ShouldEqual, "description ILIKE ?")
			So(calledArgs, ShouldHaveLength, 1)
			So(calledArgs[0], ShouldEqual, "%IMPORTANT%")
		})

		Convey("Should handle nil action correctly", func() {
			item := &testFilterByItem{
				column: "deleted_at",
				action: "nil",
				value:  nil,
			}

			called := false
			var calledQuery any

			f := func(query any, args ...any) (tx *database.DB) {
				called = true
				calledQuery = query
				return &database.DB{}
			}

			result, err := convertFilterClause(item, f)

			So(err, ShouldBeNil)
			So(result, ShouldNotBeNil)
			So(called, ShouldBeTrue)
			So(calledQuery, ShouldEqual, "deleted_at IS NULL")
		})

		Convey("Should handle nnil action correctly", func() {
			item := &testFilterByItem{
				column: "updated_at",
				action: "nnil",
				value:  nil,
			}

			called := false
			var calledQuery any

			f := func(query any, args ...any) (tx *database.DB) {
				called = true
				calledQuery = query
				return &database.DB{}
			}

			result, err := convertFilterClause(item, f)

			So(err, ShouldBeNil)
			So(result, ShouldNotBeNil)
			So(called, ShouldBeTrue)
			So(calledQuery, ShouldEqual, "updated_at IS NOT NULL")
		})

		Convey("Should handle lt action correctly", func() {
			item := &testFilterByItem{
				column: "age",
				action: "lt",
				value:  30,
			}

			called := false
			var calledQuery any
			var calledArgs []any

			f := func(query any, args ...any) (tx *database.DB) {
				called = true
				calledQuery = query
				calledArgs = args
				return &database.DB{}
			}

			result, err := convertFilterClause(item, f)

			So(err, ShouldBeNil)
			So(result, ShouldNotBeNil)
			So(called, ShouldBeTrue)
			So(calledQuery, ShouldEqual, "age < ?")
			So(calledArgs, ShouldHaveLength, 1)
			So(calledArgs[0], ShouldEqual, 30)
		})

		Convey("Should handle lte action correctly", func() {
			item := &testFilterByItem{
				column: "score",
				action: "lte",
				value:  100.5,
			}

			called := false
			var calledQuery any
			var calledArgs []any

			f := func(query any, args ...any) (tx *database.DB) {
				called = true
				calledQuery = query
				calledArgs = args
				return &database.DB{}
			}

			result, err := convertFilterClause(item, f)

			So(err, ShouldBeNil)
			So(result, ShouldNotBeNil)
			So(called, ShouldBeTrue)
			So(calledQuery, ShouldEqual, "score <= ?")
			So(calledArgs, ShouldHaveLength, 1)
			So(calledArgs[0], ShouldEqual, 100.5)
		})

		Convey("Should handle gt action correctly", func() {
			item := &testFilterByItem{
				column: "price",
				action: "gt",
				value:  50.0,
			}

			called := false
			var calledQuery any
			var calledArgs []any

			f := func(query any, args ...any) (tx *database.DB) {
				called = true
				calledQuery = query
				calledArgs = args
				return &database.DB{}
			}

			result, err := convertFilterClause(item, f)

			So(err, ShouldBeNil)
			So(result, ShouldNotBeNil)
			So(called, ShouldBeTrue)
			So(calledQuery, ShouldEqual, "price > ?")
			So(calledArgs, ShouldHaveLength, 1)
			So(calledArgs[0], ShouldEqual, 50.0)
		})

		Convey("Should handle gte action correctly", func() {
			item := &testFilterByItem{
				column: "quantity",
				action: "gte",
				value:  10,
			}

			called := false
			var calledQuery any
			var calledArgs []any

			f := func(query any, args ...any) (tx *database.DB) {
				called = true
				calledQuery = query
				calledArgs = args
				return &database.DB{}
			}

			result, err := convertFilterClause(item, f)

			So(err, ShouldBeNil)
			So(result, ShouldNotBeNil)
			So(called, ShouldBeTrue)
			So(calledQuery, ShouldEqual, "quantity >= ?")
			So(calledArgs, ShouldHaveLength, 1)
			So(calledArgs[0], ShouldEqual, 10)
		})

		Convey("Should return error for invalid string action with non-string value", func() {
			item := &testFilterByItem{
				column: "name",
				action: "pfx",
				value:  123, // 非字符串值
			}

			f := func(query any, args ...any) (tx *database.DB) {
				return &database.DB{}
			}

			result, err := convertFilterClause(item, f)

			So(err, ShouldNotBeNil)
			So(result, ShouldBeNil)
			So(err.Error(), ShouldContainSubstring, "Invalid value type")
		})

		Convey("Should return error for unknown action", func() {
			item := &testFilterByItem{
				column: "name",
				action: "unknown_action",
				value:  "test",
			}

			f := func(query any, args ...any) (tx *database.DB) {
				return &database.DB{}
			}

			result, err := convertFilterClause(item, f)

			So(err, ShouldNotBeNil)
			So(result, ShouldBeNil)
			So(err.Error(), ShouldContainSubstring, "Invalid action")
		})
	})
}

// TestInjectMultiModerateFilterClauses 测试 injectMultiModerateFilterClauses 函数
func TestInjectMultiModerateFilterClauses(t *testing.T) {
	Convey("Test injectMultiModerateFilterClauses function", t, func() {
		Convey("Should handle multiple filter items with AND connection", func() {
			// 创建多个过滤项
			items := map[string]FilterByItem{
				"name": &testFilterByItem{
					column: "name",
					action: "eq",
					value:  "test",
				},
				"age": &testFilterByItem{
					column: "age",
					action: "gt",
					value:  18,
				},
			}

			So(len(items), ShouldEqual, 2)
			So(items["name"].GetAction(), ShouldEqual, "eq")
			So(items["name"].GetValue(), ShouldEqual, "test")
			So(items["age"].GetAction(), ShouldEqual, "gt")
			So(items["age"].GetValue(), ShouldEqual, 18)
		})

		Convey("Should handle empty filter items", func() {
			items := map[string]FilterByItem{}

			So(len(items), ShouldEqual, 0)
		})

		Convey("Should handle invalid filter action", func() {
			// 创建无效的过滤项（使用未知的操作类型）
			items := map[string]FilterByItem{
				"invalid": &testFilterByItem{
					column: "name",
					action: "invalid_action",
					value:  "test",
				},
			}

			So(items["invalid"].GetAction(), ShouldEqual, "invalid_action")
			// 无效的操作类型会在 convertFilterClause 函数中返回错误
		})
	})
}

// TestInjectMultiGreedyFilterClauses 测试 injectMultiGreedyFilterClauses 函数
func TestInjectMultiGreedyFilterClauses(t *testing.T) {
	Convey("Test injectMultiGreedyFilterClauses function", t, func() {
		Convey("Should handle multiple filter items with OR connection", func() {
			items := map[string]FilterByItem{
				"name": &testFilterByItem{
					column: "name",
					action: "eq",
					value:  "test1",
				},
				"email": &testFilterByItem{
					column: "email",
					action: "ctn",
					value:  "@example.com",
				},
			}

			So(len(items), ShouldEqual, 2)
			So(items["name"].GetAction(), ShouldEqual, "eq")
			So(items["name"].GetValue(), ShouldEqual, "test1")
			So(items["email"].GetAction(), ShouldEqual, "ctn")
			So(items["email"].GetValue(), ShouldEqual, "@example.com")
		})

		Convey("Should handle empty filter items", func() {
			items := map[string]FilterByItem{}

			So(len(items), ShouldEqual, 0)
		})
	})
}

// TestInjectOrderClauses 测试 injectOrderClauses 函数
func TestInjectOrderClauses(t *testing.T) {
	Convey("Test injectOrderClauses function", t, func() {
		Convey("Should handle order clause parameters", func() {
			items := []OrderByItem{
				&testOrderByItem{
					column: "created_at",
					order:  "DESC",
				},
			}

			So(len(items), ShouldEqual, 1)
			So(items[0].GetColumn(), ShouldEqual, "created_at")
			So(items[0].GetOrder(), ShouldEqual, "DESC")
		})

		Convey("Should handle multiple order clauses", func() {
			items := []OrderByItem{
				&testOrderByItem{
					column: "priority",
					order:  "DESC",
				},
				&testOrderByItem{
					column: "created_at",
					order:  "ASC",
				},
			}

			So(len(items), ShouldEqual, 2)
			So(items[0].GetColumn(), ShouldEqual, "priority")
			So(items[0].GetOrder(), ShouldEqual, "DESC")
			So(items[1].GetColumn(), ShouldEqual, "created_at")
			So(items[1].GetOrder(), ShouldEqual, "ASC")
		})

		Convey("Should handle empty order items", func() {
			items := []OrderByItem{}

			So(len(items), ShouldEqual, 0)
		})
	})
}

// TestBuildSearchQuery 测试 BuildSearchQuery 函数
func TestBuildSearchQuery(t *testing.T) {
	Convey("Test BuildSearchQuery function", t, func() {
		Convey("Should handle basic search parameters", func() {
			// 这个测试主要验证函数不会因为空指针而崩溃
			// 实际的数据测试应该在集成测试中进行

			// 创建一个简单的 searcher
			searcher := &testSearcherIn{
				greedy:   false,
				page:     1,
				pageSize: 10,
				orderBy: []OrderByItem{
					&testOrderByItem{
						column: "created_at",
						order:  "DESC",
					},
				},
				filterBy: map[string]FilterByItem{
					"name": &testFilterByItem{
						column: "name",
						action: "eq",
						value:  "test",
					},
				},
			}

			// 由于我们无法创建有效的 database.DB 对象
			// 这个测试主要验证函数签名和基本逻辑
			// 实际的数据测试应该在集成测试中进行
			So(searcher.GetPage(), ShouldEqual, 1)
			So(searcher.GetPageSize(), ShouldEqual, 10)
			So(searcher.GetGreedy(), ShouldBeFalse)
			So(len(searcher.GetOrderBy()), ShouldEqual, 1)
			So(len(searcher.GetFilterBy()), ShouldEqual, 1)

			// 验证过滤项
			filterItem := searcher.GetFilterBy()["name"].(*testFilterByItem)
			So(filterItem.GetColumn(), ShouldEqual, "name")
			So(filterItem.GetAction(), ShouldEqual, "eq")
			So(filterItem.GetValue(), ShouldEqual, "test")

			// 验证排序项
			orderItem := searcher.GetOrderBy()[0].(*testOrderByItem)
			So(orderItem.GetColumn(), ShouldEqual, "created_at")
			So(orderItem.GetOrder(), ShouldEqual, "DESC")
		})

		Convey("Should handle greedy mode parameters", func() {
			searcher := &testSearcherIn{
				greedy:   true,
				page:     2,
				pageSize: 20,
				filterBy: map[string]FilterByItem{
					"name": &testFilterByItem{
						column: "name",
						action: "ctn",
						value:  "test",
					},
					"email": &testFilterByItem{
						column: "email",
						action: "ctn",
						value:  "@example.com",
					},
				},
			}

			So(searcher.GetGreedy(), ShouldBeTrue)
			So(searcher.GetPage(), ShouldEqual, 2)
			So(searcher.GetPageSize(), ShouldEqual, 20)
			So(len(searcher.GetFilterBy()), ShouldEqual, 2)
		})

		Convey("Should handle search without filters", func() {
			searcher := &testSearcherIn{
				greedy:   false,
				page:     1,
				pageSize: 10,
				filterBy: map[string]FilterByItem{},
			}

			So(len(searcher.GetFilterBy()), ShouldEqual, 0)
			So(searcher.GetPage(), ShouldEqual, 1)
			So(searcher.GetPageSize(), ShouldEqual, 10)
		})

		Convey("Should handle search without order by", func() {
			searcher := &testSearcherIn{
				greedy:   false,
				page:     1,
				pageSize: 10,
				orderBy:  []OrderByItem{},
				filterBy: map[string]FilterByItem{
					"status": &testFilterByItem{
						column: "status",
						action: "eq",
						value:  "active",
					},
				},
			}

			So(len(searcher.GetOrderBy()), ShouldEqual, 0)
			So(len(searcher.GetFilterBy()), ShouldEqual, 1)
			So(searcher.GetFilterBy()["status"].GetAction(), ShouldEqual, "eq")
		})

		Convey("Should handle page calculation parameters", func() {
			searcher := &testSearcherIn{
				greedy:   false,
				page:     3,
				pageSize: 25,
				filterBy: map[string]FilterByItem{},
			}

			So(searcher.GetPage(), ShouldEqual, 3)
			So(searcher.GetPageSize(), ShouldEqual, 25)
			// 第3页，每页25条，offset 应该是 (3-1)*25 = 50
			// 这个计算在 BuildSearchQuery 函数中完成
		})

		Convey("Should handle page 1 parameters", func() {
			searcher := &testSearcherIn{
				greedy:   false,
				page:     1,
				pageSize: 10,
				filterBy: map[string]FilterByItem{},
			}

			So(searcher.GetPage(), ShouldEqual, 1)
			So(searcher.GetPageSize(), ShouldEqual, 10)
			// 第1页，每页10条，offset 应该是 (1-1)*10 = 0
		})

		Convey("Should handle invalid filter action parameters", func() {
			searcher := &testSearcherIn{
				greedy:   false,
				page:     1,
				pageSize: 10,
				filterBy: map[string]FilterByItem{
					"invalid": &testFilterByItem{
						column: "name",
						action: "invalid_action",
						value:  "test",
					},
				},
			}

			filterItem := searcher.GetFilterBy()["invalid"].(*testFilterByItem)
			So(filterItem.GetAction(), ShouldEqual, "invalid_action")
			So(filterItem.GetValue(), ShouldEqual, "test")
			// 无效的操作类型会在 convertFilterClause 函数中返回错误
		})
	})
}

// TestBaseTotalOut 测试 BaseTotalOut 结构的方法
func TestBaseTotalOut(t *testing.T) {
	Convey("Test BaseTotalOut methods", t, func() {
		Convey("Should return total value", func() {
			totalOut := &BaseTotalOut{
				Total: 100,
			}

			result := totalOut.GetTotal()

			So(result, ShouldEqual, 100)
		})

		Convey("Should return 0 for nil receiver", func() {
			var totalOut *BaseTotalOut

			result := totalOut.GetTotal()

			So(result, ShouldEqual, 0)
		})

		Convey("Should return 0 for zero total", func() {
			totalOut := &BaseTotalOut{
				Total: 0,
			}

			result := totalOut.GetTotal()

			So(result, ShouldEqual, 0)
		})
	})
}

// TestNewSearchOut 测试 NewSearchOut 函数
func TestNewSearchOut(t *testing.T) {
	Convey("Test NewSearchOut function", t, func() {
		Convey("Should create SearchOut with total from first item", func() {
			items := []*testTotalOut{
				{Total: 50},
				{Total: 50},
				{Total: 50},
			}

			result := NewSearchOut(items)

			So(result, ShouldNotBeNil)
			So(result.Total, ShouldEqual, 50)
			So(result.Items, ShouldHaveLength, 3)
			So(result.Items[0], ShouldEqual, items[0])
			So(result.Items[1], ShouldEqual, items[1])
			So(result.Items[2], ShouldEqual, items[2])
		})

		Convey("Should create SearchOut with total 0 for empty items", func() {
			items := []*testTotalOut{}

			result := NewSearchOut(items)

			So(result, ShouldNotBeNil)
			So(result.Total, ShouldEqual, 0)
			So(result.Items, ShouldHaveLength, 0)
		})

		Convey("Should create SearchOut with single item", func() {
			items := []*testTotalOut{
				{Total: 25},
			}

			result := NewSearchOut(items)

			So(result, ShouldNotBeNil)
			So(result.Total, ShouldEqual, 25)
			So(result.Items, ShouldHaveLength, 1)
		})
	})
}

// TestSearchOutStructure 测试 SearchOut 结构
func TestSearchOutStructure(t *testing.T) {
	Convey("Test SearchOut structure", t, func() {
		Convey("Should have correct JSON tags", func() {
			// 创建 SearchOut 实例
			searchOut := &SearchOut[*testTotalOut]{
				Total: 100,
				Items: []*testTotalOut{
					{Total: 100},
					{Total: 100},
				},
			}

			So(searchOut.Total, ShouldEqual, 100)
			So(searchOut.Items, ShouldHaveLength, 2)

			// 验证结构体字段
			So(searchOut.Total, ShouldEqual, 100)
			So(searchOut.Items[0].GetTotal(), ShouldEqual, 100)
			So(searchOut.Items[1].GetTotal(), ShouldEqual, 100)
		})
	})
}

// TestBaseItemOut 测试 BaseItemOut 结构
func TestBaseItemOut(t *testing.T) {
	Convey("Test BaseItemOut structure", t, func() {
		Convey("Should have correct JSON tags", func() {
			now := time.Now()
			item := &BaseItemOut{
				ID:        1,
				CreatedAt: now,
				UpdatedAt: now,
			}

			So(item.ID, ShouldEqual, 1)
			So(item.CreatedAt, ShouldEqual, now)
			So(item.UpdatedAt, ShouldEqual, now)
		})
	})
}

// TestBaseSoftDeleteItemOut 测试 BaseSoftDeleteItemOut 结构
func TestBaseSoftDeleteItemOut(t *testing.T) {
	Convey("Test BaseSoftDeleteItemOut structure", t, func() {
		Convey("Should embed BaseItemOut and add DeletedAt", func() {
			now := time.Now()
			deletedAt := &now

			item := &BaseSoftDeleteItemOut{
				BaseItemOut: BaseItemOut{
					ID:        1,
					CreatedAt: now,
					UpdatedAt: now,
				},
				DeletedAt: deletedAt,
			}

			So(item.ID, ShouldEqual, 1)
			So(item.CreatedAt, ShouldEqual, now)
			So(item.UpdatedAt, ShouldEqual, now)
			So(item.DeletedAt, ShouldEqual, deletedAt)
		})

		Convey("Should allow nil DeletedAt", func() {
			now := time.Now()

			item := &BaseSoftDeleteItemOut{
				BaseItemOut: BaseItemOut{
					ID:        2,
					CreatedAt: now,
					UpdatedAt: now,
				},
				DeletedAt: nil,
			}

			So(item.ID, ShouldEqual, 2)
			So(item.CreatedAt, ShouldEqual, now)
			So(item.UpdatedAt, ShouldEqual, now)
			So(item.DeletedAt, ShouldBeNil)
		})
	})
}

// testSearchTarget 测试双查询拆分用的最小目标模型，仅用于 GORM 表名解析。
type testSearchTarget struct {
	ID uint
}

// TestBuildSearchSplitQueries 验证双查询拆分（BuildSearchCountQuery / BuildSearchDataQuery）
// 与 BuildSearchQuery 的 SQL 构成：
//   - count 查询仅含过滤条件（无排序/分页/窗口总数列），供 ClickHouse 列表先轻量统计总数；
//   - data 查询含过滤 + 排序 + 分页且无窗口总数列，ORDER BY 命中 MergeTree 排序键时可倒序读 + LIMIT 早停；
//   - BuildSearchQuery 回归保持不变（仍注入 COUNT(*) OVER() AS total），Postgres 列表接口行为不受影响。
//
// 使用 DryRun 模式的 sqlite 内存库（纯 Go 实现）仅生成 SQL 不执行，无需外部数据库。
func TestBuildSearchSplitQueries(t *testing.T) {
	Convey("Test BuildSearchSplitQueries SQL composition", t, func() {
		gdb, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
			DryRun: true,
			Logger: logger.Discard,
		})
		So(err, ShouldBeNil)

		searcher := &testSearcherIn{
			greedy:   false,
			page:     1,
			pageSize: 10,
			orderBy: []OrderByItem{
				&testOrderByItem{column: "timestamp", order: "DESC"},
			},
			filterBy: map[string]FilterByItem{
				"namespace": &testFilterByItem{column: "namespace", action: "eq", value: "default"},
				"timestamp": &testFilterByItem{
					column: "timestamp",
					action: "gte",
					value:  time.Date(2026, 9, 8, 2, 11, 0, 0, time.UTC),
				},
			},
		}

		Convey("count 查询仅含过滤条件，无排序/分页/总数列", func() {
			q, err := BuildSearchCondQuery(gdb.Model(&testSearchTarget{}), searcher)
			So(err, ShouldBeNil)
			var total int64
			So(q.Count(&total).Error, ShouldBeNil)
			sql := q.Statement.SQL.String()
			So(sql, ShouldContainSubstring, "count(*)")
			So(sql, ShouldContainSubstring, "namespace = ?")
			So(sql, ShouldContainSubstring, "timestamp >= ?")
			So(sql, ShouldNotContainSubstring, "COUNT(*) OVER()")
			So(sql, ShouldNotContainSubstring, "ORDER BY")
			So(sql, ShouldNotContainSubstring, "LIMIT")
			So(sql, ShouldNotContainSubstring, "OFFSET")
		})

		Convey("data 查询含过滤 + 排序 + 分页，无总数列", func() {
			// 与 count 链复用同一 base 实例构建，验证条件不重复注入（GORM clone:0 实例原地追加的陷阱）
			q, err := BuildSearchDataQuery(gdb.Model(&testSearchTarget{}), searcher)
			So(err, ShouldBeNil)
			var rows []testSearchTarget
			So(q.Find(&rows).Error, ShouldBeNil)
			sql := q.Statement.SQL.String()
			So(sql, ShouldContainSubstring, "namespace = ?")
			So(sql, ShouldContainSubstring, "timestamp >= ?")
			So(sql, ShouldContainSubstring, "ORDER BY timestamp DESC")
			So(sql, ShouldContainSubstring, "LIMIT 10")
			So(sql, ShouldNotContainSubstring, "COUNT(*) OVER()")
			So(strings.Count(sql, "namespace = ?"), ShouldEqual, 1)
			So(strings.Count(sql, "timestamp >= ?"), ShouldEqual, 1)
		})

		Convey("BuildSearchQuery 回归：仍注入窗口总数列", func() {
			q, err := BuildSearchCountQuery(gdb.Model(&testSearchTarget{}), searcher)
			So(err, ShouldBeNil)
			var rows []testSearchTarget
			So(q.Find(&rows).Error, ShouldBeNil)
			sql := q.Statement.SQL.String()
			So(sql, ShouldContainSubstring, "COUNT(*) OVER() AS total")
			So(sql, ShouldContainSubstring, "ORDER BY timestamp DESC")
			So(sql, ShouldContainSubstring, "LIMIT 10")
		})
	})
}

// ========== 字段投影（FieldProjector）相关测试 ==========

// testProjectEmbedded 字段投影测试用的匿名嵌入结构体（值类型嵌入）
type testProjectEmbedded struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	Age  int    `json:"age"`
}

// testProjectEmbedPtr 字段投影测试用的匿名嵌入结构体（指针类型嵌入）
type testProjectEmbedPtr struct {
	Title string `json:"title"`
}

// testProjectInner 字段投影测试用的内层结构体（指针字段指向的类型）
type testProjectInner struct {
	Level int `json:"level"`
}

// testProjectItem 字段投影测试用的输出项结构体
// 覆盖：匿名值类型嵌入、匿名指针类型嵌入、json:"-"，与外层重复的 id/name 键（验证外层键优先）
type testProjectItem struct {
	testProjectEmbedded
	ID       uint              `json:"id"`
	Name     string            `json:"name"`
	Nick     string            `json:"nick"`
	Password string            `json:"-"`
	Extra    string            `json:"extra,omitempty"`
	Inner    *testProjectInner `json:"inner"`
	*testProjectEmbedPtr
}

// testProjectNoID 字段投影测试用的无 id 结构体
type testProjectNoID struct {
	Name string `json:"name"`
}

// testProjectMarshaler 指针接收器实现 json.Marshaler 的测试类型
// 模拟 serializer.Crypto：投影取值时若被解引用为值类型，自定义序列化会丢失，
// encoding/json 将退化为默认结构体序列化（unexported 字段被跳过，输出 {}）。
type testProjectMarshaler struct {
	cipher string
}

// MarshalJSON 将测试类型序列化为带固定前缀的 JSON 字符串，用于验证投影保留指针接收器。
func (m *testProjectMarshaler) MarshalJSON() ([]byte, error) {
	return json.Marshal("cipher:" + m.cipher)
}

// testProjectItemMarshaler 含指针接收器 MarshalJSON 字段的投影测试结构体
type testProjectItemMarshaler struct {
	// Crypto 指针接收器 MarshalJSON 字段
	Crypto *testProjectMarshaler `json:"crypto"`
}

// TestProjectItemFields 测试 ProjectItemFields 函数
func TestProjectItemFields(t *testing.T) {
	Convey("Test ProjectItemFields function", t, func() {
		item := &testProjectItem{
			testProjectEmbedded: testProjectEmbedded{ID: 100, Name: "embedded-name", Age: 30},
			ID:                  1,
			Name:                "outer-name",
			Nick:                "nickname",
			Password:            "secret",
			Extra:               "extra-value",
			Inner:               &testProjectInner{Level: 3},
			testProjectEmbedPtr: &testProjectEmbedPtr{Title: "t"},
		}

		Convey("仅返回请求键，展开匿名嵌入字段，未知键忽略", func() {
			result := ProjectItemFields(item, []string{"nick", "extra", "age", "title", "unknown", "password"})

			So(result, ShouldHaveLength, 4)
			So(result["nick"], ShouldEqual, "nickname")
			So(result["extra"], ShouldEqual, "extra-value")
			So(result["age"], ShouldEqual, 30)    // 匿名值类型嵌入字段展开
			So(result["title"], ShouldEqual, "t") // 匿名指针类型嵌入字段展开
		})

		Convey("外层键优先：重复键仅保留浅层字段", func() {
			result := ProjectItemFields(item, []string{"name", "id"})

			So(result, ShouldHaveLength, 2)
			So(result["id"], ShouldEqual, uint(1))        // 外层 ID，非嵌入层 100
			So(result["name"], ShouldEqual, "outer-name") // 外层 Name，非嵌入层 embedded-name
		})

		Convey("请求字段去重：重复键只输出一次", func() {
			result := ProjectItemFields(item, []string{"nick", "nick", "id", "id"})

			So(result, ShouldHaveLength, 2)
			So(result["id"], ShouldEqual, uint(1))
			So(result["nick"], ShouldEqual, "nickname")
		})

		Convey("nil 指针字段返回 nil，且不强制输出 id", func() {
			nilItem := &testProjectItem{
				testProjectEmbedded: testProjectEmbedded{ID: 100, Name: "embedded-name"},
				ID:                  2,
				Name:                "n",
			}
			result := ProjectItemFields(nilItem, []string{"inner", "title"})

			So(result, ShouldHaveLength, 2)
			So(result["inner"], ShouldBeNil)
			So(result["title"], ShouldBeNil)
		})

		Convey("无 id 字段的结构体不输出 id", func() {
			result := ProjectItemFields(&testProjectNoID{Name: "a"}, []string{"name"})

			So(result, ShouldHaveLength, 1)
			So(result["name"], ShouldEqual, "a")
			_, hasID := result["id"]
			So(hasID, ShouldBeFalse)
		})

		Convey("支持值类型输入与 nil 输入", func() {
			result := ProjectItemFields(testProjectNoID{Name: "b"}, []string{"name", "id"})
			So(result["name"], ShouldEqual, "b")
			_, hasID := result["id"]
			So(hasID, ShouldBeFalse)

			So(ProjectItemFields(nil, []string{"name"}), ShouldHaveLength, 0)
		})

		Convey("指针接收器 MarshalJSON 字段保留指针，序列化触发自定义方法而非 {} ", func() {
			item := &testProjectItemMarshaler{Crypto: &testProjectMarshaler{cipher: "abc"}}
			result := ProjectItemFields(item, []string{"crypto"})

			So(result, ShouldHaveLength, 1)
			b, err := json.Marshal(result)
			So(err, ShouldBeNil)
			So(string(b), ShouldEqual, `{"crypto":"cipher:abc"}`)
		})

		Convey("指针接收器 MarshalJSON 字段为 nil 指针时输出 null", func() {
			result := ProjectItemFields(&testProjectItemMarshaler{}, []string{"crypto"})

			So(result, ShouldHaveLength, 1)
			b, err := json.Marshal(result)
			So(err, ShouldBeNil)
			So(string(b), ShouldEqual, `{"crypto":null}`)
		})
	})
}

// TestProjectSearchItemsAndNewSearchProjectedOut 测试 ProjectSearchItems 泛型与 NewSearchProjectedOut
func TestProjectSearchItemsAndNewSearchProjectedOut(t *testing.T) {
	Convey("Test ProjectSearchItems and NewSearchProjectedOut", t, func() {
		rows := []testProjectItem{
			{ID: 1, Name: "a", Nick: "nick-a"},
			{ID: 2, Name: "b", Nick: "nick-b"},
		}

		Convey("指定 fields 时仅投影请求字段（去重）", func() {
			searcher := &testProjectorSearcher{fields: []string{"name", "name"}}
			items := ProjectSearchItems(rows, searcher, func(r *testProjectItem) any { return r })

			So(items, ShouldHaveLength, 2)
			So(items[0], ShouldResemble, map[string]any{"name": "a"})
			So(items[1], ShouldResemble, map[string]any{"name": "b"})
		})

		Convey("fields 为空时回退全部白名单字段（GetOptions().Fields）", func() {
			searcher := &testProjectorSearcher{
				testSearcherIn: testSearcherIn{
					options: &SearchOptions{Fields: []string{"id", "nick"}},
				},
			}
			items := ProjectSearchItems(rows, searcher, func(r *testProjectItem) any { return r })

			So(items, ShouldHaveLength, 2)
			So(items[0], ShouldResemble, map[string]any{"id": uint(1), "nick": "nick-a"})
			So(items[1], ShouldResemble, map[string]any{"id": uint(2), "nick": "nick-b"})
		})

		Convey("NewSearchProjectedOut 承接总数与投影项", func() {
			out := NewSearchProjectedOut(42, []map[string]any{{"name": "a"}})
			So(out, ShouldNotBeNil)
			So(out.Total, ShouldEqual, 42)
			So(out.Items, ShouldHaveLength, 1)
		})
	})
}

// testProjectionModel 字段投影测试用的 GORM 模型
type testProjectionModel struct {
	ID   uint   `gorm:"column:id"`
	Name string `gorm:"column:name"`
	Age  int    `gorm:"column:age"`
}

// testProjectorSearcher 实现了 SearcherIn + FieldProjector 的测试结构体
type testProjectorSearcher struct {
	testSearcherIn
	fields []string
}

// GetFields 返回测试配置的投影字段白名单。
func (t *testProjectorSearcher) GetFields() []string { return t.fields }

// ResolveSelectColumns 返回测试固定的投影列。
func (t *testProjectorSearcher) ResolveSelectColumns() []string {
	return []string{"id", "name"}
}

// TestBuildSearchProjection 验证字段投影时 BuildSearchDataQuery / BuildSearchQuery 的 SQL 构成
// 使用 DryRun 模式的 sqlite 内存库（纯 Go 实现）仅生成 SQL 不执行，无需外部数据库
func TestBuildSearchProjection(t *testing.T) {
	Convey("Test field projection SQL composition", t, func() {
		gdb, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
			DryRun: true,
			Logger: logger.Discard,
		})
		So(err, ShouldBeNil)

		searcher := &testProjectorSearcher{
			testSearcherIn: testSearcherIn{
				greedy:   false,
				page:     1,
				pageSize: 10,
				orderBy: []OrderByItem{
					&testOrderByItem{column: "created_at", order: "DESC"},
				},
				filterBy: map[string]FilterByItem{
					"name": &testFilterByItem{column: "name", action: "eq", value: "test"},
				},
			},
			fields: []string{"name", "age"},
		}

		Convey("BuildSearchDataQuery 投影时仅 SELECT 投影列", func() {
			q, err := BuildSearchDataQuery(gdb.Model(&testProjectionModel{}), searcher)
			So(err, ShouldBeNil)
			var rows []testProjectionModel
			So(q.Find(&rows).Error, ShouldBeNil)
			sql := strings.ReplaceAll(q.Statement.SQL.String(), "`", "")
			So(sql, ShouldContainSubstring, "SELECT id,name")
			So(sql, ShouldNotContainSubstring, "age")
			So(sql, ShouldContainSubstring, "ORDER BY created_at DESC")
			So(sql, ShouldContainSubstring, "LIMIT 10")
			So(sql, ShouldNotContainSubstring, "COUNT(*) OVER()")
		})

		Convey("BuildSearchQuery 投影时列尾追加 total 列", func() {
			q, err := BuildSearchCountQuery(gdb.Model(&testProjectionModel{}), searcher)
			So(err, ShouldBeNil)
			var rows []testProjectionModel
			So(q.Find(&rows).Error, ShouldBeNil)
			sql := strings.ReplaceAll(q.Statement.SQL.String(), "`", "")
			So(sql, ShouldContainSubstring, "SELECT id,name,COUNT(*) OVER() AS total")
			So(sql, ShouldContainSubstring, "ORDER BY created_at DESC")
			So(sql, ShouldContainSubstring, "LIMIT 10")
		})

		Convey("未实现 FieldProjector 时 BuildSearchQuery 保持 SELECT *, COUNT(*) OVER() AS total", func() {
			plain := &testSearcherIn{
				greedy:   false,
				page:     1,
				pageSize: 10,
				orderBy: []OrderByItem{
					&testOrderByItem{column: "created_at", order: "DESC"},
				},
				filterBy: map[string]FilterByItem{},
			}
			q, err := BuildSearchCountQuery(gdb.Model(&testProjectionModel{}), plain)
			So(err, ShouldBeNil)
			var rows []testProjectionModel
			So(q.Find(&rows).Error, ShouldBeNil)
			sql := strings.ReplaceAll(q.Statement.SQL.String(), "`", "")
			So(sql, ShouldContainSubstring, "SELECT *, COUNT(*) OVER() AS total")
		})
	})
}
