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
package features

import (
	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/base"
)

// IsEnabled 具备可查询启用状态的依赖提供方接口（如由 *mail.Sender 满足）：用于查询
// 某依赖能力当前是否可用。定义在使用方（features）以解耦与具体实现包的依赖：底层能力
// 判断（如 SMTP 配置完整性）被封装进实现方的 Enabled，features 不直接读取其配置结构。
type IsEnabled interface {
	// Enabled 报告依赖能力当前是否可用。
	Enabled() bool
}

// Reasoned 在启用状态之外还能报告「不可用原因」的依赖提供方接口（由 *ai.Runtime 满足）：
// 原因文案由实现方按请求语言本地化（如 LLM 档位未绑定），features 只转发不解读。
// 实现该接口的功能在列表接口中带上原因，供前端向用户说明「为什么功能不可用」；
// 未实现的功能（如密码重置）原因留空，前端回退到通用提示。
type Reasoned interface {
	IsEnabled
	// DisabledReason 返回依赖不可用的原因（按请求语言本地化；可用时为空串）。
	DisabledReason(lang *base.Language) string
}

// Key 全局功能开关键常量。
const (
	// PasswordResetKey 密码重置功能：依赖系统 SMTP 邮件能力，未启用或未配置 SMTP 时关闭
	PasswordResetKey = "passwordReset"
	// AiKey AI 助手功能（RAG + Agent）：依赖模型注册表中对话档位与 embedding 档位的
	// 可用模型绑定（已启用且类型匹配），缺失任意一项时关闭
	AiKey = "ai"
)

// Feature 全局功能开关接口：由具体功能实现，供注册表聚合枚举与查询。
type Feature interface {
	// Key 返回功能键（稳定标识，如 passwordReset）
	Key() string
	// Enabled 返回该功能当前是否开启
	Enabled() bool
}

// Out 功能开关输出项，供列表接口序列化给前端。
type Out struct {
	// Key 功能键
	Key string `json:"key"`
	// Enabled 该功能是否开启
	Enabled bool `json:"enabled"`
	// Reason 该功能不可用的原因（按请求语言本地化；功能可用或实现方未提供原因为空串），
	// 供前端展示「为什么功能不可用」而非只给一个 false
	Reason string `json:"reason"`
}

// Registry 全局功能开关注册表：按装配顺序维护功能集合，供枚举与按键查询。
type Registry struct {
	// features 已注册功能集合，按装配顺序保存。
	features []Feature
}

// NewRegistry 依据装配顺序构造功能注册表。
func NewRegistry(features ...Feature) *Registry {
	return &Registry{features: features}
}

// List 依装配顺序返回全部功能开关快照（含启停状态），并带上实现方按请求语言给出的
// 不可用原因；未实现 Reasoned 的功能原因为空串。
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
type passwordResetFeature struct {
	sender IsEnabled
}

// Key 返回密码重置功能键。
func (f *passwordResetFeature) Key() string { return PasswordResetKey }

// Enabled 判断密码重置功能是否可用：由系统邮件直发器报告其能力（Sender.Enabled）。
func (f *passwordResetFeature) Enabled() bool { return f.sender.Enabled() }

// PasswordReset 构造密码重置功能开关。
func PasswordReset(sender IsEnabled) Feature {
	return &passwordResetFeature{sender: sender}
}

// aiFeature AI 助手功能开关：委托 AI 运行时（ai.Runtime.Enabled / ai.Runtime.DisabledReason）
// 实时判断，仅当对话档位与 embedding 档位均有可用模型绑定（已启用且类型匹配）时开启，避免开放
// 无意义的功能入口；LLM 接入细节对 features 不可见（只依赖 Reasoned 行为契约）。
type aiFeature struct {
	aiRuntime Reasoned
}

// Key 返回 AI 助手功能键。
func (f *aiFeature) Key() string { return AiKey }

// Enabled 判断 AI 助手功能是否可用：由 AI 运行时报告其当前可用性
// （ai.Runtime.Enabled，随配置变更热重载）。
func (f *aiFeature) Enabled() bool { return f.aiRuntime.Enabled() }

// DisabledReason 返回 AI 助手不可用的原因（按请求语言本地化；可用时为空串），
// 直接转发 AI 运行时的说明（ai.Runtime.DisabledReason），features 不解读其内容。
func (f *aiFeature) DisabledReason(lang *base.Language) string {
	return f.aiRuntime.DisabledReason(lang)
}

// Ai 构造 AI 助手功能开关。
func Ai(enabled Reasoned) Feature {
	return &aiFeature{aiRuntime: enabled}
}
