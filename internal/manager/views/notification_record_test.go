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
	"zhongkui.com/zhongkui/internal/pkg/testdb"
)

// createNotificationRecordTestDB 返回共享 PostgreSQL 测试库（testdb.Core 迁移并清空全部业务表）。
func createNotificationRecordTestDB(t *testing.T) *database.DB {
	return testdb.Core(t)
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
// PostgreSQL 下目标必须引用已存在的渠道（channel_id 外键），故内部先创建一条
// 命名空间内的渠道作为父行并回填 ChannelID，避免外键约束失败。
func insertNotificationTarget(db *database.DB, namespace, name string) uint {
	channel := &core.NotificationChannel{NamespaceName: namespace, Name: name + "-channel", Type: "email", Enabled: true}
	So(db.Create(channel).Error, ShouldBeNil)
	target := &core.NotificationTarget{
		NamespaceName: namespace, Name: name, Type: "webhook",
		ChannelID: channel.ID, Enabled: true,
	}
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
		db := createNotificationRecordTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
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
		db := createNotificationRecordTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
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
		db := createNotificationRecordTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
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
		db := createNotificationRecordTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupNotificationRecordTestRouter(db, middlewares.DefaultNamespace)

		// 删除前先落库目标，再删除该目标，使记录引用已不存在的目标
		channelID := insertNotificationRecordChannel(db, middlewares.DefaultNamespace, "main-channel", "email")
		targetID := insertNotificationTarget(db, middlewares.DefaultNamespace, "gone-target")
		recordID := insertNotificationRecord(db, &core.NotificationRecord{
			NamespaceName: middlewares.DefaultNamespace,
			EventType:     core.EventTypeTagEventRecord, EventID: 1, TargetID: targetID, ChannelID: channelID,
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
		db := createNotificationRecordTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		channelID := insertNotificationRecordChannel(db, middlewares.DefaultNamespace, "main-channel", "email")
		targetID := insertNotificationTarget(db, middlewares.DefaultNamespace, "ops-email")
		recordID := insertNotificationRecord(db, &core.NotificationRecord{
			NamespaceName: middlewares.DefaultNamespace,
			EventType:     core.EventTypeTagEventRecord, EventID: 1, TargetID: targetID, ChannelID: channelID,
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
