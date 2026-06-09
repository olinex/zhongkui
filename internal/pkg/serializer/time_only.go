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

type TimeOnly time.Time

func (t TimeOnly) String() string {
	return t.Format("15:04:05")
}

func (t TimeOnly) Format(layout string) string {
	return time.Time(t).Format(layout)
}

func (t TimeOnly) IsZero() bool {
	return time.Time(t).IsZero()
}

func (t TimeOnly) Hour() int {
	return time.Time(t).Hour()
}

func (t TimeOnly) Minute() int {
	return time.Time(t).Minute()
}

func (t TimeOnly) Second() int {
	return time.Time(t).Second()
}

func (t TimeOnly) Nanosecond() int {
	return time.Time(t).Nanosecond()
}

func (t TimeOnly) Add(d time.Duration) TimeOnly {
	return TimeOnly(time.Time(t).Add(d))
}

func (t TimeOnly) Sub(other TimeOnly) time.Duration {
	return time.Time(t).Sub(time.Time(other))
}

func (t TimeOnly) Before(other TimeOnly) bool {
	return time.Time(t).Before(time.Time(other))
}

func (t TimeOnly) After(other TimeOnly) bool {
	return time.Time(t).After(time.Time(other))
}

func (t TimeOnly) Equal(other TimeOnly) bool {
	return time.Time(t).Equal(time.Time(other))
}

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
