package serializer

import (
	"database/sql/driver"
	"encoding/json"
	"strings"
	"time"

	"zhongkui.com/zhongkui/internal/pkg/errors"
)

var (
	timeLayouts []string = []string{
		"15:04:05",
		"15:04",
		"3:04:05 PM",
		"3:04 PM",
		"150405",
		"1504",
	}
)

// ParseTimeOnly 解析时间文本为 TimeOnly。
// 依次尝试多种常见时间布局（如 15:04:05、3:04 PM、150405 等），全部失败时返回错误。
func ParseTimeOnly(s string) (TimeOnly, error) {
	// 尝试每种支持的格式
	for _, layout := range timeLayouts {
		t, err := time.ParseInLocation(layout, s, time.UTC)
		if err == nil {
			return TimeOnly(time.Date(0, 1, 1, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC)), nil
		}
	}

	return TimeOnly{}, errors.Newf("Invalid time format: %s", s)
}

// TimeOnly 表示仅包含时刻（时分秒）的时间类型，底层为 time.Time。
// 日期部分固定为 0001-01-01，用于只需要时刻的字段序列化与时刻运算。
type TimeOnly time.Time

// String 返回 HH:mm:ss 格式的时刻字符串。
func (t TimeOnly) String() string {
	return t.Format("15:04:05")
}

// Format 按给定布局格式化时刻，行为与 time.Time.Format 一致。
func (t TimeOnly) Format(layout string) string {
	return time.Time(t).Format(layout)
}

// IsZero 判断时刻是否为零值。
func (t TimeOnly) IsZero() bool {
	return time.Time(t).IsZero()
}

// Hour 返回时刻的小时。
func (t TimeOnly) Hour() int {
	return time.Time(t).Hour()
}

// Minute 返回时刻的分钟。
func (t TimeOnly) Minute() int {
	return time.Time(t).Minute()
}

// Second 返回时刻的秒。
func (t TimeOnly) Second() int {
	return time.Time(t).Second()
}

// Nanosecond 返回时刻的纳秒部分。
func (t TimeOnly) Nanosecond() int {
	return time.Time(t).Nanosecond()
}

// Add 返回在时刻上增加指定时长后的新时刻。
func (t TimeOnly) Add(d time.Duration) TimeOnly {
	return TimeOnly(time.Time(t).Add(d))
}

// Sub 返回与另一时刻的时间差。
func (t TimeOnly) Sub(other TimeOnly) time.Duration {
	return time.Time(t).Sub(time.Time(other))
}

// Before 判断时刻是否早于另一时刻。
func (t TimeOnly) Before(other TimeOnly) bool {
	return time.Time(t).Before(time.Time(other))
}

// After 判断时刻是否晚于另一时刻。
func (t TimeOnly) After(other TimeOnly) bool {
	return time.Time(t).After(time.Time(other))
}

// Equal 判断时刻是否与另一时刻相等。
func (t TimeOnly) Equal(other TimeOnly) bool {
	return time.Time(t).Equal(time.Time(other))
}

// UnmarshalJSON 实现 json.Unmarshaler，接受 HH:mm:ss 字符串，null 或空串按零值处理。
func (t *TimeOnly) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), "\"")
	if s == "null" || s == "" {
		return nil
	}
	if err := t.Scan(s); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// MarshalJSON 实现 json.Marshaler，输出 HH:mm:ss 字符串，零值输出 null。
func (t TimeOnly) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte("null"), nil
	}
	bytes, err := json.Marshal(t.String())
	if err != nil {
		return bytes, errors.WithStack(err)
	}
	return bytes, nil
}

// Scan 实现 sql.Scanner，兼容 time.Time、时间字符串与 NULL。
func (t *TimeOnly) Scan(value any) error {
	if value == nil {
		return nil
	}

	switch v := value.(type) {
	case time.Time:
		*t = TimeOnly(time.Date(0, 1, 1, v.Hour(), v.Minute(), v.Second(), v.Nanosecond(), time.UTC))
		return nil
	case string:
		time, err := ParseTimeOnly(v)
		if err != nil {
			return errors.WithStack(err)
		}
		*t = time
		return nil
	default:
		return errors.Newf("Unsupported time type: %T", value)
	}
}

// 实现 driver.Valuer 接口，用于数据库写入
func (t TimeOnly) Value() (driver.Value, error) {
	if t.IsZero() {
		return nil, nil
	}
	return t.String(), nil
}

// Now 返回当前时间（只包含时分秒）
func NowTimeOnly() TimeOnly {
	now := time.Now()
	return TimeOnly(time.Date(0, 1, 1, now.Hour(), now.Minute(), now.Second(), now.Nanosecond(), time.UTC))
}

// Time 创建一个新的 TimeOnly
func Time(hour, minute, second, nanosecond int) TimeOnly {
	return TimeOnly(time.Date(0, 1, 1, hour, minute, second, nanosecond, time.UTC))
}
