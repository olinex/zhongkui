// Package features 提供全局功能开关（feature flags）能力。
//
// 前端可通过统一的功能开关列表接口查询各项功能是否开启（以及不可用原因），从而把相关入口
// 置为不可用态而非隐藏，并引导用户前往开通入口；后端视图层亦通过同一注册表判断某功能是否
// 可用，使"功能可用性判断"收敛到单一事实来源，避免各视图层直接读取底层配置（如 [smtp]）
// 自行判断。功能开关的有无与启停由依赖注入装配（internal/provider）在启动期一次性构建。
//
// 约定：
//   - 新增功能：实现 Feature 接口（Key + Enabled），并在 provider 装配处加入典型构造器；
//   - Key 采用开头小写驼峰命名（如 passwordReset），作为前端识别该功能的稳定标识；
//   - Enabled 值应为可真实反映功能可用性的派生判断（如依赖配置完整、依赖服务可用）。
//
// Package features provides the global feature-flag capability. The frontend can query a
// unified feature-toggle list endpoint to learn which features are enabled and decide
// whether to show related components; the backend view layer uses the same registry to tell
// whether a feature is usable, so the "feature availability" decision converges to a single
// source of truth instead of each view reading underlying configs (e.g. [smtp]). Whether a
// feature exists and is enabled is assembled once at startup via dependency injection.
package features

import (
	// internal
	"zhongkui.com/zhongkui/internal/pkg/base"
)

// IsEnabled 具备可查询启用状态的依赖提供方接口（如由 *mail.Sender 满足）：用于查询
// 某依赖能力当前是否可用。定义在使用方（features）以解耦与具体实现包的依赖：底层能力
// 判断（如 SMTP 配置完整性）被封装进实现方的 Enabled，features 不直接读取其配置结构。
// IsEnabled is the provider interface that reports whether a dependency capability is
// currently available (e.g. satisfied by *mail.Sender). It is defined on the consumer side
// (features) to decouple from concrete implementation packages: the underlying capability
// check (e.g. SMTP-config completeness) is encapsulated inside the implementer's Enabled,
// so features does not read their config structures directly.
type IsEnabled interface {
	Enabled() bool
}

// Reasoned 在启用状态之外还能报告「不可用原因」的依赖提供方接口（由 *ai.Runtime 满足）：
// 原因文案由实现方按请求语言本地化（如 LLM 档位未绑定），features 只转发不解读。
// 实现该接口的功能在列表接口中带上原因，供前端向用户说明「为什么功能不可用」；
// 未实现的功能（如密码重置）原因留空，前端回退到通用提示。
// Reasoned is the provider interface that reports the reason of unavailability in addition
// to the enabled state (satisfied by *ai.Runtime): the reason text is localized to the
// request language by the implementer (e.g. an unbound LLM slot) and features only relays
// it without interpreting it. A feature implementing this interface carries its reason in
// the list endpoint so the frontend can tell users why the feature is unavailable; a feature
// without it (e.g. password reset) yields an empty reason and the frontend falls back to a
// generic hint.
type Reasoned interface {
	IsEnabled
	DisabledReason(lang *base.Language) string
}

// Key 全局功能开关键常量。
// Key constants of global feature toggles.
const (
	// PasswordResetKey 密码重置功能：依赖系统 SMTP 邮件能力，未启用或未配置 SMTP 时关闭
	// PasswordResetKey the password-reset feature; depends on the system SMTP email
	// capability and is disabled when SMTP is not enabled or not configured
	PasswordResetKey = "passwordReset"
	// AiKey AI 助手功能（RAG + Agent）：依赖模型注册表中对话档位与 embedding 档位的
	// 可用模型绑定（已启用且类型匹配），缺失任意一项时关闭
	// AiKey the AI assistant feature (RAG + Agent); depends on the usable model bindings of
	// the chat and embedding slots in the model registry (enabled with a matching kind),
	// disabled when either is missing
	AiKey = "ai"
)

// Feature 全局功能开关接口：由具体功能实现，供注册表聚合枚举与查询。
// Feature is the interface of a global feature toggle, implemented by concrete
// features and aggregated/enumerated/queried by the registry.
type Feature interface {
	// Key 返回功能键（稳定标识，如 passwordReset）
	// Key returns the feature key (a stable identifier, e.g. passwordReset)
	Key() string
	// Enabled 返回该功能当前是否开启
	// Enabled reports whether the feature is currently enabled
	Enabled() bool
}

// Out 功能开关输出项，供列表接口序列化给前端。
// Out is a feature-toggle output item serialized to the frontend by the list interface.
type Out struct {
	// Key 功能键
	// Key the feature key
	Key string `json:"key"`
	// Enabled 该功能是否开启
	// Enabled whether the feature is enabled
	Enabled bool `json:"enabled"`
	// Reason 该功能不可用的原因（按请求语言本地化；功能可用或实现方未提供原因为空串），
	// 供前端展示「为什么功能不可用」而非只给一个 false
	// Reason the localized reason why the feature is unavailable (empty when the feature is
	// enabled or the implementer gives no reason), letting the frontend explain why instead
	// of reporting a bare false
	Reason string `json:"reason"`
}

