// RegisterContext 路由注册上下文：聚合全局引擎、API 版本组与公共中间件实例，
// 供各业务子包的 RegisterRoutes 注册函数复用。
//
// 该结构体承载自原 internal/manager/router 包迁移而来的共享注册信息，
// 字段全部导出，业务子包（notification/tag/traffic/user/admin/datasource/etl/plaintext/ai）
// 经由它完成自己域内的路由注册，从而让每个子路由模块内聚在所属子包内。
package views

import (
	"github.com/gin-gonic/gin"
	"github.com/samber/do/v2"
)

// RegisterContext 路由注册上下文。
type RegisterContext struct {
	// Engine 全局 Gin 引擎。
	Engine *gin.Engine
	// V1Group /api/v1 API 版本路由组。
	V1Group *gin.RouterGroup
	// Injector 依赖注入容器。
	Injector do.Injector

	// Authenticator 认证器中间件，将认证器实例注入请求上下文。
	Authenticator gin.HandlerFunc
	// ForceAuthenticate 强制认证中间件，要求请求必须通过 JWT 认证。
	ForceAuthenticate gin.HandlerFunc
	// Policy RBAC 策略中间件，将策略引擎注入请求上下文。
	Policy gin.HandlerFunc
	// Authorize 权限验证中间件：基于请求路径（api:{FullPath}）+ HTTP 方法动态授权。
	Authorize gin.HandlerFunc
	// CoreDatabase 数据库中间件，将核心数据库连接注入请求上下文。
	CoreDatabase gin.HandlerFunc
	// RawDatabase raw 库中间件，供标签事件规则 dry-run 查询 tag_summings。
	RawDatabase gin.HandlerFunc
	// S3 S3 客户端中间件，供原始流量 body 明文读取。
	S3 gin.HandlerFunc
	// ObjectCache S3 通用加密对象缓存中间件，供知识库原件上传/下载/清理使用。
	ObjectCache gin.HandlerFunc
	// Etcd etcd 客户端中间件，将 etcd 客户端注入请求上下文。
	Etcd gin.HandlerFunc
	// Namespace 命名空间中间件，处理多租户命名空间隔离。
	Namespace gin.HandlerFunc
	// PipelineInspector 管道检查中间件，将管道检查器注入请求上下文。
	PipelineInspector gin.HandlerFunc
	// PipelineClient 管道投递客户端中间件，将 asynq 投递客户端注入请求上下文。
	PipelineClient gin.HandlerFunc
	// TagRuleEngine 标签规则引擎中间件，将标签引擎注入请求上下文。
	TagRuleEngine gin.HandlerFunc
	// FeatureRegistry 全局功能开关注册表中间件。
	FeatureRegistry gin.HandlerFunc
	// PasswordResetLimiter 密码重置申请限流中间件，将限流器注入请求上下文。
	PasswordResetLimiter gin.HandlerFunc
	// MessagePool 消息渠道连接池中间件。
	MessagePool gin.HandlerFunc
	// SQLiteVTabPools SQLite vtable 池持有者中间件。
	SQLiteVTabPools gin.HandlerFunc
	// Ai AI 助手组件中间件。
	Ai gin.HandlerFunc
}
