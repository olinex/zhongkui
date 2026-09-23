package admin

import (
	// third-party
	"github.com/gin-gonic/gin"
	"github.com/samber/do/v2"

	// internal
	"zhongkui.com/zhongkui/internal/manager/response"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/pkg/status"
	"zhongkui.com/zhongkui/internal/provider"
)

// DependencyItemOut 单个外部依赖状态输出项。
// 字段为基本类型，逐字段映射自 status.Status，不引用模型层结构体。
// DependencyItemOut is an output item for a single external dependency status.
// Its fields are all primitive types mapped field-by-field from status.Status, and it
// does not reference any model-layer struct.
type DependencyItemOut struct {
	// Name 依赖名称（如 core-database）
	// Name the dependency name (e.g. core-database)
	Name string `json:"name"`
	// Category 依赖类别（如 database / redis）
	// Category the dependency category (e.g. database / redis)
	Category string `json:"category"`
	// Configured 该依赖是否已配置
	// Configured whether the dependency is configured
	Configured bool `json:"configured"`
	// Reachable 该依赖是否可达
	// Reachable whether the dependency is reachable
	Reachable bool `json:"reachable"`
	// Status 稳定状态枚举 ok / unreachable / disabled
	// Status the stable status enum ok / unreachable / disabled
	Status string `json:"status"`
	// Latency 探测耗时（毫秒时长）
	// Latency probe latency in milliseconds
	Latency serializer.DurationMS `json:"latency,omitempty"`
	// Message 不可达原因
	// Message the unreachable reason
	Message string `json:"message,omitempty"`
}

// DependencyListOut 依赖状态列表输出。
// DependencyListOut is the dependency status list output.
type DependencyListOut struct {
	// Items 全量外部依赖状态项
	// Items all external dependency status items
	Items []DependencyItemOut `json:"items"`
}

// newDependencyItemOut 将 status.Status 逐字段转换为视图层独立输出项。
// newDependencyItemOut maps a status.Status field-by-field to a standalone view-layer output item.
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
// DependencyListGet handles the dependency status list request, returning the full
// set of external dependency statuses. Because it needs the injector to build the probe
// registry, it returns a gin.HandlerFunc closure.
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
