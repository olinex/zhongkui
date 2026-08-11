// Package routes 提供 HTTP 路由注册和管理功能。
// 本文件包含标签识别规则管理路由的集成测试：
// 覆盖规则 CRUD、启停、语法校验、dry-run、版本快照/裁剪/回滚、
// 删除级联，以及写路径后引擎缓存失效与广播降级（未绑定 etcd 快速失败）。
package routes

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	. "github.com/smartystreets/goconvey/convey"
	"go.uber.org/zap"

	"zhongkui.com/zhongkui/internal/manager"
	"zhongkui.com/zhongkui/internal/models/tag"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/tagengine"
)

// tagRuleTargetModel 引擎识别测试用的目标模型，其字段经 BuildInput 序列化为输入文档。
type tagRuleTargetModel struct {
	Namespace string `json:"namespace"`
	Method    string `json:"method"`
	Path      string `json:"path"`
}

// tagRuleValidRego 通过语法与契约校验的合法 Rego 规则源码。
const tagRuleValidRego = `package zki

tags contains "admin-login" if {
	input.method == "POST"
}`

// tagRuleModel 目标模型类型键，作为规则创建的 model 入参。
var tagRuleModel = func() string {
	t, err := tagengine.TypeKey(&tagRuleTargetModel{})
	if err != nil {
		panic("failed to compute tag rule target model: " + err.Error())
	}
	return t
}()

// newTagRuleTestLogger 创建测试用日志记录器。
func newTagRuleTestLogger() *logging.Logger {
	config := zap.NewProductionConfig()
	config.OutputPaths = []string{}
	zapLogger, _ := config.Build()
	return &logging.Logger{SugaredLogger: zapLogger.Sugar()}
}

// createTagRuleTestDB 创建 sqlite 内存库并迁移 tag_rules / tag_rule_versions 表。
// 内存库必须限制为单连接，避免多个连接各自持有独立的内存数据库。
func createTagRuleTestDB() *database.DB {
	cfg := &database.Config{Driver: "sqlite", DSN: ":memory:"}
	db, err := database.NewDatabase(cfg, newTagRuleTestLogger())
	if err != nil {
		panic("failed to create test database: " + err.Error())
	}
	if sqlDB, err := db.DB(); err != nil {
		panic("failed to get underlying sql.DB: " + err.Error())
	} else {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := db.AutoMigrate(&tag.TagRule{}, &tag.TagRuleVersion{}); err != nil {
		panic("failed to migrate test database: " + err.Error())
	}
	return db
}

// registerTagRuleTestRoutes 注册标签规则管理路由（与 router.go 的注册保持一致）。
func registerTagRuleTestRoutes(r *gin.Engine, engine *tagengine.Engine) {
	// 测试引擎未绑定 etcd：写路径 BroadcastChange 快速失败，验证广播降级不阻断响应
	engineMiddleware := manager.TagRuleEngineMiddleware(engine)
	admin := r.Group("/admin")
	{
		admin.POST("/tag-rule/list", tagRuleListGet)
		admin.OPTIONS("/tag-rule/list", ListOptionsGinHandler[*TagRuleListGetSearchIn, *TagRuleListOut])
		admin.POST("/tag-rule/create", engineMiddleware, tagRuleCreate)
		admin.POST("/tag-rule/:id/get", tagRuleDetailGet)
		admin.POST("/tag-rule/:id/update", engineMiddleware, tagRuleUpdate)
		admin.POST("/tag-rule/:id/delete", engineMiddleware, tagRuleDelete)
		admin.POST("/tag-rule/:id/enable", engineMiddleware, tagRuleSetEnabled(true))
		admin.POST("/tag-rule/:id/disable", engineMiddleware, tagRuleSetEnabled(false))
		admin.POST("/tag-rule/validate", tagRuleValidate)
		admin.POST("/tag-rule/single/dry-run", tagRuleDryRun)
		admin.POST("/tag-rule/group/dry-run", tagRuleDryRunGroup)
		admin.POST("/tag-rule/:id/version/list", tagRuleVersionList)
		admin.POST("/tag-rule/:id/version/:version/rollback", engineMiddleware, tagRuleVersionRollback)
	}
}

// setupTagRuleTestRouter 创建测试路由引擎，注入 i18n、日志、数据库、
// 命名空间（固定 default）与标签引擎中间件。
func setupTagRuleTestRouter(db *database.DB, engine *tagengine.Engine) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(setupI18nMiddleware())
	r.Use(manager.ZapLoggerMiddleware(newTagRuleTestLogger()))
	r.Use(func(c *gin.Context) {
		c.Set(manager.CoreDatabaseCtxName, db)
		c.Next()
	})
	// 模拟生产环境 NamespaceMiddleware：固定使用 default 命名空间
	r.Use(func(c *gin.Context) {
		c.Set(manager.NamespaceCtxName, manager.DefaultNamespace)
		c.Next()
	})
	registerTagRuleTestRoutes(r, engine)
	return r
}

