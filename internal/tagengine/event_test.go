package tagengine

import (
	"context"
	"database/sql/driver"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/go-sqlite"
	. "github.com/smartystreets/goconvey/convey"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

// registerSQLiteHasFunc 在 sqlite 驱动上注册 ClickHouse 的 has(tags, label) 标量函数，
// 使检测 SQL 可在 sqlite 上真实执行（tags 以逗号分隔的 TEXT 存储）。
// 注册对驱动全局生效且仅需一次，供 Detect 端到端测试使用。
var registerSQLiteHasFunc = sync.OnceFunc(func() {
	err := sqlite.RegisterScalarFunction("has", 2, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		tags, ok := args[0].(string)
		if !ok {
			return int64(0), nil
		}
		label, _ := args[1].(string)
		for _, t := range strings.Split(tags, ",") {
			if t == label {
				return int64(1), nil
			}
		}
		return int64(0), nil
	})
	if err != nil {
		panic(errors.WithStack(err))
	}
})

// TestTranslateExpression 表驱动验证标签组合表达式翻译为 clause 表达式树的渲染结果与参数顺序。
func TestTranslateExpression(t *testing.T) {
	Convey("Test TranslateExpression", t, func() {
		logger, err := logging.NewDefaultLogger()
		So(err, ShouldBeNil)
		db, err := database.NewDatabase(&database.Config{Driver: "sqlite", DSN: ":memory:"}, logger)
		So(err, ShouldBeNil)
		testCases := []struct {
			name      string
			expr      string
			predicate string
			args      []string
			valid     bool
		}{
			{name: "单个标签", expr: "A", predicate: "has(tags, ?)", args: []string{"A"}, valid: true},
			{name: "AND 组合", expr: "A AND B", predicate: "(has(tags, ?) AND has(tags, ?))", args: []string{"A", "B"}, valid: true},
			{name: "NOT 单标签", expr: "NOT A", predicate: "NOT has(tags, ?)", args: []string{"A"}, valid: true},
			{name: "OR 组合", expr: "A OR B", predicate: "(has(tags, ?) OR has(tags, ?))", args: []string{"A", "B"}, valid: true},
			{name: "括号与 AND", expr: "(A OR B) AND C", predicate: "((has(tags, ?) OR has(tags, ?)) AND has(tags, ?))", args: []string{"A", "B", "C"}, valid: true},
			{name: "NOT 括号", expr: "NOT (A AND B)", predicate: "NOT (has(tags, ?) AND has(tags, ?))", args: []string{"A", "B"}, valid: true},
			{name: "非法表达式", expr: "A AND", valid: false},
		}
		for _, tc := range testCases {
			Convey(tc.name, func() {
				expr, err := TranslateExpression(tc.expr)
				if tc.valid {
					So(err, ShouldBeNil)
					So(expr, ShouldNotBeNil)
					predicate, vars := renderClause(db, expr)
					So(predicate, ShouldEqual, tc.predicate)
					args := make([]any, len(tc.args))
					for i, a := range tc.args {
						args[i] = a
					}
					So(vars, ShouldResemble, args)
				} else {
					So(err, ShouldNotBeNil)
				}
			})
		}
	})
}

// renderClause 在 DryRun 会话中渲染 clause.Expression 为 SQL 片段并收集绑定参数，供测试断言使用。
func renderClause(db *gorm.DB, e clause.Expression) (string, []any) {
	stmt := db.Session(&gorm.Session{DryRun: true}).Statement
	stmt.SQL.Reset()
	stmt.Vars = nil
	e.Build(stmt)
	return stmt.SQL.String(), stmt.Vars
}

