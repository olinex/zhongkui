// Package serializer 提供 GORM 字段与 JSON 序列化相关的自定义类型及其编解码实现。
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

// HexUUID 是以二进制格式存储的 UUID 类型（底层为 datatypes.BinUUID），实现 JSON、
// 数据库编解码与 GORM 列类型映射。
type HexUUID datatypes.BinUUID

// NewHexUUIDv4 生成一个新的随机（v4）HexUUID。
func NewHexUUIDv4() HexUUID {
	return HexUUID(datatypes.NewBinUUIDv4())
}

// NewNilHexUUIDv4 返回一个全零（nil）的 HexUUID。
func NewNilHexUUIDv4() HexUUID {
	return HexUUID(datatypes.NewNilBinUUID())
}

// MustHexUUIDv4FromString 由 UUID 字符串解析出 HexUUID，解析失败时 panic。
func MustHexUUIDv4FromString(data string) HexUUID {
	u := datatypes.BinUUIDFromString(data)
	return HexUUID(u)
}

// HexUUIDv4FromString 由 UUID 字符串解析出 HexUUID，解析失败时返回错误。
func HexUUIDv4FromString(data string) (HexUUID, error) {
	if tmp, err := uuid.Parse(data); err != nil {
		return NewNilHexUUIDv4(), errors.WithStack(err)
	} else {
		u := datatypes.BinUUID(tmp)
		return HexUUID(u), nil
	}
}

// IsNil 判断该 UUID 是否为零值。
func (h HexUUID) IsNil() bool {
	return datatypes.BinUUID(h).IsNil()
}

// IsEmpty 判断该 UUID 是否为空。
func (h HexUUID) IsEmpty() bool {
	return datatypes.BinUUID(h).IsEmpty()
}

// String 返回该 UUID 的字符串形式。
func (h HexUUID) String() string {
	return datatypes.BinUUID(h).String()
}

// UnmarshalJSON 实现 json.Unmarshaler，支持 UUID 字符串与 null 形式。
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

// MarshalJSON 实现 json.Marshaler，输出 UUID 字符串。
func (h HexUUID) MarshalJSON() ([]byte, error) {
	return json.Marshal(h.String())
}

// Value 实现 driver.Valuer，返回 UUID 字符串以写入数据库。
func (h HexUUID) Value() (driver.Value, error) {
	return h.String(), nil
}

// Scan 实现 sql.Scanner，支持 [16]byte、[]byte、string 与 nil 形式的取值。
func (h *HexUUID) Scan(value any) error {
	if value == nil {
		*h = HexUUID(datatypes.NewNilBinUUID())
		return nil
	}
	switch v := value.(type) {
	case [16]byte:
		*h = HexUUID(datatypes.BinUUID(v))
	case []byte:
		if len(v) == 16 {
			*h = HexUUID(datatypes.BinUUID(v))
		} else {
			// 尝试按 UUID 字符串解析
			u, err := uuid.Parse(string(v))
			if err != nil {
				return errors.WithStack(err)
			}
			*h = HexUUID(u)
		}
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

// GormDBDataType 实现 GORM 的列类型映射：按数据库方言返回 UUID / FixedString(16)。
func (h HexUUID) GormDBDataType(db *database.DB, field *schema.Field) string {
	switch db.Dialector.Name() {
	case "postgres":
		return "UUID"
	case "clickhouse":
		return "FixedString(16)"
	default:
		return ""
	}
}

// HexUUIDs 是 HexUUID 的切片类型，实现 sql.Scanner 与 driver.Valuer，
// 用于 ClickHouse Array(UUID)（驱动返回 []uuid.UUID）与 PostgreSQL JSONB
// （JSON 数组文本）之间的双向转换；JSON 序列化行为与 []HexUUID 完全一致
// （元素各自按 UUID 字符串编码）。
type HexUUIDs []HexUUID

// Scan 实现 sql.Scanner，支持 ClickHouse Array(UUID)（[]uuid.UUID）、
// JSON 数组文本（[]byte / string，如 PostgreSQL JSONB 经驱动返回）与 nil。
func (h *HexUUIDs) Scan(value any) error {
	switch v := value.(type) {
	case nil:
		*h = nil
	case []uuid.UUID:
		out := make(HexUUIDs, 0, len(v))
		for _, u := range v {
			out = append(out, HexUUID(u))
		}
		*h = out
	case []byte:
		return json.Unmarshal(v, h)
	case string:
		return json.Unmarshal([]byte(v), h)
	default:
		return errors.Newf("unsupported scan type: %T", value)
	}
	return nil
}

// Value 实现 driver.Valuer，返回 []uuid.UUID 以匹配 ClickHouse Array(UUID)
// 列的原生驱动类型（Go 侧经 database/sql 透传给驱动的数组绑定）。
func (h HexUUIDs) Value() (driver.Value, error) {
	out := make([]uuid.UUID, 0, len(h))
	for _, id := range h {
		out = append(out, uuid.UUID(id))
	}
	return out, nil
}

// GormDataType 实现 GormDataTypeInterface，返回 string 占位类型，
// 使 GORM 不将该字段当作关联关系解析（切片字段默认会被当作 has-many 关联尝试解析）；
// 实际列类型由字段 gorm tag 的 type 显式指定（如 Array(UUID) / JSONB）。
func (HexUUIDs) GormDataType() string {
	return string(schema.String)
}

// Capitalize 返回首字母大写后的字符串。
func Capitalize(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}
