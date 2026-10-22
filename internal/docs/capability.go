package docs

import (
	// 标准库
	"strings"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/capability"
	"zhongkuisec.com/zhongkui/internal/manager"
)

// capabilityProjection 生成 AI 能力地图页：内容与 RAG capability 分区的整体地图同源。
func capabilityProjection() Projection {
	content := capability.MapMarkdown(manager.CollectCapabilities())
	// 运行时地图自带 H1 标题；投影页由导出器统一加 H1，这里去掉重复标题。
	if idx := strings.Index(content, "\n\n"); idx >= 0 {
		content = content[idx+2:]
	}
	return Projection{
		Title:       "AI 能力地图",
		Summary:     "按业务域列出 AI 助手可感知的原生接口、访问性质与关联实体。",
		Roles:       []Role{RoleDev, RoleSecOps},
		Scene:       "需要确认 AI 助手能感知哪些业务能力、或新增能力注册项时阅读。",
		Output:      GeneratedRoot + "/reference/capability-map.md",
		SourceLabel: "internal/manager/views",
		Content:     content,
	}
}
