// Package routes 提供 HTTP 路由注册和管理功能。
// 本文件包含通知模板（NotificationTemplate）管理路由的集成测试：
// 覆盖事件枚举、列表过滤、创建成功/重复/非法校验、更新（强制 override）、
// 恢复正常默认（restore-default）与渲染预览（render-test）的成功及失败路径。
package views

import (
	// stdlib
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	// third-party
	"github.com/gin-gonic/gin"
	. "github.com/smartystreets/goconvey/convey"

	// internal
	"zhongkui.com/zhongkui/internal/manager/middlewares"
	"zhongkui.com/zhongkui/internal/manager/response"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/notify"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/testdb"
)

// createNotificationTemplateTestDB 返回共享 PostgreSQL 测试库（testdb.Core 迁移并清空全部业务表）。
func createNotificationTemplateTestDB(t *testing.T) *database.DB {
	return testdb.Core(t)
}

// registerNotificationTemplateTestRoutes 注册通知模板管理路由（与 router.go 的注册保持一致）。
func registerNotificationTemplateTestRoutes(admin *gin.RouterGroup) {
	group := admin.Group("/notification/template")
	{
		group.POST("/events", NotificationTemplateEventsGet)
		group.POST("/list", NotificationTemplateListGet)
		group.POST("/create", NotificationTemplateCreate)
		group.POST("/update", NotificationTemplateUpdate)
		group.POST("/restore-default", NotificationTemplateRestoreDefault)
		group.POST("/render-test", NotificationTemplateRenderTest)
	}
}

// setupNotificationTemplateTestRouter 创建测试路由引擎，注入指定命名空间。
func setupNotificationTemplateTestRouter(db *database.DB, namespace string) *gin.Engine {
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
	registerNotificationTemplateTestRoutes(r.Group(""))
	return r
}

