package grpc

import (
	"context"

	g "google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// WrapperServerStream 是 gRPC ServerStream 的包装器
// 主要作用是在流上下文(Context)中注入用户ID信息
// 这样在流处理过程中可以通过上下文获取用户身份信息
type WrapperServerStream struct {
	userID string         // 用户ID，从认证信息中提取
	inner  g.ServerStream // 底层的 gRPC ServerStream
}

// NewWrapperServerStream 创建新的包装器流
// 参数:
//   - userID: 从认证信息中提取的用户ID
//   - inner: 底层的 gRPC ServerStream
//
// 返回: 包装后的 ServerStream
func NewWrapperServerStream(userID string, inner g.ServerStream) *WrapperServerStream {
	return &WrapperServerStream{
		userID,
		inner,
	}
}

// SetHeader 设置响应头信息
// 这是对底层 ServerStream SetHeader 方法的包装
func (ss *WrapperServerStream) SetHeader(md metadata.MD) error {
	return ss.inner.SendHeader(md)
}

// SendHeader 发送响应头信息
// 这是对底层 ServerStream SendHeader 方法的包装
func (ss *WrapperServerStream) SendHeader(md metadata.MD) error {
	return ss.inner.SendHeader(md)
}

// SetTrailer 设置响应尾信息
// 这是对底层 ServerStream SetTrailer 方法的包装
func (ss *WrapperServerStream) SetTrailer(md metadata.MD) {
	ss.inner.SetTrailer(md)
}

// Context 返回包装后的上下文
// 核心功能：在底层上下文中注入用户ID信息
// 这样在流处理过程中可以通过 ExtractUserID 函数获取用户身份
func (ss *WrapperServerStream) Context() context.Context {
	return context.WithValue(ss.inner.Context(), AccessKeyUserId, ss.userID)
}

// SendMsg 发送消息到客户端
// 这是对底层 ServerStream SendMsg 方法的包装
func (ss *WrapperServerStream) SendMsg(m any) error {
	return ss.inner.SendMsg(m)
}

// RecvMsg 从客户端接收消息
// 这是对底层 ServerStream RecvMsg 方法的包装
func (ss *WrapperServerStream) RecvMsg(m any) error {
	return ss.inner.RecvMsg(m)
}
