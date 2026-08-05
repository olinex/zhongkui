package grpc

import (
	"context"
	"fmt"
	"sync"

	g "google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// AccessKeyUserId 是上下文(Context)中存储用户ID的键名
// 当身份验证成功后，用户ID会被存储到上下文中，后续处理可以通过该键名获取用户身份信息
const (
	AccessKeyUserId = "access_key_user_id"
)

// ExtractUserID 从上下文中提取用户ID
// 该函数用于在gRPC处理函数中获取经过身份验证的用户ID
// 参数:
//   - ctx: 包含用户ID的上下文
//
// 返回:
//   - string: 从上下文中提取的用户ID字符串
//
// 注意: 调用此函数前必须确保上下文已通过身份验证并包含用户ID
func ExtractUserID(ctx context.Context) string {
	return ctx.Value(AccessKeyUserId).(string)
}

// AuthRouter 是gRPC身份验证路由器
// 负责管理和路由不同的身份验证器，支持多种认证协议
// 结构体字段:
//   - lock: 读写锁，保护authes映射的并发访问
//   - authes: 身份验证器映射，键为验证器名称，值为Authenticator接口实现
//   - config: 身份验证服务器配置，包含认证相关的配置选项
type AuthRouter struct {
	lock   sync.RWMutex
	authes map[string]Authenticator
	config *AuthServerConfig
}

// NewAuthRouter 创建新的身份验证路由器
// 参数:
//   - config: 身份验证服务器配置
//
// 返回:
//   - *AuthRouter: 新创建的身份验证路由器实例
func NewAuthRouter(config *AuthServerConfig) *AuthRouter {
	return &AuthRouter{
		lock:   sync.RWMutex{},
		authes: nil,
		config: config,
	}
}

// Register 注册身份验证器到路由器
// 该方法用于注册一个或多个身份验证器，每个验证器对应一种认证协议
// 参数:
//   - authenticators: 身份验证器映射，键为验证器名称，值为Authenticator接口实现
//
// 返回:
//   - error: 如果验证器已经注册过，则返回错误；否则返回nil
//
// 注意: 该方法只能调用一次，重复调用会返回错误
func (a *AuthRouter) Register(authenticators map[string]Authenticator) error {
	a.lock.Lock()
	defer a.lock.Unlock()
	if a.authes != nil {
		return errors.New("auth handlers already registered")
	}
	a.authes = authenticators
	return nil
}

// Get 根据名称获取身份验证器
// 该方法用于根据验证器名称查找对应的身份验证器实现
// 参数:
//   - name: 身份验证器名称
//
// 返回:
//   - Authenticator: 找到的身份验证器，如果未找到则返回nil
func (a *AuthRouter) Get(name string) Authenticator {
	a.lock.RLock()
	defer a.lock.RUnlock()
	return a.authes[name]
}

// auth 执行身份验证的核心逻辑
// 该方法从gRPC请求的元数据(Metadata)中提取认证信息，验证请求的合法性
// 验证步骤:
//  1. 从上下文中提取元数据(Metadata)
//  2. 检查必须的元数据字段是否存在(:authority, authorize, authorization)
//  3. 根据认证类型获取对应的身份验证器
//  4. 调用验证器的Handle方法进行具体验证
//
// 参数:
//   - ctx: 包含请求元数据的上下文
//   - method: gRPC方法全名，格式为"/package.Service/Method"
//
// 返回:
//   - string: 验证成功后返回的用户ID
//   - error: 验证失败时返回的错误信息
func (a *AuthRouter) auth(ctx context.Context, method string) (string, error) {
	if md, ok := metadata.FromIncomingContext(ctx); !ok {
		return "", ErrMissingMetadata
	} else if authority := md[":authority"]; len(authority) == 0 {
		return "", ErrBadMetadata
	} else if authorizes := md[MetadataKeyAuthorize]; len(authorizes) == 0 {
		return "", ErrBadMetadata
	} else if authorizes[0] == ProtocolAccessKeySecret && !a.config.AccessKeyEnabled {
		return "", nil
	} else if authorizations := md[MetadataKeyAuthorization]; len(authorizations) == 0 {
		return "", ErrBadMetadata
	} else if authenticator := a.Get(authorizes[0]); authenticator == nil {
		return "", ErrInvalidAuthType
	} else if userID, err := authenticator.Handle(ctx, fmt.Sprintf("https://%s%s", authority[0], method), authorizations[0]); err != nil {
		return "", errors.WithStack(err)
	} else {
		return userID, nil
	}
}

// authUnary 单元式gRPC调用的身份验证拦截器
// 该方法是gRPC UnaryInterceptor的实现，用于验证单元式RPC调用
// 验证流程:
//  1. 调用auth方法验证请求身份
//  2. 验证成功后，将用户ID注入到上下文中
//  3. 调用原始处理器继续处理请求
//
// 参数:
//   - ctx: 请求上下文
//   - req: 请求参数
//   - info: gRPC服务和方法信息
//   - handler: 原始请求处理器
//
// 返回:
//   - any: 处理结果
//   - error: 处理过程中发生的错误
func (a *AuthRouter) authUnary(ctx context.Context, req any, info *g.UnaryServerInfo, handler g.UnaryHandler) (any, error) {
	if userID, err := a.auth(ctx, info.FullMethod); err != nil {
		return nil, errors.WithStack(err)
	} else {
		return handler(context.WithValue(ctx, AccessKeyUserId, userID), req)
	}
}

// authStream 流式gRPC调用的身份验证拦截器
// 该方法是gRPC StreamInterceptor的实现，用于验证流式RPC调用
// 验证流程:
//  1. 从流上下文中调用auth方法验证请求身份
//  2. 验证成功后，创建包装器流(WrapperServerStream)
//  3. 包装器流会将用户ID注入到流上下文中
//  4. 调用原始处理器继续处理流
//
// 参数:
//   - srv: 服务实现
//   - ss: 服务器流
//   - info: gRPC流服务和方法信息
//   - handler: 原始流处理器
//
// 返回:
//   - error: 处理过程中发生的错误
func (a *AuthRouter) authStream(srv any, ss g.ServerStream, info *g.StreamServerInfo, handler g.StreamHandler) error {
	if userID, err := a.auth(ss.Context(), info.FullMethod); err != nil {
		return errors.WithStack(err)
	} else {
		wrapper := NewWrapperServerStream(userID, ss)
		return handler(srv, wrapper)
	}
}
