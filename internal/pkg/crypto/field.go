// Package crypto 提供密码学相关工具：随机字符串生成、密码哈希，以及加密/摘要字段
// （crypto / hash tag）的标签解析、加密模式构造与带密钥摘要。
//
// 本文件提供字段级安全能力的通用底层：标签解析、加密模式构造与带密钥摘要。
// 三类结构体标签与序列化类型一一绑定：
//   - crypto 标签    ↔ serializer.Crypto（可逆加密，mode/offset/length）
//   - hash 标签      ↔ serializer.Hash（固定 HMAC-SHA256，仅 offset/length）
//   - s3crypto 标签  ↔ serializer.S3Crypto（S3 对象加密，固定 zstd+AES-GCM 管道，仅 offset/length）
//
// 视图层明文查看（internal/manager/views/plaintext/plaintext.go）与 GORM 序列化器
// （internal/pkg/serializer）都只是本层能力的借用者，避免各自重复实现
// 「tag → 密钥派生 → 模式构造 / 摘要」链路。
package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"

	"zhongkuisec.com/zhongkui/internal/pkg/crypto/key"
	"zhongkuisec.com/zhongkui/internal/pkg/crypto/mode"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	t "zhongkuisec.com/zhongkui/internal/pkg/tag"
	"zhongkuisec.com/zhongkui/internal/pkg/utils"
)

// CryptoTagKey 加密字段的结构体标签键（字段类型须为 serializer.Crypto）。
const CryptoTagKey = "crypto"

// HashTagKey 摘要字段的结构体标签键（字段类型须为 serializer.Hash）。
const HashTagKey = "hash"

// S3CryptoTagKey S3 对象字段的结构体标签键（字段类型须为 serializer.S3Crypto）。
const S3CryptoTagKey = "s3crypto"

// CryptoTag 加密字段标签内容（crypto 标签解析结果）。
type CryptoTag struct {
	// Mode 加密模式标识（由 mode 包注册表解析，内置 aes-gcm）
	Mode string `tag:"mode"`
	// Offset 派生密钥的偏移量
	Offset int `tag:"offset"`
	// Length 派生密钥的长度
	Length int `tag:"length"`
}

// HashTag 摘要字段标签内容（hash 标签解析结果）。
// 摘要算法在代码层固定为 HMAC-SHA256（见 DigestForField / HashKeyForField），
// 标签只声明密钥派生参数，不再重复 mode，避免与代码中的固定算法漂移。
type HashTag struct {
	// Offset 派生密钥的偏移量
	Offset int `tag:"offset"`
	// Length 派生密钥的长度
	Length int `tag:"length"`
}

// S3CryptoTag S3 对象字段标签内容（s3crypto 标签解析结果）。
// 对象内容固定使用 zstd + AES-GCM 管道，算法在代码层固定，故标签只声明密钥派生参数。
type S3CryptoTag struct {
	// Offset 派生密钥的偏移量
	Offset int `tag:"offset"`
	// Length 派生密钥的长度
	Length int `tag:"length"`
}

// ParseCryptoTag 解析 crypto 标签字符串为 CryptoTag。
func ParseCryptoTag(tag string) (CryptoTag, error) {
	var cTag CryptoTag
	if err := t.Unmarshal(tag, &cTag); err != nil {
		return CryptoTag{}, errors.WithStack(err)
	}
	return cTag, nil
}

// ParseHashTag 解析 hash 标签字符串为 HashTag。
func ParseHashTag(tag string) (HashTag, error) {
	var hTag HashTag
	if err := t.Unmarshal(tag, &hTag); err != nil {
		return HashTag{}, errors.WithStack(err)
	}
	return hTag, nil
}

// ParseS3CryptoTag 解析 s3crypto 标签字符串为 S3CryptoTag。
func ParseS3CryptoTag(tag string) (S3CryptoTag, error) {
	var sTag S3CryptoTag
	if err := t.Unmarshal(tag, &sTag); err != nil {
		return S3CryptoTag{}, errors.WithStack(err)
	}
	return sTag, nil
}

