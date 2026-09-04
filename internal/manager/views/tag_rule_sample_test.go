// Package routes 提供 HTTP 路由注册和管理功能。
// 本文件包含标签规则验证样例管理路由的集成测试：
// 覆盖样例的列表查询、详情获取、创建（模型注册校验）、
// 更新（model/input 不可变、仅 name/description 可改）与删除。
package views

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	. "github.com/smartystreets/goconvey/convey"

	"zhongkui.com/zhongkui/internal/manager/middlewares"
	"zhongkui.com/zhongkui/internal/manager/response"
	"zhongkui.com/zhongkui/internal/models/tag"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/testdb"
)

// tagRuleSampleTestNamespace 标签规则样例测试专用命名空间，
// 与迁移种子的 default 命名空间 HttpTransport 基线样例（002_rbac_fixtures.sql）隔离，
// 使列表全量计数类断言不受基线数据影响。
const tagRuleSampleTestNamespace = "tag-rule-sample-test"

// createTagRuleSampleTestDB 返回共享 PostgreSQL 测试库（testdb.Core 迁移并清空全部业务表）。
func createTagRuleSampleTestDB(t *testing.T) *database.DB {
	return testdb.Core(t)
}

// setupTagRuleSampleTestRouter 创建测试路由引擎，注入 i18n、日志、数据库与命名空间中间件
// （模拟生产环境 NamespaceMiddleware：固定使用 default 命名空间）。
func setupTagRuleSampleTestRouter(db *database.DB) *gin.Engine {
	return setupTagRuleSampleTestRouterWithNamespace(db, middlewares.DefaultNamespace)
}

// setupTagRuleSampleTestRouterWithNamespace 创建测试路由引擎，注入 i18n、日志、
// 数据库与指定命名空间，用于多命名空间隔离场景。
func setupTagRuleSampleTestRouterWithNamespace(db *database.DB, namespace string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(setupI18nMiddleware())
	r.Use(middlewares.ZapLoggerMiddleware(newTagRuleTestLogger()))
	r.Use(func(c *gin.Context) {
		c.Set(middlewares.CoreDatabaseCtxName, db)
		c.Next()
	})
	r.Use(func(c *gin.Context) {
		c.Set(middlewares.NamespaceCtxName, namespace)
		c.Next()
	})
	registerTagRuleSampleTestRoutes(r)
	return r
}

// registerTagRuleSampleTestRoutes 注册标签规则样例管理路由（与 router.go 的注册保持一致）。
func registerTagRuleSampleTestRoutes(r *gin.Engine) {
	admin := r.Group("/tag-event")
	{
		admin.POST("/tag-rule-sample/list", TagRuleSampleListGet)
		admin.OPTIONS("/tag-rule-sample/list", ListMetaGinHandler[*TagRuleSampleListGetSearchIn, *TagRuleSampleListOut]())
		admin.POST("/tag-rule-sample/create", TagRuleSampleCreate)
		admin.POST("/tag-rule-sample/:id/get", TagRuleSampleDetailGet)
		admin.POST("/tag-rule-sample/:id/update", TagRuleSampleUpdate)
		admin.POST("/tag-rule-sample/delete", TagRuleSampleBatchDelete)
	}
}

// sampleIDPath 拼接样例路由路径。
func sampleIDPath(action string, id uint) string {
	return "/tag-event/tag-rule-sample/" + strconv.FormatUint(uint64(id), 10) + action
}

