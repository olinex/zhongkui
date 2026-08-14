package serializer

import (
	"database/sql/driver"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// DurationMS 基于毫秒的时间差类型。
// 序列化输出整数毫秒（与 gRPC 协议、前端 JSON 契约一致），文本与反序列化
// 同时接受可读字符串（如 "5s"、"1m30s"）与整数毫秒，用于以毫秒为单位的
// 时长配置（如抓包读超时时间）的交互输入与配置解析。
type DurationMS int64

// Std 返回 DurationMS 等价的 time.Duration，用于与标准库 time 包协作。
func (d DurationMS) Std() time.Duration {
	return time.Duration(d) * time.Millisecond
}

// String 返回可读的时长字符串，如 "5s"、"1m30s"；零值返回 "0s"。
func (d DurationMS) String() string {
	return d.Std().String()
}

// Milliseconds 返回毫秒数。
func (d DurationMS) Milliseconds() int64 {
	return int64(d)
}

// IsZero 判断时长是否为 0。
func (d DurationMS) IsZero() bool {
	return d == 0
}

// unmarshalText 解析时长文本：优先按 time.ParseDuration（如 "5s"、"1m30s"），
// 纯数字按毫秒处理（兼容整数毫秒格式）；空文本按 0 处理。
func (d *DurationMS) unmarshalText(text []byte) error {
	s := strings.TrimSpace(string(text))
	if s == "" {
		*d = 0
		return nil
	}
	if dur, err := time.ParseDuration(s); err == nil {
		*d = DurationMS(dur / time.Millisecond)
		return nil
	} else if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		*d = DurationMS(n)
		return nil
	} else {
		return errors.Newf("Invalid duration format: %s", s)
	}
}

// UnmarshalText 实现 encoding.TextUnmarshaler，支持 "5s"、"1m30s" 与整数毫秒。
func (d *DurationMS) UnmarshalText(text []byte) error {
	return d.unmarshalText(text)
}

// MarshalText 实现 encoding.TextMarshaler，输出可读字符串（如 "5s"）。
func (d DurationMS) MarshalText() ([]byte, error) {
	return []byte(d.String()), nil
}

// UnmarshalJSON 实现 json.Unmarshaler，接受 "5s" 字符串或整数毫秒。
func (d *DurationMS) UnmarshalJSON(data []byte) error {
	s := strings.TrimSpace(string(data))
	if s == "" || s == "null" {
		*d = 0
		return nil
	}
	if s[0] == '"' {
		var raw string
		if err := json.Unmarshal(data, &raw); err != nil {
			return errors.WithStack(err)
		}
		return d.unmarshalText([]byte(raw))
	}
	return d.unmarshalText([]byte(s))
}

// MarshalJSON 实现 json.Marshaler，输出整数毫秒（与 gRPC 协议保持一致）。
func (d DurationMS) MarshalJSON() ([]byte, error) {
	return json.Marshal(int64(d))
}

// Value 实现 driver.Valuer，数据库以整数毫秒（BIGINT）存储。
func (d DurationMS) Value() (driver.Value, error) {
	return int64(d), nil
}

// Scan 实现 sql.Scanner，兼容整数毫秒、文本时长与 NULL。
func (d *DurationMS) Scan(value any) error {
	switch v := value.(type) {
	case nil:
		*d = 0
		return nil
	case int64:
		*d = DurationMS(v)
		return nil
	case float64:
		*d = DurationMS(int64(v))
		return nil
	case []byte:
		return d.unmarshalText(v)
	case string:
		return d.unmarshalText([]byte(v))
	default:
		return errors.Newf("Unsupported duration type: %T", value)
	}
}

// DurationNS 基于 time.DurationNS 的时间差类型。
// JSON 与文本序列化统一输出可读字符串（如 "5s"、"1m30s"），反序列化同样接受
// 该字符串格式（并兼容整数纳秒），保证序列化与反序列化对称。
// 数据库存储保持整数纳秒（BIGINT），经 Valuer/Scanner 对称读写。
type DurationNS time.Duration

// Std 返回等价的 time.Duration，用于与标准库 time 包协作（参数传递、运算等）。
func (d DurationNS) Std() time.Duration {
	return time.Duration(d)
}

