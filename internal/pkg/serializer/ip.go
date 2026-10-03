package serializer

import (
	"database/sql/driver"
	"encoding/json"
	"net"
	"strings"

	"gorm.io/gorm/schema"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// IPAddr 是以 net.IP 为底层的通用 IP 地址类型，支持数据库、JSON 与文本之间的序列化。
type IPAddr net.IP

// NewIPAddr 从任意值（字符串或字节切片）解析并返回一个 *IPAddr。
func NewIPAddr(value any) (*IPAddr, error) {
	var ip IPAddr
	if err := ip.Scan(value); err != nil {
		return nil, errors.WithStack(err)
	}
	return &ip, nil
}

// String 返回 IP 地址的可读字符串表示。
func (i IPAddr) String() string {
	return net.IP(i).String()
}

// UnmarshalJSON 实现 json.Unmarshaler 接口，从 JSON 值解析 IP 地址。
func (i *IPAddr) UnmarshalJSON(data []byte) error {
	ip := string(data)
	if ip == "null" || ip == "" {
		return nil
	}
	// 移除引号
	ip = strings.Trim(ip, "\"")
	if err := i.Scan(ip); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// MarshalJSON 实现 json.Marshaler 接口，将 IP 地址序列化为 JSON 字符串。
func (i IPAddr) MarshalJSON() ([]byte, error) {
	return json.Marshal(i.String())
}

// GormDBDataType 返回该类型在 GORM 中对应的数据库列类型（postgres 为 INET，
// clickhouse 为 IPv6，其余为空）。
func (i IPAddr) GormDBDataType(db *database.DB, field *schema.Field) string {
	switch db.Dialector.Name() {
	case "postgres":
		return "INET"
	case "clickhouse":
		// IPv6 原生类型：16 字节定长（IPv4 写入时提升为 IPv4-mapped IPv6），
		// 支持文本渲染与字符串隐式比较，可读性优于 FixedString(16) 原始二进制
		return "IPv6"
	default:
		return ""
	}
}

// Value 实现 driver.Valuer 接口，返回写入数据库的值（统一为 16 字节形式）；空地址返回 nil。
func (i IPAddr) Value() (driver.Value, error) {
	if len(i) == 0 {
		return nil, nil
	}
	// 统一以 net.IP 底层 16 字节原始字节（IPv4 提升为 IPv4-mapped IPv6 形式）写入，
	// 与 IPv6Addr 的写入格式保持一致，兼容 ClickHouse IPv6 原生类型列
	return net.IP(i).To16(), nil
}

// Scan 实现 sql.Scanner 接口，从数据库值解析 IP 地址；nil 值置为空地址。
func (i *IPAddr) Scan(value any) error {
	if value == nil {
		*i = nil
		return nil
	}
	var ip net.IP
	switch v := value.(type) {
	case string:
		if len(v) == 16 {
			// ClickHouse FixedString(16) 驱动返回原生 16 字节二进制（IPv4 为 IPv4-mapped IPv6）
			ip = net.IP(v).To16()
		} else {
			ip = net.ParseIP(v)
		}
	case []byte:
		if len(v) == 16 {
			ip = net.IP(v).To16()
		} else {
			ip = net.ParseIP(string(v))
		}
	case net.IP:
		ip = v
	default:
		return errors.Newf(`Unsupport ip value type: %T`, value)
	}
	*i = IPAddr(ip)
	return nil
}

// IPv4Addr 是以 net.IP 为底层的 IPv4 地址类型，支持数据库、JSON 与文本之间的序列化。
type IPv4Addr net.IP

// NewIPv4Addr 从任意值（字符串或字节切片）解析并返回一个 *IPv4Addr。
func NewIPv4Addr(value any) (*IPv4Addr, error) {
	var ip IPv4Addr
	if err := ip.Scan(value); err != nil {
		return nil, errors.WithStack(err)
	}
	return &ip, nil
}

// String 返回 IPv4 地址的可读字符串表示。
func (i IPv4Addr) String() string {
	return net.IP(i).String()
}

// UnmarshalJSON 实现 json.Unmarshaler 接口，从 JSON 值解析 IPv4 地址。
func (i *IPv4Addr) UnmarshalJSON(data []byte) error {
	ip := string(data)
	if ip == "null" || ip == "" {
		return nil
	}
	// 移除引号
	ip = strings.Trim(ip, "\"")
	if err := i.Scan(ip); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// MarshalJSON 实现 json.Marshaler 接口，将 IPv4 地址序列化为 JSON 字符串。
func (i IPv4Addr) MarshalJSON() ([]byte, error) {
	return json.Marshal(i.String())
}

// GormDBDataType 返回该类型在 GORM 中对应的数据库列类型（postgres 为 INET，
// clickhouse 为 IPv4，其余为空）。
func (i IPv4Addr) GormDBDataType(db *database.DB, field *schema.Field) string {
	switch db.Dialector.Name() {
	case "postgres":
		return "INET"
	case "clickhouse":
		// IPv4 原生类型：4 字节定长（UInt32 封装），支持文本渲染与字符串隐式比较
		return "IPv4"
	default:
		return ""
	}
}

// Value 实现 driver.Valuer 接口，返回写入数据库的值（4 字节形式）；空地址返回 nil。
func (i IPv4Addr) Value() (driver.Value, error) {
	if len(i) == 0 {
		return nil, nil
	}
	return net.IP(i).To4(), nil
}

// Scan 实现 sql.Scanner 接口，从数据库值解析 IPv4 地址；非 IPv4 时返回错误。
func (i *IPv4Addr) Scan(value any) error {
	if value == nil {
		*i = nil
		return nil
	}
	var ip net.IP
	switch v := value.(type) {
	case string:
		if len(v) == 4 {
			// ClickHouse FixedString(4) 驱动返回 string 类型的原始二进制 IP
			ip = net.IP(v).To4()
		} else {
			ip = net.ParseIP(v)
		}
	case []byte:
		if len(v) == 4 {
			ip = net.IP(v).To4()
		} else {
			ip = net.ParseIP(string(v))
		}
	case net.IP:
		ip = v
	default:
		return errors.Newf(`Unsupport ipv4 value type: %T`, value)
	}
	if ip.To4() == nil {
		return errors.Newf("Not valid ipv4")
	}
	*i = IPv4Addr(ip.To4())
	return nil
}

// IPv6Addr 是以 net.IP 为底层的 IPv6 地址类型，支持数据库、JSON 与文本之间的序列化。
type IPv6Addr net.IP

// NewIPv6Addr 从任意值（字符串或字节切片）解析并返回一个 *IPv6Addr。
func NewIPv6Addr(value any) (*IPv6Addr, error) {
	var ip IPv6Addr
	if err := ip.Scan(value); err != nil {
		return nil, errors.WithStack(err)
	}
	return &ip, nil
}

// String 返回 IPv6 地址的可读字符串表示。
func (i IPv6Addr) String() string {
	return net.IP(i).String()
}

// UnmarshalJSON 实现 json.Unmarshaler 接口，从 JSON 值解析 IPv6 地址。
func (i *IPv6Addr) UnmarshalJSON(data []byte) error {
	ip := string(data)
	if ip == "null" || ip == "" {
		return nil
	}
	// 移除引号
	ip = strings.Trim(ip, "\"")
	if err := i.Scan(ip); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// MarshalJSON 实现 json.Marshaler 接口，将 IPv6 地址序列化为 JSON 字符串。
func (i IPv6Addr) MarshalJSON() ([]byte, error) {
	return json.Marshal(i.String())
}

// GormDBDataType 返回该类型在 GORM 中对应的数据库列类型（postgres 为 INET，
// clickhouse 为 IPv6，其余为空）。
func (i IPv6Addr) GormDBDataType(db *database.DB, field *schema.Field) string {
	switch db.Dialector.Name() {
	case "postgres":
		return "INET"
	case "clickhouse":
		// IPv6 原生类型：16 字节定长，支持文本渲染与字符串隐式比较
		return "IPv6"
	default:
		return ""
	}
}

// Value 实现 driver.Valuer 接口，返回写入数据库的值（16 字节形式）；空地址返回 nil。
func (i IPv6Addr) Value() (driver.Value, error) {
	if len(i) == 0 {
		return nil, nil
	}
	return net.IP(i).To16(), nil
}

// Scan 实现 sql.Scanner 接口，从数据库值解析 IPv6 地址；非 IPv6 时返回错误。
func (i *IPv6Addr) Scan(value any) error {
	if value == nil {
		*i = nil
		return nil
	}
	var ip net.IP
	switch v := value.(type) {
	case string:
		if len(v) == 16 {
			// ClickHouse FixedString(16) 驱动返回 string 类型的原始二进制 IP
			ip = net.IP(v).To16()
		} else {
			ip = net.ParseIP(v)
		}
	case []byte:
		if len(v) == 16 {
			ip = net.IP(v).To16()
		} else {
			ip = net.ParseIP(string(v))
		}
	case net.IP:
		ip = v
	default:
		return errors.Newf(`Unsupport ipv6 value type: %T`, value)
	}
	if ip.To16() == nil {
		return errors.Newf("Not valid ipv6")
	}
	*i = IPv6Addr(ip.To16())
	return nil
}
