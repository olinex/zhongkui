package otel

import (
	"github.com/creasty/defaults"
	"github.com/kelseyhightower/envconfig"
)

const (
	Prefix          = "OTEL"
	Http            = "http"
	Grpc            = "grpc"
	ProviderOtlp    = "otlp"
	ProviderStdout  = "stdout"
	SamplerAlwaysOn = "always_on"
	SamplerHead     = "head"
	SamplerTail     = "tail"
)

var (
	Env OtelEnv
)

type OtelEnv struct {
	Enabled      bool    `envconfig:"ENABLED" default:"false"`
	Provider     string  `envconfig:"PROVIDER" default:"otlp"`
	OtlpProtocol string  `envconfig:"OTLP_PROTOCOL" default:"grpc"`
	SamplerType  string  `envconfig:"SAMPLER_TYPE" default:"always_on"`
	SamplerRatio float64 `envconfig:"SAMPLER_RATIO" default:"1.0"`
	TailOnError  bool    `envconfig:"TAIL_ON_ERROR" default:"true"`
}

func init() {
	if err := defaults.Set(&Env); err != nil {
		panic(err)
	} else if err := envconfig.Process(Prefix, &Env); err != nil {
		panic(err)
	}
}