// CryptoFieldTag 按字段路径解析模型字段的 crypto 标签为 CryptoTag（不做密钥派生）。
// nameTag 指定路径段匹配的字段命名标签（如 "json"、"gorm"）：明文接口的 field 参数即
// 字段 json 名，故传入 "json"（与权限点 object、前端契约一致）。
// 字段解析委托 internal/pkg/utils.ExtractFieldTag：
//   - 单段（如 "payload"）：浅层优先递归查找，始终返回最外层（直接声明）字段的 crypto 标签，
//     与落库列一致（内嵌结构的同名嵌入字段被外层遮蔽，不参与列映射）；
//   - 多段（如 "PacketIPv6.Payload"）：按显式路径逐层下降，精确定位内层嵌入字段。
//
// 背景：结构体嵌入会导致内外层同 json 名字段并存（如 PacketIPv6TCP 内嵌 PacketIPv6 且
// 两者均声明 Payload）。单段路径解析到最外层，多段路径可定位内层，从而消除歧义。
func CryptoFieldTag(model any, path, nameTag string) (CryptoTag, error) {
	raw, err := utils.ExtractFieldTag(model, path, nameTag, CryptoTagKey)
	if err != nil {
		return CryptoTag{}, errors.WithStack(err)
	}
	if raw == "" {
		return CryptoTag{}, errors.Newf("field %s is not a crypto field", path)
	}
	return ParseCryptoTag(raw)
}

// HashFieldTag 按字段路径解析模型字段的 hash 标签为 HashTag（不做密钥派生）。
// 字段解析语义与 CryptoFieldTag 一致：nameTag 通常传入 "json"，
// 单段路径浅层优先、多段路径逐层下降。
func HashFieldTag(model any, path, nameTag string) (HashTag, error) {
	raw, err := utils.ExtractFieldTag(model, path, nameTag, HashTagKey)
	if err != nil {
		return HashTag{}, errors.WithStack(err)
	}
	if raw == "" {
		return HashTag{}, errors.Newf("field %s is not a hash field", path)
	}
	return ParseHashTag(raw)
}

// ModeForTag 依据 CryptoTag 派生密钥并构造对应的加密模式实例，是整个加密字段
// 解密链路（tag → 密钥 → 模式 → 解密）的通用前置步骤。
//
// 参数：
//   - cTag: 加密字段标签（mode/offset/length），来自 crypto tag 解析
//
// 返回值：
//   - mode.Mode: 对应加密模式（由 mode 包注册表解析，内置 AES-GCM；
//     新增加密模式实现 Mode 接口并调用 mode.Register 注册即可，无需改动本函数）
//   - error: 密钥派生器未初始化、密钥派生失败或未知加密模式时返回错误
func ModeForTag(cTag CryptoTag) (mode.Mode, error) {
	kd, err := key.GetKeyDerivation()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	cryptoKey, err := kd.DeriveChildKey(cTag.Offset, cTag.Length)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return mode.New(cTag.Mode, cryptoKey, nil)
}

// ModeFromTag 解析 crypto 标签字符串并构造加密模式，供 GORM crypto 序列化器
// （读库扫描 Scan）使用。
func ModeFromTag(tag string) (mode.Mode, error) {
	cTag, err := ParseCryptoTag(tag)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return ModeForTag(cTag)
}

// ModeForField 按字段路径解析模型字段的 crypto 标签并构造加密模式，供视图层
// 明文查看接口（CryptoPlaintextGet）使用。nameTag 与 CryptoFieldTag 同义：明文接口的
// field 参数为字段 json 名，故传入 "json"。
func ModeForField(model any, path, nameTag string) (mode.Mode, error) {
	cTag, err := CryptoFieldTag(model, path, nameTag)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return ModeForTag(cTag)
}

// HashKeyForTag 依据 hash 标签派生 HMAC 摘要密钥，是整个带密钥摘要链路
// （tag → 密钥）的通用前置步骤。摘要算法固定为 HMAC-SHA256，故标签不含算法标识。
//
// 参数：
//   - hTag: 摘要字段标签（offset/length），来自 hash tag 解析
//
// 返回值：
//   - []byte: 派生出的 HMAC 密钥
//   - error: 密钥派生器未初始化或密钥派生失败时返回错误
func HashKeyForTag(hTag HashTag) ([]byte, error) {
	kd, err := key.GetKeyDerivation()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	cryptoKey, err := kd.DeriveChildKey(hTag.Offset, hTag.Length)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return cryptoKey, nil
}