// doNotificationTemplateRequest 执行一次通知模板 HTTP 请求并返回响应记录器。
func doNotificationTemplateRequest(r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
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

// notificationTemplateListItems 将列表响应解析为输出项，供断言使用。
func notificationTemplateListItems(r *gin.Engine, body map[string]any) (total uint, items []*NotificationTemplateListOut) {
	w := doNotificationTemplateRequest(r, http.MethodPost, "/notification/template/list", body)
	So(w.Code, ShouldEqual, http.StatusOK)
	var resp struct {
		Code int `json:"code"`
		Data *struct {
			Total uint                           `json:"total"`
			Items []*NotificationTemplateListOut `json:"items"`
		} `json:"data"`
	}
	So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
	So(resp.Code, ShouldEqual, http.StatusOK)
	return resp.Data.Total, resp.Data.Items
}

// TestNotificationTemplateEventsGet 事件模板枚举返回注册表内容。
func TestNotificationTemplateEventsGet(t *testing.T) {
	Convey("测试通知模板事件枚举", t, func() {
		db := createNotificationTemplateTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupNotificationTemplateTestRouter(db, middlewares.DefaultNamespace)

		w := doNotificationTemplateRequest(r, http.MethodPost, "/notification/template/events", nil)
		So(w.Code, ShouldEqual, http.StatusOK)
		var resp struct {
			Code int                            `json:"code"`
			Data *NotificationTemplateEventsOut `json:"data"`
		}
		So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
		So(resp.Code, ShouldEqual, http.StatusOK)
		So(resp.Data.Events, ShouldNotBeEmpty)
		// 注册表中应含标签事件告警，且携带其支持渠道/语言与默认标题
		found := false
		for _, e := range resp.Data.Events {
			if e.Event == core.EventTypeTagEventRecord {
				found = true
				So(e.Template, ShouldNotBeEmpty)
				So(e.Channels, ShouldContain, "*")
				So(e.Languages, ShouldNotBeEmpty)
				So(e.DefaultTitle, ShouldNotBeEmpty)
				So(e.DefaultContent, ShouldNotBeEmpty)
			}
		}
		So(found, ShouldBeTrue)
	})
}

// TestNotificationTemplateList 列表查询与事件/来源过滤。
func TestNotificationTemplateList(t *testing.T) {
	Convey("测试通知模板列表查询", t, func() {
		db := createNotificationTemplateTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		ns := middlewares.DefaultNamespace
		r := setupNotificationTemplateTestRouter(db, ns)
		// 种子化内置默认模板（source=default，zh-Hans 与 en 两行）
		So(notify.SeedDefaultTemplates(db, ns), ShouldBeNil)

		Convey("默认列表返回全部默认模板行", func() {
			total, items := notificationTemplateListItems(r, map[string]any{"pageSize": 100})
			So(total, ShouldEqual, 2)
			So(items, ShouldHaveLength, 2)
			for _, item := range items {
				So(item.Source, ShouldEqual, core.TemplateSourceDefault)
				So(item.Event, ShouldEqual, core.EventTypeTagEventRecord)
			}
		})

		Convey("按事件过滤", func() {
			total, _ := notificationTemplateListItems(r, map[string]any{"event": core.EventTypeTagEventRecord, "pageSize": 100})
			So(total, ShouldEqual, 2)
		})

		Convey("按事件 + 来源过滤返回若干默认模板", func() {
			total, items := notificationTemplateListItems(r, map[string]any{
				"event":    core.EventTypeTagEventRecord,
				"source":   core.TemplateSourceDefault,
				"pageSize": 100,
			})
			So(total, ShouldEqual, 2)
			So(items, ShouldHaveLength, 2)
		})

		Convey("创建来源为 override 的模板后可按来源过滤", func() {
			w := doNotificationTemplateRequest(r, http.MethodPost, "/notification/template/create", map[string]any{
				"event":       core.EventTypeTagEventRecord,
				"channelType": "*",
				"language":    "*",
				"title":       "覆盖标题 {{.RuleName}}",
				"content":     "覆盖正文",
			})
			So(w.Code, ShouldEqual, http.StatusOK)

			total, items := notificationTemplateListItems(r, map[string]any{"source": core.TemplateSourceOverride, "pageSize": 100})
			So(total, ShouldEqual, 1)
			So(items[0].Language, ShouldEqual, "*")
			So(items[0].Source, ShouldEqual, core.TemplateSourceOverride)
		})
	})
}

// TestNotificationTemplateCreate 创建成功、重复 400 与非法事件/渠道 400。
func TestNotificationTemplateCreate(t *testing.T) {
	Convey("测试通知模板创建", t, func() {
		db := createNotificationTemplateTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		ns := middlewares.DefaultNamespace
		r := setupNotificationTemplateTestRouter(db, ns)

		createBody := func() map[string]any {
			return map[string]any{
				"event":       core.EventTypeTagEventRecord,
				"channelType": "*",
				"language":    "en",
				"title":       "Tag Event Alert: {{.RuleName}}",
				"content":     "Override content",
				"description": "custom",
			}
		}

		Convey("创建成功且来源为 override", func() {
			w := doNotificationTemplateRequest(r, http.MethodPost, "/notification/template/create", createBody())
			So(w.Code, ShouldEqual, http.StatusOK)
			var template core.NotificationTemplate
			So(db.Where("namespace_name = ? AND language = ?", ns, "en").First(&template).Error, ShouldBeNil)
			So(template.Event, ShouldEqual, core.EventTypeTagEventRecord)
			So(template.Title, ShouldContainSubstring, "Tag Event Alert")
			So(template.Source, ShouldEqual, core.TemplateSourceOverride)
		})

		Convey("重复组合（事件/渠道/语言）返回 400", func() {
			So(doNotificationTemplateRequest(r, http.MethodPost, "/notification/template/create", createBody()).Code, ShouldEqual, http.StatusOK)
			w := doNotificationTemplateRequest(r, http.MethodPost, "/notification/template/create", createBody())
			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})

		Convey("未知事件返回 400 且不落库", func() {
			body := createBody()
			body["event"] = "no-such-event"
			w := doNotificationTemplateRequest(r, http.MethodPost, "/notification/template/create", body)
			So(w.Code, ShouldEqual, http.StatusBadRequest)
			var count int64
			So(db.Model(&core.NotificationTemplate{}).Count(&count).Error, ShouldBeNil)
			So(count, ShouldEqual, 0)
		})

		Convey("不支持的渠道类型返回 400", func() {
			body := createBody()
			body["channelType"] = "no-such-channel"
			w := doNotificationTemplateRequest(r, http.MethodPost, "/notification/template/create", body)
			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})
	})
}

// TestNotificationTemplateUpdate 更新生效且强制 override，不存在的模板返回 404。
func TestNotificationTemplateUpdate(t *testing.T) {
	Convey("测试通知模板更新", t, func() {
		db := createNotificationTemplateTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		ns := middlewares.DefaultNamespace
		r := setupNotificationTemplateTestRouter(db, ns)
		So(notify.SeedDefaultTemplates(db, ns), ShouldBeNil)

		var defaultTmpl core.NotificationTemplate
		So(db.Where("namespace_name = ? AND language = ?", ns, "zh-Hans").First(&defaultTmpl).Error, ShouldBeNil)
		So(defaultTmpl.Source, ShouldEqual, core.TemplateSourceDefault)

		Convey("更新标题/正文/描述并强制 override", func() {
			w := doNotificationTemplateRequest(r, http.MethodPost, "/notification/template/update", map[string]any{
				"id":          defaultTmpl.ID,
				"title":       "新标题 {{.RuleName}}",
				"content":     "新正文",
				"description": "updated",
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var updated core.NotificationTemplate
			So(db.First(&updated, defaultTmpl.ID).Error, ShouldBeNil)
			So(updated.Title, ShouldContainSubstring, "新标题")
			So(updated.Content, ShouldEqual, "新正文")
			So(updated.Description, ShouldEqual, "updated")
			So(updated.Source, ShouldEqual, core.TemplateSourceOverride)
		})

		Convey("不存在的模板返回 404", func() {
			w := doNotificationTemplateRequest(r, http.MethodPost, "/notification/template/update", map[string]any{
				"id":      99999,
				"title":   "x",
				"content": "y",
			})
			So(w.Code, ShouldEqual, http.StatusNotFound)
		})
	})
}

// TestNotificationTemplateRestoreDefault 恢复默认将事件模板重置为代码内置默认。
func TestNotificationTemplateRestoreDefault(t *testing.T) {
	Convey("测试通知模板恢复默认", t, func() {
		db := createNotificationTemplateTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		ns := middlewares.DefaultNamespace
		r := setupNotificationTemplateTestRouter(db, ns)
		So(notify.SeedDefaultTemplates(db, ns), ShouldBeNil)

		// 覆盖 en 语言默认模板，使其 source=override
		var enTmpl core.NotificationTemplate
		So(db.Where("namespace_name = ? AND language = ?", ns, "en").First(&enTmpl).Error, ShouldBeNil)
		So(db.Model(&core.NotificationTemplate{}).Where("id = ?", enTmpl.ID).
			Updates(map[string]any{"title": "CUSTOM TITLE", "source": core.TemplateSourceOverride}).Error, ShouldBeNil)

		w := doNotificationTemplateRequest(r, http.MethodPost, "/notification/template/restore-default", map[string]any{
			"event":    core.EventTypeTagEventRecord,
			"language": "zh-Hans",
		})
		So(w.Code, ShouldEqual, http.StatusOK)

		// 恢复后 en 语言重新成为默认模板（标题回退到注册表默认）
		var restored core.NotificationTemplate
		So(db.Where("namespace_name = ? AND language = ?", ns, "en").First(&restored).Error, ShouldBeNil)
		So(restored.Source, ShouldEqual, core.TemplateSourceDefault)
		So(restored.Title, ShouldNotEqual, "CUSTOM TITLE")
		So(restored.Title, ShouldNotBeEmpty)

		// 无 override 残留，仅剩该事件的默认模板行
		var overrideCount int64
		So(db.Model(&core.NotificationTemplate{}).Where("namespace_name = ? AND event = ? AND source = ?",
			ns, core.EventTypeTagEventRecord, core.TemplateSourceOverride).Count(&overrideCount).Error, ShouldBeNil)
		So(overrideCount, ShouldEqual, 0)
	})
}

// TestNotificationTemplateRenderTest 渲染预览成功与失败路径。
func TestNotificationTemplateRenderTest(t *testing.T) {
	Convey("测试通知模板渲染预览", t, func() {
		db := createNotificationTemplateTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		ns := middlewares.DefaultNamespace
		r := setupNotificationTemplateTestRouter(db, ns)
		So(notify.SeedDefaultTemplates(db, ns), ShouldBeNil)

		Convey("成功路径：按默认语言渲染内置默认模板", func() {
			w := doNotificationTemplateRequest(r, http.MethodPost, "/notification/template/render-test", map[string]any{
				"event":  core.EventTypeTagEventRecord,
				"params": map[string]any{"RuleName": "告警规则", "Namespace": ns},
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Code int                                `json:"code"`
				Data *NotificationTemplateRenderTestOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, http.StatusOK)
			So(resp.Data.ChannelType, ShouldEqual, "*") // 缺省渠道回退 "*"
			So(resp.Data.Language, ShouldNotBeEmpty)    // 缺省语言回退平台默认
			So(resp.Data.Title, ShouldContainSubstring, "告警规则")
			So(resp.Data.Content, ShouldContainSubstring, "告警规则")
		})

		Convey("未知事件返回 400 + CodeNotificationSendFailed", func() {
			w := doNotificationTemplateRequest(r, http.MethodPost, "/notification/template/render-test", map[string]any{
				"event": "no-such-event",
			})
			So(w.Code, ShouldEqual, http.StatusBadRequest)
			var resp struct {
				Code int `json:"code"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, response.CodeNotificationSendFailed)
		})

		Convey("非法模板返回 400 + CodeNotificationSendFailed", func() {
			// 删除默认 en 行后插入一条非法模板（ChannelType/Language 与默认 zh-Hans 相区分）
			So(db.Where("namespace_name = ? AND language = ?", ns, "en").Delete(&core.NotificationTemplate{}).Error, ShouldBeNil)
			So(db.Create(&core.NotificationTemplate{
				NamespaceName: ns,
				Event:         core.EventTypeTagEventRecord,
				ChannelType:   "*",
				Language:      "en",
				Title:         "bad {{template syntax", // 非法 Go 模板，渲染失败
				Content:       "content",
				Source:        core.TemplateSourceOverride,
			}).Error, ShouldBeNil)

			w := doNotificationTemplateRequest(r, http.MethodPost, "/notification/template/render-test", map[string]any{
				"event":       core.EventTypeTagEventRecord,
				"channelType": "*",
				"language":    "en",
			})
			So(w.Code, ShouldEqual, http.StatusBadRequest)
			var resp struct {
				Code int `json:"code"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, response.CodeNotificationSendFailed)
		})
	})
}
