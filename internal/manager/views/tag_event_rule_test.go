// Package routes 提供 HTTP 路由注册和管理功能。
// 本文件包含标签事件规则管理路由的集成测试：
// 覆盖规则 CRUD、启停、表达式语法校验、dry-run（在 raw 库的 tag_summings 上真实执行检测）、
// 版本快照/裁剪/回滚、删除级联与命名空间隔离。
package views

import (
	// stdlib
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	// third-party
	"github.com/gin-gonic/gin"
	. "github.com/smartystreets/goconvey/convey"

	// internal
	"zhongkui.com/zhongkui/internal/manager/middlewares"
	"zhongkui.com/zhongkui/internal/manager/response"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/models/tag"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/pkg/testdb"
)

// tagEventRuleValidExpr 可通过 has(tags, ?) 命中的单标签合法表达式。
const tagEventRuleValidExpr = "admin-login"

// tagEventRuleInvalidExpr 语法非法的表达式（括号未闭合），用于创建/更新/dry-run 的拒绝场景。
const tagEventRuleInvalidExpr = "admin AND ("

// tagEventRuleWindow dry-run 检测使用的默认时间窗口。
const tagEventRuleWindow = 10 * time.Minute

// tagEventRuleDryRunNow dry-run 测试的检测时间点（UTC），相对当前时刻向后偏移并分钟对齐。
// raw 库 tag_summings 表带 72 小时 TTL，种子数据 minute 须落在当前时刻附近，
// 否则插入即被 TTL 清除导致任何条件都不命中；dry-run 请求显式携带该时间，保证窗口边界确定性。
var tagEventRuleDryRunNow = time.Now().UTC().Add(-2 * time.Minute).Truncate(time.Minute)

// createTagEventRuleTestDB 返回共享 PostgreSQL 测试库（testdb.Core 迁移并清空全部业务表）。
func createTagEventRuleTestDB(t *testing.T) *database.DB {
	return testdb.Core(t)
}

// createTagEventRuleRawDB 返回共享 ClickHouse 测试库（testdb.Raw 迁移并清空全部业务表），
// 含 raw 迁移定义的 tag_summings 表，供 dry-run 接口的真实检测 SQL 执行。
func createTagEventRuleRawDB(t *testing.T) *database.DB {
	return testdb.Raw(t)
}

// registerTagEventRuleTestRoutes 注册标签事件规则管理路由（与 router.go 的注册保持一致），
// dry-run 需要 raw 库中间件注入 tag_summings 查询连接。
func registerTagEventRuleTestRoutes(r *gin.Engine, rawDB *database.DB) {
	rawDatabaseMiddleware := middlewares.RawDatabaseMiddleware(rawDB)
	admin := r.Group("/tag-event")
	{
		admin.POST("/tag-event-rule/list", TagEventRuleListGet)
		admin.OPTIONS("/tag-event-rule/list", ListMetaGinHandler[*TagEventRuleListGetSearchIn, *TagEventRuleListOut]())
		admin.POST("/tag-event-rule/create", TagEventRuleCreate)
		admin.POST("/tag-event-rule/:id/get", TagEventRuleDetailGet)
		admin.POST("/tag-event-rule/:id/update", TagEventRuleUpdate)
		admin.POST("/tag-event-rule/delete", TagEventRuleBatchDelete)
		admin.POST("/tag-event-rule/enable", TagEventRuleBatchSetEnabled(true))
		admin.POST("/tag-event-rule/disable", TagEventRuleBatchSetEnabled(false))
		admin.POST("/tag-event-rule/validate", TagEventRuleValidate)
		admin.POST("/tag-event-rule/dry-run", rawDatabaseMiddleware, TagEventRuleDryRun)
		admin.POST("/tag-event-rule/:id/version/list", TagEventRuleVersionList)
		admin.POST("/tag-event-rule/:id/version/:version/rollback", TagEventRuleVersionRollback)
		admin.POST("/record/close", TagEventRecordBatchClose)
	}
}

// setupTagEventRuleTestRouter 创建测试路由引擎，注入 i18n、日志、数据库与固定 default 命名空间。
func setupTagEventRuleTestRouter(db, rawDB *database.DB) *gin.Engine {
	return setupTagEventRuleTestRouterWithNamespace(db, rawDB, middlewares.DefaultNamespace)
}

// setupTagEventRuleTestRouterWithNamespace 创建测试路由引擎，注入指定命名空间，
// 用于验证多租户命名空间隔离。
func setupTagEventRuleTestRouterWithNamespace(db, rawDB *database.DB, namespace string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(setupI18nMiddleware())
	r.Use(middlewares.ZapLoggerMiddleware(newTagRuleTestLogger()))
	r.Use(func(c *gin.Context) {
		c.Set(middlewares.CoreDatabaseCtxName, db)
		c.Next()
	})
	// 模拟生产环境 NamespaceMiddleware：固定注入指定命名空间
	r.Use(func(c *gin.Context) {
		c.Set(middlewares.NamespaceCtxName, namespace)
		c.Next()
	})
	registerTagEventRuleTestRoutes(r, rawDB)
	return r
}

// doTagEventRuleRequest 执行一次标签事件规则 HTTP 请求并返回响应记录器。
func doTagEventRuleRequest(r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			panic("failed to marshal test request body: " + err.Error())
		}
		reader = bytes.NewReader(data)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// createTagEventRule 通过创建接口落库一条标签事件规则，返回规则 ID。
func createTagEventRule(t *testing.T, db *database.DB, r *gin.Engine, name string, matchAll, enabled bool, conds []map[string]any) uint {
	t.Helper()
	w := doTagEventRuleRequest(r, http.MethodPost, "/tag-event/tag-event-rule/create", map[string]any{
		"name":        name,
		"description": "test rule",
		"matchAll":    matchAll,
		"enabled":     enabled,
		"conditions":  conds,
	})
	So(w.Code, ShouldEqual, http.StatusOK)
	var rule tag.TagEventRule
	So(db.Where("name = ?", name).First(&rule).Error, ShouldBeNil)
	return rule.ID
}

