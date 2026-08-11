package tag_event

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

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
		window := 5 * time.Minute

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
				database.ToUTCDatetime64(now.Add(-window), 6),
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
				Window:        5 * time.Minute,
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

// TestNewTagEventEmitTask 验证事件载荷编码与任务类型。
func TestNewTagEventEmitTask(t *testing.T) {
	Convey("Test NewTagEventEmitTask", t, func() {
		Convey("编码后的任务类型与载荷可还原原始事件", func() {
			ev := &TagEvent{
				Namespace:  "default",
				RuleID:     1,
				RuleName:   "rule-a",
				MatchAll:   true,
				DetectedAt: time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC),
				Hits: []TagEventHit{
					{ConditionID: 1, ConditionName: "cond", Model: "m", Count: 5, Threshold: 3, Window: 5 * time.Minute},
				},
			}
			task, err := NewTagEventEmitTask(ev)
			So(err, ShouldBeNil)
			So(task.Type(), ShouldEqual, TagEventEmitTask)
			var decoded TagEvent
			So(json.Unmarshal(task.Payload(), &decoded), ShouldBeNil)
			So(&decoded, ShouldResemble, ev)
		})

		Convey("nil 事件返回错误", func() {
			_, err := NewTagEventEmitTask(nil)
			So(err, ShouldNotBeNil)
		})
	})
}
