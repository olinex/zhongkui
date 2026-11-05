package grpc

const (
	// ProtocolAccessKeySecret 基于访问密钥的身份验证协议标识，作为 authorize metadata 的取值。
	ProtocolAccessKeySecret = "accesskeySecret"
	// MetadataKeyAuthorization 承载客户端签名凭证的 gRPC metadata 键。
	MetadataKeyAuthorization = "authorization"
	// MetadataKeyAuthorize 指定鉴权协议类型的 gRPC metadata 键。
	MetadataKeyAuthorize = "authorize"
)
