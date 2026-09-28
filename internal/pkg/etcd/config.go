// Package etcd 提供了 etcd 客户端的配置管理与连接创建功能。
// 该包封装了 etcd client/v3 客户端的配置结构，支持 TLS 安全连接，
// 并提供了配置描述和配置提取的便捷方法，方便从 viper 配置中加载 etcd 相关配置。
package etcd

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"os"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
	clientv3 "go.etcd.io/etcd/client/v3"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

// PREFIX 是 etcd 配置在 viper 中的顶级键前缀，所有 etcd 相关配置均以此前缀开始。
const Prefix = "etcd"

// TLSConfig 定义了 etcd 客户端的 TLS 安全连接配置。
// 支持两种方式提供 CA 证书：直接内联 PEM 证书内容（Cert）或指定证书文件路径（CertFilePath）。
// 两者互斥，不能同时使用。
//
// 字段说明:
//   - Enabled: 是否启用 TLS 连接，默认为 false
//   - Domain: TLS 服务器名称，用于证书验证时的 ServerName
//   - Cert: 内联的 PEM 编码证书内容，与 CertFilePath 互斥
//   - CertFilePath: PEM 编码证书文件路径，与 Cert 互斥
type TLSConfig struct {
	Enabled      bool   `mapstructure:"enabled" default:"false" comment:"为 etcd 客户端连接启用 TLS (ref: https://pkg.go.dev/go.etcd.io/etcd/client/v3#Config-TLS)"`
	Domain       string `mapstructure:"domain" default:"-" comment:"TLS 服务器名称，用于证书验证 (ref: https://pkg.go.dev/go.etcd.io/etcd/client/v3#Config-TLS)"`
	Cert         string `mapstructure:"cert" default:"-" comment:"内联 PEM 编码证书内容，不能与 CertFilePath 同时使用 (ref: https://pkg.go.dev/go.etcd.io/etcd/client/v3#Config-TLS)"`
	CertFilePath string `mapstructure:"certFilePath" default:"-" comment:"PEM 编码证书文件路径，不能与 Cert 同时使用 (ref: https://pkg.go.dev/go.etcd.io/etcd/client/v3#Config-TLS)"`
}

// ToStandardConfig 将 TLSConfig 转换为标准库的 *tls.Config 对象。
// 该方法根据配置中的证书来源（内联或文件）构建 TLS 配置，
// 并将 CA 证书添加到根证书池中用于服务器证书验证。
//
// 转换规则:
//   - 当接收者为 nil 或 Enabled 为 false 时，返回 nil 配置且无错误
//   - 当 Cert 不为空时，直接使用 Cert 作为 PEM 证书内容
//   - 当 CertFilePath 不为空时，从文件读取 PEM 证书内容
//   - 当两者均为空时，返回 "invalid tls config" 错误
//   - 当 PEM 证书内容无法被解析时，返回 "failed to append certs" 错误
//
// 参数: 无（使用接收者自身的字段）
//
// 返回值:
//   - config: 转换后的标准 TLS 配置对象，如果 TLS 未启用则为 nil
//   - err: 转换过程中的错误，如证书配置无效或证书内容无法解析
func (t *TLSConfig) ToStandardConfig() (config *tls.Config, err error) {
	if t == nil || !t.Enabled {
		return
	}
	var cert []byte
	if t.Cert != "" {
		cert = []byte(t.Cert)
	} else if t.CertFilePath != "" {
		cert, err = os.ReadFile(t.CertFilePath)
		if err != nil {
			return
		}
	} else {
		err = errors.New("invalid tls config")
		return
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(cert)) {
		err = errors.New("failed to append certs")
		return
	}
	config = &tls.Config{ServerName: t.Domain, RootCAs: pool}
	return
}

