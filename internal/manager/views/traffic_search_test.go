package views

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"
	"github.com/smartystreets/goconvey/convey"
)

// TestIpv4TcpListGetSearchInBinding 验证流量列表请求体与后端契约的一致性：
// createdAtFrom/createdAtTo（分区裁剪）与 namespace 必填；timestampFrom/timestampTo 选填，
// 只关心采集时间时才提供。
func TestIpv4TcpListGetSearchInBinding(t *testing.T) {
	gin.SetMode(gin.TestMode)

	convey.Convey("完整时间范围请求体应绑定成功", t, func() {
		body := `{
			"greedy": false,
			"page": 1,
			"pageSize": 10,
			"orderBy": [{"name": "timestamp", "order": -1}],
			"filterBy": {
				"timestampFrom": {"action": "gte", "value": "2026-08-20T00:00:00Z"},
				"timestampTo": {"action": "lte", "value": "2026-08-20T23:59:59Z"},
				"createdAtFrom": {"action": "gte", "value": "2026-08-20T00:00:00Z"},
				"createdAtTo": {"action": "lte", "value": "2026-08-20T23:59:59Z"},
				"namespace": {"action": "eq", "value": "default"}
			}
		}`

		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
		c.Request.Header.Set("Content-Type", "application/json")

		var in Ipv4TcpListGetSearchIn
		convey.So(defaults.Set(&in), convey.ShouldBeNil)
		convey.So(c.ShouldBindBodyWithJSON(&in), convey.ShouldBeNil)
		convey.So(in.FilterBy.TimestampFrom.Value, convey.ShouldEqual, time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC))
		convey.So(in.FilterBy.TimestampTo.Value, convey.ShouldEqual, time.Date(2026, 8, 20, 23, 59, 59, 0, time.UTC))
		convey.So(in.FilterBy.Namespace.Value, convey.ShouldEqual, "default")
	})

	convey.Convey("仅提供 createdAt 与 namespace（timestamp 选填）应绑定成功", t, func() {
		body := `{
			"page": 1,
			"pageSize": 10,
			"filterBy": {
				"createdAtFrom": {"action": "gte", "value": "2026-08-20T00:00:00Z"},
				"createdAtTo": {"action": "lte", "value": "2026-08-20T23:59:59Z"},
				"namespace": {"action": "eq", "value": "default"}
			}
		}`

		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
		c.Request.Header.Set("Content-Type", "application/json")

		var in Ipv4TcpListGetSearchIn
		convey.So(defaults.Set(&in), convey.ShouldBeNil)
		convey.So(c.ShouldBindBodyWithJSON(&in), convey.ShouldBeNil)
		convey.So(in.FilterBy.TimestampFrom, convey.ShouldBeNil)
		convey.So(in.FilterBy.TimestampTo, convey.ShouldBeNil)
		convey.So(in.FilterBy.CreatedAtFrom.Value, convey.ShouldEqual, time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC))
	})

	convey.Convey("缺失 createdAt 等必填项应返回验证错误", t, func() {
		body := `{
			"page": 1,
			"pageSize": 10,
			"filterBy": {
				"timestamp": {"action": "gte", "value": "2026-08-20T00:00:00Z"},
				"created_at": {"action": "gte", "value": "2026-08-20T00:00:00Z"},
				"namespace": {"action": "eq", "value": "default"}
			}
		}`

		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
		c.Request.Header.Set("Content-Type", "application/json")

		var in Ipv4TcpListGetSearchIn
		convey.So(defaults.Set(&in), convey.ShouldBeNil)
		convey.So(c.ShouldBindBodyWithJSON(&in), convey.ShouldNotBeNil)
	})
}

// TestNetworkStackListGetSearchInBinding 验证 HTTP 传输关联网络堆栈列表请求体与后端契约的一致性：
// createdAt 与 traceId（流关联键）必填；timestamp 选填；namespace/sessionId/cli2Srv 为可选过滤项。
func TestNetworkStackListGetSearchInBinding(t *testing.T) {
	gin.SetMode(gin.TestMode)

	convey.Convey("完整请求体（含 traceId 与时间范围）应绑定成功", t, func() {
		body := `{
			"greedy": false,
			"page": 1,
			"pageSize": 10,
			"orderBy": [{"name": "timestamp", "order": -1}],
			"filterBy": {
				"timestampFrom": {"action": "gte", "value": "2026-08-20T00:00:00Z"},
				"timestampTo": {"action": "lte", "value": "2026-08-20T23:59:59Z"},
				"createdAtFrom": {"action": "gte", "value": "2026-08-20T00:00:00Z"},
				"createdAtTo": {"action": "lte", "value": "2026-08-20T23:59:59Z"},
				"traceID": {"action": "eq", "value": "0c0a2b3c4d5e6f708192a3b4c5d6e7f8"},
				"namespace": {"action": "eq", "value": "default"},
				"cli2Srv": {"action": "eq", "value": true}
			}
		}`

		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
		c.Request.Header.Set("Content-Type", "application/json")

		var in NetworkStackListGetSearchIn
		convey.So(defaults.Set(&in), convey.ShouldBeNil)
		convey.So(c.ShouldBindBodyWithJSON(&in), convey.ShouldBeNil)
		convey.So(in.FilterBy.TimestampFrom.Value, convey.ShouldEqual, time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC))
		convey.So(in.FilterBy.TraceID.Value, convey.ShouldNotBeNil)
		convey.So(in.FilterBy.Namespace.Value, convey.ShouldEqual, "default")
		convey.So(in.FilterBy.Cli2Srv.Value, convey.ShouldBeTrue)
	})

	convey.Convey("缺失 traceId（流关联键）应返回验证错误", t, func() {
		body := `{
			"page": 1,
			"pageSize": 10,
			"filterBy": {
				"timestampFrom": {"action": "gte", "value": "2026-08-20T00:00:00Z"},
				"timestampTo": {"action": "lte", "value": "2026-08-20T23:59:59Z"},
				"createdAtFrom": {"action": "gte", "value": "2026-08-20T00:00:00Z"},
				"createdAtTo": {"action": "lte", "value": "2026-08-20T23:59:59Z"}
			}
		}`

		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
		c.Request.Header.Set("Content-Type", "application/json")

		var in NetworkStackListGetSearchIn
		convey.So(defaults.Set(&in), convey.ShouldBeNil)
		convey.So(c.ShouldBindBodyWithJSON(&in), convey.ShouldNotBeNil)
	})

	convey.Convey("缺失 createdAt / traceId / namespace 等必填项应返回验证错误", t, func() {
		body := `{
			"page": 1,
			"pageSize": 10,
			"filterBy": {
				"traceID": {"action": "eq", "value": "0c0a2b3c4d5e6f708192a3b4c5d6e7f8"}
			}
		}`

		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
		c.Request.Header.Set("Content-Type", "application/json")

		var in NetworkStackListGetSearchIn
		convey.So(defaults.Set(&in), convey.ShouldBeNil)
		convey.So(c.ShouldBindBodyWithJSON(&in), convey.ShouldNotBeNil)
	})
}