// tagEventRuleCondition 构造一条合法条件请求体（名称固定 cond-1，窗口/阈值/启用状态固定）。
func tagEventRuleCondition(expr string) map[string]any {
	return map[string]any{
		"name":       "cond-1",
		"window":     tagEventRuleWindow,
		"threshold":  3,
		"expression": expr,
		"enabled":    true,
	}
}

// tagEventRuleIDPath 拼接标签事件规则路由路径。
func tagEventRuleIDPath(action string, id uint) string {
	return "/tag-event/tag-event-rule/" + strconv.FormatUint(uint64(id), 10) + action
}

// TestTagEventRuleCreateAndDetail 创建与详情查询全链路。
func TestTagEventRuleCreateAndDetail(t *testing.T) {
	Convey("测试标签事件规则创建与详情查询", t, func() {
		db := createTagEventRuleTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		rawDB := createTagEventRuleRawDB(t)
		r := setupTagEventRuleTestRouter(db, rawDB)

		Convey("合法规则创建成功且详情返回嵌套条件与零版本统计", func() {
			ruleID := createTagEventRule(t, db, r, "rule-a", false, true, []map[string]any{
				tagEventRuleCondition(tagEventRuleValidExpr),
			})

			w := doTagEventRuleRequest(r, http.MethodPost, tagEventRuleIDPath("/get", ruleID), nil)
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Code int                    `json:"code"`
				Data *TagEventRuleDetailOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, http.StatusOK)
			So(resp.Data, ShouldNotBeNil)
			So(resp.Data.Name, ShouldEqual, "rule-a")
			So(resp.Data.MatchAll, ShouldBeFalse)
			So(resp.Data.Enabled, ShouldBeTrue)
			So(resp.Data.Version, ShouldEqual, 0)
			So(resp.Data.VersionCount, ShouldEqual, 0)
			So(resp.Data.Conditions, ShouldHaveLength, 1)
			So(resp.Data.Conditions[0].Name, ShouldEqual, "cond-1")
			So(resp.Data.Conditions[0].Expression, ShouldEqual, tagEventRuleValidExpr)
			So(resp.Data.Conditions[0].Threshold, ShouldEqual, 3)
			So(resp.Data.Conditions[0].Window, ShouldEqual, tagEventRuleWindow)
			So(resp.Data.Conditions[0].Enabled, ShouldBeTrue)
		})

		Convey("条件名称不合法被拒绝", func() {
			w := doTagEventRuleRequest(r, http.MethodPost, "/tag-event/tag-event-rule/create", map[string]any{
				"name": "bad-cond-name",
				"conditions": []map[string]any{{
					"name":       "",
					"window":     tagEventRuleWindow,
					"threshold":  1,
					"expression": tagEventRuleValidExpr,
					"enabled":    true,
				}},
			})
			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})
	})
}

// TestTagEventRuleConditionBoundsRejected 条件窗口/阈值超出上限被拒绝。
func TestTagEventRuleConditionBoundsRejected(t *testing.T) {
	Convey("测试条件窗口与阈值上下限校验", t, func() {
		db := createTagEventRuleTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		rawDB := createTagEventRuleRawDB(t)
		r := setupTagEventRuleTestRouter(db, rawDB)

		Convey("窗口超过 7 天上限被拒绝", func() {
			w := doTagEventRuleRequest(r, http.MethodPost, "/tag-event/tag-event-rule/create", map[string]any{
				"name": "bad-window",
				"conditions": []map[string]any{{
					"name":       "cond-1",
					"window":     (8 * 24 * time.Hour).Nanoseconds(),
					"threshold":  1,
					"expression": tagEventRuleValidExpr,
					"enabled":    true,
				}},
			})
			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})

		Convey("阈值超过 1000000 上限被拒绝", func() {
			w := doTagEventRuleRequest(r, http.MethodPost, "/tag-event/tag-event-rule/create", map[string]any{
				"name": "bad-threshold",
				"conditions": []map[string]any{{
					"name":       "cond-1",
					"window":     tagEventRuleWindow,
					"threshold":  1000001,
					"expression": tagEventRuleValidExpr,
					"enabled":    true,
				}},
			})
			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})
	})
}

