package base

import (
	"os"
	"testing"
)

// TestEnvLangInit 验证包级 init 后 Env.Lang 的值。
// 预期：环境变量 ZKI_LANG 未设置时，defaults.Set + envconfig.Process 应把 Lang 置为 "zh-Hans"。
func TestEnvLangInit(t *testing.T) {
	t.Logf("ZKI_LANG=%q", os.Getenv("ZKI_LANG"))
	t.Logf("Env.Lang=%q", Env.Lang)
	t.Logf("Env.Environment=%q", Env.Environment)
	t.Logf("Env.Debug=%v", Env.Debug)
	t.Logf("Env.ErrorStack=%v", Env.ErrorStack)
	t.Logf("Env.LocaleDirPath=%q", Env.LocaleDirPath)
	if Env.Lang != "zh-Hans" {
		t.Errorf("Env.Lang = %q, want %q", Env.Lang, "zh-Hans")
	}
}

// TestDefaultLanguageInit 验证 DefaultLanguage 是否在 init 完成后取到 Env.Lang 的默认值。
// 该测试用于定位语言默认值失效的根因：包级变量初始化先于 init() 执行。
func TestDefaultLanguageInit(t *testing.T) {
	t.Logf("DefaultLanguage=%q", DefaultLanguage.String())
	if DefaultLanguage.String() != "zh-Hans" {
		t.Errorf("DefaultLanguage = %q, want %q", DefaultLanguage.String(), "zh-Hans")
	}
}
