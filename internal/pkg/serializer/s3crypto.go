package serializer

import (
	"context"
	"encoding/json"
	"reflect"

	"gorm.io/gorm/schema"
	"zhongkuisec.com/zhongkui/internal/pkg/crypto"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// s3CryptoFieldType S3 对象字段绑定的序列化类型。
var s3CryptoFieldType = reflect.TypeOf(S3Crypto{})

// S3Crypto 是 S3 对象键的类型化封装：数据库列只保存对象键，对象内容以
// zstd + AES-GCM 加密后存于 S3。字段类型须为 serializer.S3Crypto 并携带
// s3crypto 标签，密钥由标签中的 offset/length 派生，每个字段使用独立密钥。
//
// 与 Crypto 的区别：Crypto 的密文随列值落库，读库即完成解密；S3Crypto 只持有
// 对象键，实际加解密与上传下载由业务层显式执行（storeBody、Zkcn、ObjectCache），
// 避免序列化器在读库（含列表查询）时触发远程 I/O。
type S3Crypto struct {
	// key S3 对象键，空字符串表示无对象
	key string
}

// NewS3Crypto 以对象键构造 S3Crypto，供写库侧与扫描侧使用。
func NewS3Crypto(key string) S3Crypto {
	return S3Crypto{key: key}
}

// Key 返回 S3 对象键；零值返回空字符串。
func (s S3Crypto) Key() string {
	return s.key
}

// String 返回 S3 对象键，便于日志输出与字符串拼接。
func (s S3Crypto) String() string {
	return s.key
}

// MarshalJSON 将 S3Crypto 序列化为对象键字符串；零值输出空字符串，
// 与改造前「字段直接存对象键字符串」的 JSON 契约保持一致。
func (s S3Crypto) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.key)
}

// UnmarshalJSON 从 JSON 恢复 S3Crypto 实例，支持两种取值形式：
//   - 字符串：S3 对象键（与 MarshalJSON 输出一致）；
//   - 其他取值（如 {}、null 占位）：视为无对象，恢复零值。
//
// 该能力使标签规则样例等持久化 JSON 文档可反序列化回目标模型
// （如 *network.HttpTransport 的正文对象键字段）。
func (s *S3Crypto) UnmarshalJSON(data []byte) error {
	// 非字符串取值（{} / null 等占位形式）按零值处理
	if len(data) == 0 || data[0] != '"' {
		s.key = ""
		return nil
	}
	var key string
	if err := json.Unmarshal(data, &key); err != nil {
		return errors.WithStack(err)
	}
	s.key = key
	return nil
}

// S3CryptoSerializer 实现 GORM S3 对象键字段的序列化器：只做对象键的
// string/[]byte 互转、空值归一与标签绑定校验，不做任何 S3 网络 I/O。
type S3CryptoSerializer struct{}

// validateTag 校验字段类型为 serializer.S3Crypto 且携带 s3crypto 标签，并解析 offset/length。
// 对象内容固定使用 zstd + AES-GCM 管道，标签不再携带 mode，避免与代码中的固定算法漂移。
func (ss S3CryptoSerializer) validateTag(field *schema.Field) (crypto.S3CryptoTag, error) {
	if err := validateFieldBinding(field, crypto.S3CryptoTagKey, s3CryptoFieldType); err != nil {
		return crypto.S3CryptoTag{}, errors.WithStack(err)
	}
	sTag, err := crypto.ParseS3CryptoTag(field.StructField.Tag.Get(crypto.S3CryptoTagKey))
	if err != nil {
		return crypto.S3CryptoTag{}, errors.WithStack(err)
	}
	return sTag, nil
}

// Scan 从数据库读取对象键，包装为 S3Crypto；nil、空字节与空字符串均置零值。
func (ss S3CryptoSerializer) Scan(ctx context.Context, field *schema.Field, dst reflect.Value, dbValue any) error {
	if _, err := ss.validateTag(field); err != nil {
		return errors.WithStack(err)
	}
	// 断言数据库值为 []byte 或 string（ClickHouse String 列返回 string 类型）
	var raw string
	switch v := dbValue.(type) {
	case nil:
	case []byte:
		raw = string(v)
	case string:
		raw = v
	default:
		return errors.Newf(`s3crypto: expected []byte or string, got %T`, dbValue)
	}
	field.ReflectValueOf(ctx, dst).Set(reflect.ValueOf(NewS3Crypto(raw)))
	return nil
}

// Value 校验字段标签后返回对象键字符串；零值返回空字符串
// （匹配 VARCHAR(512) NOT NULL DEFAULT ” 与 ClickHouse String 列定义，不写 NULL）。
func (ss S3CryptoSerializer) Value(ctx context.Context, field *schema.Field, dst reflect.Value, fieldValue any) (any, error) {
	if _, err := ss.validateTag(field); err != nil {
		return nil, errors.WithStack(err)
	}
	if fieldValue == nil {
		return "", nil
	}
	s, ok := fieldValue.(S3Crypto)
	if !ok {
		return nil, errors.Newf(`s3crypto: field value must be serializer.S3Crypto, got %T`, fieldValue)
	}
	return s.key, nil
}

func init() {
	// 注册 S3CryptoSerializer
	schema.RegisterSerializer("s3crypto", S3CryptoSerializer{})
}
