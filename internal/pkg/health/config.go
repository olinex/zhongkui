// Package health 提供项目全局统一的服务健康探针机制。
// 它面向 k8s 部署编排，暴露标准 liveness / readiness / startup HTTP 探针端点，
// 并可选地经 etcd 服务目录上报各服务运行状态、复用既有 OpenTelemetry 全局管道
// 发布探针状态指标与追踪，避免重复实现埋点与导出能力。
//
// Package health provides a project-wide unified service health-probe mechanism
// targeting k8s deployment orchestration. It exposes standard liveness/readiness/
// startup HTTP probe endpoints, optionally reports per-service running state into an
// etcd service directory, and reuses the existing OpenTelemetry global pipeline to
// publish probe-state metrics and tracing, avoiding duplicated instrumentation and
// export infrastructure.
package health

import (
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
	"zhongkui.com/zhongkui/internal/pkg/config"
)

// Prefix 是健康探针配置在 Viper 中的键前缀：
// 所有健康探针配置项均位于 "health" 前缀之下（如 health.host / health.registry.ttl）。
const Prefix = "health"

// registryEnabledDefault 常量保留 RegistryConfig 默认语义的可读性；
// 实际默认值由 default tag 统一承载。
// (config.go 进一步以 RegistryConfig 承载 etcd 上报相关配置。)

// RegistryConfig 定义 etcd 服务状态注册的相关配置。
// 控制健康探针服务是否将自身运行状态发布到 etcd 服务目录，供 web admin 聚合展示。
// RegistryConfig controls the etcd service-state registration settings: whether the
// health probe service publishes its own running state into the etcd service
// directory for the web admin to aggregate and display.
type RegistryConfig struct {
	// Enabled 是否启用 etcd 服务状态注册
	// Enabled whether etcd service-state registration is enabled
	Enabled bool `mapstructure:"enabled" default:"true" comment:"是否将本服务运行状态上报到 etcd 服务目录"`
	// KeyPrefix etcd 服务目录键前缀（相对全局前缀，须以 '/' 结尾）。
	// 完整键为 "{GlobalPrefix}/{KeyPrefix}/{serviceName}/{instanceID}"。
	// KeyPrefix the etcd service-directory key prefix (relative to the global prefix,
	// must end with '/'). The full key is
	// "{GlobalPrefix}/{KeyPrefix}/{serviceName}/{instanceID}".
	KeyPrefix string `mapstructure:"keyPrefix" default:"services/" comment:"etcd 服务目录键前缀，相对全局前缀，须以 / 结尾"`
	// TTL 服务注册键的租赁 TTL，超过未续约即过期消失
	// TTL the lease TTL of the service-registration key; the key expires if not renewed
	TTL time.Duration `mapstructure:"ttl" default:"15s" comment:"服务注册键租赁 TTL，超时未续约即过期"`
}

// Config 是健康探针的顶层配置：独立 HTTP 探针服务监听地址 + 就绪超时 + etcd 上报。
// Config is the top-level health-probe config: independent HTTP probe server binding,
// readiness timeout and etcd reporting.
type Config struct {
	// Enabled 是否启用健康探针 HTTP 服务
	// Enabled whether the health probe HTTP service is enabled
	Enabled bool `mapstructure:"enabled" default:"true" comment:"是否启用健康探针 HTTP 服务"`
	// Host 健康探针服务监听地址
	// Host the health probe service listen address
	Host string `mapstructure:"host" default:"0.0.0.0" comment:"健康探针服务监听地址"`
	// Port 健康探针服务监听端口
	// Port the health probe service listen port
	Port int `mapstructure:"port" default:"19090" comment:"健康探针服务监听端口"`
	// ReadyTimeout 就绪探测单个依赖的独立超时
	// ReadyTimeout the per-dependency independent timeout for readiness probes
	ReadyTimeout time.Duration `mapstructure:"readyTimeout" default:"3s" comment:"就绪探测单个依赖的独立超时"`
	// Registry etcd 服务状态注册相关配置
	// Registry the etcd service-state registration settings
	Registry RegistryConfig `mapstructure:"registry" comment:"etcd 服务状态注册配置"`
}

// ExtractConfig 从 Viper 实例中提取并校验健康探针配置。
// ExtractConfig extracts and validates the health probe config from a Viper instance.
func ExtractConfig(v *viper.Viper, validate *validator.Validate) (*Config, error) {
	return config.ExtractConfig[Config](v, validate, Prefix)
}

// DescribeConfig 返回健康探针配置的元信息描述。
// DescribeConfig returns the health probe config metadata descriptions.
func DescribeConfig() config.ConfigDescriptions {
	return config.DescribeConfig[Config](Prefix)
}