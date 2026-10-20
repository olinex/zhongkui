package serializer

import (
	"context"
	"encoding/json"
	"reflect"

	"gorm.io/gorm/schema"
	"zhongkuisec.com/zhongkui/internal/pkg/crypto"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// Hash 是数据库摘要字段的类型化封装：字段持有由 crypto 层预计算的十六进制
// HMAC-SHA256 摘要，序列化器只负责 string/[]byte 互转、nil 安全与 JSON，不做
// 加解密。摘要单向且确定性（同一密钥与内容恒得同一摘要，可等值匹配与建唯一索引），
// 与可逆且随机的 Crypto 语义相反，故不能复用 Crypto 承载。
type Hash struct {
	// digest 十六进制 HMAC-SHA256 摘要，零值表示未设置（空摘要）
	digest string
}

// NewHashDigest 以十六进制摘要构造 Hash 实例，供写库侧使用。
func NewHashDigest(digest string) Hash {
	return Hash{digest: digest}
}

// Digest 返回十六进制摘要；未设置值（零值 Hash）时返回空字符串。
func (h Hash) Digest() string {
	return h.digest
}

// String 返回十六进制摘要，便于日志输出与字符串拼接。
func (h Hash) String() string {
	return h.digest
}

// MarshalJSON 将 Hash 序列化为 JSON 字符串（零值输出空字符串）。
func (h Hash) MarshalJSON() ([]byte, error) {
	return json.Marshal(h.digest)
}

// UnmarshalJSON 从 JSON 恢复 Hash 实例，支持两种取值形式：
//   - 字符串：十六进制摘要，与 MarshalJSON 输出一致；
//   - null 或空串：视为未设置，恢复零值 Hash。
func (h *Hash) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		h.digest = ""
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return errors.WithStack(err)
	}
	h.digest = s
	return nil
}

// HashSerializer 实现 GORM 摘要字段的序列化器：仅做类型包装与标签模式校验，
// 摘要本身由 crypto 层在写库前预计算（见 crypto.DigestForField / crypto.HashKeyForField）。
type HashSerializer struct{}

// validateTag 解析字段的 secure 标签并校验算法标识为 hmac-sha256，
// 防止把 serializer:hash 误挂到可逆加密（aes-gcm）字段上。
func (hs HashSerializer) validateTag(field *schema.Field) (crypto.CryptoTag, error) {
	cTag, err := crypto.ParseCryptoTag(field.StructField.Tag.Get(crypto.SecurityTagKey))
	if err != nil {
		return crypto.CryptoTag{}, errors.WithStack(err)
	}
	if cTag.Mode != crypto.HmacSHA256 {
		return crypto.CryptoTag{}, errors.Newf(`hash: field mode must be %q, got %q`, crypto.HmacSHA256, cTag.Mode)
	}
	return cTag, nil
}

// Scan 从数据库读取字段值，包装为 Hash；空值与空字符串置零值。
func (hs HashSerializer) Scan(ctx context.Context, field *schema.Field, dst reflect.Value, dbValue any) error {
	// 断言数据库值为 []byte 或 string（ClickHouse String 列返回 string 类型）
	var raw string
	switch v := dbValue.(type) {
	case nil:
	case []byte:
		raw = string(v)
	case string:
		raw = v
	default:
		return errors.Newf(`hash: expected []byte or string, got %T`, dbValue)
	}
	field.ReflectValueOf(ctx, dst).Set(reflect.ValueOf(NewHashDigest(raw)))
	return nil
}

// Value 校验字段标签后返回已预计算的十六进制摘要字符串，空摘要返回空字符串
// （匹配 CHAR(64) NOT NULL DEFAULT ” 列定义）。
func (hs HashSerializer) Value(ctx context.Context, field *schema.Field, dst reflect.Value, fieldValue any) (any, error) {
	if _, err := hs.validateTag(field); err != nil {
		return nil, errors.WithStack(err)
	}
	h, ok := fieldValue.(Hash)
	if !ok {
		return nil, errors.Newf(`hash: field value must be serializer.Hash, got %T`, fieldValue)
	}
	return h.digest, nil
}

func init() {
	// 注册 HashSerializer
	schema.RegisterSerializer("hash", HashSerializer{})
}
