package cmd

import (
	"context"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"go.uber.org/zap"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/cliinput"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/translate"
)

var (
	once    sync.Once
	cliI18n *translate.I18n
)

// cliLang 返回 CLI 的显示语言
// 优先读取 base.Env.Lang（由 ZKI_LANG 环境变量注入），未设置或解析失败时回退到简体中文
func cliLang() *translate.Language {
	if v := base.Env.Lang; v != "" {
		if lang, err := translate.ParseLanguage(v); err == nil {
			return lang
		}
	}
	zh, _ := translate.ParseLanguage("zh-Hans")
	return zh
}

// getCliI18n 惰性初始化 CLI 专用的 i18n 实例
// 仅加载消息包（不注册验证器翻译），用于命令树本地化与运行时提示翻译
func getCliI18n() *translate.I18n {
	once.Do(func() {
		zh, _ := translate.ParseLanguage("zh-Hans")
		en, _ := translate.ParseLanguage("en")
		cfg := &translate.Config{
			DefaultLocale:  *cliLang(),
			SupportLocales: []translate.Language{*zh, *en},
			RootPath:       "locales",
		}
		// 消息包加载无需输出日志，使用 no-op logger 避免污染 CLI 输出
		logger := &logging.Logger{SugaredLogger: zap.NewNop().Sugar()}
		cliI18n = translate.NewMessageBundle(context.Background(), cfg, logger)
	})
	return cliI18n
}

// cliT 翻译 CLI 消息
// key 为消息 ID，fallback 为中文默认文案，data 为可选的模板数据
func cliT(key, fallback string, data ...map[string]any) string {
	return getCliI18n().Localize(cliLang().StdTag(), key, fallback, data...)
}

// localizeCmdTree 遍历命令树，将命令描述与标志说明本地化为 CLI 语言
// 命令文本（Short/Long/Example）与标志 usage 在无对应翻译时回退为中文原文
func localizeCmdTree() {
	lang := cliLang().StdTag()
	i := getCliI18n()
	localize := func(key, fallback string) string {
		return i.Localize(lang, key, fallback)
	}

	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		path := ""
		if c != RootCmd {
			path = strings.TrimPrefix(c.CommandPath(), RootCmd.Name()+" ")
		}
		cmdKey := "cmd"
		flagPrefix := "flag"
		if path != "" {
			cmdKey += "." + path
			flagPrefix += "." + path
		}
		c.Short = localize(cmdKey+".short", c.Short)
		c.Long = localize(cmdKey+".long", c.Long)
		c.Example = localize(cmdKey+".example", c.Example)

		translateFlags := func(fs *pflag.FlagSet) {
			fs.VisitAll(func(f *pflag.Flag) {
				f.Usage = localize(flagPrefix+"."+f.Name, f.Usage)
			})
		}
		translateFlags(c.LocalFlags())
		translateFlags(c.InheritedFlags())

		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(RootCmd)
	// help 命令由 SetHelpCommand 注册（Execute 阶段才挂载到命令树），需单独本地化
	walk(helpCmd)
}

func init() {
	// 将 cliinput 交互提示接入统一翻译链路（key + 中文回退文案）
	cliinput.SetTranslator(func(key, fallback string) string {
		return cliT(key, fallback)
	})
}