// HashKeyForField 按字段路径解析模型字段的 hash 标签并派生 HMAC 摘要密钥，供需要
// 流式计算摘要（正文体积大，无法一次性持有全部数据）的调用方复用同一条标签解析与
// 密钥派生链路，避免各自就地实现；一次性摘要直接用 DigestForField 即可。
//
// nameTag 与 HashFieldTag 同义：字段路径按 json 名匹配，故通常传入 "json"。
//
// 参数：
//   - model: 声明该字段的模型实例（或指针）
//   - path: 字段路径（json 名，多段时逐层下降）
//   - nameTag: 路径段匹配的字段命名标签（与 HashFieldTag 同义）
//
// 返回值：
//   - []byte: 派生出的 HMAC 密钥
//   - error: 字段无 hash 标签或密钥派生失败时返回错误
func HashKeyForField(model any, path, nameTag string) ([]byte, error) {
	hTag, err := HashFieldTag(model, path, nameTag)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return HashKeyForTag(hTag)
}

// S3CryptoKeyForTag 依据 s3crypto 标签派生 S3 对象加密子密钥。
// 对象内容固定使用 zstd + AES-GCM 管道，故标签只声明 offset/length。
//
// 参数：
//   - sTag: S3 对象字段标签（offset/length）
//
// 返回值：
//   - []byte: 派生出的 AES 子密钥
//   - error: 密钥派生器未初始化或派生失败时返回错误
func S3CryptoKeyForTag(sTag S3CryptoTag) ([]byte, error) {
	kd, err := key.GetKeyDerivation()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	cryptoKey, err := kd.DeriveChildKey(sTag.Offset, sTag.Length)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return cryptoKey, nil
}

// S3CryptoKeyForField 按字段路径解析模型字段的 s3crypto 标签并派生 S3 加密密钥。
// nameTag 与 CryptoFieldTag 同义：网络模型字段有 json 标签，传 "json"；
// 无 json 标签的 AI 模型传 ""（按 Go 字段名匹配）。
//
// 参数：
//   - model: 声明该字段的模型实例（或指针）
//   - path: 字段路径（nameTag 对应的字段名，多段时逐层下降）
//   - nameTag: 路径段匹配的字段命名标签
//
// 返回值：
//   - []byte: 派生出的 S3 加密子密钥
//   - error: 字段无 s3crypto 标签或密钥派生失败时返回错误
func S3CryptoKeyForField(model any, path, nameTag string) ([]byte, error) {
	raw, err := utils.ExtractFieldTag(model, path, nameTag, S3CryptoTagKey)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if raw == "" {
		return nil, errors.Newf("field %s is not an s3crypto field", path)
	}
	sTag, err := ParseS3CryptoTag(raw)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return S3CryptoKeyForTag(sTag)
}

// DigestForField 按字段路径解析模型字段的 hash 标签，派生 HMAC 密钥并对数据计算
// 十六进制 HMAC-SHA256 摘要，供需要确定性等值的摘要字段（如内容去重的 hash）使用：
// 摘要不可逆推原文，且同一密钥与内容恒得同一摘要，可安全地做等值匹配与唯一索引。
//
// 与 ModeForField 同构（tag → 密钥派生 → 结果），使摘要字段与加密字段共用同一条
// 标签解析与密钥派生链路，不必由调用方各自就地实现。
//
// 参数：
//   - model: 声明该字段的模型实例（或指针）
//   - path: 字段路径（json 名，多段时逐层下降）
//   - nameTag: 路径段匹配的字段命名标签（与 ModeForField 同义）
//   - data: 待摘要的原始数据
//
// 返回值：
//   - string: 64 个字符的十六进制 HMAC-SHA256 摘要
//   - error: 字段无 hash 标签或密钥派生失败时返回错误
func DigestForField(model any, path, nameTag string, data []byte) (string, error) {
	cryptoKey, err := HashKeyForField(model, path, nameTag)
	if err != nil {
		return "", errors.WithStack(err)
	}
	mac := hmac.New(sha256.New, cryptoKey)
	mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil)), nil
}