// TestTagEventRuleCreateInvalidExpressionRejected 非法表达式被拒绝落库。
func TestTagEventRuleCreateInvalidExpressionRejected(t *testing.T) {
	Convey("测试非法表达式的创建拒绝逻辑", t, func() {
		db := createTagEventRuleTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		rawDB := createTagEventRuleRawDB(t)
		r := setupTagEventRuleTestRouter(db, rawDB)

		Convey("表达式语法非法返回带下标与消息的错误详情且不落库", func() {
			w := doTagEventRuleRequest(r, http.MethodPost, "/tag-event/tag-event-rule/create", map[string]any{
				"name": "bad-expr",
				"conditions": []map[string]any{{
					"name":       "cond-1",
					"window":     tagEventRuleWindow,
					"threshold":  1,
					"expression": tagEventRuleInvalidExpr,
					"enabled":    true,
				}},
			})
			So(w.Code, ShouldEqual, http.StatusBadRequest)
			var resp struct {
				Code    int                           `json:"code"`
				Details []TagEventRuleValidationError `json:"details"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, response.CodeTagEventRuleInvalid)
			So(resp.Details, ShouldHaveLength, 1)
			So(resp.Details[0].Index, ShouldEqual, 0)
			So(resp.Details[0].Message, ShouldNotBeEmpty)

			var count int64
			So(db.Model(&tag.TagEventRule{}).Where("name = ?", "bad-expr").Count(&count).Error, ShouldBeNil)
			So(count, ShouldEqual, 0)
		})

		Convey("对照：合法表达式创建成功", func() {
			w := doTagEventRuleRequest(r, http.MethodPost, "/tag-event/tag-event-rule/create", map[string]any{
				"name":       "good-expr",
				"conditions": []map[string]any{tagEventRuleCondition(tagEventRuleValidExpr)},
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var count int64
			So(db.Model(&tag.TagEventRule{}).Where("name = ?", "good-expr").Count(&count).Error, ShouldBeNil)
			So(count, ShouldEqual, 1)
		})
	})
}

// TestTagEventRuleUpdateAndVersion 更新生成版本快照且内容正确。
func TestTagEventRuleUpdateAndVersion(t *testing.T) {
	Convey("测试标签事件规则更新与版本快照", t, func() {
		db := createTagEventRuleTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		rawDB := createTagEventRuleRawDB(t)
		r := setupTagEventRuleTestRouter(db, rawDB)
		ruleID := createTagEventRule(t, db, r, "rule-up", false, true, []map[string]any{
			tagEventRuleCondition(tagEventRuleValidExpr),
		})

		w := doTagEventRuleRequest(r, http.MethodPost, tagEventRuleIDPath("/update", ruleID), map[string]any{
			"description": "updated description",
			"matchAll":    true,
			"enabled":     true,
			"conditions":  []map[string]any{tagEventRuleCondition("risk"), tagEventRuleCondition("geo")},
		})
		So(w.Code, ShouldEqual, http.StatusOK)

		Convey("更新前内容被快照为版本 1 且含旧条件", func() {
			w := doTagEventRuleRequest(r, http.MethodPost, tagEventRuleIDPath("/version/list", ruleID), nil)
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data struct {
					Items []*TagEventRuleVersionOut `json:"items"`
				} `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Items, ShouldHaveLength, 1)
			So(resp.Data.Items[0].Version, ShouldEqual, 1)
			So(resp.Data.Items[0].Name, ShouldEqual, "rule-up")
			So(resp.Data.Items[0].MatchAll, ShouldBeFalse)
			So(resp.Data.Items[0].Conditions, ShouldHaveLength, 1)
			So(resp.Data.Items[0].Conditions[0].Name, ShouldEqual, "cond-1")
			So(resp.Data.Items[0].Conditions[0].Expression, ShouldEqual, tagEventRuleValidExpr)
		})

		Convey("详情返回最新版本号、版本数与新条件集合", func() {
			w := doTagEventRuleRequest(r, http.MethodPost, tagEventRuleIDPath("/get", ruleID), nil)
			var resp struct {
				Data *TagEventRuleDetailOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Description, ShouldEqual, "updated description")
			So(resp.Data.MatchAll, ShouldBeTrue)
			So(resp.Data.Version, ShouldEqual, 1)
			So(resp.Data.VersionCount, ShouldEqual, 1)
			So(resp.Data.Conditions, ShouldHaveLength, 2)
			So(resp.Data.Conditions[0].Expression, ShouldEqual, "risk")
			So(resp.Data.Conditions[1].Expression, ShouldEqual, "geo")
		})

		Convey("非法表达式更新被拒绝且不产生版本", func() {
			w := doTagEventRuleRequest(r, http.MethodPost, tagEventRuleIDPath("/update", ruleID), map[string]any{
				"conditions": []map[string]any{{
					"name":       "cond-bad",
					"window":     tagEventRuleWindow,
					"threshold":  1,
					"expression": tagEventRuleInvalidExpr,
					"enabled":    true,
				}},
			})
			So(w.Code, ShouldEqual, http.StatusBadRequest)
			var resp struct {
				Code int `json:"code"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, response.CodeTagEventRuleInvalid)

			var count int64
			So(db.Model(&tag.TagEventRuleVersion{}).Where("rule_id = ?", ruleID).Count(&count).Error, ShouldBeNil)
			So(count, ShouldEqual, 1)
		})
	})
}

// TestTagEventRulePruneVersions 历史版本超过上限时裁剪最旧版本。
func TestTagEventRulePruneVersions(t *testing.T) {
	Convey("测试历史版本裁剪", t, func() {
		db := createTagEventRuleTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		rawDB := createTagEventRuleRawDB(t)
		r := setupTagEventRuleTestRouter(db, rawDB)
		ruleID := createTagEventRule(t, db, r, "rule-prune", false, true, []map[string]any{
			tagEventRuleCondition(tagEventRuleValidExpr),
		})

		// 连续更新 32 次，产生 32 个版本，应仅保留最新 30 个
		for i := 0; i < 32; i++ {
			w := doTagEventRuleRequest(r, http.MethodPost, tagEventRuleIDPath("/update", ruleID), map[string]any{
				"conditions": []map[string]any{tagEventRuleCondition("tag-" + strconv.Itoa(i))},
			})
			So(w.Code, ShouldEqual, http.StatusOK)
		}

		var count int64
		So(db.Model(&tag.TagEventRuleVersion{}).Where("rule_id = ?", ruleID).Count(&count).Error, ShouldBeNil)
		So(count, ShouldEqual, tag.MaxTagEventRuleVersions)

		w := doTagEventRuleRequest(r, http.MethodPost, tagEventRuleIDPath("/version/list", ruleID), nil)
		var resp struct {
			Data struct {
				Items []*TagEventRuleVersionOut `json:"items"`
			} `json:"data"`
		}
		So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
		So(resp.Data.Items, ShouldHaveLength, tag.MaxTagEventRuleVersions)
		// 版本号倒序，保留的是最新 30 个（3..32），最早的 1、2 被裁剪
		So(resp.Data.Items[0].Version, ShouldEqual, 32)
		So(resp.Data.Items[len(resp.Data.Items)-1].Version, ShouldEqual, 3)
	})
}

// TestTagEventRuleRollback 回滚恢复目标版本内容，且回滚前内容成为新版本。
func TestTagEventRuleRollback(t *testing.T) {
	Convey("测试标签事件规则回滚", t, func() {
		db := createTagEventRuleTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		rawDB := createTagEventRuleRawDB(t)
		r := setupTagEventRuleTestRouter(db, rawDB)
		ruleID := createTagEventRule(t, db, r, "rule-rb", false, true, []map[string]any{
			tagEventRuleCondition(tagEventRuleValidExpr),
		})

		// 第一次更新：产生版本 1（快照创建时内容）
		w := doTagEventRuleRequest(r, http.MethodPost, tagEventRuleIDPath("/update", ruleID), map[string]any{
			"description": "v2-content",
			"matchAll":    false,
			"enabled":     true,
			"conditions":  []map[string]any{tagEventRuleCondition("risk")},
		})
		So(w.Code, ShouldEqual, http.StatusOK)
		// 第二次更新：产生版本 2（快照第一次更新后内容）
		w = doTagEventRuleRequest(r, http.MethodPost, tagEventRuleIDPath("/update", ruleID), map[string]any{
			"description": "v3-content",
			"matchAll":    false,
			"enabled":     true,
			"conditions":  []map[string]any{tagEventRuleCondition("geo")},
		})
		So(w.Code, ShouldEqual, http.StatusOK)

		Convey("回滚到版本 1 恢复原始内容且回滚前内容被快照为新版本", func() {
			w := doTagEventRuleRequest(r, http.MethodPost, tagEventRuleIDPath("/version/1/rollback", ruleID), nil)
			So(w.Code, ShouldEqual, http.StatusOK)

			// 规则头恢复为版本 1（创建时）内容
			var rule tag.TagEventRule
			So(db.First(&rule, ruleID).Error, ShouldBeNil)
			So(rule.Description, ShouldEqual, "test rule")
			So(rule.MatchAll, ShouldBeFalse)
			So(rule.Enabled, ShouldBeTrue)

			// 条件恢复为版本 1 内容
			var conds []*tag.TagEventCondition
			So(db.Where("tag_event_rule_id = ?", ruleID).Order("id ASC").Find(&conds).Error, ShouldBeNil)
			So(conds, ShouldHaveLength, 1)
			So(conds[0].Expression, ShouldEqual, tagEventRuleValidExpr)

			// 版本总数 3：v3（回滚前内容）、v2、v1
			var count int64
			So(db.Model(&tag.TagEventRuleVersion{}).Where("rule_id = ?", ruleID).Count(&count).Error, ShouldBeNil)
			So(count, ShouldEqual, 3)

			w = doTagEventRuleRequest(r, http.MethodPost, tagEventRuleIDPath("/version/list", ruleID), nil)
			var resp struct {
				Data struct {
					Items []*TagEventRuleVersionOut `json:"items"`
				} `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Items, ShouldHaveLength, 3)
			So(resp.Data.Items[0].Version, ShouldEqual, 3)
			So(resp.Data.Items[0].Description, ShouldEqual, "v3-content")
			So(resp.Data.Items[0].Conditions, ShouldHaveLength, 1)
			So(resp.Data.Items[0].Conditions[0].Expression, ShouldEqual, "geo")
			So(resp.Data.Items[1].Version, ShouldEqual, 2)
			So(resp.Data.Items[2].Version, ShouldEqual, 1)
		})

		Convey("目标版本不存在返回 404", func() {
			w := doTagEventRuleRequest(r, http.MethodPost, tagEventRuleIDPath("/version/99/rollback", ruleID), nil)
			So(w.Code, ShouldEqual, http.StatusNotFound)
		})
	})
}

