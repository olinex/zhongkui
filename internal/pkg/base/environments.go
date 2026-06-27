package base

import (
	"github.com/creasty/defaults"
	"github.com/kelseyhightower/envconfig"
)

var (
	Env GlobalEnv
)

type GlobalEnv struct {
	Environment string `envconfig:"ENVIRONMENT" default:"product"`
	Debug       bool   `envconfig:"DEBUG" default:"false"`
	ErrorStack  bool   `envconfig:"ERROR_STACK" default:"false"`
}

func init() {
	if err := defaults.Set(&Env); err != nil {
		panic(err)
	} else if err := envconfig.Process(GLOBAL_PREFIX, &Env); err != nil {
		panic(err)
	}
}
