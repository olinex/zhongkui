package otel

import (
	"github.com/creasty/defaults"
	"github.com/kelseyhightower/envconfig"
)

const (
	PREFIX            = "OTEL"
	HTTP              = "http"
	GRPC              = "grpc"
	PROVIDER_OTLP     = "otlp"
	PROVIDER_STDOUT   = "stdout"
	SAMPLER_ALWAYS_ON = "always_on"
	SAMPLER_HEAD      = "head"
	SAMPLER_TAIL      = "tail"
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
	} else if err := envconfig.Process(PREFIX, &Env); err != nil {
		panic(err)
	}
}
