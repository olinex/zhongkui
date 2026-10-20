package utils

// 本文件承载三个与业务无关的编码类纯计算能力：Hash（文本摘要）、EncodeConvert（编码转换）与
// GenerateUUID（UUID 生成）。它们不依赖 eino 等上层组件，只依赖标准库与既有第三方
// github.com/google/uuid；internal/ai/tools 中的同名工具由此处的导出函数驱动，仅负责入参 JSON
// 解析与结果序列化。
//
// 错误信息一律为英文、描述性、不带工具名前缀（工具名前缀由适配器补齐），故最终对外字符串与重构前
// 保持一致。

import (
	// 标准库
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"hash"
	"html"
	"net/url"
	"strings"

	// 第三方
	"github.com/google/uuid"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// ============================================================================
// Hash：文本摘要
// ============================================================================

// hashAlgorithms 是 hash 工具支持的算法取值空间（枚举），键为算法名，值为其构造函数。
var hashAlgorithms = map[string]func() hash.Hash{
	"md5":    md5.New,
	"sha1":   sha1.New,
	"sha256": sha256.New,
	"sha512": sha512.New,
}

// HashAlgorithmNames 是 hash 算法取值空间的字符串清单（字典序固定），供参数 schema 的枚举与错误
// 文案渲染复用。
var HashAlgorithmNames = []string{"md5", "sha1", "sha256", "sha512"}

// HashResult 是 Hash 成功时返回的载荷：摘要的十六进制与小写 base64 两种表示。
type HashResult struct {
	// Algorithm 实际使用的哈希算法
	Algorithm string `json:"algorithm"`
	// Hex 摘要的十六进制（小写）表示
	Hex string `json:"hex"`
	// Base64 摘要的小写 base64 表示
	Base64 string `json:"base64"`
}

// Hash 对 UTF-8 文本 input 以 algorithm 指定的算法计算摘要并返回 HashResult。algorithm 先去除首尾
// 空白；算法越界或文本为空时返回英文错误（工具名前缀由适配器补齐）。
func Hash(input, algorithm string) (HashResult, error) {
	algorithm = strings.TrimSpace(algorithm)
	newHash, ok := hashAlgorithms[algorithm]
	if !ok {
		return HashResult{}, errors.Newf("invalid algorithm %q, expected one of %s",
			algorithm, strings.Join(HashAlgorithmNames, ", "))
	}
	if input == "" {
		return HashResult{}, errors.New("text must be non-empty")
	}
	h := newHash()
	if _, err := h.Write([]byte(input)); err != nil {
		return HashResult{}, errors.WithStack(err)
	}
	sum := h.Sum(nil)
	return HashResult{
		Algorithm: algorithm,
		Hex:       hex.EncodeToString(sum),
		Base64:    base64.StdEncoding.EncodeToString(sum),
	}, nil
}

// ============================================================================
// EncodeConvert：编码转换
// ============================================================================

// encodeSchemes 是 encode_convert 支持的编码方案枚举：键为方案名，值为一对编解码函数。
var encodeSchemes = map[string]struct {
	encode func(string) string
	decode func(string) (string, error)
}{
	"base64": {
		encode: func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) },
		decode: func(s string) (string, error) {
			raw, err := base64.StdEncoding.DecodeString(s)
			return string(raw), errors.WithStack(err)
		},
	},
	"base64url": {
		encode: func(s string) string { return base64.URLEncoding.EncodeToString([]byte(s)) },
		decode: func(s string) (string, error) {
			raw, err := base64.URLEncoding.DecodeString(s)
			return string(raw), errors.WithStack(err)
		},
	},
	"hex": {
		encode: func(s string) string { return hex.EncodeToString([]byte(s)) },
		decode: func(s string) (string, error) {
			raw, err := hex.DecodeString(s)
			return string(raw), errors.WithStack(err)
		},
	},
	"url": {
		encode: func(s string) string { return url.QueryEscape(s) },
		decode: func(s string) (string, error) { return url.QueryUnescape(s) },
	},
	"html": {
		encode: func(s string) string { return html.EscapeString(s) },
		decode: func(s string) (string, error) { return html.UnescapeString(s), nil },
	},
}

