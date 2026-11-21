// 本文件实现 crypto.reveal 内置函数：按字段名解密目标模型的敏感字段。
// 敏感字段不出现在规则 input 文档中；求值前由引擎 / 视图把目标实例的秘密材料
// （Crypto 明文或密文、S3 对象键）绑定到 ctx，内置函数只按字段名取值。
//
// 性能：同一数据记录会被组级合并查询与逐规则明细评估多次求值，若每条规则都解密
// 同一字段会重复做 AES 解密甚至重复下载 S3 对象。故按字段名在「一次识别」内做
// single-flight 缓存（sync.Once），成功与错误都只计算一次；缓存随求值上下文的
// WithSensitiveTarget 创建，不跨数据记录保留。

package tagengine

import (
	"context"
	"encoding/json"
	"io"
	"sync"

	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/types"

	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

const (
	// revealName 自定义 OPA 内置函数名：crypto.reveal(field)。
	// field 为目标模型输入文档的 json 字段名。
	revealName = "crypto.reveal"
)

var (
	// ErrRevealNotActivated 表示求值上下文未绑定敏感字段秘密表，crypto.reveal 不可用。
	ErrRevealNotActivated = errors.New("tagengine: crypto.reveal not activated")
	// ErrRevealRuntimeNotActivated 表示求值上下文未绑定 S3 读取运行时，
	// 无法解密 S3Crypto 字段（Crypto 字段不依赖该运行时）。
	ErrRevealRuntimeNotActivated = errors.New("tagengine: crypto.reveal s3 runtime not activated")
)

// S3ObjectLoader 读取并解密 S3 加密对象，返回明文数据流。
// 生产环境由 s3.NewZkcn(client, key, cryptoKey, 0).Load 实现；测试可注入假实现。
type S3ObjectLoader func(ctx context.Context, objectKey string, cryptoKey []byte) (io.ReadCloser, error)

// sensitiveRuntime 解密所需的进程级运行时配置。
type sensitiveRuntime struct {
	// loader S3 对象读取器（nil 时不支持 S3Crypto 字段）
	loader S3ObjectLoader
	// maxBytes S3 正文读取上限（字节），超限报错
	maxBytes int64
}

// sensitiveRuntimeCtxKey 敏感字段运行时在 context 中的私有键。
type sensitiveRuntimeCtxKey struct{}

// revealEntry 单字段的单飞结果。
type revealEntry struct {
	// once 并发调用只执行一次，其余调用阻塞等待结果
	once sync.Once
	// value 解密后的明文
	value string
	// err 解密 / 下载错误（与 value 一同缓存，避免重复失败的网络请求）
	err error
}

// revealCache 一次识别内的解密结果缓存，键为 json 字段名。
type revealCache struct {
	mu      sync.Mutex
	entries map[string]*revealEntry
}

// newRevealCache 创建空缓存。
func newRevealCache() *revealCache {
	return &revealCache{entries: make(map[string]*revealEntry)}
}

// do 返回字段 name 的解密结果；并发或重复调用只执行一次 fn。
func (c *revealCache) do(name string, fn func() (string, error)) (string, error) {
	c.mu.Lock()
	entry, ok := c.entries[name]
	if !ok {
		entry = &revealEntry{}
		c.entries[name] = entry
	}
	c.mu.Unlock()
	entry.once.Do(func() {
		entry.value, entry.err = fn()
	})
	return entry.value, entry.err
}

// sensitiveContext 一次求值的敏感字段上下文：字段计划 + 秘密表 + 运行时 + 缓存。
type sensitiveContext struct {
	// ctx 求值上下文的 Go 侧引用，供 S3 下载使用
	ctx context.Context
	// plan 目标模型字段计划
	plan *sensitivePlan
	// secrets json 字段名 → 字段秘密材料
	secrets map[string]*sensitiveSecret
	// runtime S3 读取运行时（可为 nil）
	runtime *sensitiveRuntime
	// cache 本次识别的解密结果缓存
	cache *revealCache
}

// sensitiveContextCtxKey 敏感字段上下文在 context 中的私有键。
type sensitiveContextCtxKey struct{}

// WithSensitiveRuntimeContext 返回绑定了 S3 读取器与正文大小上限的 ctx。
// 引擎在每次识别前绑定；视图 dry-run / 样例校验复用同一运行时。
func WithSensitiveRuntimeContext(ctx context.Context, loader S3ObjectLoader, maxBytes int64) context.Context {
	return context.WithValue(ctx, sensitiveRuntimeCtxKey{}, &sensitiveRuntime{loader: loader, maxBytes: maxBytes})
}

// runtimeFromContext 从 ctx 提取敏感字段运行时；未绑定时返回 nil。
func runtimeFromContext(ctx context.Context) *sensitiveRuntime {
	if runtime, ok := ctx.Value(sensitiveRuntimeCtxKey{}).(*sensitiveRuntime); ok {
		return runtime
	}
	return nil
}

// WithSensitiveTarget 绑定目标模型的字段计划、从实例提取的秘密表与全新解密缓存。
// 敏感字段不出现在 input 文档中，crypto.reveal 只从此处取值；同一 ctx 被组级
// 合并查询与逐规则明细评估复用时共享缓存，保证同一字段只解密一次。
func WithSensitiveTarget(ctx context.Context, typeKey string, target any) context.Context {
	plan := sensitivePlanFor(typeKey)
	return context.WithValue(ctx, sensitiveContextCtxKey{}, &sensitiveContext{
		ctx:     ctx,
		plan:    plan,
		secrets: collectSensitiveSecrets(target, plan),
		runtime: runtimeFromContext(ctx),
		cache:   newRevealCache(),
	})
}

// sensitiveContextFromContext 从 ctx 提取敏感字段上下文；未绑定时返回 nil。
func sensitiveContextFromContext(ctx context.Context) *sensitiveContext {
	if sensitive, ok := ctx.Value(sensitiveContextCtxKey{}).(*sensitiveContext); ok {
		return sensitive
	}
	return nil
}

// BindSensitiveInput 供 dry-run 使用：输入文档即目标模型 JSON，反序列化为实例后
// 绑定敏感字段秘密表，并返回剔除敏感字段后的规则输入副本（规则看不到密文 / 对象键）。
// 未注册模型或输入无法反序列化为目标模型时不绑定秘密表（crypto.reveal 将报未激活），
// 仍返回剔除敏感字段后的输入，保证其余字段照常参与评估。
func BindSensitiveInput(ctx context.Context, typeKey string, input map[string]any) (context.Context, map[string]any) {
	sanitized := WithoutSensitiveFields(typeKey, input)
	instance, err := NewTargetInstance(typeKey)
	if err != nil {
		return ctx, sanitized
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return ctx, sanitized
	}
	if err := json.Unmarshal(raw, instance); err != nil {
		return ctx, sanitized
	}
	return WithSensitiveTarget(ctx, typeKey, instance), sanitized
}

// revealDecl 是 crypto.reveal 内置函数的声明：(string) → string。
// Memoize 开启使单次 OPA 求值内相同字段复用结果；跨求值去重由 revealCache 承担。
var revealDecl = &rego.Function{
	Name:        revealName,
	Description: "reveal a crypto / s3crypto field of the current target model",
	Decl: types.NewFunction(
		types.Args(types.S),
		types.S,
	),
	Memoize: true,
}

// revealBuiltin 是 crypto.reveal 内置函数的实现：解析字段计划并解密。
// 未绑定秘密表、字段不可解密、解密失败、S3 读取失败或超限均返回错误；
// 生产识别中错误令规则体不满足，dry-run 与样例校验的 StrictBuiltinErrors 会让错误浮出。
func revealBuiltin(bctx rego.BuiltinContext, fieldTerm *ast.Term) (*ast.Term, error) {
	field, ok := fieldTerm.Value.(ast.String)
	if !ok {
		return nil, errors.Newf("tagengine: %s: argument must be a string, got %T", revealName, fieldTerm.Value)
	}
	sensitive := sensitiveContextFromContext(bctx.Context)
	if sensitive == nil || sensitive.plan == nil {
		return nil, ErrRevealNotActivated
	}
	name := string(field)
	target, ok := sensitive.plan.fields[name]
	if !ok {
		return nil, errors.Newf("tagengine: %s: field %q is not a decryptable field", revealName, name)
	}
	value, err := sensitive.cache.do(name, func() (string, error) {
		return sensitive.reveal(name, target)
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return ast.StringTerm(value), nil
}

// reveal 从秘密表取出字段材料并解密。
func (s *sensitiveContext) reveal(name string, field *sensitiveField) (string, error) {
	secret, ok := s.secrets[name]
	if !ok || secret == nil {
		return "", nil
	}
	switch field.kind {
	case sensitiveKindCrypto:
		// 运行期实例通常只有明文：直接返回，零解密
		if len(secret.plain) > 0 {
			return string(secret.plain), nil
		}
		if len(secret.cipher) == 0 {
			return "", nil
		}
		m, err := field.cryptoMode()
		if err != nil {
			return "", errors.WithStack(err)
		}
		plain, err := m.Decrypt(secret.cipher)
		if err != nil {
			return "", errors.WithStack(err)
		}
		return string(plain), nil
	case sensitiveKindS3Crypto:
		return s.loadObject(name, secret.objectKey, field)
	default:
		return "", errors.Newf("tagengine: %s: field %q has unsupported sensitive kind", revealName, name)
	}
}

// loadObject 下载并解密 S3Crypto 字段对应的对象，按运行时上限截断校验。
func (s *sensitiveContext) loadObject(name, objectKey string, field *sensitiveField) (string, error) {
	if objectKey == "" {
		return "", nil
	}
	if s.runtime == nil || s.runtime.loader == nil {
		return "", ErrRevealRuntimeNotActivated
	}
	cryptoKey, err := field.s3CryptoKey()
	if err != nil {
		return "", errors.WithStack(err)
	}
	reader, err := s.runtime.loader(s.ctx, objectKey, cryptoKey)
	if err != nil {
		return "", errors.WithStack(err)
	}
	// 对象不存在 / 空对象按空内容处理（与 s3.Zkcn.Load 语义一致）
	if reader == nil {
		return "", nil
	}
	defer reader.Close()
	source := io.Reader(reader)
	if s.runtime.maxBytes > 0 {
		source = io.LimitReader(reader, s.runtime.maxBytes+1)
	}
	data, err := io.ReadAll(source)
	if err != nil {
		return "", errors.WithStack(err)
	}
	if s.runtime.maxBytes > 0 && int64(len(data)) > s.runtime.maxBytes {
		return "", errors.Newf("tagengine: %s: field %q content exceeds limit of %d bytes", revealName, name, s.runtime.maxBytes)
	}
	return string(data), nil
}

// init 全局注册 crypto.reveal：同时注册 ast 声明与 topdown 实现，
// 使校验 / dry-run / 样例校验 / 识别全路径可用。
func init() {
	rego.RegisterBuiltin1(revealDecl, revealBuiltin)
}