// doTagRuleRequest 执行一次标签规则 HTTP 请求并返回响应记录器。
func doTagRuleRequest(r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
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

// createTagRule 通过创建接口落库一条规则，返回规则 ID。
func createTagRule(t *testing.T, db *database.DB, r *gin.Engine, name, rego string) uint {
	t.Helper()
	w := doTagRuleRequest(r, http.MethodPost, "/admin/tag-rule/create", map[string]any{
		"name":        name,
		"description": "test rule",
		"model":       tagRuleModel,
		"rego":        rego,
		"enabled":     true,
	})
	So(w.Code, ShouldEqual, http.StatusOK)
	var rule tag.TagRule
	So(db.Where("name = ?", name).First(&rule).Error, ShouldBeNil)
	return rule.ID
}

// ruleIDPath 拼接规则路由路径。
func ruleIDPath(action string, id uint) string {
	return "/admin/tag-rule/" + strconv.FormatUint(uint64(id), 10) + action
}

// TestTagRuleCreateAndDetail 创建与详情查询全链路。
func TestTagRuleCreateAndDetail(t *testing.T) {
	Convey("测试标签规则创建与详情查询", t, func() {
		db := createTagRuleTestDB()
		engine := tagengine.NewEngine(db, newTagRuleTestLogger(), nil)
		r := setupTagRuleTestRouter(db, engine)

		Convey("合法规则创建成功且不产生历史版本", func() {
			ruleID := createTagRule(t, db, r, "rule-a", tagRuleValidRego)

			w := doTagRuleRequest(r, http.MethodPost, ruleIDPath("/get", ruleID), nil)
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Code int               `json:"code"`
				Data *TagRuleDetailOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, http.StatusOK)
			So(resp.Data, ShouldNotBeNil)
			So(resp.Data.Name, ShouldEqual, "rule-a")
			So(resp.Data.Model, ShouldEqual, tagRuleModel)
			So(resp.Data.Enabled, ShouldBeTrue)
			So(resp.Data.Rego, ShouldContainSubstring, "package zki")
			So(resp.Data.Version, ShouldEqual, 0)
			So(resp.Data.VersionCount, ShouldEqual, 0)
		})

		Convey("规则名不合法被拒绝", func() {
			w := doTagRuleRequest(r, http.MethodPost, "/admin/tag-rule/create", map[string]any{
				"name":    "bad name!",
				"model":   tagRuleModel,
				"rego":    tagRuleValidRego,
				"enabled": true,
			})
			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})
	})
}