// TestTagRuleSampleCreateAndDetail 样例创建与详情查询全链路。
func TestTagRuleSampleCreateAndDetail(t *testing.T) {
	Convey("测试标签规则样例创建与详情查询", t, func() {
		db := createTagRuleSampleTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupTagRuleSampleTestRouter(db)

		Convey("合法样例创建成功且详情一致", func() {
			w := doTagRuleRequest(r, http.MethodPost, "/tag-event/tag-rule-sample/create", map[string]any{
				"model":       tagRuleModel,
				"name":        "sample-a",
				"description": "test sample",
				"input": map[string]any{
					"namespace": "default",
					"method":    "POST",
					"path":      "/login",
				},
			})
			So(w.Code, ShouldEqual, http.StatusOK)

			var sample tag.TagRuleSample
			So(db.Where("name = ?", "sample-a").First(&sample).Error, ShouldBeNil)
			So(sample.Model, ShouldEqual, tagRuleModel)

			w = doTagRuleRequest(r, http.MethodPost, sampleIDPath("/get", sample.ID), nil)
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Code int                     `json:"code"`
				Data *TagRuleSampleDetailOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, http.StatusOK)
			So(resp.Data, ShouldNotBeNil)
			So(resp.Data.ID, ShouldEqual, sample.ID)
			So(resp.Data.Model, ShouldEqual, tagRuleModel)
			So(resp.Data.Name, ShouldEqual, "sample-a")
			So(resp.Data.Description, ShouldEqual, "test sample")
			So(resp.Data.Input, ShouldContainKey, "method")
			So(resp.Data.Input["method"], ShouldEqual, "POST")
		})

		Convey("未注册目标模型的样例被拒绝", func() {
			w := doTagRuleRequest(r, http.MethodPost, "/tag-event/tag-rule-sample/create", map[string]any{
				"model": "zhongkui.com/zhongkui/internal/models/unknown.Foo",
				"name":  "sample-bad",
				"input": map[string]any{},
			})
			So(w.Code, ShouldEqual, http.StatusBadRequest)
			var resp struct {
				Code int `json:"code"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, response.CodeTagRuleModelNotRegistered)
		})

		Convey("输入文档无法反序列化为目标模型被拒绝", func() {
			w := doTagRuleRequest(r, http.MethodPost, "/tag-event/tag-rule-sample/create", map[string]any{
				"model": tagRuleModel,
				"name":  "sample-bad-input",
				// method 为字符串字段，数值取值反序列化失败
				"input": map[string]any{"method": 123},
			})
			So(w.Code, ShouldEqual, http.StatusBadRequest)
			var resp struct {
				Code int `json:"code"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, response.CodeTagRuleSampleInputInvalid)
		})

		Convey("HttpTransport 样例输入含加密密文字符串可创建成功", func() {
			w := doTagRuleRequest(r, http.MethodPost, "/tag-event/tag-rule-sample/create", map[string]any{
				"model": httpTransportTypeKey,
				"name":  "sample-crypto",
				"input": map[string]any{
					"method":              "POST",
					"reqHdrAuthorization": "aGVsbG8=",
					"reqHdrCookie":        "",
				},
			})
			So(w.Code, ShouldEqual, http.StatusOK)
		})

		Convey("样例名不合法被拒绝", func() {
			w := doTagRuleRequest(r, http.MethodPost, "/tag-event/tag-rule-sample/create", map[string]any{
				"model": tagRuleModel,
				"name":  "bad name!",
				"input": map[string]any{},
			})
			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})

		Convey("同一模型下重名样例被拒绝落库", func() {
			body := map[string]any{
				"model": tagRuleModel,
				"name":  "dup-name",
				"input": map[string]any{"method": "GET"},
			}
			So(doTagRuleRequest(r, http.MethodPost, "/tag-event/tag-rule-sample/create", body).Code, ShouldEqual, http.StatusOK)
			// 重名撞唯一索引 uq_tag_rule_samples_namespace_name_model_name，按仓库惯例经 500 意外错误返回
			So(doTagRuleRequest(r, http.MethodPost, "/tag-event/tag-rule-sample/create", body).Code, ShouldEqual, http.StatusInternalServerError)
		})
	})
}

// TestTagRuleSampleUpdateImmutability 更新仅允许修改 name/description，model/input 不可变。
func TestTagRuleSampleUpdateImmutability(t *testing.T) {
	Convey("测试标签规则样例更新（model/input 不可变）", t, func() {
		db := createTagRuleSampleTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupTagRuleSampleTestRouter(db)

		input := map[string]any{"namespace": "default", "method": "POST", "path": "/login"}
		w := doTagRuleRequest(r, http.MethodPost, "/tag-event/tag-rule-sample/create", map[string]any{
			"model": tagRuleModel,
			"name":  "sample-up",
			"input": input,
		})
		So(w.Code, ShouldEqual, http.StatusOK)
		var sample tag.TagRuleSample
		So(db.Where("name = ?", "sample-up").First(&sample).Error, ShouldBeNil)
		originalInput := string(sample.Input)

		Convey("更新 name/description 成功且 model/input 不变", func() {
			w = doTagRuleRequest(r, http.MethodPost, sampleIDPath("/update", sample.ID), map[string]any{
				"name":        "sample-up-renamed",
				"description": "updated description",
			})
			So(w.Code, ShouldEqual, http.StatusOK)

			var updated tag.TagRuleSample
			So(db.First(&updated, sample.ID).Error, ShouldBeNil)
			So(updated.Name, ShouldEqual, "sample-up-renamed")
			So(updated.Description, ShouldEqual, "updated description")
			So(updated.Model, ShouldEqual, tagRuleModel)
			So(string(updated.Input), ShouldEqual, originalInput)
		})

		Convey("重命名与既有样例冲突被拒绝", func() {
			So(doTagRuleRequest(r, http.MethodPost, "/tag-event/tag-rule-sample/create", map[string]any{
				"model": tagRuleModel,
				"name":  "existing-name",
				"input": map[string]any{},
			}).Code, ShouldEqual, http.StatusOK)
			w = doTagRuleRequest(r, http.MethodPost, sampleIDPath("/update", sample.ID), map[string]any{
				"name": "existing-name",
			})
			// 撞唯一索引按仓库惯例经 500 意外错误返回
			So(w.Code, ShouldEqual, http.StatusInternalServerError)
		})

		Convey("更新不存在的样例返回 404", func() {
			w = doTagRuleRequest(r, http.MethodPost, sampleIDPath("/update", 99999), map[string]any{
				"name": "whatever",
			})
			So(w.Code, ShouldEqual, http.StatusNotFound)
		})
	})
}

