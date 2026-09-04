// Package provider 集中管理依赖注入（samber/do/v2）的 provider 工厂与注入/调用函数。
// 本文件提供全局功能开关注册表（features.Registry）的依赖装配。
package provider

import (
	// third-party
	"github.com/samber/do/v2"

	// internal
	"zhongkui.com/zhongkui/internal/manager/features"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// newFeatureRegistryProvider 装配全局功能开关注册表：构建系统邮件直发器（Sender）
// 以提供各项功能开关（如密码重置依赖 Sender.Enabled 报告的系统邮件能力），
// 统一以一致的装配顺序聚合。
func newFeatureRegistryProvider() do.Provider[*features.Registry] {
	return func(i do.Injector) (*features.Registry, error) {
		sender, err := InvokeMailSender(i)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return features.NewRegistry(
			features.PasswordReset(sender),
		), nil
	}
}

// InjectFeatureRegistry 向依赖注入器注册功能开关注册表 provider。
func InjectFeatureRegistry(i do.Injector) {
	do.Provide(i, newFeatureRegistryProvider())
}

// InvokeFeatureRegistry 从依赖注入器解析功能开关注册表，错误在调用方处理。
func InvokeFeatureRegistry(i do.Injector) (*features.Registry, error) {
	return do.Invoke[*features.Registry](i)
}

// MustInvokeFeatureRegistry 从依赖注入器解析功能开关注册表；失败时 panic（仅用于启动关键依赖）。
func MustInvokeFeatureRegistry(i do.Injector) *features.Registry {
	return do.MustInvoke[*features.Registry](i)
}
