// Package routes 提供 HTTP 路由注册和管理功能。
// 本文件包含通知发送记录（NotificationRecord）只读路由的集成测试：
// 覆盖列表（含目标名称补全）、过滤（状态/事件类型/渠道类型/创建时间范围）、
// 详情（payload/error/messageId/notifiedAt）、目标已删除时的尽力补全与命名空间隔离。
package views

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	. "github.com/smartystreets/goconvey/convey"

	"zhongkui.com/zhongkui/internal/manager/middlewares"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/database"
)

// createNotificationRecordTestDB 创建 sqlite 内存库并迁移 notify 相关模型表。
// 内存库必须限制为单连接，避免多个连接各自持有独立的内存数据库。
func createNotificationRecordTestDB() *database.DB {
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
	if err := db.AutoMigrate(&core.NotificationTarget{}, &core.NotificationChannel{}, &core.NotificationRecord{}); err != nil {
		panic("failed to migrate test database: " + err.Error())
	}
	if err := convertNotificationRecordNotifiedAtColumn(db); err != nil {
		panic("failed to convert notified_at column: " + err.Error())
	}
	return db
}

// convertNotificationRecordNotifiedAtColumn 将 notification_records.notified_at 列从
// TIMESTAMPTZ 改为 datetime。glebarez/sqlite 驱动仅在列声明类型为 DATE/DATETIME/TIMESTAMP
// 时才把文本值解析为 time.Time，而模型对 PostgreSQL 声明为 TIMESTAMPTZ，导致 sqlite 下
// 无法把字符串扫描进 *time.Time；测试库改为 datetime 以复用驱动的时间解析能力。
// 在 AutoMigrate 之后、写入数据之前执行，此时表为空，无需迁移存量数据。
func convertNotificationRecordNotifiedAtColumn(db *database.DB) error {
	if err := db.Exec("ALTER TABLE notification_records RENAME COLUMN notified_at TO notified_at_tmp").Error; err != nil {
		return err
	}
	if err := db.Exec("ALTER TABLE notification_records ADD COLUMN notified_at datetime").Error; err != nil {
		return err
	}
	return db.Exec("ALTER TABLE notification_records DROP COLUMN notified_at_tmp").Error
}

// registerNotificationRecordTestRoutes 注册通知记录测试路由（与 router.go 的注册保持一致）。
func registerNotificationRecordTestRoutes(r *gin.Engine) {
	group := r.Group("/notification/record")
	{
		group.POST("/list", NotificationRecordListGet)
		group.OPTIONS("/list", ListMetaGinHandler[*NotificationRecordListGetSearchIn, *NotificationRecordListOut]())
		group.POST("/:id/get", NotificationRecordDetailGet)
	}
}

// setupNotificationRecordTestRouter 创建测试路由引擎，注入 i18n、日志、数据库与固定 default 命名空间。
func setupNotificationRecordTestRouter(db *database.DB, namespace string) *gin.Engine {
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
	registerNotificationRecordTestRoutes(r)
	return r
}

