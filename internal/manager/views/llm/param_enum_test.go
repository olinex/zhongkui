package llm

import (
	// stdlib
	"testing"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/enum"
	pkgllm "zhongkui.com/zhongkui/internal/pkg/llm"
)

// TestParamSpecsEnumTypesRegistered 守卫参数注册表的枚举类引用不漂移：声明了 EnumType 的参数，
// 其枚举类必须已在枚举注册表登记，且封闭取值清单内的每个字面量都是该类条目的 value。
// 否则前端会按一个不存在的枚举类取 label，或对清单内的取值取不到 label 而裸显字面量。
// TestParamSpecsEnumTypesRegistered guards the enum-class references of the parameter registry
// from drifting: a parameter declaring EnumType must reference a class registered in the enum
// registry and every literal of its closed value space must be a value of that class. Otherwise
// the frontend would resolve labels against a nonexistent class, or fail to resolve one for a
// listed value and display the bare literal.
func TestParamSpecsEnumTypesRegistered(t *testing.T) {
	for _, spec := range pkgllm.Specs {
		if spec.EnumType == "" {
			continue
		}
		class, ok := enum.Get(spec.EnumType)
		if !ok {
			t.Errorf("parameter %q references the unregistered enum class %q", spec.Key, spec.EnumType)
			continue
		}
		for _, option := range spec.Options {
			if _, ok := class.EntryByValue(option); !ok {
				t.Errorf("parameter %q lists option %q which is not a value of the enum class %q",
					spec.Key, option, spec.EnumType)
			}
		}
	}
}

// TestLlmSlotEnumMatchesSlotCatalog 守卫档位枚举类与档位清单不漂移：枚举类 llmSlot 的条目
// 必须与 pkgllm.AllSlots 一一对应（value 相同、顺序一致）。二者分别声明在 search.toml 与
// pkgllm 中，而参数 agent.defaultSlot 的取值清单取自后者、展示 label 取自前者，任一侧新增
// 档位而未同步另一侧，都会让该档位在下拉里丢掉本地化文案。
// TestLlmSlotEnumMatchesSlotCatalog guards the slot enum class and the slot catalog from drifting:
// the entries of the llmSlot class must match pkgllm.AllSlots one to one (same values, same
// order). They are declared in search.toml and pkgllm respectively, while the value space of the
// agent.defaultSlot parameter comes from the latter and its display labels from the former: adding
// a slot on one side only would strip that slot of its localized copy in the select.
func TestLlmSlotEnumMatchesSlotCatalog(t *testing.T) {
	class, ok := enum.Get("llmSlot")
	if !ok {
		t.Fatal("the llmSlot enum class is not registered")
	}
	if len(class.Entries) != len(pkgllm.AllSlots) {
		t.Fatalf("the llmSlot enum class has %d entries, pkgllm.AllSlots has %d",
			len(class.Entries), len(pkgllm.AllSlots))
	}
	for i, slot := range pkgllm.AllSlots {
		if class.Entries[i].Value != string(slot) {
			t.Errorf("llmSlot entry #%d is %q, pkgllm.AllSlots #%d is %q",
				i, class.Entries[i].Value, i, slot)
		}
	}
}
