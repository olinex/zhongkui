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

type MacAddr net.HardwareAddr

func NewMacAddr(value any) (*MacAddr, error) {
	var m MacAddr
	if err := m.Scan(value); err != nil {
		return nil, errors.WithStack(err)
	}
	return &m, nil
}

func (m MacAddr) String() string {
	return net.HardwareAddr(m).String()
}

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

func (m MacAddr) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.String())
}

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

func (m MacAddr) Value() (driver.Value, error) {
	if len(m) == 0 {
		return nil, nil
	}
	return []byte(m), nil
}

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