// TestTagEventRuleDeleteCascade 删除规则时级联删除其全部条件与历史版本。
func TestTagEventRuleDeleteCascade(t *testing.T) {
	Convey("测试标签事件规则删除级联", t, func() {
		db := createTagEventRuleTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		rawDB := createTagEventRuleRawDB(t)
		r := setupTagEventRuleTestRouter(db, rawDB)
		ruleID := createTagEventRule(t, db, r, "rule-del", false, true, []map[string]any{
			tagEventRuleCondition(tagEventRuleValidExpr),
		})

		// 更新产生一条历史版本
		w := doTagEventRuleRequest(r, http.MethodPost, tagEventRuleIDPath("/update", ruleID), map[string]any{
			"conditions": []map[string]any{tagEventRuleCondition("risk")},
		})
		So(w.Code, ShouldEqual, http.StatusOK)

		w = doTagEventRuleRequest(r, http.MethodPost, "/tag-event/tag-event-rule/delete", map[string]any{"ids": []uint64{uint64(ruleID)}})
		So(w.Code, ShouldEqual, http.StatusOK)
		var resp struct {
			Code int            `json:"code"`
			Data BatchResultOut `json:"data"`
		}
		So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
		So(resp.Code, ShouldEqual, http.StatusOK)
		So(resp.Data.Succeeded, ShouldEqual, 1)
		So(len(resp.Data.Failed), ShouldEqual, 0)

		var ruleCount, condCount, versionCount int64
		So(db.Model(&tag.TagEventRule{}).Where("id = ?", ruleID).Count(&ruleCount).Error, ShouldBeNil)
		So(db.Model(&tag.TagEventCondition{}).Where("tag_event_rule_id = ?", ruleID).Count(&condCount).Error, ShouldBeNil)
		So(db.Model(&tag.TagEventRuleVersion{}).Where("rule_id = ?", ruleID).Count(&versionCount).Error, ShouldBeNil)
		So(ruleCount, ShouldEqual, 0)
		So(condCount, ShouldEqual, 0)
		So(versionCount, ShouldEqual, 0)

		Convey("重复删除幂等成功", func() {
			w := doTagEventRuleRequest(r, http.MethodPost, "/tag-event/tag-event-rule/delete", map[string]any{"ids": []uint64{uint64(ruleID)}})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Code int            `json:"code"`
				Data BatchResultOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, http.StatusOK)
			So(resp.Data.Succeeded, ShouldEqual, 1)
		})
	})
}

