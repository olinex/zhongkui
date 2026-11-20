// 本文件实现可解密敏感字段的字段计划（sensitivePlan）与求值上下文的秘密表。
// 字段计划按目标模型类型键缓存，供 BuildInput（从规则输入文档中剔除敏感字段）与
// crypto.reveal 内置函数（按字段名读取秘密表并解密）共用；字段解析语义与
// InputMetadata 的反射推导保持一致：BFS 展开匿名嵌入结构体、浅层优先、键名取 json tag。
//
// 敏感字段不出现在 Rego 的 input 文档中，规则只能经 crypto.reveal(field) 获取明文：
//   - Crypto：运行期实例通常只有明文（写库时才加密），秘密表直接保存明文，reveal 零解密；
//     读库实例可能有密文，则在 reveal 时才解密；
//   - S3Crypto：秘密表保存对象键，reveal 时按字段密钥下载并解密。

package tagengine

import (
	"reflect"
	"sync"

	"zhongkuisec.com/zhongkui/internal/pkg/crypto"
	"zhongkuisec.com/zhongkui/internal/pkg/crypto/mode"
	"zhongkuisec.com/zhongkui/internal/pkg/enum"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// sensitiveKind 敏感字段的可解密类型（枚举类 sensitiveKind）。
type sensitiveKind string

const (
	// sensitiveKindCrypto 列内可逆加密字段（serializer.Crypto + crypto tag）。
	sensitiveKindCrypto sensitiveKind = "crypto"
	// sensitiveKindS3Crypto S3 对象加密字段（serializer.S3Crypto + s3crypto tag）。
	sensitiveKindS3Crypto sensitiveKind = "s3crypto"
)

// sensitiveKinds 是可解密类型的取值空间，声明顺序即枚举类条目顺序。
var sensitiveKinds = []sensitiveKind{sensitiveKindCrypto, sensitiveKindS3Crypto}

// init 注册枚举类 sensitiveKind（取值空间固定，条目在注册时快照）。
func init() {
	enum.Register(enum.Type{Name: "sensitiveKind", Entries: enum.EntriesOf(sensitiveKinds)})
}

// sensitiveField 目标模型中的一个可解密字段。
type sensitiveField struct {
	// kind 字段的可解密类型
	kind sensitiveKind
	// index 从目标结构体到该字段的反射索引路径（支持匿名嵌入提升）
	index []int
	// cryptoTag crypto 标签解析结果（kind == sensitiveKindCrypto 时有效）
	cryptoTag crypto.CryptoTag
	// s3Tag s3crypto 标签解析结果（kind == sensitiveKindS3Crypto 时有效）
	s3Tag crypto.S3CryptoTag
	// modeOnce/modeValue/modeErr 懒派生并缓存加密模式，避免每条数据重复派生密钥
	modeOnce  sync.Once
	modeValue mode.Mode
	modeErr   error
	// keyOnce/keyValue/keyErr 懒派生并缓存 S3 子密钥
	keyOnce  sync.Once
	keyValue []byte
	keyErr   error
}

// cryptoMode 返回该 Crypto 字段的加密模式（并发安全，派生一次后复用）。
func (f *sensitiveField) cryptoMode() (mode.Mode, error) {
	f.modeOnce.Do(func() {
		f.modeValue, f.modeErr = crypto.ModeForTag(f.cryptoTag)
	})
	return f.modeValue, f.modeErr
}

// s3CryptoKey 返回该 S3Crypto 字段的对象加密子密钥（并发安全，派生一次后复用）。
func (f *sensitiveField) s3CryptoKey() ([]byte, error) {
	f.keyOnce.Do(func() {
		f.keyValue, f.keyErr = crypto.S3CryptoKeyForTag(f.s3Tag)
	})
	return f.keyValue, f.keyErr
}

// sensitiveSecret 字段在求值上下文中的秘密材料。
// 明文存在时 reveal 直接返回（运行期零解密）；仅密文时 reveal 才按字段模式解密。
type sensitiveSecret struct {
	// kind 字段的可解密类型
	kind sensitiveKind
	// plain Crypto 明文字节（优先使用）
	plain []byte
	// cipher Crypto 密文字节（plain 为空时使用）
	cipher []byte
	// objectKey S3Crypto 对象键
	objectKey string
}

// sensitivePlan 目标模型的敏感字段计划：json 字段名 → 字段元信息。
type sensitivePlan struct {
	// typeKey 目标模型类型键
	typeKey string
	// fields json 字段名 → 可解密字段
	fields map[string]*sensitiveField
}

// sensitivePlans 按目标模型类型键缓存字段计划，避免每次 BuildInput / 求值重复反射。
var sensitivePlans sync.Map // string → *sensitivePlan

// sensitivePlanFor 返回目标模型的敏感字段计划；未注册模型返回空计划。
func sensitivePlanFor(typeKey string) *sensitivePlan {
	if v, ok := sensitivePlans.Load(typeKey); ok {
		return v.(*sensitivePlan)
	}
	plan := &sensitivePlan{typeKey: typeKey, fields: make(map[string]*sensitiveField)}
	if target, ok := TargetInfo(typeKey); ok {
		plan.fields = collectSensitiveFields(reflect.TypeOf(target.Type))
	}
	actual, _ := sensitivePlans.LoadOrStore(typeKey, plan)
	return actual.(*sensitivePlan)
}

// collectSensitiveFields 反射遍历结构体并收集可解密字段。
// 按层 BFS 展开匿名嵌入结构体，保证同名字段"浅层优先"（与 json.Marshal 提升语义一致）；
// 仅收集携带对应标签的 serializer.Crypto / serializer.S3Crypto 字段。
func collectSensitiveFields(typ reflect.Type) map[string]*sensitiveField {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	fields := make(map[string]*sensitiveField)
	if typ.Kind() != reflect.Struct {
		return fields
	}
	// pending 描述待展开结构体：typ 为当前结构体类型，index 为从目标类型到该结构体的反射路径
	type pending struct {
		typ   reflect.Type
		index []int
	}
	queue := []pending{{typ: typ}}
	for len(queue) > 0 {
		level := queue
		queue = nil
		for _, p := range level {
			for i := 0; i < p.typ.NumField(); i++ {
				sf := p.typ.Field(i)
				// 非导出字段不参与 JSON 序列化，也不参与规则输入
				if sf.PkgPath != "" {
					continue
				}
				jsonName := jsonNameOf(sf)
				index := append(append([]int(nil), p.index...), i)
				if sf.Anonymous && jsonName == "" {
					ft := sf.Type
					for ft.Kind() == reflect.Pointer {
						ft = ft.Elem()
					}
					if ft.Kind() == reflect.Struct {
						queue = append(queue, pending{typ: ft, index: index})
					}
					continue
				}
				if jsonName == "" {
					continue
				}
				// 浅层优先：同名字段已记录时跳过深层字段
				if _, exists := fields[jsonName]; exists {
					continue
				}
				ft := sf.Type
				for ft.Kind() == reflect.Pointer {
					ft = ft.Elem()
				}
				switch ft.String() {
				case "serializer.Crypto":
					raw := sf.Tag.Get(crypto.CryptoTagKey)
					if raw == "" {
						continue
					}
					tag, err := crypto.ParseCryptoTag(raw)
					if err != nil {
						continue
					}
					fields[jsonName] = &sensitiveField{kind: sensitiveKindCrypto, index: index, cryptoTag: tag}
				case "serializer.S3Crypto":
					raw := sf.Tag.Get(crypto.S3CryptoTagKey)
					if raw == "" {
						continue
					}
					tag, err := crypto.ParseS3CryptoTag(raw)
					if err != nil {
						continue
					}
					fields[jsonName] = &sensitiveField{kind: sensitiveKindS3Crypto, index: index, s3Tag: tag}
				}
			}
		}
	}
	return fields
}

// WithoutSensitiveFields 返回剔除敏感字段后的输入文档副本。
// 敏感字段只经 crypto.reveal 暴露明文，不出现在规则可见的 input 文档中；
// 无敏感字段时原样返回（避免无谓拷贝）。
func WithoutSensitiveFields(typeKey string, input map[string]any) map[string]any {
	plan := sensitivePlanFor(typeKey)
	if len(plan.fields) == 0 {
		return input
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		if _, sensitive := plan.fields[key]; sensitive {
			continue
		}
		out[key] = value
	}
	return out
}

// collectSensitiveSecrets 从目标实例提取敏感字段的秘密材料：
//   - Crypto：优先取明文（运行期实例，reveal 零解密）；明文为空时取密文，供 reveal 延迟解密；
//   - S3Crypto：取对象键，供 reveal 延迟下载解密。
//
// 返回的键为 json 字段名，与字段计划、crypto.reveal 的参数一致。
func collectSensitiveSecrets(target any, plan *sensitivePlan) map[string]*sensitiveSecret {
	secrets := make(map[string]*sensitiveSecret, len(plan.fields))
	v := reflect.ValueOf(target)
	for v.IsValid() && (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) {
		if v.IsNil() {
			return secrets
		}
		v = v.Elem()
	}
	if !v.IsValid() || v.Kind() != reflect.Struct {
		return secrets
	}
	for name, field := range plan.fields {
		fv := v.FieldByIndex(field.index)
		if !fv.IsValid() {
			continue
		}
		switch field.kind {
		case sensitiveKindCrypto:
			secret := &sensitiveSecret{kind: sensitiveKindCrypto}
			if fv.Kind() == reflect.Pointer {
				if fv.IsNil() {
					secrets[name] = secret
					continue
				}
				fv = fv.Elem()
			}
			if fv.Kind() != reflect.Struct {
				continue
			}
			value, ok := fv.Interface().(serializer.Crypto)
			if !ok {
				continue
			}
			if plain := value.Value(); len(plain) > 0 {
				secret.plain = plain
			} else if ciphertext := value.Cipher(); len(ciphertext) > 0 {
				secret.cipher = ciphertext
			}
			secrets[name] = secret
		case sensitiveKindS3Crypto:
			if fv.Kind() != reflect.Struct {
				continue
			}
			value, ok := fv.Interface().(serializer.S3Crypto)
			if !ok {
				continue
			}
			secrets[name] = &sensitiveSecret{kind: sensitiveKindS3Crypto, objectKey: value.Key()}
		}
	}
	return secrets
}
