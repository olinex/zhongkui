// Package features 提供全局功能开关（feature flags）能力。
//
// 前端可通过统一的功能开关列表接口查询各项功能是否开启，从而决定是否展示相关组件；
// 后端视图层亦通过同一注册表判断某功能是否可用，使"功能可用性判断"收敛到单一事实来源，
// 避免各视图层直接读取底层配置（如 [smtp]）自行判断。
// 功能开关的有无与启停由依赖注入装配（internal/provider）在启动期一次性构建。
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

// Key 全局功能开关键常量。
// Key constants of global feature toggles.
const (
	// PasswordResetKey 密码重置功能：依赖系统 SMTP 邮件能力，未启用或未配置 SMTP 时关闭
	// PasswordResetKey the password-reset feature; depends on the system SMTP email
	// capability and is disabled when SMTP is not enabled or not configured
	PasswordResetKey = "passwordReset"
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

// List 依装配顺序返回全部功能开关快照（含启停状态）。
// List returns a snapshot of all feature toggles (with their enabled state) in assembly order.
func (r *Registry) List() []Out {
	items := make([]Out, 0, len(r.features))
	for _, feature := range r.features {
		items = append(items, Out{Key: feature.Key(), Enabled: feature.Enabled()})
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
