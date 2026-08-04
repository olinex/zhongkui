package base

import (
	"github.com/creasty/defaults"
	"github.com/kelseyhightower/envconfig"
	"github.com/pterm/pterm"
)

var (
	Env GlobalEnv
)

type GlobalEnv struct {
	Environment string `envconfig:"ENVIRONMENT" default:"product"`
	Debug       bool   `envconfig:"DEBUG" default:"false"`
	ErrorStack  bool   `envconfig:"ERROR_STACK" default:"false"`
	// Lang CLI 显示语言，通过 ZKI_LANG 环境变量设置（如 en / zh-Hans），默认 zh-Hans
	Lang string `envconfig:"LANG" default:"zh-Hans"`
}

func init() {
	if err := defaults.Set(&Env); err != nil {
		panic(err)
	} else if err := envconfig.Process(GLOBAL_PREFIX, &Env); err != nil {
		panic(err)
	} else if Env.Debug {
		pterm.EnableDebugMessages()
		pterm.Debug.Println("pterm debug is enabled")
	}
}