// TestTagEventRuleSetEnabled 启用/停用规则状态切换且幂等。
func TestTagEventRuleSetEnabled(t *testing.T) {
	Convey("测试标签事件规则启停", t, func() {
		db := createTagEventRuleTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		rawDB := createTagEventRuleRawDB(t)
		r := setupTagEventRuleTestRouter(db, rawDB)
		ruleID := createTagEventRule(t, db, r, "rule-io", false, true, []map[string]any{
			tagEventRuleCondition(tagEventRuleValidExpr),
		})

		w := doTagEventRuleRequest(r, http.MethodPost, "/tag-event/tag-event-rule/disable", map[string]any{"ids": []uint64{uint64(ruleID)}})
		So(w.Code, ShouldEqual, http.StatusOK)
		var resp struct {
			Code int            `json:"code"`
			Data BatchResultOut `json:"data"`
		}
		So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
		So(resp.Data.Succeeded, ShouldEqual, 1)
		var rule tag.TagEventRule
		So(db.First(&rule, ruleID).Error, ShouldBeNil)
		So(rule.Enabled, ShouldBeFalse)

		// 重复停用幂等返回成功
		w = doTagEventRuleRequest(r, http.MethodPost, "/tag-event/tag-event-rule/disable", map[string]any{"ids": []uint64{uint64(ruleID)}})
		So(w.Code, ShouldEqual, http.StatusOK)

		w = doTagEventRuleRequest(r, http.MethodPost, "/tag-event/tag-event-rule/enable", map[string]any{"ids": []uint64{uint64(ruleID)}})
		So(w.Code, ShouldEqual, http.StatusOK)
		So(db.First(&rule, ruleID).Error, ShouldBeNil)
		So(rule.Enabled, ShouldBeTrue)

		// 重复启用幂等返回成功
		w = doTagEventRuleRequest(r, http.MethodPost, "/tag-event/tag-event-rule/enable", map[string]any{"ids": []uint64{uint64(ruleID)}})
		So(w.Code, ShouldEqual, http.StatusOK)
	})
}

