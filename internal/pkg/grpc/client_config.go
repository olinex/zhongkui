// Package grpc 提供gRPC客户端配置结构和工具。
// 本文件包含gRPC客户端连接的配置定义，包括TLS、身份验证、保活和服务配置。
package grpc

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
	g "google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// ClientPrefix 是gRPC客户端设置的配置前缀。
// 用于在配置系统中为gRPC客户端配置命名空间。
const ClientPrefix = "grpc.client"

// unaryClientInterceptor 是gRPC客户端的单元拦截器。
// 该拦截器在处理每个单元RPC调用时执行，主要作用是将调用的方法路径存储到上下文(Context)中，
// 以便后续的身份验证或日志记录等中间件可以访问该信息。
//
// 参数：
//   - ctx: 调用上下文
//   - method: 调用的gRPC方法完整路径，格式为"/package.Service/Method"
//   - req: 请求参数
//   - reply: 响应参数
//   - cc: 客户端连接
//   - invoker: 实际的RPC调用函数
//   - opts: 调用选项
//
// 返回：
//   - error: 调用过程中发生的错误
func unaryClientInterceptor(ctx context.Context, method string, req, reply any, cc *g.ClientConn, invoker g.UnaryInvoker, opts ...g.CallOption) error {
	// 将方法路径设置到上下文中，供后续处理使用
	ctx = context.WithValue(ctx, "method", method)
	return invoker(ctx, method, req, reply, cc, opts...)
}

// StreamClientInterceptor 是gRPC客户端的流式拦截器。
// 该拦截器在处理流式RPC调用时执行，主要作用是将流式调用的方法路径存储到上下文中，
// 以便后续的身份验证或日志记录等中间件可以访问该信息。
//
// 参数：
//   - ctx: 调用上下文
//   - desc: 流式调用的描述信息
//   - cc: 客户端连接
//   - method: 调用的gRPC方法完整路径
//   - streamer: 实际的流式调用函数
//   - opts: 调用选项
//
// 返回：
//   - grpc.ClientStream: 客户端流对象
//   - error: 调用过程中发生的错误
func StreamClientInterceptor(ctx context.Context, desc *g.StreamDesc, cc *g.ClientConn, method string, streamer g.Streamer, opts ...g.CallOption) (g.ClientStream, error) {
	// 将方法路径存入上下文，供后续处理使用
	ctx = context.WithValue(ctx, "method", method)

	return streamer(ctx, desc, cc, method, opts...)
}

// TlsClientConfig 定义用于保护gRPC客户端连接的TLS配置。
// 支持内联证书字符串和文件路径两种方式加载证书，用于建立安全的gRPC连接。
type TlsClientConfig struct {
	// Enabled 指示是否为gRPC客户端启用TLS加密。
	// 当为false时，客户端将使用明文连接，不进行加密。
	// 默认为false。
	Enabled bool `mapstructure:"enabled" default:"false" comment:"为 gRPC 客户端连接启用 TLS (ref: https://pkg.go.dev/google.golang.org/grpc/credentials#NewClientTLSFromCert)"`

	// Domain 是TLS服务器名称验证(SNI)的域名。
	// 在建立TLS连接时，客户端会验证服务器证书的Common Name或Subject Alternative Name是否匹配此域名。
	// 如果为空，则不进行服务器名称验证。
	Domain string `mapstructure:"domain" default:"-" comment:"TLS 服务器名称验证（SNI）域名 (ref: https://pkg.go.dev/google.golang.org/grpc/credentials#NewClientTLSFromCert)"`

	// Cert 包含PEM编码的证书字节字符串(内联方式)。
	// 直接将证书内容作为字符串配置，适用于证书内容较短或需要通过配置管理工具分发的情况。
	// 注意：不能与CertFilePath同时使用，两者互斥。
	Cert string `mapstructure:"cert" default:"-" comment:"内联 PEM 编码证书内容，不能与 CertFilePath 同时使用 (ref: https://pkg.go.dev/google.golang.org/grpc/credentials#NewClientTLSFromCert)"`

	// CertFilePath 是包含PEM编码证书的文件路径。
	// 通过文件系统路径加载证书，适用于证书文件较大或需要动态更新的情况。
	// 注意：不能与Cert同时使用，两者互斥。
	CertFilePath string `mapstructure:"certFilePath" default:"-" comment:"PEM 编码证书文件路径，不能与 Cert 同时使用 (ref: https://pkg.go.dev/google.golang.org/grpc/credentials#NewClientTLSFromFile)"`
}

