package routes

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
// timestampFrom/timestampTo/createdAtFrom/createdAtTo 四个时间范围项均为必填
// （action + value），与前端 DateRangeField 生成的请求格式对应。
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

	convey.Convey("缺失时间范围项（旧格式）应返回验证错误", t, func() {
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
