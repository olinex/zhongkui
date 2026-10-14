package admin

import (
	// 第三方
	"github.com/gin-gonic/gin"
	"github.com/samber/do/v2"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/manager/response"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
	"zhongkuisec.com/zhongkui/internal/pkg/status"
	"zhongkuisec.com/zhongkui/internal/provider"
)

// DependencyItemOut 单个外部依赖状态输出项。
// 字段为基本类型，逐字段映射自 status.Status，不引用模型层结构体。
type DependencyItemOut struct {
	// Name 依赖名称（如 core-database）
	Name string `json:"name"`
	// Category 依赖类别（如 database / redis）
	Category string `json:"category"`
	// Configured 该依赖是否已配置
	Configured bool `json:"configured"`
	// Reachable 该依赖是否可达
	Reachable bool `json:"reachable"`
	// Status 稳定状态枚举 ok / unreachable / disabled
	Status string `json:"status"`
	// Latency 探测耗时（毫秒时长）
	Latency serializer.DurationMS `json:"latency,omitempty"`
	// Message 不可达原因
	Message string `json:"message,omitempty"`
}

// DependencyListOut 依赖状态列表输出。
type DependencyListOut struct {
	// Items 全量外部依赖状态项
	Items []DependencyItemOut `json:"items"`
}

// newDependencyItemOut 将 status.Status 逐字段转换为视图层独立输出项。
func newDependencyItemOut(st status.Status) DependencyItemOut {
	return DependencyItemOut{
		Name:       st.Name,
		Category:   st.Category,
		Configured: st.Configured,
		Reachable:  st.Reachable,
		Status:     st.State(),
		Latency:    serializer.DurationMS(st.Latency.Milliseconds()),
		Message:    st.Err,
	}
}

// DependencyListGet 处理依赖状态列表请求；返回全量外部依赖状态。
// 由于需访问注入器构造探测注册表，返回 gin.HandlerFunc 闭包。
func DependencyListGet(i do.Injector) gin.HandlerFunc {
	return func(c *gin.Context) {
		reg, err := provider.NewDependencyRegistry(c, i)
		if err != nil {
			response.CreateAccidentError(err).AbortWithGinH(c)
			return
		}
		results := reg.Run(c)
		out := make([]DependencyItemOut, 0, len(results))
		for _, st := range results {
			out = append(out, newDependencyItemOut(st))
		}
		response.SimpleOK.AbortWithGinW(c, DependencyListOut{Items: out})
	}
}