// TestTagRuleCreateInvalidRegoRejected 非法语法与组级冲突的规则被拒绝落库。
func TestTagRuleCreateInvalidRegoRejected(t *testing.T) {
	Convey("测试非法规则的拒绝逻辑", t, func() {
		db := createTagRuleTestDB()
		engine := tagengine.NewEngine(db, newTagRuleTestLogger(), nil)
		r := setupTagRuleTestRouter(db, engine)

		Convey("语法非法返回带行列号的错误详情", func() {
			w := doTagRuleRequest(r, http.MethodPost, "/admin/tag-rule/create", map[string]any{
				"name":    "bad-syntax",
				"model":   tagRuleModel,
				"rego":    "package zki\n\ntags contains if {",
				"enabled": true,
			})
			So(w.Code, ShouldEqual, http.StatusBadRequest)
			var resp struct {
				Code    int                   `json:"code"`
				Details []tagengine.RegoError `json:"details"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, manager.CodeTagRuleInvalid)
			So(len(resp.Details), ShouldBeGreaterThan, 0)
			So(resp.Details[0].Message, ShouldNotBeEmpty)

			var count int64
			So(db.Model(&tag.TagRule{}).Where("name = ?", "bad-syntax").Count(&count).Error, ShouldBeNil)
			So(count, ShouldEqual, 0)
		})

		Convey("组级冲突（tags := 与 tags contains 并存）被拒绝", func() {
			createTagRule(t, db, r, "full-assign", "package zki\n\ntags := {\"a\"}")
			w := doTagRuleRequest(r, http.MethodPost, "/admin/tag-rule/create", map[string]any{
				"name":    "partial",
				"model":   tagRuleModel,
				"rego":    "package zki\n\ntags contains \"b\"",
				"enabled": true,
			})
			So(w.Code, ShouldEqual, http.StatusBadRequest)
			var resp struct {
				Code int `json:"code"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, manager.CodeTagRuleInvalid)

			var count int64
			So(db.Model(&tag.TagRule{}).Where("name = ?", "partial").Count(&count).Error, ShouldBeNil)
			So(count, ShouldEqual, 0)
		})
	})
}