// TestTagEventRuleList 列表查询与分页过滤。
func TestTagEventRuleList(t *testing.T) {
	Convey("测试标签事件规则列表查询", t, func() {
		db := createTagEventRuleTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		rawDB := createTagEventRuleRawDB(t)
		r := setupTagEventRuleTestRouter(db, rawDB)
		createTagEventRule(t, db, r, "rule-a", false, true, []map[string]any{tagEventRuleCondition(tagEventRuleValidExpr)})
		createTagEventRule(t, db, r, "rule-b", true, false, []map[string]any{tagEventRuleCondition("risk")})
		createTagEventRule(t, db, r, "rule-c", false, true, []map[string]any{tagEventRuleCondition("geo")})

		Convey("按名称等值过滤", func() {
			w := doTagEventRuleRequest(r, http.MethodPost, "/tag-event/tag-event-rule/list", map[string]any{
				"filterBy": map[string]any{
					"name": map[string]any{"action": "eq", "value": "rule-a"},
				},
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *struct {
					Total uint                   `json:"total"`
					Items []*TagEventRuleListOut `json:"items"`
				} `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Total, ShouldEqual, 1)
			So(resp.Data.Items, ShouldHaveLength, 1)
			So(resp.Data.Items[0].Name, ShouldEqual, "rule-a")
		})

		Convey("按启用状态等值过滤", func() {
			w := doTagEventRuleRequest(r, http.MethodPost, "/tag-event/tag-event-rule/list", map[string]any{
				"filterBy": map[string]any{
					"enabled": map[string]any{"action": "eq", "value": true},
				},
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *struct {
					Total uint                   `json:"total"`
					Items []*TagEventRuleListOut `json:"items"`
				} `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Total, ShouldEqual, 2)
		})

		Convey("按匹配策略等值过滤", func() {
			w := doTagEventRuleRequest(r, http.MethodPost, "/tag-event/tag-event-rule/list", map[string]any{
				"filterBy": map[string]any{
					"matchAll": map[string]any{"action": "eq", "value": true},
				},
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *struct {
					Total uint                   `json:"total"`
					Items []*TagEventRuleListOut `json:"items"`
				} `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Total, ShouldEqual, 1)
			So(resp.Data.Items[0].Name, ShouldEqual, "rule-b")
		})

		Convey("默认分页返回全量列表且 total 来自 items 嵌入的 BaseTotalOut", func() {
			w := doTagEventRuleRequest(r, http.MethodPost, "/tag-event/tag-event-rule/list", map[string]any{
				"pageSize": 10,
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *struct {
					Total uint                   `json:"total"`
					Items []*TagEventRuleListOut `json:"items"`
				} `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Total, ShouldEqual, 3)
			So(resp.Data.Items, ShouldHaveLength, 3)
		})

		Convey("列表选项接口返回过滤与列元数据", func() {
			w := doTagEventRuleRequest(r, http.MethodOptions, "/tag-event/tag-event-rule/list", nil)
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *ListMeta `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(len(resp.Data.OrderBy), ShouldEqual, 2)
			So(len(resp.Data.FilterBy), ShouldBeGreaterThan, 0)
			So(len(resp.Data.Headers), ShouldBeGreaterThan, 0)
		})
	})
}

// TestTagEventRuleValidate 表达式语法校验接口返回结构化结果。
func TestTagEventRuleValidate(t *testing.T) {
	Convey("测试标签事件规则表达式校验接口", t, func() {
		db := createTagEventRuleTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		rawDB := createTagEventRuleRawDB(t)
		r := setupTagEventRuleTestRouter(db, rawDB)

		Convey("合法表达式返回 valid=true 且 refined 不含多余空格", func() {
			w := doTagEventRuleRequest(r, http.MethodPost, "/tag-event/tag-event-rule/validate", map[string]any{
				"expression": "A AND B OR NOT C",
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data TagEventRuleValidateOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Valid, ShouldBeTrue)
			So(resp.Data.Refined, ShouldEqual, "A AND B OR NOT C")
			So(resp.Data.Refined, ShouldNotContainSubstring, "  ")
			So(resp.Data.Error, ShouldBeEmpty)
		})

		Convey("非法表达式返回 valid=false 且 error 非空", func() {
			w := doTagEventRuleRequest(r, http.MethodPost, "/tag-event/tag-event-rule/validate", map[string]any{
				"expression": tagEventRuleInvalidExpr,
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data TagEventRuleValidateOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Valid, ShouldBeFalse)
			So(resp.Data.Error, ShouldNotBeEmpty)
		})
	})
}

// TestTagEventRuleDryRun dry-run 接口对候选规则在 tag_summings 上执行真实检测。
// raw 库预置数据与请求时间对齐（固定 UTC 时刻），窗口边界由 minute 文本比较保证确定性。
func TestTagEventRuleDryRun(t *testing.T) {
	Convey("测试标签事件规则 dry-run 接口", t, func() {
		db := createTagEventRuleTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		rawDB := createTagEventRuleRawDB(t)
		r := setupTagEventRuleTestRouter(db, rawDB)

		// 在 raw 库种入 tag_summings 检测数据：m1/d1/i1 在检测窗口（now-10min 内）累计命中 5 次，
		// 达到阈值 3 触发事件。testdb.Raw 每次调用都会 TRUNCATE，故需在每个叶子外层体重新种入。
		seedSumming := func(minute time.Time, model, dimension, item string, cnt uint) {
			sum := &tag.TagSumming{
				Namespace: middlewares.DefaultNamespace,
				Minute:    minute,
				Model:     model,
				Dimension: dimension,
				Item:      item,
				Cnt:       cnt,
			}
			sum.Tags = []string{tagEventRuleValidExpr}
			So(rawDB.Create(sum).Error, ShouldBeNil)
		}
		seedSumming(tagEventRuleDryRunNow.Add(-5*time.Minute), "m1", "d1", "i1", 3)
		seedSumming(tagEventRuleDryRunNow.Add(-2*time.Minute), "m1", "d1", "i1", 2)

		// dryRun 构造一次 dry-run 请求，overrides 可覆盖默认字段
		dryRun := func(conds []map[string]any, overrides map[string]any) *httptest.ResponseRecorder {
			body := map[string]any{
				"name":       "dry-run-rule",
				"matchAll":   false,
				"enabled":    true,
				"time":       tagEventRuleDryRunNow.Format(time.RFC3339),
				"conditions": conds,
			}
			for k, v := range overrides {
				body[k] = v
			}
			return doTagEventRuleRequest(r, http.MethodPost, "/tag-event/tag-event-rule/dry-run", body)
		}

		Convey("窗口内累计命中阈值触发事件且 hits 含分组详情", func() {
			w := dryRun([]map[string]any{{
				"model": "m1", "dimension": "d1", "item": "i1",
				"name": "cond-1", "window": tagEventRuleWindow, "threshold": 3,
				"expression": tagEventRuleValidExpr, "enabled": true,
			}}, nil)
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *TagEventRuleDryRunOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.RuleName, ShouldEqual, "dry-run-rule")
			So(resp.Data.DetectedAt.Equal(tagEventRuleDryRunNow), ShouldBeTrue)
			So(resp.Data.Events, ShouldHaveLength, 1)
			So(resp.Data.Events[0].Hits, ShouldHaveLength, 1)
			hit := resp.Data.Events[0].Hits[0]
			So(hit.ConditionName, ShouldEqual, "cond-1")
			So(hit.Model, ShouldEqual, "m1")
			So(hit.Count, ShouldEqual, uint64(5))
			So(hit.Count > uint64(hit.Threshold), ShouldBeTrue)
		})

		Convey("matchAll=false 时任一条件命中即触发", func() {
			w := dryRun([]map[string]any{
				{
					"model": "m1", "dimension": "d1", "item": "i1",
					"name": "cond-1", "window": tagEventRuleWindow, "threshold": 3,
					"expression": tagEventRuleValidExpr, "enabled": true,
				},
				{
					"model": "m3", "dimension": "d3", "item": "i3",
					"name": "cond-2", "window": tagEventRuleWindow, "threshold": 5,
					"expression": tagEventRuleValidExpr, "enabled": true,
				},
			}, nil)
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *TagEventRuleDryRunOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			// 仅命中条件 cond-1 产出事件
			So(resp.Data.Events, ShouldHaveLength, 1)
			So(resp.Data.Events[0].Hits, ShouldHaveLength, 1)
			So(resp.Data.Events[0].Hits[0].ConditionName, ShouldEqual, "cond-1")
		})

		Convey("阈值高于累计命中数时不触发", func() {
			w := dryRun([]map[string]any{{
				"model": "m3", "dimension": "d3", "item": "i3",
				"name": "cond-3", "window": tagEventRuleWindow, "threshold": 5,
				"expression": tagEventRuleValidExpr, "enabled": true,
			}}, nil)
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *TagEventRuleDryRunOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Events, ShouldBeEmpty)
		})

		Convey("model 与预置数据不匹配时不触发", func() {
			w := dryRun([]map[string]any{{
				"model": "m9",
				"name":  "cond-9", "window": tagEventRuleWindow, "threshold": 1,
				"expression": tagEventRuleValidExpr, "enabled": true,
			}}, nil)
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *TagEventRuleDryRunOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Events, ShouldBeEmpty)
		})

		Convey("携带非法表达式返回 400 与业务错误码", func() {
			w := dryRun([]map[string]any{{
				"name": "cond-bad", "window": tagEventRuleWindow, "threshold": 1,
				"expression": tagEventRuleInvalidExpr, "enabled": true,
			}}, nil)
			So(w.Code, ShouldEqual, http.StatusBadRequest)
			var resp struct {
				Code    int                           `json:"code"`
				Details []TagEventRuleValidationError `json:"details"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, response.CodeTagEventRuleInvalid)
			So(resp.Details, ShouldHaveLength, 1)
			So(resp.Details[0].Index, ShouldEqual, 0)
		})

		Convey("规则停用时直接返回空事件列表", func() {
			w := dryRun([]map[string]any{{
				"model": "m1", "dimension": "d1", "item": "i1",
				"name": "cond-1", "window": tagEventRuleWindow, "threshold": 3,
				"expression": tagEventRuleValidExpr, "enabled": true,
			}}, map[string]any{"enabled": false})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *TagEventRuleDryRunOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Events, ShouldBeEmpty)
		})
	})
}

