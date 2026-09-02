// Package routes 提供 HTTP 路由注册和管理功能。
// 本文件包含通知渠道管理路由的集成测试：
// 覆盖渠道类型枚举 schema、创建/详情、列表过滤、更新（名称不可变）、
// 批量删除、批量启停与命名空间隔离。
package views

import (
	// stdlib
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	// third-party
	"github.com/gin-gonic/gin"
	. "github.com/smartystreets/goconvey/convey"

	// internal
	"zhongkui.com/zhongkui/internal/manager/middlewares"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/notify"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/testdb"
)

// createNotificationChannelTestDB 返回共享 PostgreSQL 测试库（testdb.Core 迁移并清空全部业务表）。
func createNotificationChannelTestDB(t *testing.T) *database.DB {
	return testdb.Core(t)
}

// registerNotificationChannelTestRoutes 注册通知渠道管理路由（与 router.go 的注册保持一致）。
func registerNotificationChannelTestRoutes(r *gin.Engine) {
	group := r.Group("/notification/channel")
	{
		group.POST("/list", NotificationChannelListGet)
		group.OPTIONS("/list", ListMetaGinHandler[*NotificationChannelListGetSearchIn, *NotificationChannelListOut]())
		group.POST("/create", NotificationChannelCreate)
		group.POST("/:id/get", NotificationChannelDetailGet)
		group.POST("/:id/update", NotificationChannelUpdate)
		group.POST("/delete", NotificationChannelBatchDelete)
		group.POST("/enable", NotificationChannelBatchSetEnabled(true))
		group.POST("/disable", NotificationChannelBatchSetEnabled(false))
		group.POST("/types", NotificationChannelTypesGet)
	}
}

// setupNotificationChannelTestRouterWithNamespace 创建测试路由引擎，注入 i18n、日志、
// 数据库与指定命名空间，用于验证多租户隔离。
func setupNotificationChannelTestRouterWithNamespace(db *database.DB, namespace string) *gin.Engine {
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
	registerNotificationChannelTestRoutes(r)
	return r
}

// setupNotificationChannelTestRouter 创建测试路由引擎，注入固定 default 命名空间。
func setupNotificationChannelTestRouter(db *database.DB) *gin.Engine {
	return setupNotificationChannelTestRouterWithNamespace(db, middlewares.DefaultNamespace)
}