// TestTagRuleUpdateAndVersion 更新生成版本快照且内容正确。
func TestTagRuleUpdateAndVersion(t *testing.T) {
	Convey("测试标签规则更新与版本快照", t, func() {
		db := createTagRuleTestDB()
		engine := tagengine.NewEngine(db, newTagRuleTestLogger(), nil)
		r := setupTagRuleTestRouter(db, engine)
		ruleID := createTagRule(t, db, r, "rule-up", tagRuleValidRego)

		newRego := `package zki

tags contains "admin-updated" if {
	input.path == "/admin"
}`
		w := doTagRuleRequest(r, http.MethodPost, ruleIDPath("/update", ruleID), map[string]any{
			"description": "updated description",
			"rego":        newRego,
			"enabled":     true,
		})
		So(w.Code, ShouldEqual, http.StatusOK)

		Convey("更新前内容被快照为版本 1", func() {
			var versions []*TagRuleVersionOut
			w := doTagRuleRequest(r, http.MethodPost, ruleIDPath("/version/list", ruleID), nil)
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data struct {
					Items []*TagRuleVersionOut `json:"items"`
				} `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			versions = resp.Data.Items
			So(len(versions), ShouldEqual, 1)
			So(versions[0].Version, ShouldEqual, 1)
			So(versions[0].Rego, ShouldEqual, tagRuleValidRego)
			So(versions[0].Name, ShouldEqual, "rule-up")
		})

		Convey("详情返回最新版本号与版本数", func() {
			w := doTagRuleRequest(r, http.MethodPost, ruleIDPath("/get", ruleID), nil)
			var resp struct {
				Data *TagRuleDetailOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Rego, ShouldEqual, newRego)
			So(resp.Data.Description, ShouldEqual, "updated description")
			So(resp.Data.Version, ShouldEqual, 1)
			So(resp.Data.VersionCount, ShouldEqual, 1)
		})

		Convey("非法内容更新被拒绝且不产生版本", func() {
			w := doTagRuleRequest(r, http.MethodPost, ruleIDPath("/update", ruleID), map[string]any{
				"rego":    "package zki\n\nbroken",
				"enabled": true,
			})
			So(w.Code, ShouldEqual, http.StatusBadRequest)
			var count int64
			So(db.Model(&tag.TagRuleVersion{}).Where("rule_id = ?", ruleID).Count(&count).Error, ShouldBeNil)
			So(count, ShouldEqual, 1)
		})
	})
}

// TestTagRulePruneVersions 历史版本超过上限时裁剪最旧版本。
func TestTagRulePruneVersions(t *testing.T) {
	Convey("测试历史版本裁剪", t, func() {
		db := createTagRuleTestDB()
		engine := tagengine.NewEngine(db, newTagRuleTestLogger(), nil)
		r := setupTagRuleTestRouter(db, engine)
		ruleID := createTagRule(t, db, r, "rule-prune", tagRuleValidRego)

		// 连续更新 32 次，产生 32 个版本，应仅保留最新 30 个
		for i := 0; i < 32; i++ {
			rego := "package zki\n\ntags contains \"v" + strconv.Itoa(i) + "\" if {\n\tinput.method == \"POST\"\n}"
			w := doTagRuleRequest(r, http.MethodPost, ruleIDPath("/update", ruleID), map[string]any{
				"rego":    rego,
				"enabled": true,
			})
			So(w.Code, ShouldEqual, http.StatusOK)
		}

		var count int64
		So(db.Model(&tag.TagRuleVersion{}).Where("rule_id = ?", ruleID).Count(&count).Error, ShouldBeNil)
		So(count, ShouldEqual, tag.MaxTagRuleVersions)

		var versions []*TagRuleVersionOut
		w := doTagRuleRequest(r, http.MethodPost, ruleIDPath("/version/list", ruleID), nil)
		var resp struct {
			Data struct {
				Items []*TagRuleVersionOut `json:"items"`
			} `json:"data"`
		}
		So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
		versions = resp.Data.Items
		So(len(versions), ShouldEqual, tag.MaxTagRuleVersions)
		// 版本号倒序，保留的是最新 30 个（3..32），最早的 1、2 被裁剪
		So(versions[0].Version, ShouldEqual, 32)
		So(versions[len(versions)-1].Version, ShouldEqual, 3)
	})
}

// TestTagRuleRollback 回滚恢复目标版本内容，且回滚前内容成为新版本。
func TestTagRuleRollback(t *testing.T) {
	Convey("测试标签规则回滚", t, func() {
		db := createTagRuleTestDB()
		engine := tagengine.NewEngine(db, newTagRuleTestLogger(), nil)
		r := setupTagRuleTestRouter(db, engine)
		ruleID := createTagRule(t, db, r, "rule-rb", tagRuleValidRego)

		secondRego := "package zki\n\ntags contains \"second\" if {\n\tinput.method == \"POST\"\n}"
		doTagRuleRequest(r, http.MethodPost, ruleIDPath("/update", ruleID), map[string]any{
			"rego":    secondRego,
			"enabled": true,
		})

		Convey("回滚到版本 1 恢复原始内容", func() {
			w := doTagRuleRequest(r, http.MethodPost, ruleIDPath("/version/1/rollback", ruleID), nil)
			So(w.Code, ShouldEqual, http.StatusOK)

			var rule tag.TagRule
			So(db.First(&rule, ruleID).Error, ShouldBeNil)
			So(rule.Rego, ShouldEqual, tagRuleValidRego)

			// 回滚前内容被快照为新版本 2，当前恢复为版本 1 的内容
			var count int64
			So(db.Model(&tag.TagRuleVersion{}).Where("rule_id = ?", ruleID).Count(&count).Error, ShouldBeNil)
			So(count, ShouldEqual, 2)

			var versions []*TagRuleVersionOut
			w = doTagRuleRequest(r, http.MethodPost, ruleIDPath("/version/list", ruleID), nil)
			var resp struct {
				Data struct {
					Items []*TagRuleVersionOut `json:"items"`
				} `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			versions = resp.Data.Items
			So(len(versions), ShouldEqual, 2)
			So(versions[0].Version, ShouldEqual, 2)
			So(versions[0].Rego, ShouldEqual, secondRego)
			So(versions[1].Version, ShouldEqual, 1)
			So(versions[1].Rego, ShouldEqual, tagRuleValidRego)
		})

		Convey("目标版本不存在返回业务错误", func() {
			w := doTagRuleRequest(r, http.MethodPost, ruleIDPath("/version/99/rollback", ruleID), nil)
			So(w.Code, ShouldEqual, http.StatusNotFound)
		})
	})
}

