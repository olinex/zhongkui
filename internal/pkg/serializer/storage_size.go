package serializer

import (
	"encoding/json"
	"strings"

	"github.com/dustin/go-humanize"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// BibyteSize64 以字节为单位的存储容量类型，底层使用 int64
// 支持 JSON/Text/Binary 多种反序列化格式，可解析如 "10 MB"、"1.5 GiB" 等人类可读的容量字符串
type BibyteSize64 uint64

// unmarshal 是内部反序列化方法，将字节切片解析为 BibyteSize 值
// 支持解析如 "10 MB"、"1.5 GiB"、"1024" 等格式的容量字符串
// 如果传入空字符串，则将值设置为 0；如果格式无法解析，则返回错误
func (s *BibyteSize64) unmarshal(text []byte) error {
	sizeStr := string(text)
	if sizeStr == "" {
		*s = 0
		return nil
	}

	bytes, err := humanize.ParseBytes(sizeStr)
	if err != nil {
		return errors.Newf("Invalid storage size format: %s", sizeStr)
	}
	*s = BibyteSize64(bytes)
	return nil
}

func (s BibyteSize64) marshal() []byte {
	return []byte(humanize.IBytes(uint64(s)))
}

// String 返回容量的人类可读字符串表示，例如 "10 MiB"、"1.5 GiB"
func (f BibyteSize64) String() string {
	return humanize.IBytes(uint64(f))
}

// KiB 将字节数转换为以 KiB（Kibibyte，1 KiB = 1024 字节）为单位的浮点数
func (f BibyteSize64) KiB() float64 {
	return float64(f) / 1024
}

// MiB 将字节数转换为以 MiB（Mebibyte，1 MiB = 1024 KiB）为单位的浮点数
func (f BibyteSize64) MiB() float64 {
	return f.KiB() / 1024
}

// GiB 将字节数转换为以 GiB（Gibibyte，1 GiB = 1024 MiB）为单位的浮点数
func (f BibyteSize64) GiB() float64 {
	return f.MiB() / 1024
}

// UnmarshalJSON 实现 json.Unmarshaler 接口，支持从 JSON 数据中解析存储容量
// 在将数据传递给内部解析方法之前，会先去除 JSON 字符串的引号
func (s *BibyteSize64) UnmarshalJSON(text []byte) error {
	sizeStr := strings.Trim(string(text), "\"")
	if sizeStr == "" {
		*s = 0
		return nil
	}
	return s.unmarshal([]byte(sizeStr))
}

func (s BibyteSize64) MarshalJSON() ([]byte, error) {
	data, err := json.Marshal(s.marshal())
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return data, nil
}

// UnmarshalText 实现 encoding.TextUnmarshaler 接口，支持从文本数据中解析存储容量
func (s *BibyteSize64) UnmarshalText(text []byte) error {
	return s.unmarshal(text)
}

func (s BibyteSize64) MarshalText() ([]byte, error) {
	return s.marshal(), nil
}

// UnmarshalBinary 实现 encoding.BinaryUnmarshaler 接口，支持从二进制数据中解析存储容量
func (s *BibyteSize64) UnmarshalBinary(text []byte) error {
	return s.unmarshal(text)
}

func (s BibyteSize64) MarshalBinary() ([]byte, error) {
	return s.marshal(), nil
}

type BibyteSize uint32

// unmarshal 是内部反序列化方法，将字节切片解析为 BibyteSize 值
// 支持解析如 "10 MB"、"1.5 GiB"、"1024" 等格式的容量字符串
// 如果传入空字符串，则将值设置为 0；如果格式无法解析，则返回错误
func (s *BibyteSize) unmarshal(text []byte) error {
	sizeStr := string(text)
	if sizeStr == "" {
		*s = 0
		return nil
	}

	bytes, err := humanize.ParseBytes(sizeStr)
	if err != nil {
		return errors.Newf("Invalid storage size format: %s", sizeStr)
	}
	*s = BibyteSize(bytes)
	return nil
}

func (s BibyteSize) marshal() []byte {
	return []byte(humanize.IBytes(uint64(s)))
}

// String 返回容量的人类可读字符串表示，例如 "10 MiB"、"1.5 GiB"
func (f BibyteSize) String() string {
	return humanize.IBytes(uint64(f))
}

// KiB 将字节数转换为以 KiB（Kibibyte，1 KiB = 1024 字节）为单位的浮点数
func (f BibyteSize) KiB() float32 {
	return float32(f) / 1024
}

// MiB 将字节数转换为以 MiB（Mebibyte，1 MiB = 1024 KiB）为单位的浮点数
func (f BibyteSize) MiB() float32 {
	return f.KiB() / 1024
}

// GiB 将字节数转换为以 GiB（Gibibyte，1 GiB = 1024 MiB）为单位的浮点数
func (f BibyteSize) GiB() float32 {
	return f.MiB() / 1024
}

// UnmarshalJSON 实现 json.Unmarshaler 接口，支持从 JSON 数据中解析存储容量
// 在将数据传递给内部解析方法之前，会先去除 JSON 字符串的引号
func (s *BibyteSize) UnmarshalJSON(text []byte) error {
	sizeStr := strings.Trim(string(text), "\"")
	if sizeStr == "" {
		*s = 0
		return nil
	}
	return s.unmarshal([]byte(sizeStr))
}

func (s BibyteSize) MarshalJSON() ([]byte, error) {
	data, err := json.Marshal(s.marshal())
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return data, nil
}

// UnmarshalText 实现 encoding.TextUnmarshaler 接口，支持从文本数据中解析存储容量
func (s *BibyteSize) UnmarshalText(text []byte) error {
	return s.unmarshal(text)
}

func (s BibyteSize) MarshalText() ([]byte, error) {
	return s.marshal(), nil
}

// UnmarshalBinary 实现 encoding.BinaryUnmarshaler 接口，支持从二进制数据中解析存储容量
func (s *BibyteSize) UnmarshalBinary(text []byte) error {
	return s.unmarshal(text)
}

func (s BibyteSize) MarshalBinary() ([]byte, error) {
	return s.marshal(), nil
}
