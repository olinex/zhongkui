// Package manager 提供系统管理模块的核心功能，包括认证、授权、
// 中间件、配置管理、数据库操作和任务队列管道等。
package manager

import (
	"github.com/gin-gonic/gin"
	"zhongkui.com/zhongkui/internal/pkg/pipeline"
)

// PipelineInspectorCtxName Pipeline 检查器在 Gin 上下文中存储的键名。
// 用于在中间件和请求处理函数之间传递任务队列检查器实例。
const (
	PipelineInspectorCtxName = "pipeline-inspector"
)

// PipelineInspectorMiddleware 创建一个 Gin 中间件，用于将 Pipeline 检查器实例注入到请求上下文中。
// 该中间件将传入的 Inspector 实例存储到 Gin 上下文中，使得后续的请求处理函数
// 可以通过 GetPipelineInspector 函数获取检查器实例，用于监控和管理任务队列状态。
//
// 参数:
//   - inspector: *pipeline.Inspector - Pipeline 检查器实例，用于监控和管理 asynq 任务队列。
//     该实例通常由 pipeline.NewInspector 创建，需要传入有效的 Redis 客户端。
//
// 返回:
//   - gin.HandlerFunc - Gin 中间件处理函数，将检查器实例设置到上下文后调用后续处理函数。
//
// 使用示例:
//
//	inspector := pipeline.NewInspector(redisClient)
//	router := gin.New()
//	router.Use(PipelineInspectorMiddleware(inspector))
func PipelineInspectorMiddleware(inspector *pipeline.Inspector) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(PipelineInspectorCtxName, inspector)
		c.Next()
	}
}

// GetPipelineInspector 从 Gin 请求上下文中获取 Pipeline 检查器实例。
// 该函数与 PipelineInspectorMiddleware 配合使用，用于在请求处理函数中
// 获取由中间件注入的检查器实例，以便进行任务队列的监控和管理操作。
//
// 参数:
//   - c: *gin.Context - Gin 请求上下文，必须经过 PipelineInspectorMiddleware 中间件处理。
//
// 返回:
//   - *pipeline.Inspector - 从上下文中获取的 Pipeline 检查器实例。
//
// 注意事项:
//   - 如果上下文中不存在检查器实例（即未使用 PipelineInspectorMiddleware 中间件），
//     该函数会触发 panic。因此调用前必须确保已正确注册中间件。
//   - 该函数主要用于需要监控任务队列状态的请求处理场景，如查看队列信息、
//     列出任务、管理任务（删除、归档、重新排队）等。
//
// 使用示例:
//
//	func listQueues(c *gin.Context) {
//	    inspector := GetPipelineInspector(c)
//	    queues, err := inspector.ListQueues()
//	    // 处理结果...
//	}
func GetPipelineInspector(c *gin.Context) *pipeline.Inspector {
	if inspector, ok := c.Get(PipelineInspectorCtxName); ok {
		return inspector.(*pipeline.Inspector)
	}
	panic("pipeline inspector does not exists")
}