// ToOption 将TlsClientConfig转换为gRPC客户端的TLS连接选项。
// 根据配置的Enabled标志和证书加载方式，生成对应的gRPC DialOption。
//
// 处理逻辑：
// 1. 如果配置为nil或Enabled为false，返回不安全的连接选项(insecure.NewCredentials)
// 2. 如果配置了内联证书字符串(Cert)，从字符串加载证书
// 3. 如果配置了证书文件路径(CertFilePath)，从文件系统加载证书
// 4. 如果既没有Cert也没有CertFilePath，返回无效配置错误
//
// 返回：
//   - option: gRPC连接选项，用于配置客户端的传输层安全设置
//   - err: 转换过程中发生的错误，如证书加载失败或配置无效
func (t *TlsClientConfig) ToOption() (option g.DialOption, err error) {
	// 检查配置是否有效或是否启用TLS
	if t == nil || !t.Enabled {
		// 返回不安全的连接选项(明文传输)
		return g.WithTransportCredentials(insecure.NewCredentials()), nil
	}

	var cert credentials.TransportCredentials

	// 优先处理内联证书字符串方式
	if t.Cert != "" {
		// 创建新的证书池
		pool := x509.NewCertPool()
		// 将PEM编码的证书添加到证书池中
		if !pool.AppendCertsFromPEM([]byte(t.Cert)) {
			err = errors.New("failed to append certs")
			return
		}
		// 从证书池创建客户端TLS凭证
		cert = credentials.NewClientTLSFromCert(pool, t.Domain)
	} else if t.CertFilePath != "" {
		// 从文件路径加载证书
		cert, err = credentials.NewClientTLSFromFile(t.CertFilePath, t.Domain)
		if err != nil {
			return
		}
	} else {
		// 既没有内联证书也没有文件路径，配置无效
		err = errors.New("invalid tls config")
		return
	}

	// 将TLS凭证包装为gRPC连接选项
	option = g.WithTransportCredentials(cert)
	return
}

// AuthClientConfig 定义gRPC客户端的身份验证配置。
// 支持基于访问密钥的身份验证协议，用于向gRPC服务器提供身份凭证。
type AuthClientConfig struct {
	// Enabled 指示是否为gRPC客户端启用身份验证。
	// 当为true时，客户端会在每个RPC调用中包含身份验证信息。
	// 默认为false。
	Enabled bool `mapstructure:"enabled" default:"false" comment:"为 gRPC 客户端启用身份验证 (ref: https://pkg.go.dev/google.golang.org/grpc#WithPerRPCCredentials)"`

	// Authorize 指定使用的身份验证协议类型。
	// 当前支持的协议：
	//   - "accessKeySecret": 基于访问密钥的身份验证
	// 此字段用于确定使用哪种身份验证机制生成凭证。
	Authorize string `mapstructure:"authorize" default:"-" comment:"身份验证协议：accessKeySecret 表示基于访问密钥认证 (ref: https://pkg.go.dev/google.golang.org/grpc#WithPerRPCCredentials)"`

	// AccessKeyID 是访问密钥ID，用于身份验证。
	// 与AccessKeySecret配对使用，作为客户端的身份标识。
	// 在访问密钥身份验证协议中，此ID会包含在请求头中。
	AccessKeyID string `mapstructure:"accessKeyID" default:"-" comment:"访问密钥 ID，用于身份验证 (ref: internal/pkg/grpc/authenticator.go)"`

	// AccessKeySecret 是访问密钥，用于身份验证。
	// 与AccessKeyID配对使用，用于生成身份验证令牌或签名。
	// 注意：此密钥应妥善保管，不应硬编码在代码中或公开暴露。
	AccessKeySecret string `mapstructure:"accessKeySecret" default:"-" comment:"访问密钥，用于身份验证 (ref: internal/pkg/grpc/authenticator.go)"`
}

// ToOption 将AuthClientConfig转换为gRPC客户端的身份验证连接选项。
// 根据配置的Enabled标志和身份验证协议类型，生成对应的gRPC DialOption。
//
// 处理逻辑：
// 1. 如果配置为nil或Enabled为false，返回nil选项(不启用身份验证)
// 2. 根据Authorize字段选择身份验证协议实现
// 3. 创建对应的PerRPCCredentials实现
// 4. 将凭证包装为gRPC连接选项
//
// 支持的协议：
//   - ProtocolAccessKeySecret: 基于访问密钥的身份验证
//
// 返回：
//   - option: gRPC连接选项，用于配置客户端的每RPC凭证
//   - err: 转换过程中发生的错误，如协议不支持或配置无效
func (a *AuthClientConfig) ToOption() (option g.DialOption, err error) {
	// 检查配置是否有效或是否启用身份验证
	if a == nil || !a.Enabled {
		// 返回nil选项，表示不启用身份验证
		return
	}

	var ts credentials.PerRPCCredentials

	// 根据身份验证协议类型选择不同的凭证实现
	switch a.Authorize {
	case ProtocolAccessKeySecret:
		// 创建基于访问密钥的身份验证凭证
		ts = NewAccessKeySecretPerRPCCredentials(a.AccessKeyID, a.AccessKeySecret)
	default:
		// 不支持的协议类型
		err = errors.New("invalid auth protocol")
		return
	}

	// 将凭证包装为gRPC连接选项
	// 使用AuthWrapperPerRPCCredentials包装原始凭证，添加协议类型信息
	option = g.WithPerRPCCredentials(NewAuthWrapperPerRPCCredentials(a.Authorize, ts))
	return
}

