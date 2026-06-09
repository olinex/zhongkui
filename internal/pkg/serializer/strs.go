package serializer

import (
	"encoding/json"
	"strings"
	"unicode"

	"database/sql/driver"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm/schema"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

type HexUUID datatypes.BinUUID

func NewHexUUIDv4() HexUUID {
	return HexUUID(datatypes.NewBinUUIDv4())
}

func NewNilHexUUIDv4() HexUUID {
	return HexUUID(datatypes.NewNilBinUUID())
}

func MustHexUUIDv4FromString(data string) HexUUID {
	u := datatypes.BinUUIDFromString(data)
	return HexUUID(u)
}

func HexUUIDv4FromString(data string) (HexUUID, error) {
	if tmp, err := uuid.Parse(data); err != nil {
		return NewNilHexUUIDv4(), errors.WithStack(err)
	} else {
		u := datatypes.BinUUID(tmp)
		return HexUUID(u), nil
	}
}

func (h HexUUID) IsNil() bool {
	return datatypes.BinUUID(h).IsNil()
}

func (h HexUUID) IsEmpty() bool {
	return datatypes.BinUUID(h).IsEmpty()
}

func (h HexUUID) String() string {
	return datatypes.BinUUID(h).String()
}

func (h *HexUUID) UnmarshalJSON(data []byte) error {
	id := string(data)
	if id == "null" || id == "" {
		return nil
	}
	// 移除引号
	id = strings.Trim(id, "\"")
	if err := h.Scan(id); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (h HexUUID) MarshalJSON() ([]byte, error) {
	return json.Marshal(h.String())
}

func (h HexUUID) Value() (driver.Value, error) {
	return h.String(), nil
}

func (h *HexUUID) Scan(value any) error {
	if value == nil {
		*h = HexUUID(datatypes.NewNilBinUUID())
		return nil
	}
	switch v := value.(type) {
	case [16]byte:
		*h = HexUUID(datatypes.BinUUID(v))
	case []byte:
		*h = HexUUID(datatypes.BinUUID(v))
	case string:
		if v == "" {
			*h = HexUUID(datatypes.NewNilBinUUID())
			return nil
		}
		u, err := uuid.Parse(v)
		if err != nil {
			return errors.WithStack(err)
		}
		*h = HexUUID(u)
		return nil
	default:
		return errors.Newf("Unsupported date type: %T", value)
	}
	return nil
}

func (h HexUUID) GormDBDataType(db *database.DB, field *schema.Field) string {
	switch db.Dialector.Name() {
	case "postgres":
		return "UUID"
	case "clickhouse":
		return "UUID CODEC(ZSTD(3))"
	default:
		return ""
	}
}

func Capitalize(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}
