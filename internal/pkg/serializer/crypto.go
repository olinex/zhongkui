package serializer

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"reflect"
	"unsafe"

	"gorm.io/gorm/schema"
	"zhongkui.com/zhongkui/internal/pkg/crypto"
	"zhongkui.com/zhongkui/internal/pkg/crypto/mode"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// Crypto 持有加密字段的明文与密文
//   - value: 明文，读写库时由 crypto serializer 加解密，GetPayload 等业务读取明文
//   - cipher: 密文，Scan 读库时保留，供 MarshalJSON 输出 base64 密文（详情展示），避免明文泄露
type Crypto struct {
	value  []byte
	cipher []byte
}

func NewCrypto(value []byte) *Crypto {
	return &Crypto{value: value}
}

func (c *Crypto) Value() []byte {
	return c.value
}

// Cipher 返回原始密文字节（Scan 读库时保留），供详情输出编码为 base64 密文文本；
// 写库侧构造或未读库的 Crypto（cipher 为空）返回 nil，避免泄露明文。
// Cipher returns the raw ciphertext bytes retained on Scan, used by detail output to
// encode a base64 ciphertext; returns nil for a write-side or un-scanned Crypto (empty
// cipher), preventing plaintext leakage.
func (c *Crypto) Cipher() []byte {
	if c == nil {
		return nil
	}
	return c.cipher
}

// setBytesField 通过 unsafe.Pointer 绕过只读检查，设置 Crypto 的 unexported 字节字段
// 注意：value/cipher 均为 unexported 字段，在 Go 1.22+ 中 reflect.Set 会因 flagRO 被拒绝
func setBytesField(cryptoValue reflect.Value, fieldName string, data []byte) {
	valueField := cryptoValue.FieldByName(fieldName)
	if !valueField.IsValid() {
		return
	}
	reflect.NewAt(valueField.Type(), unsafe.Pointer(valueField.UnsafeAddr())).Elem().Set(reflect.ValueOf(data))
}

// MarshalJSON 将 Crypto 序列化为 base64 密文
// 读库场景（详情接口）经 Scan 保留密文后输出密文，前端可回传明文接口解密；
// 密文为空（如空数据或写库侧构造的 Crypto）时输出空字符串，避免泄露明文。
func (c *Crypto) MarshalJSON() ([]byte, error) {
	if c == nil || len(c.cipher) == 0 {
		return []byte(`""`), nil
	}
	return json.Marshal(base64.StdEncoding.EncodeToString(c.cipher))
}

// UnmarshalJSON 从 JSON 恢复 Crypto 实例，支持两种取值形式：
//   - 字符串：base64 密文（与 MarshalJSON 输出一致），解码后回填 cipher，不触碰明文；
//   - 其他取值（如 {}、null 占位）：视为无密文，恢复零值 Crypto。
//
// 该能力使标签规则样例等持久化 JSON 文档可反序列化回目标模型
// （如 *network.HttpTransport 的加密字段），与规则运行期样例校验的反序列化路径保持一致。
func (c *Crypto) UnmarshalJSON(data []byte) error {
	// 非字符串取值（{} / null 等占位形式）按零值处理
	if len(data) == 0 || data[0] != '"' {
		c.cipher = nil
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return errors.WithStack(err)
	}
	if s == "" {
		c.cipher = nil
		return nil
	}
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		// 兼容无 padding 的 base64 编码（RawStdEncoding）
		if raw, err = base64.RawStdEncoding.DecodeString(s); err != nil {
			return errors.WithStack(err)
		}
	}
	c.cipher = raw
	return nil
}

type CryptoSerializer struct{}

func (cs CryptoSerializer) getTagMode(field *schema.Field) (mode.Mode, error) {
	// 标签解析、密钥派生与模式构造统一走 crypto 层，与视图层明文查看链路一致
	return crypto.ModeFromTag(field.StructField.Tag.Get(crypto.SecurityTagKey))
}

func (cs CryptoSerializer) Scan(ctx context.Context, field *schema.Field, dst reflect.Value, dbValue any) error {
	// 断言数据库值为 []byte 或 string（ClickHouse String 列返回 string 类型）
	var raw []byte
	switch v := dbValue.(type) {
	case nil:
		raw = nil
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return errors.Newf(`expected []byte or string, got %T`, dbValue)
	}
	// 数据不足 AES-GCM 最小密文长度（nonce 12 + tag 16 = 28 字节）视为空
	// 兼容 ClickHouse 驱动对已消费的嵌入结构体字段传入特殊零值的情况
	if len(raw) < 28 {
		field.ReflectValueOf(ctx, dst).Set(reflect.Zero(field.FieldType))
		return nil
	}
	// 解析标签
	m, err := cs.getTagMode(field)
	if err != nil {
		return errors.WithStack(err)
	}
	// 解密
	plainText, err := m.Decrypt(raw)
	if err != nil {
		return errors.WithStack(err)
	}
	// 获取字段值（指向 Crypto 的指针）
	fieldValue := field.ReflectValueOf(ctx, dst)
	if fieldValue.Kind() != reflect.Pointer {
		return errors.New("crypto: field must be a pointer to Crypto")
	}
	// 如果指针为 nil，先分配一个新的 Crypto 实例
	if fieldValue.IsNil() {
		fieldValue.Set(reflect.New(fieldValue.Type().Elem()))
	}
	fieldValue = fieldValue.Elem()
	if fieldValue.Kind() != reflect.Struct {
		return errors.New("crypto: field must be a struct (Crypto)")
	}
	// 设置解密后的明文与原始密文
	// 注意：value/cipher 是 unexported 字段，在 Go 1.22+ 中 reflect.Set 会因 flagRO 被拒绝
	// 因此通过 unsafe.Pointer 绕过只读检查
	setBytesField(fieldValue, "value", plainText)
	setBytesField(fieldValue, "cipher", raw)
	return nil
}

func (cs CryptoSerializer) Value(ctx context.Context, field *schema.Field, dst reflect.Value, fieldValue any) (any, error) {
	// 如果字段值为 nil，返回 nil（对应数据库 NULL）
	if fieldValue == nil {
		return nil, nil
	}
	// 获取字段值的反射对象
	v := reflect.ValueOf(fieldValue)
	if v.Kind() != reflect.Pointer {
		return nil, errors.New("crypto: field value must be a pointer to Crypto")
	}
	// 处理 nil 指针的情况：(*Crypto)(nil) 作为 interface{} 是非 nil 的，
	// 但底层指针为 nil，此时应返回 NULL
	if v.IsNil() {
		return nil, nil
	}
	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return nil, errors.New("crypto: field value must be a struct (Crypto)")
	}
	// 获取 value 字段（[]byte）
	valueField := v.FieldByName("value")
	if !valueField.IsValid() {
		return nil, errors.New("crypto: Crypto missing value field")
	}
	plaintext := valueField.Bytes()
	// 空数据无需加密，直接返回 nil（对应数据库 NULL）
	if len(plaintext) == 0 {
		return nil, nil
	}
	// 解析标签
	m, err := cs.getTagMode(field)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// 加密
	cipherText, err := m.Encrypt(plaintext)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return cipherText, nil
}

func init() {
	// 注册 CryptoSerializer
	schema.RegisterSerializer("crypto", CryptoSerializer{})
}
