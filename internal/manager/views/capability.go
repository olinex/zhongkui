// 本文件承载 AI 能力元数据的声明与派生：能力随接口一起在视图层声明，写入接口目录后由
// CapabilityItems 统一导出。能力地图、API 知识分区与 api_invoke 白名单均消费这一份数据，
// 禁止在 pkg 或配置中另行维护能力表。
package views

import (
	// 标准库
	"sort"
	"strings"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/capability"
)

// CapabilityMeta 描述一个原生接口对外暴露的 AI 能力语义；随 RegisterApi / RegisterListApi
// 一并声明，方法、路径与用途由接口声明本身提供，此处只补充业务域、意图与可调用属性。
type CapabilityMeta struct {
	// ID 能力稳定标识，全局唯一。
	ID string
	// Domain 业务域标识，用于能力地图分组。
	Domain string
	// Intent 一句话用户意图，面向模型与读者。
	Intent string
	// Entities 该能力涉及的数据表。
	Entities []string
	// Access 只读或写入。
	Access capability.Access
	// Invokable 是否允许 api_invoke 直接调用。
	Invokable bool
}

// capabilityDomain 是能力地图中业务域的展示顺序与中文名；顺序即地图分组顺序。
type capabilityDomain struct {
	// ID 业务域标识。
	ID string
	// Label 业务域中文名。
	Label string
}

// capabilityDomains 是全部已知业务域，未登记的业务域排在最后并回退为标识本身。
var capabilityDomains = []capabilityDomain{
	{ID: "traffic", Label: "流量"},
	{ID: "tag", Label: "标签与事件"},
	{ID: "datasource", Label: "数据源"},
	{ID: "notification", Label: "通知"},
	{ID: "capturer", Label: "采集"},
	{ID: "user", Label: "用户与权限"},
	{ID: "ai", Label: "AI 助手与知识库"},
	{ID: "llm", Label: "模型与工具"},
	{ID: "etl", Label: "数据管道"},
	{ID: "security", Label: "安全与明文"},
	{ID: "platform", Label: "平台治理"},
}

// WithCapability 声明接口的 AI 能力元数据；未声明的接口不进入能力地图与 api_invoke 白名单。
func WithCapability(meta CapabilityMeta) MetadataOption {
	return func(o *metadataOptions) {
		o.capability = &meta
	}
}

// CapabilityItems 从接口目录派生能力快照：仅保留声明了 CapabilityMeta 的接口，按业务域展示
// 顺序与能力 ID 稳定排序，并补齐业务域中文名。
func CapabilityItems() []capability.Capability {
	catalog := ApiCatalog()
	items := make([]capability.Capability, 0, len(catalog))
	for _, descriptor := range catalog {
		meta := descriptor.Capability
		if meta == nil {
			continue
		}
		items = append(items, capability.Capability{
			ID:          meta.ID,
			Domain:      meta.Domain,
			DomainLabel: capabilityDomainLabel(meta.Domain),
			Intent:      meta.Intent,
			Method:      descriptor.Method,
			Path:        descriptor.Path,
			Entities:    append([]string(nil), meta.Entities...),
			Access:      meta.Access,
			Invokable:   meta.Invokable,
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		left, right := capabilityDomainRank(items[i].Domain), capabilityDomainRank(items[j].Domain)
		if left != right {
			return left < right
		}
		return items[i].ID < items[j].ID
	})
	return items
}

// capabilityDomainRank 返回业务域的展示顺序；未登记的业务域排在最后。
func capabilityDomainRank(domain string) int {
	for i, item := range capabilityDomains {
		if item.ID == domain {
			return i
		}
	}
	return len(capabilityDomains)
}

// capabilityDomainLabel 返回业务域中文名；未登记时回退为业务域标识。
func capabilityDomainLabel(domain string) string {
	for _, item := range capabilityDomains {
		if item.ID == domain {
			return item.Label
		}
	}
	if strings.TrimSpace(domain) == "" {
		return "未分类"
	}
	return domain
}