// KeepaliveClientConfig 定义gRPC客户端的保活和连接管理配置。
// 这些设置控制客户端如何维护与服务器的连接健康状态，防止连接因空闲或网络问题而断开。
type KeepaliveClientConfig struct {
	// Time 是客户端发送保活ping的时间间隔。
	// 客户端每Time持续时间发送一次保活ping，以检测连接是否仍然有效。
	// 默认为1分钟(1m)。
	Time time.Duration `mapstructure:"time" default:"1m" comment:"客户端保活 ping 发送间隔 (ref: https://pkg.go.dev/google.golang.org/grpc/keepalive#ClientParameters-Time)"`

	// Timeout 是保活ping的超时时间。
	// 如果在Timeout内未收到服务器的ping响应，客户端将认为连接已断开并关闭连接。
	// 默认为20秒(20s)。
	Timeout time.Duration `mapstructure:"timeout" default:"20s" comment:"保活 ping 超时时间，无响应则关闭连接 (ref: https://pkg.go.dev/google.golang.org/grpc/keepalive#ClientParameters-Timeout)"`

	// PermitWithoutStream 指示是否在没有活动流的情况下允许发送保活ping。
	// 当为true时，即使没有活动的RPC流，客户端也会定期发送ping以保持连接活跃。
	// 当为false时，只有在有活动流时才会发送ping。
	// 默认为true。
	PermitWithoutStream bool `mapstructure:"permitWithoutStream" default:"true" comment:"没有活动流时是否允许发送保活 ping (ref: https://pkg.go.dev/google.golang.org/grpc/keepalive#ClientParameters-PermitWithoutStream)"`
}

// ToOption 将KeepaliveClientConfig转换为gRPC客户端的保活连接选项。
// 将配置中的保活参数转换为gRPC的ClientParameters结构，并包装为DialOption。
//
// 返回：
//   - g.DialOption: gRPC连接选项，用于配置客户端的保活参数
func (k *KeepaliveClientConfig) ToOption() g.DialOption {
	// 创建gRPC客户端保活参数
	params := keepalive.ClientParameters{
		Time:                k.Time,                // ping间隔时间
		Timeout:             k.Timeout,             // ping超时时间
		PermitWithoutStream: k.PermitWithoutStream, // 是否允许无流时ping
	}

	// 将保活参数包装为gRPC连接选项
	return g.WithKeepaliveParams(params)
}

// ClientConfig 是gRPC客户端的主要配置结构。
// 将所有配置方面：网络连接、TLS安全、身份验证、保活和服务配置组合成一个全面的配置。
// 该结构体提供了从配置到gRPC连接选项的完整转换能力。
type ClientConfig struct {
	// BaseConfig 嵌入式基础配置，包含主机地址和端口信息。
	// 定义客户端要连接的服务器的网络地址。
	BaseConfig `comment:"基础配置：主机地址和端口"`

	// TLS 配置用于建立安全的gRPC连接。
	// 控制是否以及如何对客户端与服务器之间的通信进行加密。
	// 包括证书加载方式和服务器名称验证设置。
	TLS TlsClientConfig `mapstructure:"tls" comment:"TLS 安全连接配置 (ref: https://pkg.go.dev/google.golang.org/grpc/credentials#NewClientTLSFromCert)"`

	// Auth 配置用于向服务器提供身份验证凭证。
	// 定义客户端使用的身份验证协议和凭证信息。
	// 支持基于访问密钥的身份验证机制。
	Auth AuthClientConfig `mapstructure:"auth" comment:"身份验证配置 (ref: https://pkg.go.dev/google.golang.org/grpc#WithPerRPCCredentials)"`

	// Keepalive 配置用于连接管理和健康监控。
	// 控制客户端如何维护与服务器的连接，防止因空闲或网络问题而断开。
	// 包括ping间隔、超时时间和无流时的ping策略。
	Keepalive KeepaliveClientConfig `mapstructure:"keepalive" comment:"保活连接管理配置 (ref: https://pkg.go.dev/google.golang.org/grpc/keepalive#ClientParameters)"`

	// ServiceConfig 是gRPC服务配置的JSON映射。
	// 用于配置负载均衡策略、重试策略、超时设置等高级gRPC功能。
	// 该配置会传递给gRPC客户端，影响RPC调用的行为。
	// 例如：可以配置重试次数、超时时间、负载均衡算法等。
	ServiceConfig map[string]any `mapstructure:"serviceConfig" comment:"gRPC 服务配置 JSON，用于负载均衡和重试策略 (ref: https://pkg.go.dev/google.golang.org/grpc#WithDefaultServiceConfig)"`
}

