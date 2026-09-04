// Package routes 提供 HTTP 路由注册和管理功能。
// 本文件包含通知目标（NotificationTarget）管理路由的集成测试：
// 覆盖目标创建/详情/列表/更新/批量删除/批量启停、命名空间隔离与 test-send 的成功及失败路径。
package views

import (
	// stdlib
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"unsafe"

	// third-party
	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	. "github.com/smartystreets/goconvey/convey"

	// internal
	"zhongkui.com/zhongkui/internal/manager/middlewares"
	"zhongkui.com/zhongkui/internal/manager/response"
	core "zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/message"
	"zhongkui.com/zhongkui/internal/pkg/pipeline"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/pkg/testdb"
)

// mockNotificationChannelType 测试用 mock 渠道类型键，仅在本测试文件内注册。
const mockNotificationChannelType = "test-send-mock"

// mockNotificationChannel 测试用 mock 渠道实现，Send 固定返回消息 ID "mock-msg-1"。
type mockNotificationChannel struct{}

// Type 返回渠道类型键。
func (m *mockNotificationChannel) Type() string { return mockNotificationChannelType }

// Send 固定返回消息 ID "mock-msg-1"，用于验证 test-send 成功路径。
func (m *mockNotificationChannel) Send(ctx context.Context, target *message.Target, msg *message.RenderedMessage) (string, error) {
	return "mock-msg-1", nil
}

// init 将测试用 mock 渠道注册进 notify 注册表。
// 注册表为包级全局状态且重复注册会 panic，故仅在 init 注册一次。
func init() {
	message.Register(mockNotificationChannelType, func(_ json.RawMessage) (message.Channel, error) {
		return &mockNotificationChannel{}, nil
	})
}

// createNotificationTargetTestDB 返回共享 PostgreSQL 测试库（testdb.Core 迁移并清空全部业务表）。
func createNotificationTargetTestDB(t *testing.T) *database.DB {
	return testdb.Core(t)
}

// insertTestNamespace 直接落库一个命名空间，供依赖 namespace_name 外键的渠道/目标/记录测试建立父行。
func insertTestNamespace(db *database.DB, name string) {
	So(db.Create(&core.Namespace{Name: name}).Error, ShouldBeNil)
}

// registerNotificationTargetTestRoutes 注册通知目标管理路由（与 router.go 的注册保持一致）。
func registerNotificationTargetTestRoutes(r *gin.Engine) {
	admin := r.Group("/notification/target")
	{
		admin.POST("/list", NotificationTargetListGet)
		admin.OPTIONS("/list", ListMetaGinHandler[*NotificationTargetListGetSearchIn, *NotificationTargetListOut]())
		admin.POST("/create", NotificationTargetCreate)
		admin.POST("/:id/get", NotificationTargetDetailGet)
		admin.POST("/:id/update", NotificationTargetUpdate)
		admin.POST("/delete", NotificationTargetBatchDelete)
		admin.POST("/enable", NotificationTargetBatchSetEnabled(true))
		admin.POST("/disable", NotificationTargetBatchSetEnabled(false))
		admin.POST("/:id/test-send", NotificationTargetTestSend)
		admin.POST("/:id/send", NotificationTargetSend)
	}
}

// setupNotificationTargetTestRouter 创建测试路由引擎，注入指定命名空间。
func setupNotificationTargetTestRouter(db *database.DB, namespace string) *gin.Engine {
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
	registerNotificationTargetTestRoutes(r)
	return r
}