// Registry 全局功能开关注册表：按装配顺序维护功能集合，供枚举与按键查询。
// Registry is the global feature-toggle registry that keeps the assembled feature set
// for enumeration and keyed lookup.
type Registry struct {
	features []Feature
}

// NewRegistry 依据装配顺序构造功能注册表。
// NewRegistry builds a registry from the given features in assembly order.
func NewRegistry(features ...Feature) *Registry {
	return &Registry{features: features}
}

// List 依装配顺序返回全部功能开关快照（含启停状态），并带上实现方按请求语言给出的
// 不可用原因；未实现 Reasoned 的功能原因为空串。
// List returns a snapshot of all feature toggles (with their enabled state) in assembly
// order, carrying the localized unavailability reason of the implementers; a feature that
// does not implement Reasoned yields an empty reason.
func (r *Registry) List(lang *base.Language) []Out {
	items := make([]Out, 0, len(r.features))
	for _, feature := range r.features {
		item := Out{Key: feature.Key(), Enabled: feature.Enabled()}
		if reasoned, ok := feature.(Reasoned); ok {
			item.Reason = reasoned.DisabledReason(lang)
		}
		items = append(items, item)
	}
	return items
}

// IsEnabled 判断指定键的功能是否开启；未注册的功能视为关闭。
// IsEnabled reports whether the feature with the given key is enabled;
// an unregistered key is treated as disabled.
func (r *Registry) IsEnabled(key string) bool {
	for _, feature := range r.features {
		if feature.Key() == key {
			return feature.Enabled()
		}
	}
	return false
}

// passwordResetFeature 密码重置功能开关：委托系统邮件直发器（Sender.Enabled）判断，
// 仅当系统邮件能力可用时开启，避免开放无意义的功能入口；SMTP 配置细节对 features 不可见。
// passwordResetFeature is the password-reset feature toggle: it delegates to the system
// email direct-sender (Sender.Enabled) and is enabled only when the email capability is
// available, avoiding exposing a meaningless feature entry; SMTP config details stay
// invisible to features.
type passwordResetFeature struct {
	sender IsEnabled
}

// Key 返回密码重置功能键。
// Key returns the password-reset feature key.
func (f *passwordResetFeature) Key() string { return PasswordResetKey }

// Enabled 判断密码重置功能是否可用：由系统邮件直发器报告其能力（Sender.Enabled）。
// Enabled reports the password-reset availability, derived from the system email
// direct-sender capability (Sender.Enabled).
func (f *passwordResetFeature) Enabled() bool { return f.sender.Enabled() }

// PasswordReset 构造密码重置功能开关。
// PasswordReset builds the password-reset feature toggle.
func PasswordReset(sender IsEnabled) Feature {
	return &passwordResetFeature{sender: sender}
}

// aiFeature AI 助手功能开关：委托 AI 运行时（ai.Runtime.Enabled / ai.Runtime.DisabledReason）
// 实时判断，仅当对话档位与 embedding 档位均有可用模型绑定（已启用且类型匹配）时开启，避免开放
// 无意义的功能入口；LLM 接入细节对 features 不可见（只依赖 Reasoned 行为契约）。
// aiFeature is the AI assistant feature toggle: it delegates to the live AI runtime
// (ai.Runtime.Enabled / ai.Runtime.DisabledReason) and is enabled only when both the chat
// slot and the embedding slot have usable model bindings (enabled with a matching kind),
// avoiding exposing a meaningless feature entry; the LLM access details stay invisible to
// features (it depends only on the Reasoned behavior contract).
type aiFeature struct {
	aiRuntime Reasoned
}

// Key 返回 AI 助手功能键。
// Key returns the AI assistant feature key.
func (f *aiFeature) Key() string { return AiKey }

// Enabled 判断 AI 助手功能是否可用：由 AI 运行时报告其当前可用性
// （ai.Runtime.Enabled，随配置变更热重载）。
// Enabled reports the AI assistant availability, derived from the AI runtime's current
// availability (ai.Runtime.Enabled, hot-reloaded on config changes).
func (f *aiFeature) Enabled() bool { return f.aiRuntime.Enabled() }

// DisabledReason 返回 AI 助手不可用的原因（按请求语言本地化；可用时为空串），
// 直接转发 AI 运行时的说明（ai.Runtime.DisabledReason），features 不解读其内容。
// DisabledReason returns the localized reason why the AI assistant is unavailable (empty
// when available), relaying the AI runtime's explanation (ai.Runtime.DisabledReason)
// without interpreting it.
func (f *aiFeature) DisabledReason(lang *base.Language) string {
	return f.aiRuntime.DisabledReason(lang)
}

// Ai 构造 AI 助手功能开关。
// Ai builds the AI assistant feature toggle.
func Ai(enabled Reasoned) Feature {
	return &aiFeature{aiRuntime: enabled}
}