// TestTagRuleSampleDelete 样例删除成功与幂等分支。
func TestTagRuleSampleDelete(t *testing.T) {
	Convey("测试标签规则样例删除", t, func() {
		db := createTagRuleSampleTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupTagRuleSampleTestRouter(db)

		Convey("删除成功且数据落库消失", func() {
			w := doTagRuleRequest(r, http.MethodPost, "/tag-event/tag-rule-sample/create", map[string]any{
				"model": tagRuleModel,
				"name":  "sample-del",
				"input": map[string]any{"method": "DELETE"},
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var sample tag.TagRuleSample
			So(db.Where("name = ?", "sample-del").First(&sample).Error, ShouldBeNil)

			w = doTagRuleRequest(r, http.MethodPost, "/tag-event/tag-rule-sample/delete", map[string]any{"ids": []uint64{uint64(sample.ID)}})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Code int            `json:"code"`
				Data BatchResultOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, http.StatusOK)
			So(resp.Data.Succeeded, ShouldEqual, 1)
			So(len(resp.Data.Failed), ShouldEqual, 0)
			var count int64
			So(db.Model(&tag.TagRuleSample{}).Where("id = ?", sample.ID).Count(&count).Error, ShouldBeNil)
			So(count, ShouldEqual, 0)
		})

		Convey("删除不存在的样例幂等成功", func() {
			w := doTagRuleRequest(r, http.MethodPost, "/tag-event/tag-rule-sample/delete", map[string]any{"ids": []uint64{99999}})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Code int            `json:"code"`
				Data BatchResultOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Succeeded, ShouldEqual, 1)
		})
	})
}

// TestTagRuleSampleList 样例列表分页与按模型过滤。
func TestTagRuleSampleList(t *testing.T) {
	Convey("测试标签规则样例列表查询", t, func() {
		db := createTagRuleSampleTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupTagRuleSampleTestRouterWithNamespace(db, tagRuleSampleTestNamespace)

		Convey("列表返回全部样例且可按模型过滤", func() {
			So(doTagRuleRequest(r, http.MethodPost, "/tag-event/tag-rule-sample/create", map[string]any{
				"model": tagRuleModel,
				"name":  "list-a",
				"input": map[string]any{"method": "GET"},
			}).Code, ShouldEqual, http.StatusOK)
			So(doTagRuleRequest(r, http.MethodPost, "/tag-event/tag-rule-sample/create", map[string]any{
				"model": tagRuleModel,
				"name":  "list-b",
				"input": map[string]any{"method": "POST"},
			}).Code, ShouldEqual, http.StatusOK)
			So(doTagRuleRequest(r, http.MethodPost, "/tag-event/tag-rule-sample/create", map[string]any{
				"model": httpTransportTypeKey,
				"name":  "list-c",
				"input": map[string]any{"method": "PUT"},
			}).Code, ShouldEqual, http.StatusOK)

			Convey("不带过滤返回全量", func() {
				w := doTagRuleRequest(r, http.MethodPost, "/tag-event/tag-rule-sample/list", map[string]any{
					"page":     1,
					"pageSize": 10,
				})
				So(w.Code, ShouldEqual, http.StatusOK)
				var resp struct {
					Code int `json:"code"`
					Data struct {
						Total uint                    `json:"total"`
						Items []*TagRuleSampleListOut `json:"items"`
					} `json:"data"`
				}
				So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
				So(resp.Code, ShouldEqual, http.StatusOK)
				So(resp.Data.Total, ShouldEqual, 3)
				So(len(resp.Data.Items), ShouldEqual, 3)
			})

			Convey("按 model 过滤只返回对应样例", func() {
				w := doTagRuleRequest(r, http.MethodPost, "/tag-event/tag-rule-sample/list", map[string]any{
					"page":     1,
					"pageSize": 10,
					"filterBy": map[string]any{
						"model": map[string]any{"action": "eq", "value": tagRuleModel},
					},
				})
				So(w.Code, ShouldEqual, http.StatusOK)
				var resp struct {
					Code int `json:"code"`
					Data struct {
						Total uint                    `json:"total"`
						Items []*TagRuleSampleListOut `json:"items"`
					} `json:"data"`
				}
				So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
				So(resp.Data.Total, ShouldEqual, 2)
				So(len(resp.Data.Items), ShouldEqual, 2)
			})
		})
	})
}