// doNotificationTargetRequest 执行一次通知目标 HTTP 请求并返回响应记录器。
func doNotificationTargetRequest(r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
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

// insertNotificationTargetTestChannel 直接落库一条通知渠道并返回该渠道。
func insertNotificationTargetTestChannel(db *database.DB, namespace, name, typ, config string, enabled bool) *core.NotificationChannel {
	ch := &core.NotificationChannel{
		NamespaceName: namespace,
		Name:          name,
		Type:          typ,
		Enabled:       enabled,
		Config:        serializer.NewCrypto([]byte(config)),
		Description:   "test channel",
	}
	So(db.Create(ch).Error, ShouldBeNil)
	// Enabled 带 GORM default:true，Create 时会省略零值 false 交由 DB 填 true，
	// 故禁用态需显式 Update 覆盖，否则禁用的渠道会以启用态落库
	if !enabled {
		So(db.Model(ch).Update("enabled", false).Error, ShouldBeNil)
	}
	return ch
}

// notificationTargetCreateBody 构造一条目标创建请求体。
func notificationTargetCreateBody(name string, channelID uint, overrides map[string]any) map[string]any {
	body := map[string]any{
		"name":      name,
		"channelId": channelID,
		"enabled":   true,
		"config":    `{"to":"ops@example.com"}`,
		"language":  "zh-Hans",
	}
	for k, v := range overrides {
		body[k] = v
	}
	return body
}

// notificationTargetIDPath 拼接通知目标路由路径。
func notificationTargetIDPath(action string, id uint) string {
	return "/notification/target/" + strconv.FormatUint(uint64(id), 10) + action
}

// TestNotificationTargetCreateAndDetail 创建与详情查询全链路。
func TestNotificationTargetCreateAndDetail(t *testing.T) {
	Convey("测试通知目标创建与详情查询", t, func() {
		db := createNotificationTargetTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		ch := insertNotificationTargetTestChannel(db, middlewares.DefaultNamespace, "ch-email", message.TypeEmail, `{"host":"smtp.example.com","from":"alert@example.com"}`, true)
		r := setupNotificationTargetTestRouter(db, middlewares.DefaultNamespace)

		Convey("目标创建成功且详情返回派生 Type 与渠道信息", func() {
			w := doNotificationTargetRequest(r, http.MethodPost, "/notification/target/create",
				notificationTargetCreateBody("target-a", ch.ID, nil))
			So(w.Code, ShouldEqual, http.StatusOK)

			var target core.NotificationTarget
			So(db.Where("name = ?", "target-a").First(&target).Error, ShouldBeNil)
			So(target.Type, ShouldEqual, message.TypeEmail)
			So(target.Language, ShouldEqual, "zh-Hans")

			w = doNotificationTargetRequest(r, http.MethodPost, notificationTargetIDPath("/get", target.ID), nil)
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Code int                          `json:"code"`
				Data *NotificationTargetDetailOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, http.StatusOK)
			So(resp.Data.Name, ShouldEqual, "target-a")
			So(resp.Data.ChannelID, ShouldEqual, ch.ID)
			So(resp.Data.ChannelName, ShouldEqual, "ch-email")
			So(resp.Data.ChannelType, ShouldEqual, message.TypeEmail)
			So(resp.Data.ChannelEnabled, ShouldBeTrue)
			So(resp.Data.Type, ShouldEqual, message.TypeEmail)
			So(resp.Data.Enabled, ShouldBeTrue)
			So(resp.Data.Language, ShouldEqual, "zh-Hans")
		})

		Convey("目标引用不存在的渠道被拒绝（404）且不落库", func() {
			w := doNotificationTargetRequest(r, http.MethodPost, "/notification/target/create",
				notificationTargetCreateBody("target-bad", 99999, nil))
			So(w.Code, ShouldEqual, http.StatusNotFound)

			var count int64
			So(db.Model(&core.NotificationTarget{}).Where("name = ?", "target-bad").Count(&count).Error, ShouldBeNil)
			So(count, ShouldEqual, 0)
		})
	})
}

