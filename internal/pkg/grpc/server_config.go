// Package grpc 提供gRPC服务器配置结构和工具。
// 本文件包含gRPC服务器设置的配置定义，包括TLS、身份验证、保活和健康检查配置。
package grpc

import (
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/spf13/viper"
	g "google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// ServerPrefix 是gRPC服务器设置的配置前缀。
// 用于在配置系统中为gRPC服务器配置命名空间。
const ServerPrefix = "grpc.server"

// BaseConfig 定义gRPC服务器的基本网络配置。
// 指定服务器监听传入连接的主机和端口。
type BaseConfig struct {
	// Host 是gRPC服务器绑定的网络地址。
	// 默认为"127.0.0.1"用于本地绑定。
	// 对于服务器：绑定地址，对于客户端：远程服务器地址。
	Host string `mapstructure:"host" default:"127.0.0.1" comment:"gRPC 服务器/客户端主机绑定地址或远程服务器地址 (ref: https://pkg.go.dev/google.golang.org/grpc#Server)"`

	// Port 是gRPC服务器监听的TCP端口号。
	// 默认为8081。
	Port int `mapstructure:"port" default:"8081" comment:"gRPC 服务器/客户端端口号 (ref: https://pkg.go.dev/google.golang.org/grpc#Server)"`
}

// TlsServerConfig 定义用于保护gRPC服务器连接的TLS配置。
// 支持内联证书/密钥字符串和文件路径两种方式。
type TlsServerConfig struct {
	// Enabled 指示是否为gRPC服务器启用TLS加密。
	// 当为false时，服务器将使用明文连接。
	Enabled bool `mapstructure:"enabled" default:"false" comment:"为 gRPC 服务器启用 TLS (ref: https://pkg.go.dev/google.golang.org/grpc/credentials#NewServerTLSFromCert)"`

	// Cert 包含PEM编码的证书字节字符串。
	// 不能与CertFilePath同时使用。
	Cert string `mapstructure:"cert" default:"-" comment:"内联 PEM 编码证书内容，不能与 CertFilePath 同时使用 (ref: https://pkg.go.dev/google.golang.org/grpc/credentials#NewServerTLSFromCert)"`

	// Key 包含PEM编码的私钥字节字符串。
	// 不能与KeyFilePath同时使用。
	Key string `mapstructure:"key" default:"-" comment:"内联 PEM 编码私钥内容，不能与 KeyFilePath 同时使用 (ref: https://pkg.go.dev/google.golang.org/grpc/credentials#NewServerTLSFromCert)"`

	// CertFilePath 是包含PEM编码证书的文件路径。
	// 不能与Cert同时使用。
	CertFilePath string `mapstructure:"certFilePath" default:"-" comment:"PEM 编码证书文件路径，不能与 Cert 同时使用 (ref: https://pkg.go.dev/google.golang.org/grpc/credentials#NewServerTLSFromCert)"`

	// KeyFilePath 是包含PEM编码私钥的文件路径。
	// 不能与Key同时使用。
	KeyFilePath string `mapstructure:"keyFilePath" default:"-" comment:"PEM 编码私钥文件路径，不能与 Key 同时使用 (ref: https://pkg.go.dev/google.golang.org/grpc/credentials#NewServerTLSFromCert)"`
}

// ToOption 将TLS配置转换为gRPC ServerOption。
// 返回一个credentials.ServerOption，可用于在gRPC服务器上配置TLS。
// 如果TLS被禁用或配置为nil，则返回nil, nil。
//
// 该方法支持两种证书/密钥指定模式：
// 1. 内联字符串：Cert和Key字段包含PEM编码的数据
// 2. 文件路径：CertFilePath和KeyFilePath指向证书/密钥文件
//
// 返回：
//   - g.ServerOption: gRPC服务器的TLS凭据选项
//   - error: 证书加载或验证期间遇到的任何错误
func (t *TlsServerConfig) ToOption() (option g.ServerOption, err error) {
	if t == nil || !t.Enabled {
		return
	}
	var cert tls.Certificate
	if t.Cert != "" && t.Key != "" {
		cert, err = tls.X509KeyPair([]byte(t.Cert), []byte(t.Key))
		if err != nil {
			return
		}
	} else if t.CertFilePath != "" && t.KeyFilePath != "" {
		cert, err = tls.LoadX509KeyPair(t.CertFilePath, t.KeyFilePath)
		if err != nil {
			return
		}
	} else {
		err = errors.Newf("invalid tls config")
		return
	}
	option = g.Creds(credentials.NewServerTLSFromCert(&cert))
	return
}

// AuthServerConfig 定义gRPC服务器的身份验证配置。
// 当前支持基于访问密钥的身份验证。
type AuthServerConfig struct {
	// AccessKeyEnabled 指示是否启用访问密钥身份验证。
	// 当为true时，服务器将要求请求提供有效的访问密钥令牌。
	AccessKeyEnabled bool `mapstructure:"accessKeyEnabled" default:"false" comment:"启用访问密钥身份验证 (ref: https://pkg.go.dev/google.golang.org/grpc#ServerOption)"`
}

// ToOptions 将身份验证配置转换为gRPC拦截器选项。
// 创建身份验证路由器并注册提供的验证器，
// 然后返回强制执行身份验证的单元和流拦截器。
//
// 参数：
//   - authenticators: 验证器名称到Authenticator实现的地图
//
// 返回：
//   - g.ServerOption: 用于身份验证的单元拦截器
//   - g.ServerOption: 用于身份验证的流拦截器
//   - error: 路由器注册期间遇到的任何错误
func (a *AuthServerConfig) ToOptions(authenticators map[string]Authenticator) (g.ServerOption, g.ServerOption, error) {
	router := NewAuthRouter(a)
	if err := router.Register(authenticators); err != nil {
		return nil, nil, errors.WithStack(err)
	}
	return g.UnaryInterceptor(router.authUnary), g.StreamInterceptor(router.authStream), nil
}

// KeepaliveServerConfig 定义gRPC服务器的保活和连接管理设置。
// 这些设置控制连接健康检查、超时和生命周期管理。
type KeepaliveServerConfig struct {
	// Time 是服务器ping间隔。服务器每Time持续时间发送保活ping。
	// 默认为30秒。
	Time time.Duration `mapstructure:"time" default:"30s" comment:"服务器 ping 间隔，每隔 Time 时长发送保活 ping (ref: https://pkg.go.dev/google.golang.org/grpc/keepalive#ServerParameters-Time)"`

	// Timeout 是保活ping的超时时间。如果在Timeout内未收到ping响应，
	// 连接将被关闭。
	// 默认为5秒。
	Timeout time.Duration `mapstructure:"timeout" default:"5s" comment:"保活 ping 超时时间，无响应则关闭连接 (ref: https://pkg.go.dev/google.golang.org/grpc/keepalive#ServerParameters-Timeout)"`

	// MinTime 是客户端发送保活ping前必须等待的最短时间。
	// 这是一个强制执行策略，防止客户端过于频繁地发送ping。
	// 默认为5秒。
	MinTime time.Duration `mapstructure:"minTime" default:"5s" comment:"客户端发送保活 ping 前必须等待的最短时间 (ref: https://pkg.go.dev/google.golang.org/grpc/keepalive#EnforcementPolicy-MinTime)"`

	// MaxConnectionAge 是连接的最大年龄。超过此值的连接将被关闭。
	// 值为0表示禁用此功能。
	// 默认为0（禁用）。
	MaxConnectionAge time.Duration `mapstructure:"maxConnectionAge" default:"0s" comment:"连接最大年龄，超过后关闭连接，0 表示禁用 (ref: https://pkg.go.dev/google.golang.org/grpc/keepalive#ServerParameters-MaxConnectionAge)"`

	// MaxConnectionAgeGrace 是MaxConnectionAge之后强制关闭连接之前的宽限期。
	// 这允许现有的RPC完成。
	// 默认为5秒。
	MaxConnectionAgeGrace time.Duration `mapstructure:"maxConnectionAgeGrace" default:"5s" comment:"MaxConnectionAge 之后强制关闭连接前的宽限期 (ref: https://pkg.go.dev/google.golang.org/grpc/keepalive#ServerParameters-MaxConnectionAgeGrace)"`

	// MaxConnectionIdle 是连接的最大空闲时间。空闲时间超过此值的连接将被关闭。
	// 值为0表示禁用此功能。
	// 默认为5分钟。
	MaxConnectionIdle time.Duration `mapstructure:"maxConnectionIdle" default:"5m" comment:"连接最大空闲时间，超过后关闭，0 表示禁用 (ref: https://pkg.go.dev/google.golang.org/grpc/keepalive#ServerParameters-MaxConnectionIdle)"`

	// PermitWithoutStream 指示当连接上没有活动流时是否允许保活ping。
	// 默认为true。
	PermitWithoutStream bool `mapstructure:"permitWithoutStream" default:"true" comment:"没有活动流时是否允许保活 ping (ref: https://pkg.go.dev/google.golang.org/grpc/keepalive#EnforcementPolicy-PermitWithoutStream)"`
}

// ToOptions 将保活配置转换为gRPC ServerOptions。
// 创建两个选项：一个用于服务器参数，一个用于强制执行策略。
//
// 返回：
//   - g.ServerOption: 保活参数选项（时间、超时、连接年龄/空闲设置）
//   - g.ServerOption: 强制执行策略选项（最短时间、无流时允许）
func (k *KeepaliveServerConfig) ToOptions() (g.ServerOption, g.ServerOption) {
	params := keepalive.ServerParameters{
		Time:                  k.Time,
		Timeout:               k.Timeout,
		MaxConnectionAge:      k.MaxConnectionAge,
		MaxConnectionAgeGrace: k.MaxConnectionAgeGrace,
		MaxConnectionIdle:     k.MaxConnectionIdle,
	}

	policy := keepalive.EnforcementPolicy{
		MinTime:             k.MinTime,
		PermitWithoutStream: k.PermitWithoutStream,
	}

	return g.KeepaliveParams(params), g.KeepaliveEnforcementPolicy(policy)
}

// HealthServerConfig 定义gRPC服务器的健康检查服务配置。
// 这启用gRPC健康检查协议，允许外部系统监控服务器健康状态。
type HealthServerConfig struct {
	// Enabled 指示是否启用gRPC健康检查服务。
	// 默认为true。
	Enabled bool `mapstructure:"enabled" default:"true" comment:"启用 gRPC 健康检查服务 (ref: https://github.com/grpc/grpc-go/blob/master/health/server.go)"`

	// ServiceName 是用于健康检查的服务名称。
	// 在检查特定服务健康状态时使用。
	// 默认为"healthcheck"。
	ServiceName string `mapstructure:"serviceName" default:"healthcheck" comment:"健康检查的服务名称 (ref: https://github.com/grpc/grpc-go/blob/master/health/server.go)"`
}

// ServerConfig 是gRPC服务器的主要配置结构。
// 将所有配置方面：网络、TLS、身份验证、保活和健康检查组合成一个全面的配置。
type ServerConfig struct {
	BaseConfig `comment:"基础配置：主机地址和端口"` // 嵌入式基础配置（主机和端口）

	// TLS配置用于安全连接。
	// 控制是否以及如何将TLS加密应用于gRPC连接。
	TLS TlsServerConfig `mapstructure:"tls" comment:"TLS 安全连接配置 (ref: https://pkg.go.dev/google.golang.org/grpc/credentials#NewServerTLSFromCert)"`

	// 身份验证配置用于保护对gRPC服务器的访问。
	// 定义传入请求的身份验证方法和要求。
	Auth AuthServerConfig `mapstructure:"auth" comment:"gRPC 服务器身份验证配置 (ref: https://pkg.go.dev/google.golang.org/grpc#ServerOption)"`

	// 保活配置用于连接管理和健康监控。
	// 控制连接超时、ping间隔和连接生命周期。
	Keepalive KeepaliveServerConfig `mapstructure:"keepalive" comment:"保活连接管理配置 (ref: https://pkg.go.dev/google.golang.org/grpc/keepalive#ServerParameters)"`

	// gRPC健康协议的健康检查配置。
	// 启用对gRPC服务器的外部健康监控。
	Health HealthServerConfig `mapstructure:"health" comment:"gRPC 健康协议的健康检查配置 (ref: https://github.com/grpc/grpc-go/blob/master/health/server.go)"`
}

// ToOptions 将完整的服务器配置转换为gRPC服务器选项。
// 创建网络监听器并将所有配置组件（TLS、身份验证、保活）编译为gRPC ServerOptions。
//
// 这是从ServerConfig配置gRPC服务器的主要入口点。
//
// 参数：
//   - authenticators: 验证器名称到Authenticator实现的地图
//
// 返回：
//   - net.Listener: 配置的主机和端口的TCP监听器
//   - []g.ServerOption: 所有配置功能的gRPC服务器选项切片
//   - error: 配置转换或监听器创建期间遇到的任何错误
func (c *ServerConfig) ToOptions(authenticators map[string]Authenticator) (lis net.Listener, options []g.ServerOption, err error) {
	if c == nil {
		return
	}
	lis, err = net.Listen("tcp", fmt.Sprintf("%s:%d", c.Host, c.Port))
	if err != nil {
		return
	}
	tls, err := c.TLS.ToOption()
	if err != nil {
		return
	}
	if tls != nil {
		options = append(options, tls)
	}
	unaryInterceptor, streamInterceptor, err := c.Auth.ToOptions(authenticators)
	if err != nil {
		return
	}
	params, policy := c.Keepalive.ToOptions()
	options = append(
		options, params, policy, unaryInterceptor, streamInterceptor)
	return
}

// DescribeServerConfig 生成gRPC服务器设置的配置描述。
// 用于配置选项的文档、验证或UI生成。
//
// 参数：
//   - names: 要描述的特定配置名称的可选列表。
//     如果为空，描述所有服务器配置。
//
// 返回：
//   - config.ConfigDescriptions: 配置选项的结构化描述
func DescribeServerConfig(names ...string) config.ConfigDescriptions {
	return config.DescribeConfig[ServerConfig](fmt.Sprintf(`%s.{%s}`, ServerPrefix, strings.Join(names, ",")))
}

// ExtractServerConfig 从Viper配置源提取gRPC服务器配置。
// 此函数从Viper实例中的指定路径读取配置，
// 并将其解组到ServerConfig结构体中。
//
// 参数：
//   - v: 包含配置的Viper实例
//   - name: 要提取的特定服务器配置的名称
//
// 返回：
//   - *ServerConfig: 指向提取的服务器配置的指针
//   - error: 配置提取或解组期间遇到的任何错误
func ExtractServerConfig(v *viper.Viper, name string) (*ServerConfig, error) {
	return config.ExtractConfig[ServerConfig](v, fmt.Sprintf(`%s.%s`, ServerPrefix, name))
}