// TestTagEventRuleNamespaceIsolation 规则按命名空间隔离，跨命名空间访问视为不存在。
func TestTagEventRuleNamespaceIsolation(t *testing.T) {
	Convey("测试标签事件规则命名空间隔离", t, func() {
		db := createTagEventRuleTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		rawDB := createTagEventRuleRawDB(t)
		r := setupTagEventRuleTestRouter(db, rawDB)
		ruleID := createTagEventRule(t, db, r, "ns-rule", false, true, []map[string]any{
			tagEventRuleCondition(tagEventRuleValidExpr),
		})

		// 使用其他命名空间的 router 访问 default 命名空间内的规则，均应视为不存在
		rOther := setupTagEventRuleTestRouterWithNamespace(db, rawDB, "other-ns")

		Convey("详情查询跨命名空间返回 404", func() {
			w := doTagEventRuleRequest(rOther, http.MethodPost, tagEventRuleIDPath("/get", ruleID), nil)
			So(w.Code, ShouldEqual, http.StatusNotFound)
		})

		Convey("更新跨命名空间返回 404", func() {
			w := doTagEventRuleRequest(rOther, http.MethodPost, tagEventRuleIDPath("/update", ruleID), map[string]any{
				"conditions": []map[string]any{tagEventRuleCondition("risk")},
			})
			So(w.Code, ShouldEqual, http.StatusNotFound)
		})

		Convey("版本列表跨命名空间返回 404", func() {
			w := doTagEventRuleRequest(rOther, http.MethodPost, tagEventRuleIDPath("/version/list", ruleID), nil)
			So(w.Code, ShouldEqual, http.StatusNotFound)
		})

		Convey("本命名空间内访问仍正常", func() {
			w := doTagEventRuleRequest(r, http.MethodPost, tagEventRuleIDPath("/get", ruleID), nil)
			So(w.Code, ShouldEqual, http.StatusOK)
		})
	})
}

