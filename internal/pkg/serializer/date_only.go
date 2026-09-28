package serializer

import (
	"database/sql/driver"
	"encoding/json"
	"strings"
	"time"

	"zhongkui.com/zhongkui/internal/pkg/errors"
)

var (
	dateLayouts []string = []string{
		"2006-01-02",
		"2006-1-2",
		"2006/01/02",
		"2006/1/2",
		"01/02/2006",
		"1/2/2006",
		"20060102",
		"200612",
	}
)

// ParseDateOnly 解析日期文本为 DateOnly。
// 依次尝试多种常见日期布局（如 2006-01-02、2006/1/2、20060102、200612 等），全部失败时返回错误。
func ParseDateOnly(s string) (DateOnly, error) {
	for _, layout := range dateLayouts {
		t, err := time.Parse(layout, s)
		if err == nil {
			return DateOnly(t), nil
		}
	}

	return DateOnly{}, errors.Newf("Invalid date format: %s", s)
}

// DateOnly 表示仅包含日期（年月日）的时间类型，底层为 time.Time。
// 时刻部分归零，用于只需要日期的字段序列化与日期运算。
type DateOnly time.Time

// String 返回 YYYY-MM-DD 格式的日期字符串。
func (d DateOnly) String() string {
	return d.Format("2006-01-02")
}

// Format 按给定布局格式化日期，行为与 time.Time.Format 一致。
func (d DateOnly) Format(layout string) string {
	return time.Time(d).Format(layout)
}

// IsZero 判断日期是否为零值。
func (d DateOnly) IsZero() bool {
	return time.Time(d).IsZero()
}

// AddDays 返回在日期上增加指定天数后的新日期。
func (d DateOnly) AddDays(days int) DateOnly {
	return DateOnly(time.Time(d).AddDate(0, 0, days))
}

// Sub 返回与另一日期的天数差（d - other）。
func (d DateOnly) Sub(other DateOnly) int {
	d1 := time.Time(d)
	d2 := time.Time(other)
	diff := d1.Sub(d2)
	return int(diff.Hours() / 24)
}

// Year 返回日期的年份。
func (d DateOnly) Year() int {
	return time.Time(d).Year()
}

// Month 返回日期的月份。
func (d DateOnly) Month() time.Month {
	return time.Time(d).Month()
}

// Day 返回日期的日（几号）。
func (d DateOnly) Day() int {
	return time.Time(d).Day()
}

// Weekday 返回日期对应的星期。
func (d DateOnly) Weekday() time.Weekday {
	return time.Time(d).Weekday()
}

// UnmarshalJSON 实现 json.Unmarshaler，接受 YYYY-MM-DD 字符串，null 或空串按零值处理。
func (d *DateOnly) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), "\"")
	if s == "null" || s == "" {
		return nil
	}
	if err := d.Scan(s); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// MarshalJSON 实现 json.Marshaler，输出 YYYY-MM-DD 字符串，零值输出 null。
func (d DateOnly) MarshalJSON() ([]byte, error) {
	if d.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(d.String())
}

// Scan 实现 sql.Scanner，兼容 time.Time、日期字符串与 NULL。
func (d *DateOnly) Scan(value any) error {
	if value == nil {
		d = nil
		return nil
	}

	switch v := value.(type) {
	case time.Time:
		*d = DateOnly(time.Date(v.Year(), v.Month(), v.Day(), 0, 0, 0, 0, time.UTC))
		return nil
	case string:
		date, err := ParseDateOnly(v)
		if err != nil {
			return errors.WithStack(err)
		}
		*d = date
		return nil
	default:
		return errors.Newf(`Unsupported date type: %T`, value)
	}
}

// Value 实现 driver.Valuer，以 YYYY-MM-DD 字符串存储，零值写入 NULL。
func (d *DateOnly) Value() (driver.Value, error) {
	if d == nil {
		return nil, nil
	}
	if d.IsZero() {
		return nil, nil
	}
	return d.String(), nil
}

// NowDateOnly 返回当前本地日期（年月日），时间部分归一化为零值。
func NowDateOnly() DateOnly {
	now := time.Now()
	return DateOnly(time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()))
}

// Date 创建一个新的 DateOnly
func Date(year int, month time.Month, day int) DateOnly {
	return DateOnly(time.Date(year, month, day, 0, 0, 0, 0, time.UTC))
}
