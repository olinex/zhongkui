// Package plaintext 提供 /api/v1/plaintext 通用明文查看接口的子路由注册。
package plaintext

import (
	"net/http"

	"zhongkui.com/zhongkui/internal/manager/views"
)

// RegisterRoutes 注册 /api/v1/plaintext 通用明文查看接口路由。
// 全局通用接口：解密加密字段密文 / 读取 S3 body 明文，响应原始字节流。
// 权限点 object = plaintext:{crypto|s3}:{model}:{field}，action = READ（规范见 internal/pkg/base/consts.go）。
// 新增模型的加密/S3 字段：RegisterPlaintextModel 注册模型 + 补模板种子即可，无需新增接口。
func RegisterRoutes(ctx *views.RegisterContext) {
	plaintextGroup := ctx.V1Group.Group(
		"/plaintext",          // 明文查看接口路径前缀
		ctx.Authenticator,     // 认证器注入
		ctx.ForceAuthenticate, // 强制JWT认证
		ctx.Policy,            // RBAC策略引擎
		ctx.Namespace,         // 命名空间隔离
		ctx.Authorize,         // 权限验证（api:{FullPath} + HTTP 方法动态授权）
		ctx.S3,                // S3 客户端（S3 body 明文读取）
	)
	// 加密字段明文解密 - 请求携带模型注册名 + 字段 + base64 密文，按字段权限点静态授权后返回明文
	plaintextGroup.POST("/crypto", CryptoPlaintextGet)
	// 加密字段明文解密元信息
	plaintextGroup.OPTIONS("/crypto", views.StructMetaGinHandler(
		views.WithMethod(http.MethodPost, views.WithBodyInput[CryptoPlaintextIn]()),
	))
	// S3 body 明文读取 - 请求携带模型注册名 + 字段 + 对象键，按字段权限点静态授权后返回明文
	plaintextGroup.POST("/s3", S3PlaintextGet)
	// S3 body 明文读取元信息
	plaintextGroup.OPTIONS("/s3", views.StructMetaGinHandler(
		views.WithMethod(http.MethodPost, views.WithBodyInput[S3PlaintextIn]()),
	))
}
