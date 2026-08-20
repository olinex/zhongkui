package base

import (
	"github.com/creasty/defaults"
	"github.com/kelseyhightower/envconfig"
)

var (
	Env GlobalEnv
)

type GlobalEnv struct {
	// 字段名经 split_words 映射为 ZKI_<WORDS> 环境变量（如 Lang → ZKI_LANG）。
	// 不使用 envconfig:"X" tag：其值会被 envconfig 当作 Alt 回退名，
	// 当 ZKI_X 未设置时回退读取无前缀的 X，导致系统变量（如 LANG）覆盖 default 默认值。
	Environment   string `split_words:"true" default:"product"`
	Debug         bool   `split_words:"true" default:"false"`
	ErrorStack    bool   `split_words:"true" default:"false"`
	Lang          string `split_words:"true" default:"zh-Hans"`
	LocaleDirPath string `split_words:"true" default:"locales"`
}

func init() {
	if err := defaults.Set(&Env); err != nil {
		panic(err)
	} else if err := envconfig.Process(GlobalPrefix, &Env); err != nil {
		panic(err)
	}
}