// doNotificationChannelRequest 执行一次通知渠道 HTTP 请求并返回响应记录器。
func doNotificationChannelRequest(r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
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

// createNotificationChannelByName 通过创建接口落库一条通知渠道，并按其名称查询后返回渠道 ID。
func createNotificationChannelByName(db *database.DB, r *gin.Engine, name string, body map[string]any) uint {
	w := doNotificationChannelRequest(r, http.MethodPost, "/notification/channel/create", body)
	So(w.Code, ShouldEqual, http.StatusOK)
	var channel core.NotificationChannel
	So(db.Where("name = ?", name).First(&channel).Error, ShouldBeNil)
	return channel.ID
}

// seedNotificationChannelByName 直接经数据库落库一条渠道，用于不可经创建接口产生的固定数据
// （如占位符渠道 webhook，前端禁止创建、仅由命名空间种子化）。
func seedNotificationChannelByName(db *database.DB, name, typ string) uint {
	channel := core.NotificationChannel{
		NamespaceName: middlewares.DefaultNamespace,
		Name:          name,
		Type:          typ,
		Enabled:       true,
	}
	So(db.Create(&channel).Error, ShouldBeNil)
	return channel.ID
}

// notificationChannelIDPath 拼接通知渠道路由路径。
func notificationChannelIDPath(action string, id uint) string {
	return "/notification/channel/" + strconv.FormatUint(uint64(id), 10) + action
}

// TestNotificationChannelTypes 渠道类型枚举返回 email 与 webhook 及其配置 schema。
func TestNotificationChannelTypes(t *testing.T) {
	Convey("测试通知渠道类型枚举", t, func() {
		db := createNotificationChannelTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupNotificationChannelTestRouter(db)

		w := doNotificationChannelRequest(r, http.MethodPost, "/notification/channel/types", nil)
		So(w.Code, ShouldEqual, http.StatusOK)
		var resp struct {
			Data NotificationChannelTypesOut `json:"data"`
		}
		So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
		// notify 注册表为包级全局状态，其他测试可能注册 mock 类型，
		// 故按类型键索引断言，而非依赖类型总数与顺序。
		byType := make(map[string]NotificationChannelTypeOut, len(resp.Data.Types))
		for _, typ := range resp.Data.Types {
			byType[typ.Type] = typ
		}

		email, ok := byType[notify.TypeEmail]
		So(ok, ShouldBeTrue)
		So(email.Placeholder, ShouldBeFalse)
		So(email.Config.Channel, ShouldHaveLength, 5)
		So(email.Config.Channel[0].Name, ShouldEqual, "host")
		So(email.Config.Channel[0].Required, ShouldBeTrue)
		So(email.Config.Channel[1].Name, ShouldEqual, "port")
		So(email.Config.Channel[1].DefaultValue, ShouldEqual, 25)
		So(email.Config.Channel[4].Name, ShouldEqual, "from")
		So(email.Config.Target, ShouldHaveLength, 1)
		So(email.Config.Target[0].Name, ShouldEqual, "to")
		So(email.Config.Target[0].Required, ShouldBeTrue)

		wh, ok := byType[notify.TypeWebhook]
		So(ok, ShouldBeTrue)
		// webhook 渠道级配置无字段，为占位符渠道（前端禁止创建、不可删除）
		So(wh.Placeholder, ShouldBeTrue)
		So(wh.Config.Channel, ShouldHaveLength, 0)
		So(wh.Config.Target, ShouldHaveLength, 2)
		So(wh.Config.Target[0].Name, ShouldEqual, "url")
		So(wh.Config.Target[1].Name, ShouldEqual, "headers")
	})
}

// TestNotificationChannelCreateAndDetail 创建与详情查询全链路。
func TestNotificationChannelCreateAndDetail(t *testing.T) {
	Convey("测试通知渠道创建与详情查询", t, func() {
		db := createNotificationChannelTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupNotificationChannelTestRouter(db)

		Convey("合法渠道创建成功且 Config 以字符串原样返回", func() {
			cfg := `{"host":"smtp.example.com","port":465,"username":"alice","password":"secret","from":"alert@example.com"}`
			id := createNotificationChannelByName(db, r, "email-main", map[string]any{
				"name":        "email-main",
				"type":        notify.TypeEmail,
				"enabled":     true,
				"config":      cfg,
				"description": "primary email channel",
			})

			w := doNotificationChannelRequest(r, http.MethodPost, notificationChannelIDPath("/get", id), nil)
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *NotificationChannelDetailOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data, ShouldNotBeNil)
			So(resp.Data.Name, ShouldEqual, "email-main")
			So(resp.Data.Type, ShouldEqual, notify.TypeEmail)
			So(resp.Data.Enabled, ShouldBeTrue)
			// 详情接口返回 base64 密文而非明文；密文可 base64 解码且不等于原明文
			var raw struct {
				Data struct {
					Config string `json:"config"`
				} `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &raw), ShouldBeNil)
			So(raw.Data.Config, ShouldNotBeEmpty)
			So(raw.Data.Config, ShouldNotEqual, cfg)
			_, decErr := base64.StdEncoding.DecodeString(raw.Data.Config)
			So(decErr, ShouldBeNil)
			So(resp.Data.Description, ShouldEqual, "primary email channel")
		})

		Convey("未注册的渠道类型被拒绝且不落库", func() {
			w := doNotificationChannelRequest(r, http.MethodPost, "/notification/channel/create", map[string]any{
				"name":   "bad-type",
				"type":   "slack",
				"config": `{}`,
			})
			So(w.Code, ShouldEqual, http.StatusBadRequest)
			var count int64
			So(db.Model(&core.NotificationChannel{}).Where("name = ?", "bad-type").Count(&count).Error, ShouldBeNil)
			So(count, ShouldEqual, 0)
		})

		Convey("占位符渠道类型被拒绝创建且不落库", func() {
			w := doNotificationChannelRequest(r, http.MethodPost, "/notification/channel/create", map[string]any{
				"name":   "placeholder-webhook",
				"type":   notify.TypeWebhook,
				"config": `{}`,
			})
			So(w.Code, ShouldEqual, http.StatusBadRequest)
			var count int64
			So(db.Model(&core.NotificationChannel{}).Where("name = ?", "placeholder-webhook").Count(&count).Error, ShouldBeNil)
			So(count, ShouldEqual, 0)
		})
	})
}

// TestNotificationChannelList 列表查询与分页过滤。
func TestNotificationChannelList(t *testing.T) {
	Convey("测试通知渠道列表查询", t, func() {
		db := createNotificationChannelTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupNotificationChannelTestRouter(db)
		createNotificationChannelByName(db, r, "email-a", map[string]any{"name": "email-a", "type": notify.TypeEmail, "enabled": true, "config": `{"host":"h1"}`})
		// 注意：模型 Enabled 列的 gorm tag 带 default:true，GORM Create 会跳过零值（false），
		// 因此禁用态经批量停用接口（显式 Update）设置，而非在创建时传 false
		emailB := createNotificationChannelByName(db, r, "email-b", map[string]any{"name": "email-b", "type": notify.TypeEmail, "enabled": true, "config": `{"host":"h2"}`})
		w := doNotificationChannelRequest(r, http.MethodPost, "/notification/channel/disable", map[string]any{"ids": []uint64{uint64(emailB)}})
		So(w.Code, ShouldEqual, http.StatusOK)
		// webhook 为占位符渠道，前端禁止创建，直接经数据库种子一条以参与列表过滤
		seedNotificationChannelByName(db, "wh-c", notify.TypeWebhook)

		Convey("按名称等值过滤", func() {
			w := doNotificationChannelRequest(r, http.MethodPost, "/notification/channel/list", map[string]any{
				"filterBy": map[string]any{
					"name": map[string]any{"action": "eq", "value": "email-a"},
				},
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *struct {
					Total uint                          `json:"total"`
					Items []*NotificationChannelListOut `json:"items"`
				} `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Total, ShouldEqual, 1)
			So(resp.Data.Items, ShouldHaveLength, 1)
			So(resp.Data.Items[0].Name, ShouldEqual, "email-a")
		})

		Convey("按类型等值过滤", func() {
			w := doNotificationChannelRequest(r, http.MethodPost, "/notification/channel/list", map[string]any{
				"filterBy": map[string]any{
					"type": map[string]any{"action": "eq", "value": notify.TypeEmail},
				},
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *struct {
					Total uint                          `json:"total"`
					Items []*NotificationChannelListOut `json:"items"`
				} `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Total, ShouldEqual, 2)
		})

		Convey("按启用状态等值过滤", func() {
			w := doNotificationChannelRequest(r, http.MethodPost, "/notification/channel/list", map[string]any{
				"filterBy": map[string]any{
					"enabled": map[string]any{"action": "eq", "value": true},
				},
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *struct {
					Total uint                          `json:"total"`
					Items []*NotificationChannelListOut `json:"items"`
				} `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Total, ShouldEqual, 2)
		})

		Convey("默认分页返回全量列表且 total 来自 items 嵌入的 BaseTotalOut", func() {
			w := doNotificationChannelRequest(r, http.MethodPost, "/notification/channel/list", map[string]any{
				"pageSize": 10,
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *struct {
					Total uint                          `json:"total"`
					Items []*NotificationChannelListOut `json:"items"`
				} `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Total, ShouldEqual, 3)
			So(resp.Data.Items, ShouldHaveLength, 3)
		})
	})
}

// TestNotificationChannelUpdate 更新配置与启用状态生效，且名称不可变。
func TestNotificationChannelUpdate(t *testing.T) {
	Convey("测试通知渠道更新", t, func() {
		db := createNotificationChannelTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupNotificationChannelTestRouter(db)
		id := createNotificationChannelByName(db, r, "email-update", map[string]any{
			"name":    "email-update",
			"type":    notify.TypeEmail,
			"enabled": true,
			"config":  `{"host":"old"}`,
		})

		w := doNotificationChannelRequest(r, http.MethodPost, notificationChannelIDPath("/update", id), map[string]any{
			// name/type 字段虽传入，但更新不接受，应保持不变
			"name":        "renamed",
			"type":        notify.TypeWebhook,
			"enabled":     false,
			"config":      `{"host":"new"}`,
			"description": "updated",
		})
		So(w.Code, ShouldEqual, http.StatusOK)

		var channel core.NotificationChannel
		So(db.First(&channel, id).Error, ShouldBeNil)
		So(channel.Name, ShouldEqual, "email-update")
		So(channel.Type, ShouldEqual, notify.TypeEmail)
		So(channel.Enabled, ShouldBeFalse)
		// 数据库中的 config 已加密存储；经模型解密读取得到更新后的明文
		So(channel.ConfigData(), ShouldEqual, []byte(`{"host":"new"}`))
		So(channel.Description, ShouldEqual, "updated")
	})
}

// TestNotificationChannelBatchDelete 批量删除成功且幂等；占位符渠道（webhook）禁止删除。
func TestNotificationChannelBatchDelete(t *testing.T) {
	Convey("测试通知渠道批量删除", t, func() {
		db := createNotificationChannelTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupNotificationChannelTestRouter(db)
		id1 := createNotificationChannelByName(db, r, "del-1", map[string]any{"name": "del-1", "type": notify.TypeEmail, "config": `{}`})
		// 占位符渠道（webhook）前端禁止创建，直接经数据库种子一条以验证其不可删除
		whID := seedNotificationChannelByName(db, "del-wh", notify.TypeWebhook)

		Convey("非占位渠道批量删除成功", func() {
			w := doNotificationChannelRequest(r, http.MethodPost, "/notification/channel/delete", map[string]any{"ids": []uint64{uint64(id1)}})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Code int            `json:"code"`
				Data BatchResultOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Succeeded, ShouldEqual, 1)
			So(len(resp.Data.Failed), ShouldEqual, 0)

			var count int64
			So(db.Model(&core.NotificationChannel{}).Count(&count).Error, ShouldBeNil)
			So(count, ShouldEqual, 1) // 仅剩占位渠道
		})

		Convey("占位符渠道被拒绝删除并记入失败", func() {
			// 先删除非占位渠道，再验证占位渠道无法删除，二者在各自 scope 内独立
			w := doNotificationChannelRequest(r, http.MethodPost, "/notification/channel/delete", map[string]any{"ids": []uint64{uint64(id1)}})
			So(w.Code, ShouldEqual, http.StatusOK)

			w = doNotificationChannelRequest(r, http.MethodPost, "/notification/channel/delete", map[string]any{"ids": []uint64{uint64(whID)}})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data BatchResultOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Succeeded, ShouldEqual, 0)
			So(len(resp.Data.Failed), ShouldEqual, 1)
			So(resp.Data.Failed[0].Reason, ShouldContainSubstring, "placeholder")

			// 占位渠道未被删除（id1 已删，仅剩占位渠道）
			var count int64
			So(db.Model(&core.NotificationChannel{}).Count(&count).Error, ShouldBeNil)
			So(count, ShouldEqual, 1)
		})

		Convey("重复删除与不存在 ID 幂等成功", func() {
			w := doNotificationChannelRequest(r, http.MethodPost, "/notification/channel/delete", map[string]any{"ids": []uint64{uint64(id1), 99999}})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data BatchResultOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Succeeded, ShouldEqual, 2)
		})
	})
}

// TestNotificationChannelBatchSetEnabled 批量启用/停用状态切换且幂等。
func TestNotificationChannelBatchSetEnabled(t *testing.T) {
	Convey("测试通知渠道批量启停", t, func() {
		db := createNotificationChannelTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupNotificationChannelTestRouter(db)
		id := createNotificationChannelByName(db, r, "io-1", map[string]any{"name": "io-1", "type": notify.TypeEmail, "enabled": true, "config": `{}`})

		w := doNotificationChannelRequest(r, http.MethodPost, "/notification/channel/disable", map[string]any{"ids": []uint64{uint64(id)}})
		So(w.Code, ShouldEqual, http.StatusOK)
		var resp struct {
			Data BatchResultOut `json:"data"`
		}
		So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
		So(resp.Data.Succeeded, ShouldEqual, 1)
		var channel core.NotificationChannel
		So(db.First(&channel, id).Error, ShouldBeNil)
		So(channel.Enabled, ShouldBeFalse)

		// 重复停用幂等成功
		w = doNotificationChannelRequest(r, http.MethodPost, "/notification/channel/disable", map[string]any{"ids": []uint64{uint64(id)}})
		So(w.Code, ShouldEqual, http.StatusOK)

		w = doNotificationChannelRequest(r, http.MethodPost, "/notification/channel/enable", map[string]any{"ids": []uint64{uint64(id)}})
		So(w.Code, ShouldEqual, http.StatusOK)
		So(db.First(&channel, id).Error, ShouldBeNil)
		So(channel.Enabled, ShouldBeTrue)
	})
}

// TestNotificationChannelNamespaceIsolation 渠道按命名空间隔离，跨命名空间不可见。
func TestNotificationChannelNamespaceIsolation(t *testing.T) {
	Convey("测试通知渠道命名空间隔离", t, func() {
		db := createNotificationChannelTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupNotificationChannelTestRouter(db)
		id := createNotificationChannelByName(db, r, "ns-1", map[string]any{"name": "ns-1", "type": notify.TypeEmail, "config": `{}`})

		Convey("其他命名空间列表不可见 default 命名空间渠道", func() {
			rOther := setupNotificationChannelTestRouterWithNamespace(db, "other-ns")
			w := doNotificationChannelRequest(rOther, http.MethodPost, "/notification/channel/list", map[string]any{"pageSize": 10})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *struct {
					Total uint                          `json:"total"`
					Items []*NotificationChannelListOut `json:"items"`
				} `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Total, ShouldEqual, 0)
		})

		Convey("其他命名空间详情跨命名空间返回 404", func() {
			rOther := setupNotificationChannelTestRouterWithNamespace(db, "other-ns")
			w := doNotificationChannelRequest(rOther, http.MethodPost, notificationChannelIDPath("/get", id), nil)
			So(w.Code, ShouldEqual, http.StatusNotFound)
		})

		Convey("本命名空间内访问仍正常", func() {
			w := doNotificationChannelRequest(r, http.MethodPost, notificationChannelIDPath("/get", id), nil)
			So(w.Code, ShouldEqual, http.StatusOK)
		})
	})
}
