// Package views 提供 HTTP 接口的请求输入（In）与响应输出（Out）视图结构体及处理器。
// 本文件实现全局功能开关（feature flags）列表接口，供前端获知各项功能是否开启。
package views

import (
	"github.com/gin-gonic/gin"

	"zhongkui.com/zhongkui/internal/manager/middlewares"
	"zhongkui.com/zhongkui/internal/manager/response"
)

// FeatureOut 功能开关输出项：键 + 启停状态。
// FeatureOut is a feature-toggle output item: key + enabled state.
type FeatureOut struct {
	// Key 功能键（如 passwordReset）
	// Key the feature key (e.g. passwordReset)
	Key string `json:"key"`
	// Enabled 该功能当前是否开启
	// Enabled whether the feature is currently enabled
	Enabled bool `json:"enabled"`
}

// FeaturesListGetOut 功能开关列表输出：依装配顺序返回全部功能开关。
// FeaturesListGetOut is the feature-toggle list output, returning all toggles in
// assembly order.
type FeaturesListGetOut []FeatureOut

// FeaturesListGet 处理功能开关列表请求（公共接口，无需认证）。
// 读取注入的功能开关注册表并以视图层独立输出结构逐项映射返回，
// 前端据此决定是否展示相关组件（如"忘记密码"入口依赖 passwordReset 开关）。
func FeaturesListGet(c *gin.Context) {
	registry := middlewares.GetFeatureRegistry(c)
	items := registry.List()
	out := make(FeaturesListGetOut, 0, len(items))
	for _, item := range items {
		out = append(out, FeatureOut{Key: item.Key, Enabled: item.Enabled})
	}
	response.SimpleOK.AbortWithGinW(c, out)
}
