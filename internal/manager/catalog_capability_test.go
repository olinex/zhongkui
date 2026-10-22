// 本文件是视图层能力声明的守卫测试：能力由 RegisterApi / RegisterListApi 随接口一并声明，
// 本测试校验派生结果的结构不变量、与接口目录的一致性以及能力地图内容完整性。
package manager

import (
	"strings"
	"testing"

	"zhongkuisec.com/zhongkui/internal/ai/capability"
)

// TestCollectCapabilities_Invariants 校验能力快照：ID 唯一、字段齐备、当前只登记只读可调用能力，
// 且每个能力的 method/path 都能在接口目录中找到。
func TestCollectCapabilities_Invariants(t *testing.T) {
	items := CollectCapabilities()
	if len(items) == 0 {
		t.Fatalf("CollectCapabilities() 为空")
	}
	catalog := CollectApiCatalog()
	known := make(map[string]bool, len(catalog))
	for _, descriptor := range catalog {
		known[descriptor.Method+" "+descriptor.Path] = true
	}
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		if item.ID == "" || seen[item.ID] {
			t.Errorf("能力 ID 为空或重复: %q", item.ID)
		}
		seen[item.ID] = true
		if item.Domain == "" || item.DomainLabel == "" || item.Intent == "" {
			t.Errorf("能力 %q 缺少业务域、业务域中文名或意图", item.ID)
		}
		if item.Access != capability.AccessRead {
			t.Errorf("能力 %q 当前只允许登记只读能力", item.ID)
		}
		if !known[item.Method+" "+item.Path] {
			t.Errorf("能力 %q 指向未登记的接口: %s %s", item.ID, item.Method, item.Path)
		}
	}
}

// TestCollectCapabilities_Map 校验能力地图覆盖全部能力且按业务域分组。
func TestCollectCapabilities_Map(t *testing.T) {
	items := CollectCapabilities()
	first := capability.MapMarkdown(items)
	if first != capability.MapMarkdown(CollectCapabilities()) {
		t.Errorf("能力地图两次生成不一致")
	}
	for _, item := range items {
		if !strings.Contains(first, item.Path) {
			t.Errorf("能力地图缺少接口 %s", item.Path)
		}
		if !strings.Contains(first, "## "+item.DomainLabel) {
			t.Errorf("能力地图缺少业务域 %q", item.DomainLabel)
		}
	}
}
