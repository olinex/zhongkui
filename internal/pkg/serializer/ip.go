package serializer

import (
	"database/sql/driver"
	"encoding/json"
	"net"
	"strings"

	"gorm.io/gorm/schema"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

type IPAddr net.IP

func NewIPAddr(value any) (*IPAddr, error) {
	var ip IPAddr
	if err := ip.Scan(value); err != nil {
		return nil, errors.WithStack(err)
	}
	return &ip, nil
}

func (i IPAddr) String() string {
	return net.IP(i).String()
}

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

func (i IPAddr) MarshalJSON() ([]byte, error) {
	return json.Marshal(i.String())
}

func (i IPAddr) GormDBDataType(db *database.DB, field *schema.Field) string {
	switch db.Dialector.Name() {
	case "postgres":
		return "INET"
	case "clickhouse":
		return "FixedString(16) CODEC(ZSTD(3))"
	default:
		return ""
	}
}

func (i IPAddr) Value() (driver.Value, error) {
	if len(i) == 0 {
		return nil, nil
	}
	// 统一以 net.IP 底层 16 字节原始字节数组写入（IPv4 提升为 IPv4-mapped IPv6 形式），
	// 与 IPv6Addr 的 FixedString(16) 写入格式保持一致，避免定长文本的容量限制
	return []byte(net.IP(i).To16()), nil
}

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

type IPv4Addr net.IP

func NewIPv4Addr(value any) (*IPv4Addr, error) {
	var ip IPv4Addr
	if err := ip.Scan(value); err != nil {
		return nil, errors.WithStack(err)
	}
	return &ip, nil
}

func (i IPv4Addr) String() string {
	return net.IP(i).String()
}

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

func (i IPv4Addr) MarshalJSON() ([]byte, error) {
	return json.Marshal(i.String())
}

func (i IPv4Addr) GormDBDataType(db *database.DB, field *schema.Field) string {
	switch db.Dialector.Name() {
	case "postgres":
		return "INET"
	case "clickhouse":
		return "FixedString(4)"
	default:
		return ""
	}
}

func (i IPv4Addr) Value() (driver.Value, error) {
	if len(i) == 0 {
		return nil, nil
	}
	return []byte(net.IP(i).To4()), nil
}

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

type IPv6Addr net.IP

func NewIPv6Addr(value any) (*IPv6Addr, error) {
	var ip IPv6Addr
	if err := ip.Scan(value); err != nil {
		return nil, errors.WithStack(err)
	}
	return &ip, nil
}

func (i IPv6Addr) String() string {
	return net.IP(i).String()
}

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

func (i IPv6Addr) MarshalJSON() ([]byte, error) {
	return json.Marshal(i.String())
}

func (i IPv6Addr) GormDBDataType(db *database.DB, field *schema.Field) string {
	switch db.Dialector.Name() {
	case "postgres":
		return "INET"
	case "clickhouse":
		return "FixedString(16)"
	default:
		return ""
	}
}

func (i IPv6Addr) Value() (driver.Value, error) {
	if len(i) == 0 {
		return nil, nil
	}
	return []byte(net.IP(i).To16()), nil
}

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