// Config 定义了 etcd 客户端的完整配置结构。
// 该配置映射到 etcd client/v3 的 clientv3.Config，并可通过 ToEtcdConfig 方法转换。
// 所有字段均支持通过 viper 从配置文件中自动加载（使用 mapstructure 标签），
// 并具有合理的默认值（使用 default 标签）。
//
// 字段说明:
//   - Endpoints: etcd 服务器地址列表，默认为 ["localhost:2379"]
//   - Username: etcd 认证用户名
//   - Password: etcd 认证密码
//   - AutoSyncInterval: 集群成员同步间隔，0 表示禁用
//   - DialTimeout: 拨号超时时间，默认 30s
//   - DialKeepAliveTime: 保活探测发送间隔，默认 60s
//   - DialKeepAliveTimeout: 保活探测超时时间，默认 30s
//   - MaxCallSendMsgSize: 客户端发送消息最大字节数，默认 2MB
//   - MaxCallRecvMsgSize: 客户端接收消息最大字节数，默认 4GB
//   - RejectOldCluster: 是否拒绝连接旧版本集群，默认 true
//   - TLS: TLS 安全连接配置
type Config struct {
	Endpoints            []string      `mapstructure:"endpoints" default:"[\"localhost:2379\"]" comment:"etcd 服务器地址列表 (ref: https://pkg.go.dev/go.etcd.io/etcd/client/v3#Config-Endpoints)"`
	Username             string        `mapstructure:"username" default:"-" comment:"etcd 认证用户名 (ref: https://pkg.go.dev/go.etcd.io/etcd/client/v3#Config-Username)"`
	Password             string        `mapstructure:"password" default:"-" comment:"etcd 认证密码 (ref: https://pkg.go.dev/go.etcd.io/etcd/client/v3#Config-Password)"`
	AutoSyncInterval     time.Duration `mapstructure:"autoSyncInterval" default:"0" comment:"集群成员同步间隔，0 表示禁用 (ref: https://pkg.go.dev/go.etcd.io/etcd/client/v3#Config-AutoSyncInterval)"`
	DialTimeout          time.Duration `mapstructure:"dialTimeout" default:"30s" comment:"etcd 客户端拨号超时时间 (ref: https://pkg.go.dev/go.etcd.io/etcd/client/v3#Config-DialTimeout)"`
	DialKeepAliveTime    time.Duration `mapstructure:"dialKeepAliveTime" default:"60s" comment:"保活 ping 的发送频率 (ref: https://pkg.go.dev/go.etcd.io/etcd/client/v3#Config-DialKeepAliveTime)"`
	DialKeepAliveTimeout time.Duration `mapstructure:"dialKeepAliveTimeout" default:"30s" comment:"保活 ping 的超时时间 (ref: https://pkg.go.dev/go.etcd.io/etcd/client/v3#Config-DialKeepAliveTimeout)"`
	MaxCallSendMsgSize   int           `mapstructure:"maxCallSendMsgSize" default:"2097152" comment:"客户端向服务器发送消息的最大字节数 (ref: https://pkg.go.dev/go.etcd.io/etcd/client/v3#Config-MaxCallSendMsgSize)"`
	MaxCallRecvMsgSize   int           `mapstructure:"maxCallRecvMsgSize" default:"4294967296" comment:"客户端从服务器接收消息的最大字节数 (ref: https://pkg.go.dev/go.etcd.io/etcd/client/v3#Config-MaxCallRecvMsgSize)"`
	RejectOldCluster     bool          `mapstructure:"rejectOldCluster" default:"true" comment:"拒绝来自集群版本过旧的客户端请求 (ref: https://pkg.go.dev/go.etcd.io/etcd/client/v3#Config-RejectOldCluster)"`
	TLS                  TLSConfig     `mapstructure:"tls" comment:"TLS 安全连接配置 (ref: https://pkg.go.dev/go.etcd.io/etcd/client/v3#Config-TLS)"`
}

// ToEtcdConfig 将 Config 转换为 etcd client/v3 的 clientv3.Config 对象。
// 该方法是创建 etcd 客户端前的关键步骤，将内部配置结构转换为 etcd SDK 可识别的配置格式。
//
// 转换逻辑:
//   - 当接收者为 nil 时，返回空的 clientv3.Config 且无错误
//   - 先调用 TLS.ToStandardConfig() 转换 TLS 配置，若失败则返回错误
//   - 将所有字段映射到 clientv3.Config 对应字段
//   - logger 通过 Desugar() 转换为 zap 原生 Logger 供 etcd SDK 使用
//
// 参数:
//   - ctx: 上下文对象，用于控制 etcd 客户端的生命周期和取消操作
//   - logger: 日志记录器，会通过 Desugar() 转换为 zap.Logger 供 etcd SDK 内部使用
//
// 返回值:
//   - cfg: 转换后的 etcd 客户端配置对象
//   - err: 转换过程中的错误，主要为 TLS 配置转换失败
func (c *Config) ToEtcdConfig(ctx context.Context, logger *logging.Logger) (cfg clientv3.Config, err error) {
	if c == nil {
		return
	}
	tlsCfg, err := c.TLS.ToStandardConfig()
	if err != nil {
		return
	}
	cfg = clientv3.Config{
		Endpoints:            c.Endpoints,
		Username:             c.Username,
		Password:             c.Password,
		AutoSyncInterval:     c.AutoSyncInterval,
		DialKeepAliveTime:    c.DialKeepAliveTime,
		DialKeepAliveTimeout: c.DialKeepAliveTimeout,
		MaxCallSendMsgSize:   c.MaxCallSendMsgSize,
		MaxCallRecvMsgSize:   c.MaxCallRecvMsgSize,
		RejectOldCluster:     c.RejectOldCluster,
		TLS:                  tlsCfg,
		Context:              ctx,
		DialTimeout:          c.DialTimeout,
		Logger:               logger.Desugar(),
	}
	return
}

// DescribeConfig 返回 etcd 配置项的描述信息，用于生成配置文档。
func DescribeConfig() config.ConfigDescriptions {
	return config.DescribeConfig[Config](Prefix)
}

// ExtractConfig 从 viper 实例中提取并校验 etcd 配置。
func ExtractConfig(viper *viper.Viper, validate *validator.Validate) (*Config, error) {
	return config.ExtractConfig[Config](viper, validate, Prefix)
}
