package tag_event

import (
	"encoding/json"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"zhongkui.com/zhongkui/internal/tagengine"
)

// TestNewTagEventEmitTask 验证事件载荷编码与任务类型。
func TestNewTagEventEmitTask(t *testing.T) {
	Convey("Test NewTagEventEmitTask", t, func() {
		Convey("编码后的任务类型与载荷可还原原始事件", func() {
			ev := &tagengine.TagEvent{
				Namespace:  "default",
				RuleID:     1,
				RuleName:   "rule-a",
				MatchAll:   true,
				DetectedAt: time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC),
				Hits: []tagengine.TagEventHit{
					{ConditionID: 1, ConditionName: "cond", Model: "m", Count: 5, Threshold: 3, Window: 5 * time.Minute},
				},
			}
			task, err := NewTagEventEmitTask(ev)
			So(err, ShouldBeNil)
			So(task.Type(), ShouldEqual, TagEventEmitTask)
			var decoded tagengine.TagEvent
			So(json.Unmarshal(task.Payload(), &decoded), ShouldBeNil)
			So(&decoded, ShouldResemble, ev)
		})

		Convey("nil 事件返回错误", func() {
			_, err := NewTagEventEmitTask(nil)
			So(err, ShouldNotBeNil)
		})
	})
}