// doNotificationRecordRequest 执行一次通知记录 HTTP 请求并返回响应记录器。
func doNotificationRecordRequest(r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
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

// insertNotificationTarget 直接落库一个通知目标，返回其 ID。
func insertNotificationTarget(db *database.DB, namespace, name string) uint {
	target := &core.NotificationTarget{NamespaceName: namespace, Name: name, Type: "webhook", Enabled: true}
	So(db.Create(target).Error, ShouldBeNil)
	return target.ID
}

// insertNotificationRecordChannel 直接落库一个通知渠道，返回其 ID。
// 命名带 Record 前缀以避免与通知渠道/目标测试文件的同名辅助函数冲突。
func insertNotificationRecordChannel(db *database.DB, namespace, name, channelType string) uint {
	channel := &core.NotificationChannel{NamespaceName: namespace, Name: name, Type: channelType, Enabled: true}
	So(db.Create(channel).Error, ShouldBeNil)
	return channel.ID
}

// insertNotificationRecord 直接落库一条通知记录，返回其 ID。
// 调用方可先修改 record 字段（如 Status、EventType、CreatedAt）后再传入。
func insertNotificationRecord(db *database.DB, record *core.NotificationRecord) uint {
	So(db.Create(record).Error, ShouldBeNil)
	return record.ID
}

// notificationRecordIDPath 拼接通知记录详情路由路径。
func notificationRecordIDPath(action string, id uint) string {
	return "/notification/record/" + strconv.FormatUint(uint64(id), 10) + action
}

// TestNotificationRecordListAndEnrich 列表查询、目标名称补全与过滤。
func TestNotificationRecordListAndEnrich(t *testing.T) {
	Convey("测试通知记录列表、目标名称补全与过滤", t, func() {
		db := createNotificationRecordTestDB()
		r := setupNotificationRecordTestRouter(db, middlewares.DefaultNamespace)

		// 预置一个目标与一个渠道，供名称补全
		targetID := insertNotificationTarget(db, middlewares.DefaultNamespace, "ops-email")
		channelID := insertNotificationRecordChannel(db, middlewares.DefaultNamespace, "main-channel", "email")

		// 两条 success 记录与一条 failed 记录
		insertNotificationRecord(db, &core.NotificationRecord{
			NamespaceName: middlewares.DefaultNamespace,
			EventType:     core.EventTypeTagEventRecord,
			EventID:       1, TargetID: targetID, ChannelID: channelID, ChannelType: "email",
			Status: core.RecordStatusSuccess, MessageID: "m-1",
			NotifiedAt: ptrTime(time.Now().Truncate(time.Second)),
		})
		insertNotificationRecord(db, &core.NotificationRecord{
			NamespaceName: middlewares.DefaultNamespace,
			EventType:     core.EventTypeTagEventRecord,
			EventID:       2, TargetID: targetID, ChannelID: channelID, ChannelType: "email",
			Status: core.RecordStatusSuccess, MessageID: "m-2",
		})
		insertNotificationRecord(db, &core.NotificationRecord{
			NamespaceName: middlewares.DefaultNamespace,
			EventType:     core.EventTypeTagEventRecord,
			EventID:       3, TargetID: targetID, ChannelID: channelID, ChannelType: "email",
			Status: core.RecordStatusFailed, Error: "smtp timeout", MessageID: "",
		})

		Convey("列表返回全部记录且目标名称被补全", func() {
			w := doNotificationRecordRequest(r, http.MethodPost, "/notification/record/list", map[string]any{
				"pageSize": 10,
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *struct {
					Total uint                         `json:"total"`
					Items []*NotificationRecordListOut `json:"items"`
				} `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Total, ShouldEqual, 3)
			So(resp.Data.Items, ShouldHaveLength, 3)
			for _, item := range resp.Data.Items {
				So(item.TargetName, ShouldEqual, "ops-email")
				So(item.ChannelType, ShouldEqual, "email")
			}
		})

		Convey("按状态过滤仅返回 success 记录", func() {
			w := doNotificationRecordRequest(r, http.MethodPost, "/notification/record/list", map[string]any{
				"filterBy": map[string]any{
					"status": map[string]any{"action": "eq", "value": core.RecordStatusSuccess},
				},
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *struct {
					Total uint                         `json:"total"`
					Items []*NotificationRecordListOut `json:"items"`
				} `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Total, ShouldEqual, 2)
			for _, item := range resp.Data.Items {
				So(item.Status, ShouldEqual, core.RecordStatusSuccess)
			}
		})

		Convey("按事件类型与渠道类型过滤", func() {
			// 额外插入一条事件类型不同的记录
			insertNotificationRecord(db, &core.NotificationRecord{
				NamespaceName: middlewares.DefaultNamespace,
				EventType:     "custom_event",
				EventID:       4, TargetID: targetID, ChannelID: channelID, ChannelType: "webhook",
				Status: core.RecordStatusSuccess,
			})
			w := doNotificationRecordRequest(r, http.MethodPost, "/notification/record/list", map[string]any{
				"filterBy": map[string]any{
					"eventType":   map[string]any{"action": "eq", "value": core.EventTypeTagEventRecord},
					"channelType": map[string]any{"action": "eq", "value": "email"},
				},
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *struct {
					Total uint                         `json:"total"`
					Items []*NotificationRecordListOut `json:"items"`
				} `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Total, ShouldEqual, 3)
			for _, item := range resp.Data.Items {
				So(item.EventType, ShouldEqual, core.EventTypeTagEventRecord)
				So(item.ChannelType, ShouldEqual, "email")
			}
		})
	})
}

// TestNotificationRecordCreatedAtRange 按创建时间范围过滤。
func TestNotificationRecordCreatedAtRange(t *testing.T) {
	Convey("测试按创建时间范围过滤", t, func() {
		db := createNotificationRecordTestDB()
		r := setupNotificationRecordTestRouter(db, middlewares.DefaultNamespace)

		base := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
		// 插入三条记录并分别回写创建/更新时间（嵌入字段无法在结构体字面量中直接赋值）
		insertRecordWithTime := func(eventID uint, status string, createdAt time.Time) uint {
			record := &core.NotificationRecord{
				NamespaceName: middlewares.DefaultNamespace, EventType: core.EventTypeTagEventRecord,
				EventID: eventID, Status: status,
			}
			So(db.Create(record).Error, ShouldBeNil)
			So(db.Model(&core.NotificationRecord{}).Where("id = ?", record.ID).
				Updates(map[string]any{"created_at": createdAt, "updated_at": createdAt}).Error, ShouldBeNil)
			return record.ID
		}
		// 三条记录创建时间：2h 前 / 1h 前 / 半小时后
		insertRecordWithTime(1, core.RecordStatusSuccess, base.Add(-2*time.Hour))
		insertRecordWithTime(2, core.RecordStatusSuccess, base.Add(-1*time.Hour))
		insertRecordWithTime(3, core.RecordStatusFailed, base.Add(30*time.Minute))

		Convey("gte 与 lte 结合的时间窗口只返回窗口内记录", func() {
			w := doNotificationRecordRequest(r, http.MethodPost, "/notification/record/list", map[string]any{
				"filterBy": map[string]any{
					"createdAt": map[string]any{"action": "gte", "value": base.Add(-90 * time.Minute).Format(time.RFC3339)},
				},
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *struct {
					Total uint                         `json:"total"`
					Items []*NotificationRecordListOut `json:"items"`
				} `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Total, ShouldEqual, 2)
		})

		Convey("lte 时间点过滤只返回更早的记录", func() {
			w := doNotificationRecordRequest(r, http.MethodPost, "/notification/record/list", map[string]any{
				"filterBy": map[string]any{
					"createdAt": map[string]any{"action": "lte", "value": base.Add(-90 * time.Minute).Format(time.RFC3339)},
				},
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *struct {
					Total uint                         `json:"total"`
					Items []*NotificationRecordListOut `json:"items"`
				} `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Total, ShouldEqual, 1)
		})
	})
}

// TestNotificationRecordDetail 详情返回完整字段。
func TestNotificationRecordDetail(t *testing.T) {
	Convey("测试通知记录详情", t, func() {
		db := createNotificationRecordTestDB()
		r := setupNotificationRecordTestRouter(db, middlewares.DefaultNamespace)

		targetID := insertNotificationTarget(db, middlewares.DefaultNamespace, "ops-email")
		channelID := insertNotificationRecordChannel(db, middlewares.DefaultNamespace, "main-channel", "email")
		notifiedAt := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
		recordID := insertNotificationRecord(db, &core.NotificationRecord{
			NamespaceName: middlewares.DefaultNamespace,
			EventType:     core.EventTypeTagEventRecord, EventID: 1, GroupID: 5,
			TargetID: targetID, ChannelID: channelID, ChannelType: "email",
			Status: core.RecordStatusFailed, Error: "smtp timeout",
			MessageID: "channel-msg-9", Payload: `{"title":"alert"}`, NotifiedAt: &notifiedAt,
		})

		Convey("详情返回 payload/error/messageId/notifiedAt 及补全的名称", func() {
			w := doNotificationRecordRequest(r, http.MethodPost, notificationRecordIDPath("/get", recordID), nil)
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *NotificationRecordDetailOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data, ShouldNotBeNil)
			So(resp.Data.EventType, ShouldEqual, core.EventTypeTagEventRecord)
			So(resp.Data.EventID, ShouldEqual, 1)
			So(resp.Data.GroupID, ShouldEqual, 5)
			So(resp.Data.TargetName, ShouldEqual, "ops-email")
			So(resp.Data.ChannelName, ShouldEqual, "main-channel")
			So(resp.Data.ChannelType, ShouldEqual, "email")
			So(resp.Data.Status, ShouldEqual, core.RecordStatusFailed)
			So(resp.Data.Error, ShouldEqual, "smtp timeout")
			So(resp.Data.MessageID, ShouldEqual, "channel-msg-9")
			So(resp.Data.Payload, ShouldEqual, `{"title":"alert"}`)
			So(resp.Data.NotifiedAt, ShouldNotBeNil)
			So(resp.Data.NotifiedAt.Equal(notifiedAt), ShouldBeTrue)
		})

		Convey("记录不存在返回 404", func() {
			w := doNotificationRecordRequest(r, http.MethodPost, notificationRecordIDPath("/get", 99999), nil)
			So(w.Code, ShouldEqual, http.StatusNotFound)
		})
	})
}

// TestNotificationRecordDeletedTarget 目标已删除时列表与详情仍可用。
func TestNotificationRecordDeletedTarget(t *testing.T) {
	Convey("测试目标已删除时的尽力补全", t, func() {
		db := createNotificationRecordTestDB()
		r := setupNotificationRecordTestRouter(db, middlewares.DefaultNamespace)

		// 删除前先落库目标，再删除该目标，使记录引用已不存在的目标
		targetID := insertNotificationTarget(db, middlewares.DefaultNamespace, "gone-target")
		recordID := insertNotificationRecord(db, &core.NotificationRecord{
			NamespaceName: middlewares.DefaultNamespace,
			EventType:     core.EventTypeTagEventRecord, EventID: 1, TargetID: targetID, ChannelID: 1,
			ChannelType: "email", Status: core.RecordStatusSuccess,
		})
		So(db.Where("id = ?", targetID).Delete(&core.NotificationTarget{}).Error, ShouldBeNil)

		Convey("列表仍返回该记录且目标名称为空", func() {
			w := doNotificationRecordRequest(r, http.MethodPost, "/notification/record/list", map[string]any{
				"pageSize": 10,
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *struct {
					Total uint                         `json:"total"`
					Items []*NotificationRecordListOut `json:"items"`
				} `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Total, ShouldEqual, 1)
			So(resp.Data.Items[0].TargetName, ShouldEqual, "")
			So(resp.Data.Items[0].ID, ShouldEqual, recordID)
		})

		Convey("详情返回该记录且目标名称为空", func() {
			w := doNotificationRecordRequest(r, http.MethodPost, notificationRecordIDPath("/get", recordID), nil)
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Code int                          `json:"code"`
				Data *NotificationRecordDetailOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, http.StatusOK)
			So(resp.Data.TargetName, ShouldEqual, "")
		})
	})
}

// TestNotificationRecordNamespaceIsolation 记录按命名空间隔离。
func TestNotificationRecordNamespaceIsolation(t *testing.T) {
	Convey("测试通知记录命名空间隔离", t, func() {
		db := createNotificationRecordTestDB()
		targetID := insertNotificationTarget(db, middlewares.DefaultNamespace, "ops-email")
		recordID := insertNotificationRecord(db, &core.NotificationRecord{
			NamespaceName: middlewares.DefaultNamespace,
			EventType:     core.EventTypeTagEventRecord, EventID: 1, TargetID: targetID, ChannelID: 1,
			ChannelType: "email", Status: core.RecordStatusSuccess,
		})

		rOther := setupNotificationRecordTestRouter(db, "other-ns")

		Convey("跨命名空间详情返回 404", func() {
			w := doNotificationRecordRequest(rOther, http.MethodPost, notificationRecordIDPath("/get", recordID), nil)
			So(w.Code, ShouldEqual, http.StatusNotFound)
		})

		Convey("跨命名空间列表为空", func() {
			w := doNotificationRecordRequest(rOther, http.MethodPost, "/notification/record/list", map[string]any{
				"pageSize": 10,
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *struct {
					Total uint                         `json:"total"`
					Items []*NotificationRecordListOut `json:"items"`
				} `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Total, ShouldEqual, 0)
		})
	})
}

// ptrTime 返回 time.Time 的指针拷贝。
func ptrTime(t time.Time) *time.Time {
	return &t
}