// String 返回可读的时长字符串，如 "5s"、"1m30s"；零值返回 "0s"。
func (d DurationNS) String() string {
	return d.Std().String()
}

// IsZero 判断时长是否为 0。
func (d DurationNS) IsZero() bool {
	return d == 0
}

// Abs 返回时长的绝对值。
func (d DurationNS) Abs() DurationNS {
	return DurationNS(d.Std().Abs())
}

// Truncate 返回将时长截断为 m 的整数倍后得到的时长。
func (d DurationNS) Truncate(m DurationNS) DurationNS {
	return DurationNS(d.Std().Truncate(m.Std()))
}

// Round 返回将时长四舍五入为 m 的整数倍后得到的时长。
func (d DurationNS) Round(m DurationNS) DurationNS {
	return DurationNS(d.Std().Round(m.Std()))
}

// Nanoseconds 返回时长的纳秒数。
func (d DurationNS) Nanoseconds() int64 {
	return d.Std().Nanoseconds()
}

// Microseconds 返回时长的微秒数。
func (d DurationNS) Microseconds() int64 {
	return d.Std().Microseconds()
}

// Milliseconds 返回时长的毫秒数。
func (d DurationNS) Milliseconds() int64 {
	return d.Std().Milliseconds()
}

// Seconds 返回以秒为单位的时长（浮点数）。
func (d DurationNS) Seconds() float64 {
	return d.Std().Seconds()
}

// Minutes 返回以分钟为单位的时长（浮点数）。
func (d DurationNS) Minutes() float64 {
	return d.Std().Minutes()
}

// Hours 返回以小时为单位的时长（浮点数）。
func (d DurationNS) Hours() float64 {
	return d.Std().Hours()
}

// unmarshalText 解析时长文本：优先按 time.ParseDuration（如 "5s"、"1m30s"），
// 纯数字按纳秒处理（兼容整数纳秒格式）；空文本按 0 处理。
func (d *DurationNS) unmarshalText(text []byte) error {
	s := strings.TrimSpace(string(text))
	if s == "" {
		*d = 0
		return nil
	}
	if dur, err := time.ParseDuration(s); err == nil {
		*d = DurationNS(dur)
		return nil
	} else if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		*d = DurationNS(n)
		return nil
	} else {
		return errors.Newf("Invalid duration format: %s", s)
	}
}

// UnmarshalText 实现 encoding.TextUnmarshaler，支持 "5s"、"1m30s" 等格式。
func (d *DurationNS) UnmarshalText(text []byte) error {
	return d.unmarshalText(text)
}

// MarshalText 实现 encoding.TextMarshaler，输出可读字符串（如 "5s"）。
func (d DurationNS) MarshalText() ([]byte, error) {
	return []byte(d.String()), nil
}

// UnmarshalJSON 实现 json.Unmarshaler，接受 "5s" 字符串或整数纳秒。
func (d *DurationNS) UnmarshalJSON(data []byte) error {
	s := strings.TrimSpace(string(data))
	if s == "" || s == "null" {
		*d = 0
		return nil
	}
	if s[0] == '"' {
		var raw string
		if err := json.Unmarshal(data, &raw); err != nil {
			return errors.WithStack(err)
		}
		return d.unmarshalText([]byte(raw))
	}
	return d.unmarshalText([]byte(s))
}

// MarshalJSON 实现 json.Marshaler，输出可读字符串（如 "5s"）。
func (d DurationNS) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

// Value 实现 driver.Valuer，数据库以整数纳秒（BIGINT）存储。
func (d DurationNS) Value() (driver.Value, error) {
	return int64(d), nil
}

// Scan 实现 sql.Scanner，兼容整数纳秒、文本时长与 NULL。
func (d *DurationNS) Scan(value any) error {
	switch v := value.(type) {
	case nil:
		*d = 0
		return nil
	case int64:
		*d = DurationNS(v)
		return nil
	case float64:
		*d = DurationNS(int64(v))
		return nil
	case []byte:
		return d.unmarshalText(v)
	case string:
		return d.unmarshalText([]byte(v))
	default:
		return errors.Newf("Unsupported duration type: %T", value)
	}
}
