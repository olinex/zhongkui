package base

import (
	"github.com/creasty/defaults"
	"github.com/kelseyhightower/envconfig"
)

// Env 是包初始化时装配的全局环境配置实例。
var (
	Env GlobalEnv
)

// GlobalEnv 定义平台全局环境配置，字段经 split_words 映射为 ZKI_ 前缀环境变量注入。
type GlobalEnv struct {
	// 字段名经 split_words 映射为 ZKI_<WORDS> 环境变量（如 Lang → ZKI_LANG）。
	// 不使用 envconfig:"X" tag：其值会被 envconfig 当作 Alt 回退名，
	// 当 ZKI_X 未设置时回退读取无前缀的 X，导致系统变量（如 LANG）覆盖 default 默认值。
	//
	// Environment 运行环境标识（如 product、development）
	Environment string `split_words:"true" default:"product"`
	// Debug 是否开启调试模式
	Debug bool `split_words:"true" default:"false"`
	// ErrorStack 是否为错误附加调用堆栈
	ErrorStack bool `split_words:"true" default:"false"`
	// Lang 默认语言（BCP 47，如 zh-Hans）
	Lang string `split_words:"true" default:"zh-Hans"`
	// LocaleDirPath 本地化文件所在目录
	LocaleDirPath string `split_words:"true" default:"locales"`
}

func init() {
	if err := defaults.Set(&Env); err != nil {
		panic(err)
	} else if err := envconfig.Process(GlobalPrefix, &Env); err != nil {
		panic(err)
	}
}
