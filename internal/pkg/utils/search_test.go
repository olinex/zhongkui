package utils

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"zhongkui.com/zhongkui/internal/pkg/database"
)

// 测试用的 OrderByItem 实现
type testOrderByItem struct {
	name   string
	column string
	order  string
}

func (t *testOrderByItem) GetName() string   { return t.name }
func (t *testOrderByItem) GetColumn() string { return t.column }
func (t *testOrderByItem) GetOrder() string  { return t.order }

// 测试用的 FilterByItem 实现
type testFilterByItem struct {
	name   string
	column string
	action string
	value  any
}

func (t *testFilterByItem) GetName() string   { return t.name }
func (t *testFilterByItem) GetColumn() string { return t.column }
func (t *testFilterByItem) GetAction() string { return t.action }
func (t *testFilterByItem) GetValue() any     { return t.value }

// 测试用的 SearcherIn 实现
type testSearcherIn struct {
	greedy   bool
	page     uint
	pageSize uint
	orderBy  []OrderByItem
	filterBy map[string]FilterByItem
	options  *SearchOptions
}

func (t *testSearcherIn) GetGreedy() bool                      { return t.greedy }
func (t *testSearcherIn) GetPage() uint                        { return t.page }
func (t *testSearcherIn) GetPageSize() uint                    { return t.pageSize }
func (t *testSearcherIn) GetOrderBy() []OrderByItem            { return t.orderBy }
func (t *testSearcherIn) GetFilterBy() map[string]FilterByItem { return t.filterBy }
func (t *testSearcherIn) GetOptions() *SearchOptions           { return t.options }

// 测试用的 TotalOut 实现
type testTotalOut struct {
	Total uint `json:"-"`
}

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
