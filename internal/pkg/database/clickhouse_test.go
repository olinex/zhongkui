package database

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

// TestToUTCDatetime64 测试 ToUTCDatetime64 的格式化行为（回归测试）。
// gorm clickhouse 驱动对 time.Time 参数按秒级精度插值（toDateTime），
// 与 DateTime64(N) 列比较时边界判定异常，导致重复消费或数据丢失。
// 修复方式是先转 UTC 并保留 precision 位小数精度再以字符串传参，
// ClickHouse 会按列类型 DateTime64(N) 隐式解析。
func TestToUTCDatetime64(t *testing.T) {
	Convey("ToUTCDatetime64 格式化测试", t, func() {
		Convey("微秒精度（precision=6）应保留 6 位小数", func() {
			ts := time.Date(2026, 8, 8, 0, 53, 57, 606453000, time.UTC)
			So(ToUTCDatetime64(ts, 6), ShouldEqual, "2026-08-08 00:53:57.606453")
		})

		Convey("本地时区时间应转换为 UTC 墙钟时间", func() {
			shanghai := time.FixedZone("CST", 8*3600)
			// 北京时间 08:53:57.606453 对应 UTC 00:53:57.606453
			ts := time.Date(2026, 8, 8, 8, 53, 57, 606453000, shanghai)
			So(ToUTCDatetime64(ts, 6), ShouldEqual, "2026-08-08 00:53:57.606453")
		})

		Convey("末尾为零的微秒仍应补零对齐", func() {
			ts := time.Date(2026, 8, 8, 0, 54, 59, 769000000, time.UTC)
			So(ToUTCDatetime64(ts, 6), ShouldEqual, "2026-08-08 00:54:59.769000")
		})

		Convey("毫秒精度（precision=3）应保留 3 位小数", func() {
			ts := time.Date(2026, 8, 8, 0, 53, 57, 606000000, time.UTC)
			So(ToUTCDatetime64(ts, 3), ShouldEqual, "2026-08-08 00:53:57.606")
		})

		Convey("precision<=0 时不含小数部分", func() {
			ts := time.Date(2026, 8, 8, 0, 53, 57, 606453000, time.UTC)
			So(ToUTCDatetime64(ts, 0), ShouldEqual, "2026-08-08 00:53:57")
		})
	})
}
