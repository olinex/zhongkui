package views

import (
	// stdlib
	"testing"

	// internal — 枚举归属包：blank import 触发各包 init() 的注册/条目派生，保证 enum.List()
	// 覆盖全量枚举类；若某个归属包被移出本清单，其类会静默缺失，由下方空注册表断言拦截
	_ "zhongkui.com/zhongkui/internal/ai"
	_ "zhongkui.com/zhongkui/internal/ai/tools/mcp"
	_ "zhongkui.com/zhongkui/internal/models/core"
	_ "zhongkui.com/zhongkui/internal/models/network"
	_ "zhongkui.com/zhongkui/internal/models/tag"
	_ "zhongkui.com/zhongkui/internal/notify"
	_ "zhongkui.com/zhongkui/internal/pkg/message"
	_ "zhongkui.com/zhongkui/internal/pkg/sqlitevtab/vtab"
	_ "zhongkui.com/zhongkui/internal/tagengine"

	// internal — 被测依赖
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/enum"
	"zhongkui.com/zhongkui/internal/pkg/translate"
)

// missingEnumKeySentinel 是必定未注册的 i18n 键，用于验证「未命中返回键本身」这一判定前提
// 成立，避免守卫测试因判定方式失效而空转（与 param_i18n_test.go 的哨兵用法一致）。
// missingEnumKeySentinel is an i18n key guaranteed to be unregistered; it verifies the premise
// of the "a miss returns the key itself" check, so the guard never passes vacuously (mirroring
// the sentinel usage in param_i18n_test.go).
const missingEnumKeySentinel = "enum.__missingEnumClass__.__missingEntry__"

// TestEnumClassesHaveBilingualLabels 守卫「枚举注册表」与「双语 label 文案」不漂移：
// 注册表每个枚举类、每个条目的 label 键 enum.<类名>.<key> 在每种支持语言下都必须能解析，
// 否则 /admin/enum/list 会把裸 i18n 键返给前端。注册表内无文案副本，缺翻译无处回退，故新增
// 枚举类/条目忘记补语言文件时由本测试在构建阶段暴露。测试同时断言注册表非空（blank import
// 的归属包 init 确实执行）且每个类至少一条目（RegisterSource 类不空转）。
// TestEnumClassesHaveBilingualLabels guards the enum registry and the bilingual label copy from
// drifting apart: the label key enum.<className>.<key> of every class entry must resolve in every
// supported language, otherwise /admin/enum/list hands a bare i18n key to the frontend. The
// registry holds no copy to fall back on, so a missing translation has nowhere to go: forgetting a
// locale entry for a new class or entry is surfaced by this test at the build stage. It also
// asserts the registry is non-empty (the blank imports really ran their registration) and every
// class has at least one entry (RegisterSource classes do not guard vacuously).
func TestEnumClassesHaveBilingualLabels(t *testing.T) {
	if got := translate.L(base.DefaultLanguage, missingEnumKeySentinel, nil); got != missingEnumKeySentinel {
		t.Fatalf("translate.L(%q) = %q, want the key itself; the miss check of this test is unsound", missingEnumKeySentinel, got)
	}
	classes := enum.List()
	if len(classes) == 0 {
		t.Fatal("enum registry is empty: the blank-imported owner packages did not run their registration")
	}
	for _, class := range classes {
		if len(class.Entries) == 0 {
			t.Errorf("enum class %q has no entries; its label guard runs vacuously", class.Name)
			continue
		}
		for _, entry := range class.Entries {
			key := class.LabelKey(entry.Key)
			for _, lang := range base.SupportLanguages {
				if got := translate.L(&lang, key, nil); got == key {
					t.Errorf("translate[%s] misses %q declared by enum class %q", lang.String(), key, class.Name)
				}
			}
		}
	}
}
