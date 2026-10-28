// 本文件实现长期记忆（agent_memories）的存储与检索：按「命名空间 + Agent 类型 + 会话」归属与
// 隔离（记忆只在其来源会话内可见），写入时做「规范化正文带密钥 HMAC-SHA256 精确去重 + pgvector
// 余弦相似度语义合并」并同步向量列与全文检索列，检索时做「向量路 + 关键词路」混合召回、RRF
// 融合、按参数阈值内容去重后取 topK，命中异步回写使用统计。记忆正文经 crypto 序列化器加密存储
// （列类型 BYTEA）。检索结果的去重复用知识库同包的文本 n-gram Jaccard 逻辑（与写入的余弦去重
// 是两回事，勿混用）。

package memory

import (
	// 标准库
	"context"
	"encoding/json"
	"strings"

	// 第三方
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/ai/port"
	"zhongkuisec.com/zhongkui/internal/ai/retrieval"
	"zhongkuisec.com/zhongkui/internal/ai/tools"
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// extract 从被摘要的消息中抽取长期记忆并落库：用 flash 档位生成结果、容错解析 JSON，逐条经
// Store.Write 落库（归属取自会话）。返回成功条数与非致命告警；任何失败都不返回错误，
// 以告警形式由 Compact 汇总，从而绝不影响摘要结果。
func (m *Service) extract(ctx context.Context, session *ai.AgentSession, msgs []*ai.AgentMessage) (int, []string) {
	chat, err := m.resolveFlashChat()
	if err != nil {
		return 0, []string{"memory: extraction skipped: " + err.Error()}
	}
	userMessage, err := buildMessageTranscript(ctx, msgs, m.params.Int("memory.evictedMessageByteLimit"))
	if err != nil {
		return 0, []string{"memory: build extraction prompt failed: " + err.Error()}
	}
	messages := []*schema.Message{
		schema.SystemMessage(m.params.Text("memory.extractSystemPrompt")),
		userMessage,
	}
	resp, err := chat.Generate(ctx, messages)
	if err != nil {
		return 0, []string{"memory: extraction model call failed: " + err.Error()}
	}
	if resp == nil {
		return 0, []string{"memory: extraction model returned no response"}
	}
	items, err := ParseExtractedMemories(resp.Content, m.params.Int("memory.extractMaxItems"))
	if err != nil {
		return 0, []string{"memory: cannot parse the extraction response: " + err.Error()}
	}
	agentKind, err := ai.ParseAgentKind(session.AgentKind)
	if err != nil {
		return 0, []string{"memory: extraction skipped: " + err.Error()}
	}
	count := 0
	var warnings []string
	for _, item := range items {
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		// Kind 缺失按 fact 兜底（ai.ParseMemoryKind），非法取值跳过该条并记 warning。
		kind, kerr := ai.ParseMemoryKind(ai.MemoryKind(item.Kind))
		if kerr != nil {
			warnings = append(warnings, "memory: extraction skipped an item with an invalid kind: "+item.Kind)
			continue
		}
		if _, werr := m.store.Write(ctx, tools.MemoryWriteInput{
			NamespaceName: session.NamespaceName,
			UserID:        session.UserID,
			AgentKind:     agentKind,
			SessionID:     session.ID,
			Kind:          kind,
			Title:         strings.TrimSpace(item.Title),
			Content:       content,
		}); werr != nil {
			warnings = append(warnings, "memory: extraction write failed: "+werr.Error())
			continue
		}
		count++
	}
	return count, warnings
}

// resolveFlashChat 经 resolver 解析 flash 档位的对话客户端；resolver 为 nil、档位未绑定或解析
// 失败均返回英文错误（滚动摘要与记忆抽取共用低档位，绝不静默切换到其他档位）。
func (m *Service) resolveFlashChat() (model.BaseChatModel, error) {
	return port.FlashChat(m.resolver)
}

// ExtractedMemory 是压缩时模型抽取的一条长期记忆（JSON 形状）。
type ExtractedMemory struct {
	// Kind 记忆类别（可缺失，缺省 fact）
	Kind string `json:"kind"`
	// Title 记忆标题（可缺失）
	Title string `json:"title"`
	// Content 记忆正文
	Content string `json:"content"`
}

// extractionPayload 是抽取结果的 JSON 形状：{"memories":[{"kind","title","content"}]}。
type extractionPayload struct {
	Memories []ExtractedMemory `json:"memories"`
}

// ParseExtractedMemories 容错解析抽取结果：先去掉 ```json 代码围栏，再按 {"memories":[...]} 解析，
// 失败时退回裸数组解析；条数超过 max（max>0 时）则截断。空白响应返回英文错误。
func ParseExtractedMemories(content string, max int) ([]ExtractedMemory, error) {
	trimmed := contract.StripJSONFence(content)
	if strings.TrimSpace(trimmed) == "" {
		return nil, errors.New("memory: extraction response is empty")
	}
	var payload extractionPayload
	var items []ExtractedMemory
	if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
		items = payload.Memories
	} else if err := json.Unmarshal([]byte(trimmed), &items); err != nil {
		return nil, errors.Newf("memory: cannot parse the extraction response as a memory list: %q", retrieval.TruncateText(trimmed, 200))
	}
	if max > 0 && len(items) > max {
		items = items[:max]
	}
	return items, nil
}
