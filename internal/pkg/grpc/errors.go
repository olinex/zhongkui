package grpc

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	// ErrInternal 服务内部错误。
	ErrInternal = status.Errorf(codes.Internal, "internal error")
	// ErrMissingMetadata 请求缺少必要的 gRPC metadata。
	ErrMissingMetadata = status.Errorf(codes.InvalidArgument, "missing metadata")
	// ErrBadMetadata 请求携带的 gRPC metadata 不合法。
	ErrBadMetadata = status.Errorf(codes.Unauthenticated, "bad metadata")
	// ErrInvalidAuthType 鉴权协议类型不受支持。
	ErrInvalidAuthType = status.Errorf(codes.Unauthenticated, "invalid auth type")
	// ErrInvalidToken 令牌格式错误、方法名不匹配、密钥不存在或签名不匹配。
	ErrInvalidToken = status.Errorf(codes.Unauthenticated, "invalid token")
	// ErrNonceDuplicated nonce 重复，疑似重放攻击。
	ErrNonceDuplicated = status.Errorf(codes.Unauthenticated, "nonce duplicated")
	// ErrAccessKeyDisabled 访问密钥已被禁用。
	ErrAccessKeyDisabled = status.Errorf(codes.PermissionDenied, "access key is disabled")
	// ErrAccessKeyExpired 访问密钥已过期。
	ErrAccessKeyExpired = status.Errorf(codes.PermissionDenied, "access key is expired")
)
