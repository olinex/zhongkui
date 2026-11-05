// Package otel（本文件）定义 OpenTelemetry 的环境变量配置项、默认值及
// provider / 协议 / 采样器取值常量。
package otel

import (
	"github.com/creasty/defaults"
	"github.com/kelseyhightower/envconfig"
)

// OpenTelemetry 相关常量：环境变量前缀、OTLP 传输协议与 provider 取值、采样器类型。
const (
	// Prefix 环境变量统一前缀。
	Prefix = "OTEL"
	// Http 基于 HTTP 的 OTLP 传输协议。
	Http = "http"
	// Grpc 基于 gRPC 的 OTLP 传输协议。
	Grpc = "grpc"
	// ProviderOtlp 使用 OTLP 导出器。
	ProviderOtlp = "otlp"
	// ProviderStdout 使用标准输出导出器。
	ProviderStdout = "stdout"
	// SamplerAlwaysOn 始终采样。
	SamplerAlwaysOn = "always_on"
	// SamplerHead 头部采样。
	SamplerHead = "head"
	// SamplerTail 尾部采样。
	SamplerTail = "tail"
)

var (
	// Env 全局 OpenTelemetry 配置，由环境变量与默认值初始化。
	Env OtelEnv
)

// OtelEnv 描述 OpenTelemetry 的运行配置项。
type OtelEnv struct {
	// Enabled 是否启用 OpenTelemetry。
	Enabled bool `envconfig:"ENABLED" default:"false"`
	// Provider 导出器类型，取值为 otlp 或 stdout。
	Provider string `envconfig:"PROVIDER" default:"otlp"`
	// OtlpProtocol OTLP 传输协议，取值为 grpc 或 http。
	OtlpProtocol string `envconfig:"OTLP_PROTOCOL" default:"grpc"`
	// SamplerType 采样器类型，取值为 always_on、head 或 tail。
	SamplerType string `envconfig:"SAMPLER_TYPE" default:"always_on"`
	// SamplerRatio 采样比例，取值范围 0.0 ~ 1.0。
	SamplerRatio float64 `envconfig:"SAMPLER_RATIO" default:"1.0"`
	// TailOnError 尾部采样时是否对出错的链路保留采样。
	TailOnError bool `envconfig:"TAIL_ON_ERROR" default:"true"`
}

func init() {
	if err := defaults.Set(&Env); err != nil {
		panic(err)
	} else if err := envconfig.Process(Prefix, &Env); err != nil {
		panic(err)
	}
}
