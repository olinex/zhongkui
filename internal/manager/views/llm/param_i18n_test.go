package llm

import (
	// stdlib
	"fmt"
	"testing"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/base"
	pkgllm "zhongkui.com/zhongkui/internal/pkg/llm"
	"zhongkui.com/zhongkui/internal/pkg/translate"
)

// missingKeySentinel 是必定未注册的 i18n 键，用于验证「未命中返回键本身」这一判定前提
// 成立，避免守卫测试因判定方式失效而空转。
// missingKeySentinel is an i18n key that is guaranteed to be unregistered; it verifies the
// premise of the "a miss returns the key itself" check, so the guard never passes vacuously.
const missingKeySentinel = "param.__missingParameter__.comment"

// TestParamSpecsI18nKeysComplete 守卫「代码内参数注册表」与「后端 i18n 文案」不漂移：
// 注册表每个键在每种支持语言下都必须能解析出 param.<Key>.label 与 param.<Key>.comment，
// 否则 param/list 会把裸 i18n 键返给前端。注册表已不持有任何文案副本，缺翻译无处回退，
// 故新增参数忘记补语言文件时由本测试在构建阶段暴露。
// TestParamSpecsI18nKeysComplete guards the in-code parameter registry and the backend i18n
// copy from drifting apart: every registry key must resolve both param.<Key>.label and
// param.<Key>.comment in every supported language, otherwise param/list hands a bare i18n key
// to the frontend. The registry holds no copy to fall back on, so a missing translation has
// nowhere to go: forgetting a locale entry for a new parameter is surfaced by this test at the
// build stage.
func TestParamSpecsI18nKeysComplete(t *testing.T) {
	lang := base.DefaultLanguage
	if got := translate.L(lang, missingKeySentinel, nil); got != missingKeySentinel {
		t.Fatalf("translate.L(%q) = %q, want the key itself; the miss check of this test is unsound", missingKeySentinel, got)
	}
	for _, supported := range base.SupportLanguages {
		for _, spec := range pkgllm.Specs {
			for _, suffix := range []string{"label", "comment"} {
				key := fmt.Sprintf("param.%s.%s", spec.Key, suffix)
				if got := translate.L(&supported, key, nil); got == key {
					t.Errorf("translate[%s] misses %q declared by the registry", supported.String(), key)
				}
			}
		}
	}
}
