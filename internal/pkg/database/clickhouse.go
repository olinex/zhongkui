package database

import (
	"strings"
	"time"
)

// ToUTCDatetime64 将时间转换为 ClickHouse DateTime64 可解析的 UTC 字符串。
// 返回的字符串使用空格分隔格式（如 "2026-08-08 00:53:57.606453"），
// 可直接作为查询参数用于 DateTime64 列的边界比较：
//
//	query.Where("created_at > ? AND created_at <= ?",
//	    ToUTCDatetime64(prevWatermark, 6),
//	    ToUTCDatetime64(curWatermark, 6))
//
// 背景：gorm clickhouse 驱动对 time.Time 参数按秒级精度插值（toDateTime），
// 与 DateTime64(N)（微秒精度）列比较时边界判定异常，导致重复消费或数据丢失。
// 因此这里以带 precision 位小数精度的 UTC 字符串传参，ClickHouse 会按列类型
// DateTime64(N) 隐式解析，完整保留精度。注意必须使用空格分隔格式，
// RFC3339Nano 的 'T'/'Z' 写法在隐式转换时会报 TYPE_MISMATCH。
func ToUTCDatetime64(t time.Time, precision int) string {
	if precision <= 0 {
		return t.UTC().Format("2006-01-02 15:04:05")
	}
	layout := "2006-01-02 15:04:05." + strings.Repeat("0", precision)
	return t.UTC().Format(layout)
}
