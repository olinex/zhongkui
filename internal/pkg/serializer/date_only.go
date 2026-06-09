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

func ParseDateOnly(s string) (DateOnly, error) {
	for _, layout := range dateLayouts {
		t, err := time.Parse(layout, s)
		if err == nil {
			return DateOnly(t), nil
		}
	}

	return DateOnly{}, errors.Newf("Invalid date format: %s", s)
}

type DateOnly time.Time

func (d DateOnly) String() string {
	return d.Format("2006-01-02")
}

func (d DateOnly) Format(layout string) string {
	return time.Time(d).Format(layout)
}

func (d DateOnly) IsZero() bool {
	return time.Time(d).IsZero()
}

func (d DateOnly) AddDays(days int) DateOnly {
	return DateOnly(time.Time(d).AddDate(0, 0, days))
}

func (d DateOnly) Sub(other DateOnly) int {
	d1 := time.Time(d)
	d2 := time.Time(other)
	diff := d1.Sub(d2)
	return int(diff.Hours() / 24)
}

func (d DateOnly) Year() int {
	return time.Time(d).Year()
}

func (d DateOnly) Month() time.Month {
	return time.Time(d).Month()
}

func (d DateOnly) Day() int {
	return time.Time(d).Day()
}

func (d DateOnly) Weekday() time.Weekday {
	return time.Time(d).Weekday()
}

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

func (d DateOnly) MarshalJSON() ([]byte, error) {
	if d.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(d.String())
}

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

func (d DateOnly) Value() (driver.Value, error) {
	if d.IsZero() {
		return nil, nil
	}
	return d.String(), nil
}

func NowDateOnly() DateOnly {
	now := time.Now()
	return DateOnly(time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()))
}

// Date 创建一个新的 DateOnly
func Date(year int, month time.Month, day int) DateOnly {
	return DateOnly(time.Date(year, month, day, 0, 0, 0, 0, time.UTC))
}
