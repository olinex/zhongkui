// 本文件提供加密字段（secure tag）的通用能力：标签解析与加密模式构造。
// 视图层明文查看（internal/manager/views/plaintext.go）与 GORM crypto 序列化器
// （internal/pkg/serializer）都只是本层能力的借用者，避免各自重复实现
// 「tag → 密钥派生 → 模式构造」链路。
//
// This file provides the shared capabilities for encrypted fields (secure tag): tag
// parsing and crypto-mode construction. Both the view-layer plaintext lookup
// (internal/manager/views/plaintext.go) and the GORM crypto serializer
// (internal/pkg/serializer) borrow from this layer, avoiding duplicated
// "tag → key derivation → mode construction" pipelines.
package crypto

import (
	"zhongkui.com/zhongkui/internal/pkg/crypto/key"
	"zhongkui.com/zhongkui/internal/pkg/crypto/mode"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	t "zhongkui.com/zhongkui/internal/pkg/tag"
	"zhongkui.com/zhongkui/internal/pkg/utils"
)

// SecurityTagKey 加密字段的 secure 结构体标签键。
// SecurityTagKey is the secure struct tag key of encrypted fields.
const SecurityTagKey = "secure"

// CryptoTag 加密字段标签内容（secure 标签解析结果）。
// CryptoTag is the parsed content of the secure tag of an encrypted field.
type CryptoTag struct {
	Mode   string `tag:"mode"`
	Offset int    `tag:"offset"`
	Length int    `tag:"length"`
}

// ParseCryptoTag 解析 secure 标签字符串为 CryptoTag。
// ParseCryptoTag parses a secure tag string into a CryptoTag.
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
//
// FieldTag resolves the secure tag of a model field by its field path into a CryptoTag
// (no key derivation). nameTag selects the struct tag used to match path segments (e.g.
// "json", "gorm"); the plaintext API's field param is the JSON field name, so callers
// pass "json" (consistent with the permission-point object and the frontend contract).
// Field resolution is delegated to internal/pkg/utils.ExtractFieldTag:
//   - single-segment (e.g. "payload"): shallow-first recursive lookup, always returns the
//     secure tag of the outermost (directly declared) field, consistent with the stored
//     column (an embedded struct's same-name field is shadowed and not part of column
//     mapping);
//   - multi-segment (e.g. "PacketIPv6.Payload"): descends level by level along the
//     explicit path to locate the inner embedded field.
func FieldTag(model any, path, nameTag string) (CryptoTag, error) {
	secure, err := utils.ExtractFieldTag(model, path, nameTag, SecurityTagKey)
	if err != nil {
		return CryptoTag{}, err
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
//
// ModeForTag derives the child key from a CryptoTag and builds the matching crypto mode,
// the shared front half of the encrypted-field decrypt pipeline (tag → key → mode →
// decrypt).
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
// ModeFromTag parses a secure tag string and builds the crypto mode, used by the GORM
// crypto serializer (Scan).
func ModeFromTag(tag string) (mode.Mode, error) {
	cTag, err := ParseCryptoTag(tag)
	if err != nil {
		return nil, err
	}
	return ModeForTag(cTag)
}

// ModeForField 按字段路径解析模型字段的 secure 标签并构造加密模式，供视图层
// 明文查看接口（CryptoPlaintextGet）使用。nameTag 与 FieldTag 同义：明文接口的
// field 参数为字段 json 名，故传入 "json"。
// ModeForField resolves the secure tag of a model field by its field path and builds the
// crypto mode, used by the view-layer plaintext lookup (CryptoPlaintextGet). nameTag has
// the same meaning as in FieldTag: the plaintext API's field param is the JSON field
// name, so callers pass "json".
func ModeForField(model any, path, nameTag string) (mode.Mode, error) {
	cTag, err := FieldTag(model, path, nameTag)
	if err != nil {
		return nil, err
	}
	return ModeForTag(cTag)
}
