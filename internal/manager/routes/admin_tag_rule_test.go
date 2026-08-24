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
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	. "github.com/smartystreets/goconvey/convey"
	"go.uber.org/zap"
	"gorm.io/datatypes"

	"zhongkui.com/zhongkui/internal/manager"
	"zhongkui.com/zhongkui/internal/models/network"
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

// httpTransportTypeKey *network.HttpTransport 的类型键，作为模型选项/元信息/样例接口的入参。
var httpTransportTypeKey = func() string {
	t, err := tagengine.TypeKey(&network.HttpTransport{})
	if err != nil {
		panic("failed to compute http transport type key: " + err.Error())
	}
	return t
}()

// init 将测试用目标模型注册进引擎注册表，使写路径的模型注册校验（未注册即拒绝）放行。
// 注册表为包级全局状态且重复注册会 panic，故仅在此 init 注册一次。
func init() {
	tagengine.Register(tagengine.Target{Type: &tagRuleTargetModel{}, DisplayName: "model.test.tagRuleTarget"})
}

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
	if err := db.AutoMigrate(&tag.TagRule{}, &tag.TagRuleVersion{}, &tag.TagRuleSample{}); err != nil {
		panic("failed to migrate test database: " + err.Error())
	}
	return db
}

// registerTagRuleTestRoutes 注册标签规则管理路由（与 router.go 的注册保持一致）。
func registerTagRuleTestRoutes(r *gin.Engine, engine *tagengine.Engine) {
	// 测试引擎未绑定 etcd：写路径 BroadcastChange 快速失败，验证广播降级不阻断响应
	engineMiddleware := manager.TagRuleEngineMiddleware(engine)
	admin := r.Group("/traffic")
	{
		admin.POST("/tag-rule/list", tagRuleListGet)
		admin.OPTIONS("/tag-rule/list", ListMetaGinHandler[*TagRuleListGetSearchIn, *TagRuleListOut]())
		admin.POST("/tag-rule/create", engineMiddleware, tagRuleCreate)
		admin.POST("/tag-rule/:id/get", tagRuleDetailGet)
		admin.POST("/tag-rule/:id/update", engineMiddleware, tagRuleUpdate)
		admin.POST("/tag-rule/delete", engineMiddleware, tagRuleBatchDelete)
		admin.POST("/tag-rule/enable", engineMiddleware, tagRuleBatchSetEnabled(true))
		admin.POST("/tag-rule/disable", engineMiddleware, tagRuleBatchSetEnabled(false))
		admin.POST("/tag-rule/validate", tagRuleValidate)
		admin.POST("/tag-rule/model/options", tagRuleModelOptions)
		admin.POST("/tag-rule/model/metadata", tagRuleModelMetadata)
		admin.POST("/tag-rule/model/samples", tagRuleModelSamples)
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
	w := doTagRuleRequest(r, http.MethodPost, "/traffic/tag-rule/create", map[string]any{
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
	return "/traffic/tag-rule/" + strconv.FormatUint(uint64(id), 10) + action
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
			w := doTagRuleRequest(r, http.MethodPost, "/traffic/tag-rule/create", map[string]any{
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
			w := doTagRuleRequest(r, http.MethodPost, "/traffic/tag-rule/create", map[string]any{
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
			w := doTagRuleRequest(r, http.MethodPost, "/traffic/tag-rule/create", map[string]any{
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

		Convey("Rego 超过 64KiB 上限被拒绝", func() {
			// 合法的 Rego 头部 + 超长注释撑满 64KiB，触发 binding max=65536
			oversized := "package zki\n\ntags contains \"a\" if {\n\t# " + strings.Repeat("x", 65536) + "\n}"
			w := doTagRuleRequest(r, http.MethodPost, "/traffic/tag-rule/create", map[string]any{
				"name":    "oversized",
				"model":   tagRuleModel,
				"rego":    oversized,
				"enabled": true,
			})
			So(w.Code, ShouldEqual, http.StatusBadRequest)
			var resp struct {
				Code int `json:"code"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, manager.CodeBadBody)

			var count int64
			So(db.Model(&tag.TagRule{}).Where("name = ?", "oversized").Count(&count).Error, ShouldBeNil)
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

		w := doTagRuleRequest(r, http.MethodPost, "/traffic/tag-rule/delete", map[string]any{"ids": []uint64{uint64(ruleID)}})
		So(w.Code, ShouldEqual, http.StatusOK)
		var resp struct {
			Code int            `json:"code"`
			Data BatchResultOut `json:"data"`
		}
		So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
		So(resp.Code, ShouldEqual, http.StatusOK)
		So(resp.Data.Succeeded, ShouldEqual, 1)
		So(len(resp.Data.Failed), ShouldEqual, 0)

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

		w := doTagRuleRequest(r, http.MethodPost, "/traffic/tag-rule/disable", map[string]any{"ids": []uint64{uint64(ruleID)}})
		So(w.Code, ShouldEqual, http.StatusOK)
		var resp struct {
			Code int            `json:"code"`
			Data BatchResultOut `json:"data"`
		}
		So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
		So(resp.Data.Succeeded, ShouldEqual, 1)
		var rule tag.TagRule
		So(db.First(&rule, ruleID).Error, ShouldBeNil)
		So(rule.Enabled, ShouldBeFalse)

		// 重复停用幂等返回成功
		w = doTagRuleRequest(r, http.MethodPost, "/traffic/tag-rule/disable", map[string]any{"ids": []uint64{uint64(ruleID)}})
		So(w.Code, ShouldEqual, http.StatusOK)

		w = doTagRuleRequest(r, http.MethodPost, "/traffic/tag-rule/enable", map[string]any{"ids": []uint64{uint64(ruleID)}})
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
			w := doTagRuleRequest(r, http.MethodPost, "/traffic/tag-rule/validate", map[string]any{
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
			w := doTagRuleRequest(r, http.MethodPost, "/traffic/tag-rule/validate", map[string]any{
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

		Convey("携带 model 时未知 input 字段进入 warnings 且不阻断合法结果", func() {
			w := doTagRuleRequest(r, http.MethodPost, "/traffic/tag-rule/validate", map[string]any{
				"rego":  "package zki\n\ntags contains \"admin-login\" if {\n\tinput.methodd == \"POST\"\n}",
				"model": tagRuleModel,
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data TagRuleValidateOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Valid, ShouldBeTrue)
			So(len(resp.Data.Errors), ShouldEqual, 0)
			So(len(resp.Data.Warnings), ShouldEqual, 1)
			So(resp.Data.Warnings[0].Type, ShouldEqual, "warning")
			// 告警明细经接口层按请求语言（测试 localizer 为 en）翻译为英文
			So(resp.Data.Warnings[0].Message, ShouldEqual, "Field methodd does not exist in the input document of this model, please check the spelling")
			So(resp.Data.Warnings[0].Hint, ShouldEqual, "Please choose a valid field from the input field panel or the completion list")
		})

		Convey("不携带 model 时 warnings 为空数组", func() {
			w := doTagRuleRequest(r, http.MethodPost, "/traffic/tag-rule/validate", map[string]any{
				"rego": "package zki\n\ntags contains \"admin-login\" if {\n\tinput.methodd == \"POST\"\n}",
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data TagRuleValidateOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Valid, ShouldBeTrue)
			So(resp.Data.Warnings, ShouldNotBeNil)
			So(len(resp.Data.Warnings), ShouldEqual, 0)
		})
	})
}

// TestTagRuleModelOptions 目标模型选项接口返回已注册模型列表。
func TestTagRuleModelOptions(t *testing.T) {
	Convey("测试标签规则目标模型选项接口", t, func() {
		db := createTagRuleTestDB()
		engine := tagengine.NewEngine(db, newTagRuleTestLogger(), nil)
		r := setupTagRuleTestRouter(db, engine)

		Convey("返回已注册模型且含 HttpTransport", func() {
			w := doTagRuleRequest(r, http.MethodPost, "/traffic/tag-rule/model/options", nil)
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data TagRuleModelOptionsOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(len(resp.Data.Items), ShouldBeGreaterThan, 0)
			var found bool
			for _, item := range resp.Data.Items {
				if item.Value == httpTransportTypeKey {
					found = true
					So(item.Label, ShouldNotBeEmpty)
				}
			}
			So(found, ShouldBeTrue)
		})
	})
}

// TestTagRuleModelMetadata 目标模型元信息接口返回字段列表，未注册模型返回错误码 1107。
func TestTagRuleModelMetadata(t *testing.T) {
	Convey("测试标签规则目标模型元信息接口", t, func() {
		db := createTagRuleTestDB()
		engine := tagengine.NewEngine(db, newTagRuleTestLogger(), nil)
		r := setupTagRuleTestRouter(db, engine)

		Convey("HttpTransport 返回字段列表且含 method", func() {
			w := doTagRuleRequest(r, http.MethodPost, "/traffic/tag-rule/model/metadata", map[string]any{
				"model": httpTransportTypeKey,
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data TagRuleModelMetadataOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Model, ShouldEqual, httpTransportTypeKey)
			So(len(resp.Data.Fields), ShouldBeGreaterThan, 0)
			var found bool
			for _, field := range resp.Data.Fields {
				if field.Field == "method" {
					found = true
					So(field.Type, ShouldEqual, "string")
					// Description 由接口层按请求语言（测试 localizer 为 en）翻译 DescriptionKey 填充
					So(field.Description, ShouldEqual, "HTTP request method")
				}
				if field.Field == "client_ip" {
					So(field.Description, ShouldEqual, "Client IP address")
				}
				if field.Field == "tags" {
					So(field.Description, ShouldEqual, "Risk tag combination")
				}
			}
			So(found, ShouldBeTrue)
			// 合法标签维度来自 tagengine 注册表（与物化视图展开列一致），
			// Description 按请求语言（en）复用字段描述文案
			So(resp.Data.Dimensions, ShouldResemble, []TagRuleModelDimensionOut{
				{Value: "fpTcpClientSig", Description: "Client TCP signature"},
				{Value: "fpTcpServerSig", Description: "Server TCP signature"},
			})
		})

		Convey("未注册模型返回错误码 1107", func() {
			w := doTagRuleRequest(r, http.MethodPost, "/traffic/tag-rule/model/metadata", map[string]any{
				"model": "zhongkui.com/zhongkui/internal/models/network.NotExist",
			})
			So(w.Code, ShouldEqual, http.StatusBadRequest)
			var resp struct {
				Code int `json:"code"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, manager.CodeTagRuleModelNotRegistered)
		})
	})
}

// TestTagRuleModelSamples 目标模型样例接口懒供给样例并返回，未注册模型返回错误码 1107。
func TestTagRuleModelSamples(t *testing.T) {
	Convey("测试标签规则目标模型样例接口", t, func() {
		db := createTagRuleTestDB()
		engine := tagengine.NewEngine(db, newTagRuleTestLogger(), nil)
		r := setupTagRuleTestRouter(db, engine)

		Convey("HttpTransport 懒供给样例且含 http-example", func() {
			w := doTagRuleRequest(r, http.MethodPost, "/traffic/tag-rule/model/samples", map[string]any{
				"model": httpTransportTypeKey,
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data TagRuleModelSamplesOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Model, ShouldEqual, httpTransportTypeKey)
			So(len(resp.Data.Items), ShouldBeGreaterThan, 0)
			var found bool
			for _, item := range resp.Data.Items {
				if item.Name == "http-example" {
					found = true
					So(item.Input, ShouldNotBeNil)
					So(item.Input["method"], ShouldEqual, "POST")
				}
			}
			So(found, ShouldBeTrue)
		})

		Convey("未注册模型返回错误码 1107", func() {
			w := doTagRuleRequest(r, http.MethodPost, "/traffic/tag-rule/model/samples", map[string]any{
				"model": "zhongkui.com/zhongkui/internal/models/network.NotExist",
			})
			So(w.Code, ShouldEqual, http.StatusBadRequest)
			var resp struct {
				Code int `json:"code"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, manager.CodeTagRuleModelNotRegistered)
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
			w := doTagRuleRequest(r, http.MethodPost, "/traffic/tag-rule/single/dry-run", map[string]any{
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

		Convey("缺少规则名时以占位名兜底并正常评估", func() {
			w := doTagRuleRequest(r, http.MethodPost, "/traffic/tag-rule/single/dry-run", map[string]any{
				"model": tagRuleModel,
				"rego":  tagRuleValidRego,
				"input": map[string]any{"method": "POST"},
			})
			// dry-run 面向草稿阶段，规则可能尚未命名，name 允许为空，经 defaults 库以 "draft" 兜底
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *TagRuleDryRunOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Tags, ShouldResemble, []string{"admin-login"})
			So(resp.Data.Rules, ShouldContainKey, "draft")
		})

		Convey("语法非法的候选规则返回校验错误且不评估", func() {
			w := doTagRuleRequest(r, http.MethodPost, "/traffic/tag-rule/single/dry-run", map[string]any{
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
			w := doTagRuleRequest(r, http.MethodPost, "/traffic/tag-rule/single/dry-run", map[string]any{
				"model": tagRuleModel,
				"input": map[string]any{"method": "POST"},
			})
			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})

		Convey("违反契约（非 zki 包）的候选规则被拒绝，与写路径校验语义一致", func() {
			w := doTagRuleRequest(r, http.MethodPost, "/traffic/tag-rule/single/dry-run", map[string]any{
				"model": tagRuleModel,
				"name":  "other-package",
				"rego":  "package other\n\ntags contains \"x\" if {\n\tinput.method == \"POST\"\n}",
				"input": map[string]any{"method": "POST"},
			})
			// 契约不满足（包名非 zki）走公共 Rego 校验失败，返回 400 + CodeTagRuleInvalid
			So(w.Code, ShouldEqual, http.StatusBadRequest)
			var resp struct {
				Code int `json:"code"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, manager.CodeTagRuleInvalid)
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
			w := doTagRuleRequest(r, http.MethodPost, "/traffic/tag-rule/group/dry-run", map[string]any{
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
			w := doTagRuleRequest(r, http.MethodPost, "/traffic/tag-rule/group/dry-run", map[string]any{
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
			w := doTagRuleRequest(r, http.MethodPost, "/traffic/tag-rule/group/dry-run", map[string]any{
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
			w := doTagRuleRequest(r, http.MethodPost, "/traffic/tag-rule/list", map[string]any{
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
			w := doTagRuleRequest(r, http.MethodOptions, "/traffic/tag-rule/list", nil)
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
		w := doTagRuleRequest(r, http.MethodPost, "/traffic/tag-rule/create", map[string]any{
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

// TestTagRuleCreateRuntimeInvalidRejected 第二层运行期样例校验失败的规则被拒绝落库。
func TestTagRuleCreateRuntimeInvalidRejected(t *testing.T) {
	Convey("测试运行期样例校验的拒绝逻辑", t, func() {
		db := createTagRuleTestDB()
		engine := tagengine.NewEngine(db, newTagRuleTestLogger(), nil)
		r := setupTagRuleTestRouter(db, engine)

		// 预置一条 HttpTransport 验证样例（已注册模型），字段类型与真实模型一致
		targetType, err := tagengine.TypeKey(&network.HttpTransport{})
		So(err, ShouldBeNil)
		sample := &tag.TagRuleSample{
			NamespaceName: "default",
			Model:         targetType,
			Name:          "browser-post",
			Input: datatypes.JSON(`{
				"namespace": "default",
				"method": "POST",
				"path": "/v1/orders",
				"req_hdr_user_agent": "Mozilla/5.0",
				"req_hdr_content_length": 42
			}`),
			Description: "test sample",
		}
		So(db.Create(sample).Error, ShouldBeNil)

		Convey("运行期报错的规则创建被拒绝并返回错误码 1106", func() {
			w := doTagRuleRequest(r, http.MethodPost, "/traffic/tag-rule/create", map[string]any{
				"name":    "runtime-bad",
				"model":   targetType,
				"rego":    "package zki\n\ntags contains \"oops\" if {\n\tcount(input.req_hdr_content_length) > 10\n}",
				"enabled": true,
			})
			So(w.Code, ShouldEqual, http.StatusBadRequest)
			var resp struct {
				Code    int    `json:"code"`
				Summary string `json:"summary"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, manager.CodeTagRuleRuntimeInvalid)
		})

		Convey("运行期合法的规则创建成功", func() {
			w := doTagRuleRequest(r, http.MethodPost, "/traffic/tag-rule/create", map[string]any{
				"name":    "runtime-ok",
				"model":   targetType,
				"rego":    "package zki\n\ntags contains \"post\" if {\n\tinput.method == \"POST\"\n}",
				"enabled": true,
			})
			So(w.Code, ShouldEqual, http.StatusOK)
		})
	})
}

// TestTagRuleCreateModelNotRegisteredRejected 未注册目标模型的规则创建被拒绝（错误码 1107）。
// 未注册模型的规则引擎永不加载（按实际数据实例的类型键分组），属于死规则，写入时应拒绝。
func TestTagRuleCreateModelNotRegisteredRejected(t *testing.T) {
	Convey("测试未注册目标模型的拒绝逻辑", t, func() {
		db := createTagRuleTestDB()
		engine := tagengine.NewEngine(db, newTagRuleTestLogger(), nil)
		r := setupTagRuleTestRouter(db, engine)

		Convey("未注册模型的规则创建被拒绝并返回错误码 1107", func() {
			w := doTagRuleRequest(r, http.MethodPost, "/traffic/tag-rule/create", map[string]any{
				"name":    "unregistered-model",
				"model":   "unregistered.model.Type",
				"rego":    tagRuleValidRego,
				"enabled": true,
			})
			So(w.Code, ShouldEqual, http.StatusBadRequest)
			var resp struct {
				Code    int    `json:"code"`
				Summary string `json:"summary"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, manager.CodeTagRuleModelNotRegistered)
		})
	})
}