// TestBuildDetectionQuery 验证 UNION ALL 查询组装与参数顺序。
func TestBuildDetectionQuery(t *testing.T) {
	Convey("Test BuildDetectionQuery", t, func() {
		logger, err := logging.NewDefaultLogger()
		So(err, ShouldBeNil)
		db, err := database.NewDatabase(&database.Config{Driver: "sqlite", DSN: ":memory:"}, logger)
		So(err, ShouldBeNil)
		now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
		window := serializer.DurationNS(5 * time.Minute)

		Convey("单条件生成单个 SELECT 分支", func() {
			conds := []DetectionCondition{{
				ConditionID: 1,
				Namespace:   "default",
				Window:      window,
				Threshold:   3,
				Expression:  "A AND B",
			}}
			query, args, err := BuildDetectionQuery(db, conds, now)
			So(err, ShouldBeNil)
			So(query, ShouldContainSubstring, "SELECT ? AS condition_id")
			So(query, ShouldNotContainSubstring, "UNION ALL")
			So(query, ShouldContainSubstring, "GROUP BY namespace, model, dimension, item HAVING SUM(cnt) > ?")
			So(args, ShouldResemble, []any{
				uint(1),
				"default",
				database.ToUTCDatetime64(now.Add(-window.Std()), 6),
				database.ToUTCDatetime64(now, 6),
				"A",
				"B",
				3,
			})
		})

		Convey("多条件生成 UNION ALL 与多个 SELECT 分支", func() {
			conds := []DetectionCondition{
				{ConditionID: 1, Namespace: "default", Window: window, Threshold: 3, Expression: "A"},
				{ConditionID: 2, Namespace: "default", Window: window, Threshold: 5, Expression: "B"},
			}
			query, _, err := BuildDetectionQuery(db, conds, now)
			So(err, ShouldBeNil)
			So(query, ShouldContainSubstring, "UNION ALL")
			So(strings.Count(query, "SELECT ? AS condition_id"), ShouldEqual, 2)
		})

		Convey("过滤条件为空时不附加等值过滤", func() {
			conds := []DetectionCondition{{
				ConditionID: 1,
				Namespace:   "default",
				Window:      window,
				Threshold:   3,
				Expression:  "A",
			}}
			query, _, err := BuildDetectionQuery(db, conds, now)
			So(err, ShouldBeNil)
			So(query, ShouldNotContainSubstring, " AND model = ?")
			So(query, ShouldNotContainSubstring, " AND dimension = ?")
			So(query, ShouldNotContainSubstring, " AND item = ?")
		})

		Convey("设置 Dimension 后附加等值过滤", func() {
			conds := []DetectionCondition{{
				ConditionID: 1,
				Namespace:   "default",
				Dimension:   "fp_tcp_client_sig",
				Window:      window,
				Threshold:   3,
				Expression:  "A",
			}}
			query, _, err := BuildDetectionQuery(db, conds, now)
			So(err, ShouldBeNil)
			So(query, ShouldContainSubstring, " AND dimension = ?")
		})

		Convey("空条件列表返回错误", func() {
			_, _, err := BuildDetectionQuery(db, nil, now)
			So(err, ShouldNotBeNil)
		})

		Convey("非法表达式条件被跳过", func() {
			conds := []DetectionCondition{
				{ConditionID: 1, Namespace: "default", Window: window, Threshold: 3, Expression: "A"},
				{ConditionID: 2, Namespace: "default", Window: window, Threshold: 3, Expression: "A AND"},
			}
			query, _, err := BuildDetectionQuery(db, conds, now)
			So(err, ShouldBeNil)
			So(strings.Count(query, "SELECT ? AS condition_id"), ShouldEqual, 1)
		})
	})
}

// TestGroupEvents 验证命中分组到标签事件的聚合语义。
func TestGroupEvents(t *testing.T) {
	Convey("Test GroupEvents", t, func() {
		now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
		cond := func(ruleID, conditionID uint, matchAll bool, name string) DetectionCondition {
			return DetectionCondition{
				RuleID:        ruleID,
				RuleName:      "rule",
				MatchAll:      matchAll,
				ConditionID:   conditionID,
				ConditionName: name,
				Namespace:     "default",
				Threshold:     3,
				Window:        serializer.DurationNS(5 * time.Minute),
			}
		}

		Convey("any-match：任一条件命中即逐分组生成事件", func() {
			conds := []DetectionCondition{
				cond(10, 1, false, "cond1"),
				cond(10, 2, false, "cond2"),
			}
			hits := []DetectionHit{
				{ConditionID: 1, Namespace: "default", Model: "m1", Total: 4},
				{ConditionID: 1, Namespace: "default", Model: "m2", Total: 5},
			}
			events := GroupEvents(conds, hits, now)
			So(events, ShouldHaveLength, 2)
			So(events[0].MatchAll, ShouldBeFalse)
			So(events[0].Hits, ShouldHaveLength, 1)
			So(events[1].MatchAll, ShouldBeFalse)
			So(events[1].Hits, ShouldHaveLength, 1)
		})

		Convey("match_all：全部条件命中才触发一次并汇总", func() {
			conds := []DetectionCondition{
				cond(20, 1, true, "cond1"),
				cond(20, 2, true, "cond2"),
			}
			hits := []DetectionHit{
				{ConditionID: 1, Namespace: "default", Model: "m1", Total: 4},
				{ConditionID: 2, Namespace: "default", Model: "m2", Total: 5},
				{ConditionID: 2, Namespace: "default", Model: "m3", Total: 6},
			}
			events := GroupEvents(conds, hits, now)
			So(events, ShouldHaveLength, 1)
			So(events[0].MatchAll, ShouldBeTrue)
			So(events[0].Hits, ShouldHaveLength, 3)
		})

		Convey("match_all 部分命中不触发", func() {
			conds := []DetectionCondition{
				cond(30, 1, true, "cond1"),
				cond(30, 2, true, "cond2"),
			}
			hits := []DetectionHit{
				{ConditionID: 1, Namespace: "default", Model: "m1", Total: 4},
			}
			events := GroupEvents(conds, hits, now)
			So(events, ShouldBeEmpty)
		})

		Convey("多条规则互不干扰", func() {
			conds := []DetectionCondition{
				cond(10, 1, false, "cond1"),
				cond(10, 2, false, "cond2"),
				cond(20, 3, true, "cond3"),
				cond(20, 4, true, "cond4"),
			}
			hits := []DetectionHit{
				{ConditionID: 1, Namespace: "default", Model: "m1", Total: 4},
				{ConditionID: 3, Namespace: "default", Model: "m3", Total: 5},
				{ConditionID: 4, Namespace: "default", Model: "m4", Total: 6},
			}
			events := GroupEvents(conds, hits, now)
			So(events, ShouldHaveLength, 2)
			So(events[0].MatchAll, ShouldBeFalse)
			So(events[0].Hits, ShouldHaveLength, 1)
			So(events[1].MatchAll, ShouldBeTrue)
			So(events[1].Hits, ShouldHaveLength, 2)
		})
	})
}

