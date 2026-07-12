package serializer

import (
	"context"
	"reflect"

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
	Value []byte
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
	fieldValue = fieldValue.Elem()
	if fieldValue.Kind() != reflect.Struct {
		return errors.New("crypto: field must be a struct (Crypto)")
	}
	// 设置解密后的字节数组到 Value 字段
	valueField := fieldValue.FieldByName("Value")
	if !valueField.IsValid() || !valueField.CanSet() {
		return errors.New("crypto: Crypto missing Value field or not settable")
	}
	valueField.Set(reflect.ValueOf(plainText))
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
	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return nil, errors.New("crypto: field value must be a struct (Crypto)")
	}
	// 获取 Value 字段（[]byte）
	valueField := v.FieldByName("Value")
	if !valueField.IsValid() {
		return nil, errors.New("crypto: Crypto missing Value field")
	}
	plaintext, ok := valueField.Interface().([]byte)
	if !ok {
		return nil, errors.Newf("crypto: Crypto.Value must be []byte, got %T", valueField.Interface())
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
