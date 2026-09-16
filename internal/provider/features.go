// Package provider 集中管理依赖注入（samber/do/v2）的 provider 工厂与注入/调用函数。
// 本文件提供全局功能开关注册表（features.Registry）的依赖装配。
package provider

import (
	// third-party
	"github.com/samber/do/v2"

	// internal
	"zhongkui.com/zhongkui/internal/manager/features"
	"zhongkui.com/zhongkui/internal/pkg/llm"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/validator"
)

// newFeatureRegistryProvider 装配全局功能开关注册表：构建系统邮件直发器（Sender）并提取
// AI（[llm]）配置（未配置 [llm] 段时按填充默认值的空配置处理，AI 功能自然关闭），以提供
// 各项功能开关（如密码重置依赖 Sender.Enabled、AI 助手依赖 LLMConfig.Enabled），
// 统一以一致的装配顺序聚合。
func newFeatureRegistryProvider() do.Provider[*features.Registry] {
	return func(i do.Injector) (*features.Registry, error) {
		sender, err := InvokeMailSender(i)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		viper, err := InvokeViper(i)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		aiCfg, err := llm.ExtractConfigOrEmpty(viper, validator.Validate)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return features.NewRegistry(
			features.PasswordReset(sender),
			features.Ai(aiCfg),
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
