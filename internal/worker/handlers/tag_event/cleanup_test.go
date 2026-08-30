package tag_event

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"zhongkui.com/zhongkui/internal/models/tag"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

// cleanupStubTask 实现 etl.Task 的最小桩，用于向清理处理器注入指定载荷。
type cleanupStubTask struct {
	payload []byte
}

func (t cleanupStubTask) Type() string { return TagEventCleanupTask }

func (t cleanupStubTask) Payload() []byte { return t.payload }

// TestParseCleanupRetention 验证保留期解析与默认值回退。
func TestParseCleanupRetention(t *testing.T) {
	Convey("Test ParseCleanupRetention", t, func() {
		Convey("空载荷回退默认 7 天", func() {
			got, err := parseCleanupRetention(nil)
			So(err, ShouldBeNil)
			So(got, ShouldResemble, defaultCleanupRetention)
		})

		Convey("普通定时任务载荷缺失保留期时回退默认", func() {
			got, err := parseCleanupRetention([]byte(`{}`))
			So(err, ShouldBeNil)
			So(got, ShouldResemble, defaultCleanupRetention)
		})

		Convey("普通定时任务载荷顶层指定保留期时生效", func() {
			got, err := parseCleanupRetention([]byte(`{"retention":"48h"}`))
			So(err, ShouldBeNil)
			So(got, ShouldResemble, 48*time.Hour)
		})

		Convey("游标包裹载荷的保留期兼容", func() {
			got, err := parseCleanupRetention([]byte(`{"triggerAt":"2026-01-01T00:00:00Z","payload":{"retention":"48h"}}`))
			So(err, ShouldBeNil)
			So(got, ShouldResemble, 48*time.Hour)
		})
	})
}

// TestEventCleanupHandler 验证清理仅删除超过保留期的 suppressed 事件，alerting/closed/近期 suppressed 均保留，
// 且被删事件的命中明细经数据库外键级联删除。
func TestEventCleanupHandler(t *testing.T) {
	Convey("Test EventCleanupHandler", t, func() {
		logger, err := logging.NewDefaultLogger()
		So(err, ShouldBeNil)
		db := createTagEventStoreTestDB(t)

		// 超过默认保留期（7 天）的 suppressed 事件：应删除
		oldSuppressedID := insertEvent(db, newEvent(1, "Kold", tag.EventStatusSuppressed))
		setEventDetectedAt(db, oldSuppressedID, time.Now().Add(-8*24*time.Hour))
		So(db.Exec("INSERT INTO tag_event_record_hits (created_at, updated_at, tag_event_record_id) VALUES (?, ?, ?)", time.Now(), time.Now(), oldSuppressedID).Error, ShouldBeNil)

		// 保留期内的 suppressed 事件：应保留
		recentSuppressedID := insertEvent(db, newEvent(1, "Krecent", tag.EventStatusSuppressed))
		setEventDetectedAt(db, recentSuppressedID, time.Now().Add(-1*24*time.Hour))
		So(db.Exec("INSERT INTO tag_event_record_hits (created_at, updated_at, tag_event_record_id) VALUES (?, ?, ?)", time.Now(), time.Now(), recentSuppressedID).Error, ShouldBeNil)

		// 告警中 / 已关闭（已通知历史）事件：应保留
		alertID := insertEvent(db, newEvent(1, "Ka", tag.EventStatusAlerting))
		closedID := insertEvent(db, newEvent(1, "Kc", tag.EventStatusClosed))

		handler := &EventCleanupHandler{coreDB: db, logger: logger}
		So(handler.Do(context.Background(), cleanupStubTask{}), ShouldBeNil)

		So(eventStatus(db, oldSuppressedID), ShouldEqual, "")
		So(eventStatus(db, recentSuppressedID), ShouldEqual, tag.EventStatusSuppressed)
		So(eventStatus(db, alertID), ShouldEqual, tag.EventStatusAlerting)
		So(eventStatus(db, closedID), ShouldEqual, tag.EventStatusClosed)

		// 被删事件的命中明细应级联删除
		var hitCount int64
		So(db.Model(&tag.TagEventRecordHit{}).Where("tag_event_record_id = ?", oldSuppressedID).Count(&hitCount).Error, ShouldBeNil)
		So(hitCount, ShouldEqual, 0)
	})
}
