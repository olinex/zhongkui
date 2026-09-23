// Package views 提供 HTTP 接口的请求输入（In）与响应输出（Out）视图结构体及处理器。
// 本文件实现全局功能开关（feature flags）列表接口，供前端获知各项功能是否开启。
package views

import (
	"github.com/gin-gonic/gin"

	"zhongkui.com/zhongkui/internal/manager/middlewares"
	"zhongkui.com/zhongkui/internal/manager/response"
)

// FeatureOut 功能开关输出项：键 + 启停状态 + 不可用原因。
// FeatureOut is a feature-toggle output item: key + enabled state + unavailability reason.
type FeatureOut struct {
	// Key 功能键（如 passwordReset）
	// Key the feature key (e.g. passwordReset)
	Key string `json:"key"`
	// Enabled 该功能当前是否开启
	// Enabled whether the feature is currently enabled
	Enabled bool `json:"enabled"`
	// Reason 该功能不可用的原因（按请求语言本地化；可用或实现方未提供原因为空串），
	// 供前端在侧边栏提示与「功能不可用」说明页中说明原因
	// Reason the localized reason why the feature is unavailable (empty when enabled or the
	// implementer gives no reason), letting the frontend explain the reason in the sidebar
	// hint and on the "feature unavailable" page
	Reason string `json:"reason"`
}

// FeaturesListGetOut 功能开关列表输出：依装配顺序返回全部功能开关。
// FeaturesListGetOut is the feature-toggle list output, returning all toggles in
// assembly order.
type FeaturesListGetOut []FeatureOut

// FeaturesListGet 处理功能开关列表请求（公共接口，无需认证）。
// 读取注入的功能开关注册表并以视图层独立输出结构逐项映射返回（不可用原因按请求语言
// 本地化），前端据此刻画功能入口的可用性：功能关闭时相关入口不隐藏而是置为不可用态，
// 并在用户进入功能页时引导其前往开通入口。
func FeaturesListGet(c *gin.Context) {
	registry := middlewares.GetFeatureRegistry(c)
	items := registry.List(middlewares.GetLanguage(c))
	out := make(FeaturesListGetOut, 0, len(items))
	for _, item := range items {
		out = append(out, FeatureOut{Key: item.Key, Enabled: item.Enabled, Reason: item.Reason})
	}
	response.SimpleOK.AbortWithGinW(c, out)
}
