package reporter

import (
	"time"

	"go.uber.org/fx"
	"zhongkui.com/zhongkui/internal/depend"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/grpc"
)

const (
	// ACCESS_KEY_PREFIX 是访问密钥在缓存中的前缀
	// 用于在Redis缓存中为访问密钥创建唯一的键名，避免与其他缓存数据冲突
	ACCESS_KEY_PREFIX = "akp"
)

// ReporterAuthRouterFxIn 是用于身份验证路由器的依赖注入结构体
// 该结构体通过Go的Fx依赖注入框架，将必要的数据库和缓存依赖注入到身份验证器中
//
// 字段说明：
//   - fx.In: 嵌入Fx的In结构体，标识这是一个依赖注入输入结构
//   - CoreDatabaseFxIn: 核心数据库依赖，提供对核心数据库的访问
//   - CacheRedisFxIn: Redis缓存依赖，用于缓存访问密钥和防重放检测
type ReporterAuthRouterFxIn struct {
	fx.In
	depend.CoreDatabaseFxIn
	depend.CacheRedisFxIn
}

// NewReporterAuthenticators 创建并返回一个身份验证器映射
// 该函数用于初始化报告服务需要的所有身份验证器，并将它们注册到gRPC身份验证路由器中
//
// 功能说明：
//  1. 创建访问密钥池，用于管理访问密钥的缓存和查询
//  2. 创建基于访问密钥的身份验证器
//  3. 返回包含所有身份验证器的映射，键为协议类型，值为对应的验证器实现
//
// 参数：
//   - in: ReporterAuthRouterFxIn - 包含数据库和缓存依赖的注入结构
//
// 返回值：
//   - map[string]grpc.Authenticator - 身份验证器映射
//   - 键: grpc.ProtocolAccessKeySecret - 访问密钥身份验证协议
//   - 值: *grpc.AccessKeyAuthenticator - 访问密钥身份验证器实例
//
// 实现细节：
//   - 使用core.NewAccessKeyPool创建访问密钥池，设置缓存过期时间为1分钟
//   - 访问密钥池结合数据库和Redis缓存，提高密钥查询性能并支持防重放攻击
//   - 创建的身份验证器会验证请求中的签名令牌，确保请求的合法性和完整性
//   - 当前只支持访问密钥身份验证协议，未来可以扩展支持其他协议
func NewReporterAuthenticators(in ReporterAuthRouterFxIn) map[string]grpc.Authenticator {
	// 创建访问密钥池，结合数据库持久化和Redis缓存
	// 缓存过期时间设置为1分钟，平衡性能和一致性
	pool := core.NewAccessKeyPool(in.CoreDB, in.CacheRedis, time.Minute)

	// 创建基于访问密钥的身份验证器
	// 该验证器会检查请求中的签名令牌，验证访问密钥的有效性
	accessKeyAuthenticator := grpc.NewAccessKeyAuthenticator(pool)

	// 返回身份验证器映射
	// 当前只注册访问密钥身份验证协议
	return map[string]grpc.Authenticator{
		grpc.ProtocolAccessKeySecret: accessKeyAuthenticator,
	}
}
