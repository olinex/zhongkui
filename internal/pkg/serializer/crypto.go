package serializer

import (
	"context"
	"reflect"
	"unsafe"

	"gorm.io/gorm/schema"
	"zhongkui.com/zhongkui/internal/pkg/crypto/key"
	"zhongkui.com/zhongkui/internal/pkg/crypto/mode"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	t "zhongkui.com/zhongkui/internal/pkg/tag"
)

const (
	SECURITY_TAG_KEY = "secure"
)

type CryptoTag struct {
	Mode   string `tag:"mode"`
	Offset int    `tag:"offset"`
	Length int    `tag:"length"`
}

type Crypto struct {
	value []byte
}

func NewCrypto(value []byte) *Crypto {
	return &Crypto{value: value}
}

func (c *Crypto) Value() []byte {
	return c.value
}

type CryptoSerializer struct{}

func (cs CryptoSerializer) getTagMode(field *schema.Field) (mode.Mode, error) {
	var cTag CryptoTag
	tag := field.StructField.Tag.Get(SECURITY_TAG_KEY)
	if err := t.Unmarshal(tag, &cTag); err != nil {
		return nil, errors.WithStack(err)
	}
	kd := key.GetKeyDerivation()
	cryptoKey, err := kd.DeriveChildKey(cTag.Offset, cTag.Length)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var m mode.Mode
	switch cTag.Mode {
	case mode.AES_GCM:
		m, err = mode.NewAESGCMMode(cryptoKey, nil)
	default:
		return nil, errors.Newf("unknown crypto mode: %s", cTag.Mode)
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return m, nil
}

func (cs CryptoSerializer) Scan(ctx context.Context, field *schema.Field, dst reflect.Value, dbValue any) error {
	// 如果数据库值为 nil，则置零字段并返回
	if dbValue == nil {
		field.ReflectValueOf(ctx, dst).Set(reflect.Zero(field.FieldType))
		return nil
	}
	// 断言数据库值为 []byte
	cipherText, ok := dbValue.([]byte)
	if !ok {
		return errors.Newf(`expected []byte, got %T`, dbValue)
	}
	// 解析标签
	m, err := cs.getTagMode(field)
	if err != nil {
		return errors.WithStack(err)
	}
	// 解密
	plainText, err := m.Decrypt(cipherText)
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
	// 设置解密后的字节数组到 value 字段
	// 注意：value 是 unexported 字段，在 Go 1.22+ 中 reflect.Set 会因 flagRO 被拒绝
	// 因此通过 unsafe.Pointer 绕过只读检查
	valueField := fieldValue.FieldByName("value")
	if !valueField.IsValid() {
		return errors.New("crypto: Crypto missing value field")
	}
	reflect.NewAt(valueField.Type(), unsafe.Pointer(valueField.UnsafeAddr())).Elem().Set(reflect.ValueOf(plainText))
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