// TestDetect 端到端验证核心识别函数：按传入条件与时间点查询聚合统计并聚合为标签事件。
func TestDetect(t *testing.T) {
	Convey("Test Detect", t, func() {
		registerSQLiteHasFunc()
		logger, err := logging.NewDefaultLogger()
		So(err, ShouldBeNil)
		db, err := database.NewDatabase(&database.Config{Driver: "sqlite", DSN: ":memory:"}, logger)
		So(err, ShouldBeNil)
		if sqlDB, err := db.DB(); err != nil {
			t.Fatal(err)
		} else {
			sqlDB.SetMaxOpenConns(1)
		}
		// 按 raw 库迁移（001_init.sql）的 tag_summings 列定义精简建表，minute 用同一格式化字符串便于 sqlite 文本比较。
		So(db.Exec(`CREATE TABLE tag_summings (
			namespace varchar(32),
			minute text,
			model varchar(255),
			dimension varchar(128),
			item varchar(255),
			tags text,
			cnt integer
		)`).Error, ShouldBeNil)
		insert := func(minute, namespace, model, dimension, item, tags string, cnt int) {
			So(db.Exec(`INSERT INTO tag_summings (namespace, minute, model, dimension, item, tags, cnt) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				namespace, minute, model, dimension, item, tags, cnt).Error, ShouldBeNil)
		}
		now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
		window := serializer.DurationNS(5 * time.Minute)
		// 命中组：窗口内两行合计 4 > 阈值 3
		insert(database.ToUTCDatetime64(now.Add(-4*time.Minute), 6), "default", "m1", "d1", "i1", "A,B", 2)
		insert(database.ToUTCDatetime64(now.Add(-2*time.Minute), 6), "default", "m1", "d1", "i1", "A,B", 2)
		// 未命中组：缺少标签 B
		insert(database.ToUTCDatetime64(now.Add(-3*time.Minute), 6), "default", "m2", "d2", "i2", "A", 5)
		// 窗口外数据：应被时间边界排除
		insert(database.ToUTCDatetime64(now.Add(-10*time.Minute), 6), "default", "m3", "d3", "i3", "A,B", 5)
		// 不同命名空间：应被命名空间过滤排除
		insert(database.ToUTCDatetime64(now.Add(-3*time.Minute), 6), "other", "m4", "d4", "i4", "A,B", 5)

		Convey("窗口内标签组合达到阈值触发事件", func() {
			conds := []DetectionCondition{{
				RuleID:        10,
				RuleName:      "rule-a",
				MatchAll:      false,
				ConditionID:   1,
				ConditionName: "cond1",
				Namespace:     "default",
				Window:        window,
				Threshold:     3,
				Expression:    "A AND B",
			}}
			events, err := Detect(context.Background(), db, conds, now)
			So(err, ShouldBeNil)
			So(events, ShouldHaveLength, 1)
			So(events[0].RuleID, ShouldEqual, uint(10))
			So(events[0].RuleName, ShouldEqual, "rule-a")
			So(events[0].MatchAll, ShouldBeFalse)
			So(events[0].DetectedAt, ShouldResemble, now)
			So(events[0].Hits, ShouldHaveLength, 1)
			So(events[0].Hits[0].ConditionID, ShouldEqual, uint(1))
			So(events[0].Hits[0].Model, ShouldEqual, "m1")
			So(events[0].Hits[0].Count, ShouldEqual, uint64(4))
		})

		Convey("空条件返回空结果", func() {
			events, err := Detect(context.Background(), db, nil, now)
			So(err, ShouldBeNil)
			So(events, ShouldBeEmpty)
		})

		Convey("全部条件非法返回空结果而非错误", func() {
			conds := []DetectionCondition{{
				Namespace:  "default",
				Window:     window,
				Threshold:  3,
				Expression: "A AND",
			}}
			events, err := Detect(context.Background(), db, conds, now)
			So(err, ShouldBeNil)
			So(events, ShouldBeEmpty)
		})
	})
}
