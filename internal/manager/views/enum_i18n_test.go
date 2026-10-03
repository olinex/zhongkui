package views

import (
	// 标准库
	"testing"

	// internal — 枚举归属包：blank import 触发各包 init() 的注册/条目派生，保证 enum.List()
	// 覆盖全量枚举类；若某个归属包被移出本清单，其类会静默缺失，由下方空注册表断言拦截
	_ "zhongkuisec.com/zhongkui/internal/ai"
	_ "zhongkuisec.com/zhongkui/internal/ai/tools/mcp"
	_ "zhongkuisec.com/zhongkui/internal/models/ai"
	_ "zhongkuisec.com/zhongkui/internal/models/core"
	_ "zhongkuisec.com/zhongkui/internal/models/network"
	_ "zhongkuisec.com/zhongkui/internal/models/tag"
	_ "zhongkuisec.com/zhongkui/internal/notify"
	_ "zhongkuisec.com/zhongkui/internal/pkg/docconv"
	_ "zhongkuisec.com/zhongkui/internal/pkg/message"
	_ "zhongkuisec.com/zhongkui/internal/pkg/sqlitevtab/vtab"
	_ "zhongkuisec.com/zhongkui/internal/tagengine"

	// internal — 被测依赖
	"zhongkuisec.com/zhongkui/internal/pkg/base"
	"zhongkuisec.com/zhongkui/internal/pkg/enum"
	"zhongkuisec.com/zhongkui/internal/pkg/translate"
)

// missingEnumKeySentinel 是必定未注册的 i18n 键，用于验证「未命中返回键本身」这一判定前提
// 成立，避免守卫测试因判定方式失效而空转（与 param_i18n_test.go 的哨兵用法一致）。
const missingEnumKeySentinel = "enum.__missingEnumClass__.__missingEntry__"

// TestEnumClassesHaveBilingualLabels 守卫「枚举注册表」与「双语 label 文案」不漂移：
// 注册表每个枚举类、每个条目的 label 键 enum.<类名>.<key> 在每种支持语言下都必须能解析，
// 否则 /admin/enum/list 会把裸 i18n 键返给前端。注册表内无文案副本，缺翻译无处回退，故新增
// 枚举类/条目忘记补语言文件时由本测试在构建阶段暴露。测试同时断言注册表非空（blank import
// 的归属包 init 确实执行）且每个类至少一条目（RegisterSource 类不空转）。
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
