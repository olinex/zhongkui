package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/translate"
)

// TestCommandTreeKeysComplete 验证命令树中所有 i18n key 在指定回退语言（zh-Hans）下均可解析
// 命令字段（Short/Long/Example）与标志 usage 存储的即翻译键本身（cmd.*/flag.*），
// 代码中不允许硬编码文案；translate 未命中时返回 key 本身，
// 故此处断言解析结果不等于 key：任何缺失键或残留字面量都会导致本测试失败并暴露具体 key
func TestCommandTreeKeysComplete(t *testing.T) {
	// zh-Hans 为指定回退语言，其翻译文件必须覆盖全部 cmd.*/flag.* 键
	zh := base.MustParseLanguage("zh-Hans")
	missing := map[string]string{}

	check := func(key string) {
		if key == "" {
			return
		}
		if got := translate.L(zh, key, nil); got == key {
			missing[key] = key
		}
	}

	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		check(c.Short)
		check(c.Long)
		check(c.Example)

		checkFlags := func(fs *pflag.FlagSet) {
			fs.VisitAll(func(f *pflag.Flag) {
				check(f.Usage)
			})
		}
		checkFlags(c.LocalFlags())
		checkFlags(c.InheritedFlags())

		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(RootCmd)
	// help 命令由 SetHelpCommand 注册（Execute 阶段才挂载到命令树），需单独遍历
	walk(helpCmd)

	if len(missing) > 0 {
		t.Errorf("命令树存在缺失的 i18n key（zh-Hans 翻译文件未覆盖）:")
		for k := range missing {
			t.Errorf("  %s", k)
		}
	}
}