// TestTagEventRuleSuppressWindowLifecycle 验证 suppressWindow 在创建/详情/更新/版本快照/回滚全链路保持一致。
func TestTagEventRuleSuppressWindowLifecycle(t *testing.T) {
	Convey("测试 suppressWindow 全链路一致性", t, func() {
		db := createTagEventRuleTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		rawDB := createTagEventRuleRawDB(t)
		r := setupTagEventRuleTestRouter(db, rawDB)

		Convey("创建时传 30m，详情返回 30m", func() {
			w := doTagEventRuleRequest(r, http.MethodPost, "/tag-event/tag-event-rule/create", map[string]any{
				"name":           "rule-sup-create",
				"description":    "test rule",
				"matchAll":       false,
				"enabled":        true,
				"suppressWindow": (30 * time.Minute).Nanoseconds(),
				"conditions":     []map[string]any{tagEventRuleCondition(tagEventRuleValidExpr)},
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var rule tag.TagEventRule
			So(db.Where("name = ?", "rule-sup-create").First(&rule).Error, ShouldBeNil)
			So(rule.SuppressWindow, ShouldEqual, serializer.DurationNS(30*time.Minute))

			w = doTagEventRuleRequest(r, http.MethodPost, tagEventRuleIDPath("/get", rule.ID), nil)
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *TagEventRuleDetailOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.SuppressWindow, ShouldEqual, serializer.DurationNS(30*time.Minute))
		})

		Convey("更新 suppressWindow 为新值，详情返回新值", func() {
			ruleID := createTagEventRule(t, db, r, "rule-sup-update", false, true, []map[string]any{
				tagEventRuleCondition(tagEventRuleValidExpr),
			})
			w := doTagEventRuleRequest(r, http.MethodPost, tagEventRuleIDPath("/update", ruleID), map[string]any{
				"description":    "updated",
				"matchAll":       true,
				"enabled":        true,
				"suppressWindow": (90 * time.Minute).Nanoseconds(),
				"conditions":     []map[string]any{tagEventRuleCondition(tagEventRuleValidExpr)},
			})
			So(w.Code, ShouldEqual, http.StatusOK)

			var rule tag.TagEventRule
			So(db.First(&rule, ruleID).Error, ShouldBeNil)
			So(rule.SuppressWindow, ShouldEqual, serializer.DurationNS(90*time.Minute))

			w = doTagEventRuleRequest(r, http.MethodPost, tagEventRuleIDPath("/get", ruleID), nil)
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *TagEventRuleDetailOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.SuppressWindow, ShouldEqual, serializer.DurationNS(90*time.Minute))
		})

		Convey("版本快照与回滚保持 suppressWindow 一致", func() {
			// 创建时 suppressWindow = 30m
			w := doTagEventRuleRequest(r, http.MethodPost, "/tag-event/tag-event-rule/create", map[string]any{
				"name":           "rule-sup-version",
				"description":    "test rule",
				"matchAll":       false,
				"enabled":        true,
				"suppressWindow": (30 * time.Minute).Nanoseconds(),
				"conditions":     []map[string]any{tagEventRuleCondition(tagEventRuleValidExpr)},
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var rule tag.TagEventRule
			So(db.Where("name = ?", "rule-sup-version").First(&rule).Error, ShouldBeNil)

			// 更新 suppressWindow = 90m，产生版本 1（快照创建时 30m）
			w = doTagEventRuleRequest(r, http.MethodPost, tagEventRuleIDPath("/update", rule.ID), map[string]any{
				"description":    "updated",
				"matchAll":       false,
				"enabled":        true,
				"suppressWindow": (90 * time.Minute).Nanoseconds(),
				"conditions":     []map[string]any{tagEventRuleCondition(tagEventRuleValidExpr)},
			})
			So(w.Code, ShouldEqual, http.StatusOK)

			// 版本 1 快照保持创建时 30m
			w = doTagEventRuleRequest(r, http.MethodPost, tagEventRuleIDPath("/version/list", rule.ID), nil)
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data struct {
					Items []*TagEventRuleVersionOut `json:"items"`
				} `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Items, ShouldHaveLength, 1)
			So(resp.Data.Items[0].Version, ShouldEqual, 1)
			So(resp.Data.Items[0].SuppressWindow, ShouldEqual, serializer.DurationNS(30*time.Minute))

			// 回滚到版本 1，suppressWindow 恢复为 30m
			w = doTagEventRuleRequest(r, http.MethodPost, tagEventRuleIDPath("/version/1/rollback", rule.ID), nil)
			So(w.Code, ShouldEqual, http.StatusOK)
			So(db.First(&rule, rule.ID).Error, ShouldBeNil)
			So(rule.SuppressWindow, ShouldEqual, serializer.DurationNS(30*time.Minute))
		})
	})
}

// insertTagEventRecordRow 直接落库一条标签事件（CreatedAt 由 GORM 自动填充），返回其 ID。
// ruleID 必须是当前事务内真实存在的标签事件规则（tag_event_records 对 rule_id 有级联外键约束）。
func insertTagEventRecordRow(db *database.DB, ruleID uint, namespace, status string) uint {
	ev := &tag.TagEventRecord{NamespaceName: namespace, RuleID: ruleID, RuleName: "rule", DedupKey: namespace + ":" + status, Status: status}
	So(db.Create(ev).Error, ShouldBeNil)
	return ev.ID
}

// tagEventRecordStatus 查询标签事件当前状态。
func tagEventRecordStatus(db *database.DB, id uint) string {
	var status string
	So(db.Model(&tag.TagEventRecord{}).Where("id = ?", id).Pluck("status", &status).Error, ShouldBeNil)
	return status
}

// TestTagEventRecordBatchClose 验证标签事件批量关闭：批量关闭、幂等跳过与命名空间隔离。
func TestTagEventRecordBatchClose(t *testing.T) {
	Convey("测试标签事件批量关闭", t, func() {
		db := createTagEventRuleTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		rawDB := createTagEventRuleRawDB(t)
		r := setupTagEventRuleTestRouter(db, rawDB)
		// tag_event_records 表由共享测试库的 core 迁移提供（testdb.Core），无需手动建表
		// 事件记录对 rule_id 有级联外键约束，故先创建一条真实规则作为父行；
		// 命名空间 FK 要求 other-ns 命名空间真实存在，一并种入。
		ruleID := createTagEventRule(t, db, r, "record-rule", false, true, []map[string]any{tagEventRuleCondition(tagEventRuleValidExpr)})
		So(db.Create(&core.Namespace{Name: "other-ns"}).Error, ShouldBeNil)
		// 预置：当前命名空间 alerting 1 条、closed 1 条；其他命名空间 alerting 1 条
		alertID := insertTagEventRecordRow(db, ruleID, middlewares.DefaultNamespace, tag.EventStatusAlerting)
		closedID := insertTagEventRecordRow(db, ruleID, middlewares.DefaultNamespace, tag.EventStatusClosed)
		otherID := insertTagEventRecordRow(db, ruleID, "other-ns", tag.EventStatusAlerting)

		Convey("批量关闭：alerting 事件被置为 closed 且返回成功汇总", func() {
			w := doTagEventRuleRequest(r, http.MethodPost, "/tag-event/record/close", map[string]any{"ids": []uint64{uint64(alertID)}})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Code int            `json:"code"`
				Data BatchResultOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, http.StatusOK)
			So(resp.Data.Succeeded, ShouldEqual, 1)
			So(resp.Data.Failed, ShouldHaveLength, 0)
			So(tagEventRecordStatus(db, alertID), ShouldEqual, tag.EventStatusClosed)
		})

		Convey("幂等跳过：已关闭与不存在的 ID 均返回成功", func() {
			w := doTagEventRuleRequest(r, http.MethodPost, "/tag-event/record/close", map[string]any{"ids": []uint64{uint64(closedID), 99999}})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Code int            `json:"code"`
				Data BatchResultOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, http.StatusOK)
			So(resp.Data.Succeeded, ShouldEqual, 2)
			So(tagEventRecordStatus(db, closedID), ShouldEqual, tag.EventStatusClosed)
		})

		Convey("命名空间隔离：其他命名空间的事件不被关闭", func() {
			w := doTagEventRuleRequest(r, http.MethodPost, "/tag-event/record/close", map[string]any{"ids": []uint64{uint64(otherID)}})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Code int            `json:"code"`
				Data BatchResultOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, http.StatusOK)
			So(resp.Data.Succeeded, ShouldEqual, 1)
			So(tagEventRecordStatus(db, otherID), ShouldEqual, tag.EventStatusAlerting)
		})
	})
}