// TestNotificationTargetList 列表查询与分页过滤。
func TestNotificationTargetList(t *testing.T) {
	Convey("测试通知目标列表查询", t, func() {
		db := createNotificationTargetTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		chA := insertNotificationTargetTestChannel(db, middlewares.DefaultNamespace, "ch-a", message.TypeEmail, `{}`, true)
		chB := insertNotificationTargetTestChannel(db, middlewares.DefaultNamespace, "ch-b", message.TypeWebhook, `{}`, false)
		r := setupNotificationTargetTestRouter(db, middlewares.DefaultNamespace)
		createNotificationTargetTarget(r, "tg-a", chA.ID, true, "zh-Hans")
		createNotificationTargetTarget(r, "tg-b", chB.ID, false, "en")
		createNotificationTargetTarget(r, "tg-c", chA.ID, true, "zh-Hans")

		Convey("按名称等值过滤", func() {
			w := doNotificationTargetRequest(r, http.MethodPost, "/notification/target/list", map[string]any{
				"filterBy": map[string]any{"name": map[string]any{"action": "eq", "value": "tg-b"}},
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *struct {
					Total uint                         `json:"total"`
					Items []*NotificationTargetListOut `json:"items"`
				} `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Total, ShouldEqual, 1)
			So(resp.Data.Items, ShouldHaveLength, 1)
			So(resp.Data.Items[0].Name, ShouldEqual, "tg-b")
			So(resp.Data.Items[0].ChannelName, ShouldEqual, "ch-b")
			So(resp.Data.Items[0].ChannelType, ShouldEqual, message.TypeWebhook)
		})

		Convey("按渠道 ID 等值过滤", func() {
			w := doNotificationTargetRequest(r, http.MethodPost, "/notification/target/list", map[string]any{
				"filterBy": map[string]any{"channelId": map[string]any{"action": "eq", "value": chA.ID}},
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *struct {
					Total uint                         `json:"total"`
					Items []*NotificationTargetListOut `json:"items"`
				} `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Total, ShouldEqual, 2)
		})

		Convey("按语言等值过滤", func() {
			w := doNotificationTargetRequest(r, http.MethodPost, "/notification/target/list", map[string]any{
				"filterBy": map[string]any{"language": map[string]any{"action": "eq", "value": "en"}},
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *struct {
					Total uint                         `json:"total"`
					Items []*NotificationTargetListOut `json:"items"`
				} `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Total, ShouldEqual, 1)
			So(resp.Data.Items[0].Name, ShouldEqual, "tg-b")
		})

		Convey("列表选项接口返回过滤与列元数据", func() {
			w := doNotificationTargetRequest(r, http.MethodOptions, "/notification/target/list", nil)
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

// createNotificationTargetTarget 通过创建接口落库一条通知目标，返回目标 ID。
func createNotificationTargetTarget(r *gin.Engine, name string, channelID uint, enabled bool, language string) uint {
	w := doNotificationTargetRequest(r, http.MethodPost, "/notification/target/create",
		notificationTargetCreateBody(name, channelID, map[string]any{
			"enabled":  enabled,
			"language": language,
		}))
	So(w.Code, ShouldEqual, http.StatusOK)
	return notificationTargetIDByName(r, name)
}

// notificationTargetIDByName 按名称查询目标的 ID（测试辅助，直接查库）。
func notificationTargetIDByName(r *gin.Engine, name string) uint {
	// 命名空间固定 default，但此处直接通过列表接口获取 ID 更贴近集成语义；
	// 为简化直接遍历列表取匹配项。
	resp := doNotificationTargetRequest(r, http.MethodPost, "/notification/target/list", map[string]any{"pageSize": 100})
	var out struct {
		Data struct {
			Items []*NotificationTargetListOut `json:"items"`
		} `json:"data"`
	}
	So(json.Unmarshal(resp.Body.Bytes(), &out), ShouldBeNil)
	for _, item := range out.Data.Items {
		if item.Name == name {
			return item.ID
		}
	}
	return 0
}

// TestNotificationTargetUpdate 更新配置/语言生效且渠道不可变。
func TestNotificationTargetUpdate(t *testing.T) {
	Convey("测试通知目标更新", t, func() {
		db := createNotificationTargetTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		chA := insertNotificationTargetTestChannel(db, middlewares.DefaultNamespace, "ch-a", message.TypeEmail, `{}`, true)
		chB := insertNotificationTargetTestChannel(db, middlewares.DefaultNamespace, "ch-b", message.TypeWebhook, `{}`, true)
		r := setupNotificationTargetTestRouter(db, middlewares.DefaultNamespace)
		targetID := createNotificationTargetTarget(r, "tg-up", chA.ID, true, "zh-Hans")

		Convey("更新配置与语言生效，渠道 ID 保持不变", func() {
			w := doNotificationTargetRequest(r, http.MethodPost, notificationTargetIDPath("/update", targetID), map[string]any{
				"channelId":   chB.ID, // 不可变字段，应被忽略
				"enabled":     false,
				"config":      `{"url":"https://example.com/hook"}`,
				"language":    "en",
				"description": "updated desc",
			})
			So(w.Code, ShouldEqual, http.StatusOK)

			var target core.NotificationTarget
			So(db.First(&target, targetID).Error, ShouldBeNil)
			So(target.ChannelID, ShouldEqual, chA.ID) // 渠道未变更
			So(target.Type, ShouldEqual, message.TypeEmail)
			So(target.Enabled, ShouldBeFalse)
			So(target.Language, ShouldEqual, "en")
			So(target.Description, ShouldEqual, "updated desc")
			So(string(target.ConfigData()), ShouldContainSubstring, "example.com/hook")
		})
	})
}

// TestNotificationTargetBatchDeleteAndSetEnabled 批量删除与批量启停。
func TestNotificationTargetBatchDeleteAndSetEnabled(t *testing.T) {
	Convey("测试通知目标批量删除与批量启停", t, func() {
		db := createNotificationTargetTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		ch := insertNotificationTargetTestChannel(db, middlewares.DefaultNamespace, "ch-a", message.TypeEmail, `{}`, true)
		r := setupNotificationTargetTestRouter(db, middlewares.DefaultNamespace)
		id1 := createNotificationTargetTarget(r, "tg-d1", ch.ID, true, "zh-Hans")
		id2 := createNotificationTargetTarget(r, "tg-d2", ch.ID, true, "zh-Hans")

		Convey("批量停用生效且幂等", func() {
			w := doNotificationTargetRequest(r, http.MethodPost, "/notification/target/disable", map[string]any{"ids": []uint64{uint64(id1), uint64(id2)}})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Code int            `json:"code"`
				Data BatchResultOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Succeeded, ShouldEqual, 2)
			var target core.NotificationTarget
			So(db.First(&target, id1).Error, ShouldBeNil)
			So(target.Enabled, ShouldBeFalse)

			// 幂等
			w = doNotificationTargetRequest(r, http.MethodPost, "/notification/target/disable", map[string]any{"ids": []uint64{uint64(id1)}})
			So(w.Code, ShouldEqual, http.StatusOK)

			// 重新启用
			w = doNotificationTargetRequest(r, http.MethodPost, "/notification/target/enable", map[string]any{"ids": []uint64{uint64(id1)}})
			So(w.Code, ShouldEqual, http.StatusOK)
			So(db.First(&target, id1).Error, ShouldBeNil)
			So(target.Enabled, ShouldBeTrue)
		})

		Convey("批量删除生效且不存在视为成功跳过", func() {
			w := doNotificationTargetRequest(r, http.MethodPost, "/notification/target/delete", map[string]any{"ids": []uint64{uint64(id1), uint64(id2), 99999}})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Code int            `json:"code"`
				Data BatchResultOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Succeeded, ShouldEqual, 3)
			var count int64
			So(db.Model(&core.NotificationTarget{}).Where("id IN ?", []uint{id1, id2}).Count(&count).Error, ShouldBeNil)
			So(count, ShouldEqual, 0)
		})
	})
}

// TestNotificationTargetNamespaceIsolation 目标按命名空间隔离，跨命名空间访问视为不存在。
func TestNotificationTargetNamespaceIsolation(t *testing.T) {
	Convey("测试通知目标命名空间隔离", t, func() {
		db := createNotificationTargetTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		insertTestNamespace(db, "ns-a")
		ch := insertNotificationTargetTestChannel(db, "ns-a", "ch-a", message.TypeEmail, `{}`, true)
		rA := setupNotificationTargetTestRouter(db, "ns-a")
		rB := setupNotificationTargetTestRouter(db, "ns-b")
		targetID := createNotificationTargetTarget(rA, "tg-ns", ch.ID, true, "zh-Hans")

		Convey("其他命名空间目标不存在于 ns-b，本命名空间正常", func() {
			w := doNotificationTargetRequest(rB, http.MethodPost, notificationTargetIDPath("/get", targetID), nil)
			So(w.Code, ShouldEqual, http.StatusNotFound)

			w = doNotificationTargetRequest(rA, http.MethodPost, notificationTargetIDPath("/get", targetID), nil)
			So(w.Code, ShouldEqual, http.StatusOK)
		})
	})
}

// TestNotificationTargetTestSend test-send 成功路径与失败路径。
func TestNotificationTargetTestSend(t *testing.T) {
	Convey("测试通知目标 test-send", t, func() {
		db := createNotificationTargetTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		mockCh := insertNotificationTargetTestChannel(db, middlewares.DefaultNamespace, "ch-mock", mockNotificationChannelType, `{}`, true)
		badCh := insertNotificationTargetTestChannel(db, middlewares.DefaultNamespace, "ch-bad", message.TypeEmail, `not-json`, true)
		r := setupNotificationTargetTestRouter(db, middlewares.DefaultNamespace)
		mockTarget := createNotificationTargetTarget(r, "tg-mock", mockCh.ID, true, "zh-Hans")
		badTarget := createNotificationTargetTarget(r, "tg-bad", badCh.ID, true, "zh-Hans")

		Convey("成功路径：返回 MessageID、渲染文本、实际语言与所用渠道/目标参数", func() {
			w := doNotificationTargetRequest(r, http.MethodPost, notificationTargetIDPath("/test-send", mockTarget), nil)
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Code int                            `json:"code"`
				Data *NotificationTargetTestSendOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, http.StatusOK)
			So(resp.Data.MessageID, ShouldEqual, "mock-msg-1")
			So(resp.Data.Title, ShouldNotBeEmpty)
			So(resp.Data.Content, ShouldNotBeEmpty)
			So(resp.Data.Language, ShouldEqual, "zh-Hans")
			// 本次通知行为所用参数回显：渠道与目标信息 + 配置密文（base64，可经 plaintext 解密）
			So(resp.Data.Channel.Name, ShouldEqual, "ch-mock")
			So(resp.Data.Channel.Type, ShouldEqual, mockNotificationChannelType)
			So(resp.Data.Channel.Enabled, ShouldBeTrue)
			So(resp.Data.Channel.Config, ShouldNotBeNil)
			So(resp.Data.Target.Name, ShouldEqual, "tg-mock")
			So(resp.Data.Target.Language, ShouldEqual, "zh-Hans")
			So(resp.Data.Target.Enabled, ShouldBeTrue)
			So(resp.Data.Target.Config, ShouldNotBeNil)
		})

		Convey("渠道配置非法时返回正常业务错误（400 + CodeNotificationSendFailed，details 含具体原因）", func() {
			w := doNotificationTargetRequest(r, http.MethodPost, notificationTargetIDPath("/test-send", badTarget), nil)
			So(w.Code, ShouldEqual, http.StatusBadRequest)
			var resp struct {
				Code    int    `json:"code"`
				Summary string `json:"summary"`
				Details string `json:"details"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, response.CodeNotificationSendFailed)
			So(resp.Summary, ShouldNotBeEmpty)
			So(resp.Details, ShouldNotBeEmpty)
		})
	})
}

// fakePipelineEnqueuer 测试用假 asynq 投递客户端，捕获最后一次被投递的任务供断言。
// fakePipelineEnqueuer is a stubbed asynq enqueuer capturing the last enqueued task.
type fakePipelineEnqueuer struct {
	mu       sync.Mutex
	lastTask *asynq.Task
}

// EnqueueContext 记录被投递的任务并返回成功，不真正连接 Redis。
func (f *fakePipelineEnqueuer) EnqueueContext(_ context.Context, task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastTask = task
	return nil, nil
}

// Close 空实现，满足投递客户端接口。
func (f *fakePipelineEnqueuer) Close() error { return nil }

// newFakePipelineClient 构造一个投递行为被桩替换的 *pipeline.Client。
// pipeline.Client 的 enqueuer 字段为包内非导出字段，且 NewClient 需要真实 Redis 客户端，
// 单元测试通过 reflect.NewAt + unsafe 绕过非导出字段限制注入假 enqueuer，
// 使 EnqueueContext 在测试中直接命中假实现而不触达 Redis。
// newFakePipelineClient builds a *pipeline.Client whose delivery is stubbed. The
// enqueuer field is package-private and NewClient needs a real Redis client, so the
// test injects the fake via reflect.NewAt + unsafe to bypass the unexported-field
// restriction, making EnqueueContext hit the fake directly without touching Redis.
func newFakePipelineClient(f *fakePipelineEnqueuer) *pipeline.Client {
	client := &pipeline.Client{}
	field := reflect.ValueOf(client).Elem().FieldByName("cli")
	addressable := reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem()
	addressable.Set(reflect.ValueOf(f))
	return client
}

// setupNotificationTargetSendRouter 创建用于 /send 路由的测试引擎，
// 与 setupNotificationTargetTestRouter 一致，但追加注入假 pipeline 投递客户端中间件。
func setupNotificationTargetSendRouter(db *database.DB, namespace string, enq *fakePipelineEnqueuer) *gin.Engine {
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
	r.Use(middlewares.PipelineClientMiddleware(newFakePipelineClient(enq)))
	r.POST("/notification/target/:id/send", NotificationTargetSend)
	return r
}

// notificationTargetSendBody 构造一次单目标发送的请求体。
func notificationTargetSendBody() map[string]any {
	return map[string]any{
		"eventType": "tag_event",
		"eventID":   101,
		"template":  "test",
		"params":    map[string]any{"RuleID": 7},
	}
}

// insertNotificationTargetTestTarget 直接落库一条通知目标（跳过 HTTP 创建接口，
// 规避 create 接口 + 按名查 ID 在共享引擎下的不确定性），返回该目标。
// insertNotificationTargetTestTarget inserts a notification target directly into the
// database (bypassing the HTTP create endpoint and the name-based ID lookup, which are
// unreliable on a shared engine), returning the target.
func insertNotificationTargetTestTarget(db *database.DB, namespace, name string, ch *core.NotificationChannel, enabled bool) *core.NotificationTarget {
	t := &core.NotificationTarget{
		NamespaceName: namespace,
		Name:          name,
		ChannelID:     ch.ID,
		Type:          ch.Type,
		Enabled:       enabled,
		Config:        serializer.NewCrypto([]byte(`{"to":"ops@example.com"}`)),
		Language:      "zh-Hans",
		Description:   "test target",
	}
	So(db.Create(t).Error, ShouldBeNil)
	// 与渠道同理：Enabled 的 GORM default:true 会在 Create 时把零值 false 交给 DB 填 true，
	// 禁用态需显式 Update 覆盖
	if !enabled {
		So(db.Model(t).Update("enabled", false).Error, ShouldBeNil)
	}
	return t
}

// TestNotificationTargetSend 向单个通知目标发送通知：成功投递 / 目标缺失 / 目标被禁用。
func TestNotificationTargetSend(t *testing.T) {
	Convey("测试通知目标单目标发送", t, func() {
		db := createNotificationTargetTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		chEnabled := insertNotificationTargetTestChannel(db, middlewares.DefaultNamespace, "ch-on", message.TypeEmail, `{}`, true)
		chDisabled := insertNotificationTargetTestChannel(db, middlewares.DefaultNamespace, "ch-off", message.TypeEmail, `{}`, false)
		okTarget := insertNotificationTargetTestTarget(db, middlewares.DefaultNamespace, "tg-send-ok", chEnabled, true)
		disabledTarget := insertNotificationTargetTestTarget(db, middlewares.DefaultNamespace, "tg-send-off", chEnabled, false)
		r := setupNotificationTargetTestRouter(db, middlewares.DefaultNamespace)

		Convey("目标缺失时返回 404", func() {
			w := doNotificationTargetRequest(r, http.MethodPost, "/notification/target/99999/send", notificationTargetSendBody())
			So(w.Code, ShouldEqual, http.StatusNotFound)
		})

		Convey("目标被禁用时返回 400 + CodeNotificationSendFailed", func() {
			w := doNotificationTargetRequest(r, http.MethodPost, notificationTargetIDPath("/send", disabledTarget.ID), notificationTargetSendBody())
			So(w.Code, ShouldEqual, http.StatusBadRequest)
			var resp struct {
				Code int `json:"code"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, response.CodeNotificationSendFailed)
		})

		Convey("所属渠道被禁用时返回 400 + CodeNotificationSendFailed", func() {
			w := doNotificationTargetRequest(r, http.MethodPost, notificationTargetIDPath("/send", insertNotificationTargetTestTarget(db, middlewares.DefaultNamespace, "tg-ch-off", chDisabled, true).ID), notificationTargetSendBody())
			So(w.Code, ShouldEqual, http.StatusBadRequest)
			var resp struct {
				Code int `json:"code"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, response.CodeNotificationSendFailed)
		})

		Convey("请求体缺少必填字段时返回 400", func() {
			w := doNotificationTargetRequest(r, http.MethodPost, notificationTargetIDPath("/send", okTarget.ID), map[string]any{})
			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})

		Convey("目标可用且请求合法时返回 200 并投递 notify:send:target 异步任务", func() {
			enq := &fakePipelineEnqueuer{}
			rSend := setupNotificationTargetSendRouter(db, middlewares.DefaultNamespace, enq)
			w := doNotificationTargetRequest(rSend, http.MethodPost, notificationTargetIDPath("/send", okTarget.ID), notificationTargetSendBody())
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Code int                        `json:"code"`
				Data *NotificationTargetSendOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, http.StatusOK)
			So(resp.Data.TargetID, ShouldEqual, okTarget.ID)
			So(resp.Data.EventType, ShouldEqual, "tag_event")
			So(resp.Data.EventID, ShouldEqual, 101)
			// 断言已通过假投递客户端将 notify:send:target 任务入队
			enq.mu.Lock()
			task := enq.lastTask
			enq.mu.Unlock()
			So(task, ShouldNotBeNil)
			So(task.Type(), ShouldEqual, "notify:send:target")
			So(string(task.Payload()), ShouldContainSubstring, `"eventType":"tag_event"`)
			So(string(task.Payload()), ShouldContainSubstring, `"targetID":`+strconv.FormatUint(uint64(okTarget.ID), 10))
		})
	})
}
