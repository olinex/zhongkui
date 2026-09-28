package knowledge

import (
	// stdlib
	"encoding/json"
	"testing"

	// third-party
	"github.com/smartystreets/goconvey/convey"
)

// TestNewParseTask 测试知识文档解析任务的编码。
// TestNewParseTask tests the encoding of the knowledge document parse task.
func TestNewParseTask(t *testing.T) {
	convey.Convey("NewParseTask 编码知识文档解析任务", t, func() {
		convey.Convey("合法文档 ID：类型为 ai:doc:parse 且载荷往返一致", func() {
			task, err := NewParseTask(42)
			convey.So(err, convey.ShouldBeNil)
			convey.So(task, convey.ShouldNotBeNil)
			convey.So(task.Type(), convey.ShouldEqual, ParseTask)
			var decoded ParsePayload
			convey.So(json.Unmarshal(task.Payload(), &decoded), convey.ShouldBeNil)
			convey.So(decoded.DocumentID, convey.ShouldEqual, uint(42))
		})

		convey.Convey("文档 ID 为零值：返回错误", func() {
			_, err := NewParseTask(0)
			convey.So(err, convey.ShouldNotBeNil)
		})
	})
}
