// Package crypto 提供密码学相关工具：随机字符串生成、密码哈希，以及加密字段（secure tag）的
// 标签解析与加密模式构造。
//
// 本文件提供加密字段（secure tag）的通用能力：标签解析、加密模式构造与带密钥摘要。
// 视图层明文查看（internal/manager/views/plaintext.go）与 GORM crypto 序列化器
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

// SecurityTagKey 加密字段的 secure 结构体标签键。
const SecurityTagKey = "secure"

// HmacSHA256 带密钥摘要的算法标识，用于 secure 标签中需要确定性摘要的字段
// （如内容去重的 hash）：同一密钥与内容恒得同一摘要，可等值匹配与建唯一索引，
// 且无密钥无法离线爆破，区别于 aes-gcm 的可逆加密（密文随机、不可等值比较）。
const HmacSHA256 = "hmac-sha256"

// CryptoTag 加密字段标签内容（secure 标签解析结果）。
type CryptoTag struct {
	// Mode 算法标识（aes-gcm 为可逆加密，hmac-sha256 为带密钥摘要）
	Mode string `tag:"mode"`
	// Offset 派生密钥的偏移量
	Offset int `tag:"offset"`
	// Length 派生密钥的长度
	Length int `tag:"length"`
}

// ParseCryptoTag 解析 secure 标签字符串为 CryptoTag。
func ParseCryptoTag(tag string) (CryptoTag, error) {
	var cTag CryptoTag
	if err := t.Unmarshal(tag, &cTag); err != nil {
		return CryptoTag{}, errors.WithStack(err)
	}
	return cTag, nil
}

// FieldTag 按字段路径解析模型字段的 secure 标签为 CryptoTag（不做密钥派生）。
// nameTag 指定路径段匹配的字段命名标签（如 "json"、"gorm"）：明文接口的 field 参数即
// 字段 json 名，故传入 "json"（与权限点 object、前端契约一致）。
// 字段解析委托 internal/pkg/utils.ExtractFieldTag：
//   - 单段（如 "payload"）：浅层优先递归查找，始终返回最外层（直接声明）字段的 secure 标签，
//     与落库列一致（内嵌结构的同名嵌入字段被外层遮蔽，不参与列映射）；
//   - 多段（如 "PacketIPv6.Payload"）：按显式路径逐层下降，精确定位内层嵌入字段。
//
// 背景：结构体嵌入会导致内外层同 json 名字段并存（如 PacketIPv6TCP 内嵌 PacketIPv6 且
// 两者均声明 Payload）。单段路径解析到最外层，多段路径可定位内层，从而消除歧义。
func FieldTag(model any, path, nameTag string) (CryptoTag, error) {
	secure, err := utils.ExtractFieldTag(model, path, nameTag, SecurityTagKey)
	if err != nil {
		return CryptoTag{}, errors.WithStack(err)
	}
	if secure == "" {
		return CryptoTag{}, errors.Newf("field %s is not a secure field", path)
	}
	return ParseCryptoTag(secure)
}

// ModeForTag 依据 CryptoTag 派生密钥并构造对应的加密模式实例，是整个加密字段
// 解密链路（tag → 密钥 → 模式 → 解密）的通用前置步骤。
//
// 参数：
//   - cTag: 加密字段标签（mode/offset/length），来自 secure tag 解析
//
// 返回值：
//   - mode.Mode: 对应加密模式（当前支持 AES-GCM）
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
	switch cTag.Mode {
	case mode.AesGcm:
		m, err := mode.NewAESGCMMode(cryptoKey, nil)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return m, nil
	default:
		return nil, errors.Newf("unknown crypto mode: %s", cTag.Mode)
	}
}

// ModeFromTag 解析 secure 标签字符串并构造加密模式，供 GORM crypto 序列化器
// （读库扫描 Scan）使用。
func ModeFromTag(tag string) (mode.Mode, error) {
	cTag, err := ParseCryptoTag(tag)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return ModeForTag(cTag)
}

// ModeForField 按字段路径解析模型字段的 secure 标签并构造加密模式，供视图层
// 明文查看接口（CryptoPlaintextGet）使用。nameTag 与 FieldTag 同义：明文接口的
// field 参数为字段 json 名，故传入 "json"。
func ModeForField(model any, path, nameTag string) (mode.Mode, error) {
	cTag, err := FieldTag(model, path, nameTag)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return ModeForTag(cTag)
}

// HashKeyForTag 校验摘要字段标签的算法标识为 hmac-sha256 并派生摘要密钥，是整个
// 带密钥摘要链路（tag → 密钥）的通用前置步骤。
//
// 参数：
//   - cTag: 摘要字段标签（mode/offset/length），来自 secure tag 解析
//
// 返回值：
//   - []byte: 派生出的 HMAC 密钥
//   - error: 算法标识不是 hmac-sha256、密钥派生器未初始化或密钥派生失败时返回错误
func HashKeyForTag(cTag CryptoTag) ([]byte, error) {
	if cTag.Mode != HmacSHA256 {
		return nil, errors.Newf("field mode %q is not a %s field", cTag.Mode, HmacSHA256)
	}
	kd, err := key.GetKeyDerivation()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	cryptoKey, err := kd.DeriveChildKey(cTag.Offset, cTag.Length)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return cryptoKey, nil
}

// HashKeyForField 按字段路径解析模型字段的 secure 标签并派生 HMAC 摘要密钥，供需要
// 流式计算摘要（正文体积大，无法一次性持有全部数据）的调用方复用同一条标签解析与
// 密钥派生链路，避免各自就地实现；一次性摘要直接用 DigestForField 即可。
//
// nameTag 与 FieldTag 同义：字段路径按 json 名匹配，故通常传入 "json"。
//
// 参数：
//   - model: 声明该字段的模型实例（或指针）
//   - path: 字段路径（json 名，多段时逐层下降）
//   - nameTag: 路径段匹配的字段命名标签（与 FieldTag 同义）
//
// 返回值：
//   - []byte: 派生出的 HMAC 密钥
//   - error: 字段无 secure 标签、算法标识不是 hmac-sha256、密钥派生失败时返回错误
func HashKeyForField(model any, path, nameTag string) ([]byte, error) {
	cTag, err := FieldTag(model, path, nameTag)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return HashKeyForTag(cTag)
}

// DigestForField 按字段路径解析模型字段的 secure 标签，派生 HMAC 密钥并对数据计算
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
//   - error: 字段无 secure 标签、算法标识不是 hmac-sha256、密钥派生失败时返回错误
func DigestForField(model any, path, nameTag string, data []byte) (string, error) {
	cryptoKey, err := HashKeyForField(model, path, nameTag)
	if err != nil {
		return "", errors.WithStack(err)
	}
	mac := hmac.New(sha256.New, cryptoKey)
	mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil)), nil
}