// TestTagRuleDeleteCascade 删除规则时级联删除其全部历史版本。
func TestTagRuleDeleteCascade(t *testing.T) {
	Convey("测试标签规则删除级联", t, func() {
		db := createTagRuleTestDB()
		engine := tagengine.NewEngine(db, newTagRuleTestLogger(), nil)
		r := setupTagRuleTestRouter(db, engine)
		ruleID := createTagRule(t, db, r, "rule-del", tagRuleValidRego)

		doTagRuleRequest(r, http.MethodPost, ruleIDPath("/update", ruleID), map[string]any{
			"rego":    "package zki\n\ntags contains \"v2\" if {\n\tinput.method == \"POST\"\n}",
			"enabled": true,
		})

		w := doTagRuleRequest(r, http.MethodPost, ruleIDPath("/delete", ruleID), nil)
		So(w.Code, ShouldEqual, http.StatusOK)

		var ruleCount, versionCount int64
		So(db.Model(&tag.TagRule{}).Where("id = ?", ruleID).Count(&ruleCount).Error, ShouldBeNil)
		So(db.Model(&tag.TagRuleVersion{}).Where("rule_id = ?", ruleID).Count(&versionCount).Error, ShouldBeNil)
		So(ruleCount, ShouldEqual, 0)
		So(versionCount, ShouldEqual, 0)
	})
}

// TestTagRuleEnableDisable 启用/停用规则状态切换。
func TestTagRuleEnableDisable(t *testing.T) {
	Convey("测试标签规则启停", t, func() {
		db := createTagRuleTestDB()
		engine := tagengine.NewEngine(db, newTagRuleTestLogger(), nil)
		r := setupTagRuleTestRouter(db, engine)
		ruleID := createTagRule(t, db, r, "rule-io", tagRuleValidRego)

		w := doTagRuleRequest(r, http.MethodPost, ruleIDPath("/disable", ruleID), nil)
		So(w.Code, ShouldEqual, http.StatusOK)
		var rule tag.TagRule
		So(db.First(&rule, ruleID).Error, ShouldBeNil)
		So(rule.Enabled, ShouldBeFalse)

		// 重复停用幂等返回成功
		w = doTagRuleRequest(r, http.MethodPost, ruleIDPath("/disable", ruleID), nil)
		So(w.Code, ShouldEqual, http.StatusOK)

		w = doTagRuleRequest(r, http.MethodPost, ruleIDPath("/enable", ruleID), nil)
		So(w.Code, ShouldEqual, http.StatusOK)
		So(db.First(&rule, ruleID).Error, ShouldBeNil)
		So(rule.Enabled, ShouldBeTrue)
	})
}

