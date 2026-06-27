package otel

import (
	"github.com/creasty/defaults"
	"github.com/kelseyhightower/envconfig"
)

const (
	PREFIX = "OTEL"
	HTTP   = "http"
	GRPC   = "grpc"
)

var (
	Env OtelEnv
)

type OtelEnv struct {
	Enabled      bool   `envconfig:"ENABLED" default:"false"`
	OtlpProtocol string `envconfig:"OTLP_PROTOCOL" default:"grpc"`
}

func init() {
	if err := defaults.Set(&Env); err != nil {
		panic(err)
	} else if err := envconfig.Process(PREFIX, &Env); err != nil {
		panic(err)
	}
}
