package tagengine

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"zhongkui.com/zhongkui/internal/models/tag"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/pkg/testdb"
)

// TestTranslateExpression 表驱动验证标签组合表达式翻译为 clause 表达式树的渲染结果与参数顺序。
func TestTranslateExpression(t *testing.T) {
	Convey("Test TranslateExpression", t, func() {
		db := testdb.Core(t)
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
					So(normalizePlaceholders(predicate), ShouldEqual, tc.predicate)
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

// normalizePlaceholders 将 SQL 中的数据库占位符统一为 '?'。
// PostgreSQL 驱动的 gorm 会渲染成 $N 形式的占位符（如 $1、$5），而 SQLite/MySQL 渲染为 '?'，
// 该函数在断言前归一化，使 SQL 片段断言不受所在测试库方言影响。
func normalizePlaceholders(sql string) string {
	return regexp.MustCompile(`\$\d+`).ReplaceAllString(sql, "?")
}

// TestBuildDetectionQuery 验证 UNION ALL 查询组装与参数顺序。
func TestBuildDetectionQuery(t *testing.T) {
	Convey("Test BuildDetectionQuery", t, func() {
		db := testdb.Core(t)
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
			So(normalizePlaceholders(query), ShouldContainSubstring, "SELECT ? AS condition_id")
			So(normalizePlaceholders(query), ShouldContainSubstring, "SUM(cnt) AS total, arrayDistinct(arrayFlatten(groupArray(ids))) AS ids")
			So(query, ShouldNotContainSubstring, "UNION ALL")
			So(normalizePlaceholders(query), ShouldContainSubstring, "GROUP BY namespace, model, dimension, item HAVING SUM(cnt) > ?")
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
			So(strings.Count(normalizePlaceholders(query), "SELECT ? AS condition_id"), ShouldEqual, 2)
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
				Dimension:   "fpTcpClientSig",
				Window:      window,
				Threshold:   3,
				Expression:  "A",
			}}
			query, _, err := BuildDetectionQuery(db, conds, now)
			So(err, ShouldBeNil)
			So(normalizePlaceholders(query), ShouldContainSubstring, " AND dimension = ?")
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
			So(strings.Count(normalizePlaceholders(query), "SELECT ? AS condition_id"), ShouldEqual, 1)
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
		db := testdb.Raw(t)
		// tag_summings 表已由 raw 库迁移（001_init.sql）建好（ClickHouse SummingMergeTree，
		// tags 为 Array(String)，ids 为 Array(UUID)），此处按模型 TagSumming 写入测试数据。
		insert := func(minute time.Time, namespace, model, dimension, item string, tags []string, cnt uint, ids serializer.HexUUIDs) {
			sum := &tag.TagSumming{
				Namespace: namespace,
				Minute:    minute,
				Model:     model,
				Dimension: dimension,
				Item:      item,
				Cnt:       cnt,
				IDs:       ids,
			}
			sum.Tags = tags
			So(db.Create(sum).Error, ShouldBeNil)
		}
		var now = time.Now().UTC().Add(-6 * time.Minute).Truncate(time.Minute)
		window := serializer.DurationNS(5 * time.Minute)
		// hitModelTypeKey 命中分组使用的完整类型键（数据库存储语义），
		// 验证 toEventHit 对 Model 不做 value→key 收敛、直接存完整类型键。
		const hitModelTypeKey = "zhongkui.com/zhongkui/internal/models/network.HttpTransport"
		// 确定性 UUID：两组内 u2 重复出现，用于验证命中 IDs 的去重聚合
		u1 := serializer.MustHexUUIDv4FromString("11111111-2222-3333-4444-555555555555")
		u2 := serializer.MustHexUUIDv4FromString("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
		u3 := serializer.MustHexUUIDv4FromString("bbbbbbbb-cccc-dddd-eeee-ffffffffffff")
		// 命中组：窗口内两行合计 4 > 阈值 3，ids 为 {u1,u2} 与 {u2,u3}，去重后应得 {u1,u2,u3}
		insert(now.Add(-4*time.Minute), "default", hitModelTypeKey, "d1", "i1", []string{"A", "B"}, 2, serializer.HexUUIDs{u1, u2})
		insert(now.Add(-2*time.Minute), "default", hitModelTypeKey, "d1", "i1", []string{"A", "B"}, 2, serializer.HexUUIDs{u2, u3})
		// 未命中组：缺少标签 B
		insert(now.Add(-3*time.Minute), "default", "m2", "d2", "i2", []string{"A"}, 5, nil)
		// 窗口外数据：应被时间边界排除
		insert(now.Add(-10*time.Minute), "default", "m3", "d3", "i3", []string{"A", "B"}, 5, nil)
		// 不同命名空间：应被命名空间过滤排除
		insert(now.Add(-3*time.Minute), "other", "m4", "d4", "i4", []string{"A", "B"}, 5, nil)

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
			// Model 存完整类型键（value），不做短键收敛
			So(events[0].Hits[0].Model, ShouldEqual, hitModelTypeKey)
			So(events[0].Hits[0].Count, ShouldEqual, uint64(4))
			// IDs 去重聚合：{u1,u2} + {u2,u3} -> {u1,u2,u3}，ClickHouse 不保证数组顺序，按集合断言
			So(events[0].Hits[0].IDs, ShouldHaveLength, 3)
			So(sortedHexUUIDStrings(events[0].Hits[0].IDs), ShouldResemble, []string{
				u1.String(), u2.String(), u3.String(),
			})
		})

		Convey("命中分组无 ids 时命中 IDs 为空", func() {
			insert(now.Add(-1*time.Minute), "default", "m5", "d5", "i5", []string{"A"}, 2, nil)
			conds := []DetectionCondition{{
				RuleID:        10,
				RuleName:      "rule-a",
				MatchAll:      false,
				ConditionID:   2,
				ConditionName: "cond2",
				Namespace:     "default",
				// 限定仅命中 m5 分组：该分组种入的 ids 为 nil，验证命中 IDs 为空；
				// 不加过滤时 m1（cnt 4）与 m2（cnt 5）同样满足 has(tags,'A') 且 SUM(cnt)>1
				Model:      "m5",
				Dimension:  "d5",
				Item:       "i5",
				Window:     window,
				Threshold:  1,
				Expression: "A",
			}}
			events, err := Detect(context.Background(), db, conds, now)
			So(err, ShouldBeNil)
			So(events, ShouldHaveLength, 1)
			So(events[0].Hits, ShouldHaveLength, 1)
			So(events[0].Hits[0].Count, ShouldEqual, uint64(2))
			So(events[0].Hits[0].IDs, ShouldHaveLength, 0)
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

// sortedHexUUIDStrings 将 IDs 转为排序后的字符串切片，供集合相等断言使用
// （ClickHouse 不保证 groupArray 的数组顺序）。
func sortedHexUUIDStrings(ids []serializer.HexUUID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	sort.Strings(out)
	return out
}
