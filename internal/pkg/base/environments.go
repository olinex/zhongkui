package base

import (
	"github.com/kelseyhightower/envconfig"
	"sync"
)

var (
	once sync.Once
	Env  GlobalEnv
)

type GlobalEnv struct {
	Environment string
	Debug       bool
	ErrorStack  bool
	Otel        bool
}

func init() {
	once.Do(func() {
		if err := envconfig.Process(GLOBAL_PREFIX, &Env); err != nil {
			panic(err)
		}
	})
}
