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
		return "String CODEC(ZSTD(3))"
	default:
		return ""
	}
}

func (m MacAddr) Value() (driver.Value, error) {
	if len(m) == 0 {
		return nil, nil
	}
	return m.String(), nil
}

func (m *MacAddr) Scan(value any) error {
	if value == nil {
		*m = nil
		return nil
	}
	var macStr string
	switch v := value.(type) {
	case string:
		macStr = v
	case []byte:
		macStr = string(v)
	default:
		return errors.Newf("Unsupport mac value type: %T", value)
	}
	if macStr == "" {
		*m = nil
		return nil
	}
	addr, err := net.ParseMAC(macStr)
	if err != nil {
		return errors.WithStack(err)
	}
	*m = MacAddr(addr)
	return nil
}