// EncodeSchemeNames 是编码方案取值空间的字符串清单（字典序固定），供参数 schema 的枚举与错误文案
// 渲染复用。
var EncodeSchemeNames = []string{"base64", "base64url", "hex", "url", "html"}

// EncodeConvertResult 是 EncodeConvert 成功时返回的载荷。
type EncodeConvertResult struct {
	// Operation 实际执行的操作方向
	Operation string `json:"operation"`
	// Scheme 实际使用的编码方案
	Scheme string `json:"scheme"`
	// Result 转换后的文本
	Result string `json:"result"`
}

// EncodeConvert 以 scheme 指定的方案对 text 执行 operation（encode 或 decode）并返回
// EncodeConvertResult。operation 与 scheme 先去除首尾空白；操作方向非法、方案越界、文本为空或
// decode 输入非法（如非法 base64 / hex）时返回英文错误（工具名前缀由适配器补齐）。
func EncodeConvert(scheme, operation, text string) (EncodeConvertResult, error) {
	operation = strings.TrimSpace(operation)
	if operation != "encode" && operation != "decode" {
		return EncodeConvertResult{}, errors.Newf("invalid operation %q, expected encode or decode", operation)
	}
	scheme = strings.TrimSpace(scheme)
	codec, ok := encodeSchemes[scheme]
	if !ok {
		return EncodeConvertResult{}, errors.Newf("invalid scheme %q, expected one of %s",
			scheme, strings.Join(EncodeSchemeNames, ", "))
	}
	if text == "" {
		return EncodeConvertResult{}, errors.New("text must be non-empty")
	}
	var result string
	if operation == "encode" {
		result = codec.encode(text)
	} else {
		decoded, err := codec.decode(text)
		if err != nil {
			return EncodeConvertResult{}, errors.Newf("invalid %s input for decode", scheme)
		}
		result = decoded
	}
	return EncodeConvertResult{Operation: operation, Scheme: scheme, Result: result}, nil
}

// ============================================================================
// GenerateUUID：UUID 生成
// ============================================================================

// uuid 的可选入参缺省值与数量上限。
const (
	// DefaultUUIDVersion 是 version 缺省值（随机 UUID）。
	DefaultUUIDVersion = "4"
	// DefaultUUIDCount 是 count 缺省值。
	DefaultUUIDCount = 1
	// uuidMaxCount 是单次生成数量上限，超出即返回英文错误。
	uuidMaxCount = 100
)

// UUIDResult 是 GenerateUUID 成功时返回的载荷：UUIDs 为生成结果（至少一个，因而恒非空）。
type UUIDResult struct {
	// Version 实际使用的 UUID 版本
	Version string `json:"version"`
	// UUIDs 生成的 UUID 列表（字符串形式）
	UUIDs []string `json:"uuids"`
}

// GenerateUUID 按 version（"4" 或 "7"）生成 count 个 UUID 并返回 UUIDResult。version 先去除首尾
// 空白，为空则取 DefaultUUIDVersion；版本越界、数量越界（不在 1..100）或生成失败时返回英文错误
// （工具名前缀由适配器补齐）。
func GenerateUUID(version string, count int) (UUIDResult, error) {
	version = strings.TrimSpace(version)
	if version == "" {
		version = DefaultUUIDVersion
	}
	if version != "4" && version != "7" {
		return UUIDResult{}, errors.Newf("invalid version %q, expected 4 or 7", version)
	}
	if count < 1 || count > uuidMaxCount {
		return UUIDResult{}, errors.Newf("count %d out of range, expected 1..%d", count, uuidMaxCount)
	}
	ids := make([]string, 0, count)
	for i := 0; i < count; i++ {
		var (
			id  uuid.UUID
			err error
		)
		if version == "7" {
			id, err = uuid.NewV7()
		} else {
			id = uuid.New()
		}
		if err != nil {
			return UUIDResult{}, errors.WithStack(err)
		}
		ids = append(ids, id.String())
	}
	return UUIDResult{Version: version, UUIDs: ids}, nil
}