// ToOptions 将完整的客户端配置转换为gRPC连接选项和监听地址。
// 这是从ClientConfig配置gRPC客户端的主要入口点，将所有配置组件(TLS、身份验证、保活、服务配置)
// 编译为gRPC DialOptions，并生成连接地址字符串。
//
// 处理顺序：
// 1. 生成连接地址字符串(host:port格式)
// 2. 转换TLS配置为连接选项
// 3. 转换身份验证配置为连接选项(如果启用身份验证，同时添加拦截器)
// 4. 转换保活配置为连接选项
// 5. 转换服务配置为连接选项(JSON格式)
//
// 返回：
//   - lis: 连接地址字符串，格式为"host:port"
//   - options: gRPC客户端连接选项切片，包含所有配置功能
//   - err: 配置转换过程中发生的错误，如TLS证书加载失败或JSON序列化错误
func (c *ClientConfig) ToOptions() (lis string, options []g.DialOption, err error) {
	// 检查配置是否有效
	if c == nil {
		return
	}

	// 生成连接地址字符串，格式为"host:port"
	lis = fmt.Sprintf("%s:%d", c.Host, c.Port)

	var opt g.DialOption

	// 1. 转换TLS配置
	opt, err = c.TLS.ToOption()
	if err != nil {
		return
	}
	if opt != nil {
		options = append(options, opt)
	}

	// 2. 转换身份验证配置
	opt, err = c.Auth.ToOption()
	if err != nil {
		return
	}
	if opt != nil {
		// 当启用身份验证时，同时添加单元和流式拦截器
		// 拦截器会将方法路径存储到上下文中，供身份验证使用
		options = append(
			options,
			g.WithUnaryInterceptor(unaryClientInterceptor),   // 添加单元拦截器
			g.WithStreamInterceptor(StreamClientInterceptor), // 添加流式拦截器
			opt, // 添加身份验证凭证选项
		)
	}

	// 3. 转换保活配置
	opt = c.Keepalive.ToOption()
	if opt != nil {
		options = append(options, opt)
	}

	// 4. 转换服务配置
	if c.ServiceConfig != nil {
		var b []byte
		// 将服务配置映射序列化为JSON字符串
		b, err = json.Marshal(c.ServiceConfig)
		if err != nil {
			return
		}
		// 将JSON字符串包装为gRPC服务配置选项
		options = append(options, g.WithDefaultServiceConfig(string(b)))
	}

	return
}

// DescribeClientConfig 生成gRPC客户端设置的配置描述。
// 用于配置选项的文档、验证或UI生成，可以生成特定配置名称的描述或所有客户端配置的描述。
//
// 参数：
//   - names: 要描述的特定配置名称的可选列表。
//     如果为空或未提供，描述所有客户端配置。
//     如果提供名称，只描述指定的配置，格式为"grpc.client.{name1,name2,...}"。
//
// 返回：
//   - config.ConfigDescriptions: 配置选项的结构化描述，包含字段名称、类型、默认值和说明
func DescribeClientConfig(names ...string) config.ConfigDescriptions {
	// 构建配置前缀，支持描述特定名称或所有配置
	prefix := fmt.Sprintf(`%s.{%s}`, ClientPrefix, strings.Join(names, ","))
	return config.DescribeConfig[ClientConfig](prefix)
}

// ExtractClientConfig 从Viper配置中提取指定名称的gRPC客户端配置。
// 使用配置系统的提取功能，将Viper中的配置数据反序列化为ClientConfig结构体。
//
// 参数：
//   - v: Viper配置实例，包含已加载的配置数据
//   - name: 要提取的配置名称，对应配置中的"grpc.client.{name}"路径
//
// 返回：
//   - *ClientConfig: 提取的客户端配置实例
//   - error: 提取过程中发生的错误，如配置不存在或反序列化失败
func ExtractClientConfig(viper *viper.Viper, validate *validator.Validate, name string) (*ClientConfig, error) {
	// 构建完整的配置路径
	path := fmt.Sprintf(`%s.%s`, ClientPrefix, name)
	return config.ExtractConfig[ClientConfig](viper, validate, path)
}
