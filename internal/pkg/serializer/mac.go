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

// MacAddr 是以 net.HardwareAddr 为底层的 MAC 地址类型，支持数据库、JSON 与文本之间的序列化。
type MacAddr net.HardwareAddr

// NewMacAddr 从任意值（字符串或字节切片）解析并返回一个 *MacAddr。
func NewMacAddr(value any) (*MacAddr, error) {
	var m MacAddr
	if err := m.Scan(value); err != nil {
		return nil, errors.WithStack(err)
	}
	return &m, nil
}

// String 返回 MAC 地址的可读字符串表示。
func (m MacAddr) String() string {
	return net.HardwareAddr(m).String()
}

// UnmarshalJSON 实现 json.Unmarshaler 接口，从 JSON 值解析 MAC 地址。
func (m *MacAddr) UnmarshalJSON(data []byte) error {
	mac := string(data)
	if mac == "null" || mac == "" {
		return nil
	}
	// 移除引号
	mac = strings.Trim(mac, "\"")
	if err := m.Scan(mac); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// MarshalJSON 实现 json.Marshaler 接口，将 MAC 地址序列化为 JSON 字符串。
func (m MacAddr) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.String())
}

// GormDBDataType 返回该类型在 GORM 中对应的数据库列类型（postgres 为 MACADDR，
// clickhouse 为 FixedString(6)，其余为空）。
func (m MacAddr) GormDBDataType(db *database.DB, field *schema.Field) string {
	name := db.Dialector.Name()
	switch name {
	case "postgres":
		return "MACADDR"
	case "clickhouse":
		return "FixedString(6)"
	default:
		return ""
	}
}

// Value 实现 driver.Valuer 接口，返回写入数据库的值；空地址返回 nil。
func (m MacAddr) Value() (driver.Value, error) {
	if len(m) == 0 {
		return nil, nil
	}
	return []byte(m), nil
}

// Scan 实现 sql.Scanner 接口，从数据库值解析 MAC 地址；nil 或空值置为空地址。
func (m *MacAddr) Scan(value any) error {
	if value == nil {
		*m = nil
		return nil
	}
	switch v := value.(type) {
	case string:
		if v == "" {
			*m = nil
			return nil
		}
		if len(v) == 6 {
			// ClickHouse FixedString(6) 驱动返回 string 类型的原始二进制 MAC
			*m = MacAddr(net.HardwareAddr(v))
		} else {
			addr, err := net.ParseMAC(v)
			if err != nil {
				return errors.WithStack(err)
			}
			*m = MacAddr(addr)
		}
	case []byte:
		if len(v) == 0 {
			*m = nil
			return nil
		}
		if len(v) == 6 {
			// FixedString(6) 返回的原始二进制 MAC 地址
			*m = MacAddr(net.HardwareAddr(v))
		} else {
			// 字符串形式的 MAC 地址
			addr, err := net.ParseMAC(string(v))
			if err != nil {
				return errors.WithStack(err)
			}
			*m = MacAddr(addr)
		}
	default:
		return errors.Newf("Unsupport mac value type: %T", value)
	}
	return nil
}