// TestTagRuleValidate 语法校验接口返回结构化结果。
func TestTagRuleValidate(t *testing.T) {
	Convey("测试标签规则语法校验接口", t, func() {
		db := createTagRuleTestDB()
		engine := tagengine.NewEngine(db, newTagRuleTestLogger(), nil)
		r := setupTagRuleTestRouter(db, engine)

		Convey("合法规则返回 valid=true 且 errors 为空数组", func() {
			w := doTagRuleRequest(r, http.MethodPost, "/admin/tag-rule/validate", map[string]any{
				"rego": tagRuleValidRego,
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data TagRuleValidateOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Valid, ShouldBeTrue)
			So(len(resp.Data.Errors), ShouldEqual, 0)
		})

		Convey("非法规则返回 valid=false 且错误带行列号", func() {
			w := doTagRuleRequest(r, http.MethodPost, "/admin/tag-rule/validate", map[string]any{
				"rego": "package zki\n\ntags contains if {",
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data TagRuleValidateOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Valid, ShouldBeFalse)
			So(len(resp.Data.Errors), ShouldBeGreaterThan, 0)
			So(resp.Data.Errors[0].Line, ShouldBeGreaterThan, 0)
			So(resp.Data.Errors[0].Message, ShouldNotBeEmpty)
		})
	})
}

// TestTagRuleDryRun 单规则 dry-run 接口对候选规则评估输入文档。
// 输出仅含错误消息不泄露堆栈。
func TestTagRuleDryRun(t *testing.T) {
	Convey("测试标签规则单规则 dry-run 接口", t, func() {
		db := createTagRuleTestDB()
		engine := tagengine.NewEngine(db, newTagRuleTestLogger(), nil)
		r := setupTagRuleTestRouter(db, engine)

		Convey("合法候选规则命中输入文档", func() {
			w := doTagRuleRequest(r, http.MethodPost, "/admin/tag-rule/single/dry-run", map[string]any{
				"model": tagRuleModel,
				"name":  "solo",
				"rego":  tagRuleValidRego,
				"input": map[string]any{"method": "POST"},
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *TagRuleDryRunOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Tags, ShouldResemble, []string{"admin-login"})
			So(resp.Data.Error, ShouldBeEmpty)
			So(resp.Data.Rules, ShouldContainKey, "solo")
			So(resp.Data.Rules["solo"].Tags, ShouldResemble, []string{"admin-login"})
			So(resp.Data.Rules["solo"].Error, ShouldBeEmpty)
			So(resp.Data.Rules["solo"].Duration, ShouldBeGreaterThanOrEqualTo, int64(0))
		})

		Convey("缺少规则名被拒绝", func() {
			w := doTagRuleRequest(r, http.MethodPost, "/admin/tag-rule/single/dry-run", map[string]any{
				"model": tagRuleModel,
				"rego":  tagRuleValidRego,
				"input": map[string]any{"method": "POST"},
			})
			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})

		Convey("语法非法的候选规则返回校验错误且不评估", func() {
			w := doTagRuleRequest(r, http.MethodPost, "/admin/tag-rule/single/dry-run", map[string]any{
				"model": tagRuleModel,
				"name":  "bad",
				"rego":  "package zki\n\ntags contains if {",
				"input": map[string]any{"method": "POST"},
			})
			// 候选规则语法非法导致编译失败，走 invalidTagRuleError（HTTP 400 + CodeTagRuleInvalid），
			// 不产出评估结果且不泄露错误堆栈
			So(w.Code, ShouldEqual, http.StatusBadRequest)
			var resp struct {
				Code int `json:"code"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, manager.CodeTagRuleInvalid)
		})

		Convey("缺少 rego 被拒绝", func() {
			w := doTagRuleRequest(r, http.MethodPost, "/admin/tag-rule/single/dry-run", map[string]any{
				"model": tagRuleModel,
				"input": map[string]any{"method": "POST"},
			})
			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})
	})
}

// TestTagRuleDryRunGroup 规则组 dry-run 接口对候选规则集合评估输入文档。
// 输出逐规则详情与合并标签，错误仅含消息不泄露堆栈。
func TestTagRuleDryRunGroup(t *testing.T) {
	Convey("测试标签规则规则组 dry-run 接口", t, func() {
		db := createTagRuleTestDB()
		engine := tagengine.NewEngine(db, newTagRuleTestLogger(), nil)
		r := setupTagRuleTestRouter(db, engine)

		Convey("合法候选规则组命中输入文档并输出逐规则详情", func() {
			w := doTagRuleRequest(r, http.MethodPost, "/admin/tag-rule/group/dry-run", map[string]any{
				"model": tagRuleModel,
				"rules": []map[string]any{
					{"name": "rule-hit", "rego": tagRuleValidRego},
					{"name": "rule-miss", "rego": `package zki

tags contains "miss-tag" if {
    input.method == "DELETE"
}`},
				},
				"input": map[string]any{"method": "POST"},
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *TagRuleDryRunOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Tags, ShouldResemble, []string{"admin-login"})
			So(resp.Data.Error, ShouldBeEmpty)
			So(resp.Data.Rules, ShouldContainKey, "rule-hit")
			So(resp.Data.Rules["rule-hit"].Tags, ShouldResemble, []string{"admin-login"})
			So(resp.Data.Rules["rule-hit"].Error, ShouldBeEmpty)
			So(resp.Data.Rules, ShouldContainKey, "rule-miss")
			So(resp.Data.Rules["rule-miss"].Tags, ShouldBeEmpty)
			So(resp.Data.Rules["rule-miss"].Error, ShouldBeEmpty)
		})

		Convey("语法非法的候选规则返回校验错误且不评估", func() {
			w := doTagRuleRequest(r, http.MethodPost, "/admin/tag-rule/group/dry-run", map[string]any{
				"model": tagRuleModel,
				"rules": []map[string]any{
					{"name": "bad", "rego": "package zki\n\ntags contains if {"},
				},
				"input": map[string]any{"method": "POST"},
			})
			// 候选规则语法非法导致组编译失败，走 invalidTagRuleError（HTTP 400 + CodeTagRuleInvalid），
			// 不产出评估结果且不泄露错误堆栈
			So(w.Code, ShouldEqual, http.StatusBadRequest)
			var resp struct {
				Code int `json:"code"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, manager.CodeTagRuleInvalid)
		})

		Convey("空规则列表被拒绝", func() {
			w := doTagRuleRequest(r, http.MethodPost, "/admin/tag-rule/group/dry-run", map[string]any{
				"model": tagRuleModel,
				"rules": []map[string]any{},
				"input": map[string]any{"method": "POST"},
			})
			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})
	})
}

// TestTagRuleListGet 列表查询与过滤。
func TestTagRuleListGet(t *testing.T) {
	Convey("测试标签规则列表查询", t, func() {
		db := createTagRuleTestDB()
		engine := tagengine.NewEngine(db, newTagRuleTestLogger(), nil)
		r := setupTagRuleTestRouter(db, engine)
		createTagRule(t, db, r, "rule-a", tagRuleValidRego)
		createTagRule(t, db, r, "rule-b", "package zki\n\ntags contains \"b\" if {\n\tinput.method == \"GET\"\n}")

		Convey("按名称前缀过滤", func() {
			w := doTagRuleRequest(r, http.MethodPost, "/admin/tag-rule/list", map[string]any{
				"filterBy": map[string]any{
					"name": map[string]any{"action": "pfx", "value": "rule-a"},
				},
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *struct {
					Total uint              `json:"total"`
					Items []*TagRuleListOut `json:"items"`
				} `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Total, ShouldEqual, 1)
			So(resp.Data.Items[0].Name, ShouldEqual, "rule-a")
		})

		Convey("列表选项接口返回列与搜索元数据", func() {
			w := doTagRuleRequest(r, http.MethodOptions, "/admin/tag-rule/list", nil)
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *ListOptions `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(len(resp.Data.OrderBy), ShouldEqual, 2)
			So(len(resp.Data.FilterBy), ShouldBeGreaterThan, 0)
			So(len(resp.Data.Headers), ShouldBeGreaterThan, 0)
		})
	})
}

// TestTagRuleInvalidateAfterWrite 写路径后本地引擎缓存被失效。
func TestTagRuleInvalidateAfterWrite(t *testing.T) {
	Convey("测试写路径后引擎缓存失效", t, func() {
		db := createTagRuleTestDB()
		engine := tagengine.NewEngine(db, newTagRuleTestLogger(), nil)
		r := setupTagRuleTestRouter(db, engine)
		createTagRule(t, db, r, "rule-cache", tagRuleValidRego)

		// 首次识别命中 rule-cache，并填充组编译缓存
		first, err := engine.RecognizeWithDetail(t.Context(), &tagRuleTargetModel{Namespace: "default", Method: "POST"})
		So(err, ShouldBeNil)
		So(first.Tags, ShouldResemble, []string{"admin-login"})

		// 经接口新增同组规则后，引擎应重新按 DB 最新规则编译
		w := doTagRuleRequest(r, http.MethodPost, "/admin/tag-rule/create", map[string]any{
			"name":    "rule-cache-2",
			"model":   tagRuleModel,
			"rego":    "package zki\n\ntags contains \"admin-cache\" if {\n\tinput.method == \"POST\"\n}",
			"enabled": true,
		})
		So(w.Code, ShouldEqual, http.StatusOK)

		second, err := engine.RecognizeWithDetail(t.Context(), &tagRuleTargetModel{Namespace: "default", Method: "POST"})
		So(err, ShouldBeNil)
		So(second.Tags, ShouldResemble, []string{"admin-cache", "admin-login"})
	})
}
