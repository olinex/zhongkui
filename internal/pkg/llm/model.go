package llm

import (
	// 标准库
	"strings"
	"time"

	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// ModelKind 是 LLM 模型类型标识：对话模型（chat）与向量化模型（embedding）。
// 与 llm_models.kind 列的取值一致，决定构造哪一类客户端以及可绑定的档位范围。
type ModelKind string

// 模型类型常量。
const (
	// ModelKindChat 对话/生成模型
	ModelKindChat ModelKind = "chat"
	// ModelKindEmbedding 向量化模型
	ModelKindEmbedding ModelKind = "embedding"
)

// ParseModelKind 解析并校验模型类型取值（忽略首尾空白；不区分大小写）：仅 chat / embedding
// 为合法取值，其余取值返回英文错误。类型清单与取值规范化只在本包声明，调用方不再各自校验。
func ParseModelKind(value ModelKind) (ModelKind, error) {
	switch kind := ModelKind(strings.ToLower(strings.TrimSpace(string(value)))); kind {
	case ModelKindChat, ModelKindEmbedding:
		return kind, nil
	default:
		return "", errors.Newf("invalid model kind %q: expected chat or embedding", value)
	}
}

// Slot 是模型档位槽位标识：flash（轻量）、standard（标准）、advanced（增强）三档对话模型
// 与 embedding（向量化）一个固定槽位。与 llm_bindings.slot 列的取值一致。
type Slot string

// 档位常量。
const (
	// SlotFlash 轻量档：低时延、低成本的对话档位
	SlotFlash Slot = "flash"
	// SlotStandard 标准档：日常问答的默认对话档位
	SlotStandard Slot = "standard"
	// SlotAdvanced 增强档：复杂推理与长输出的对话档位
	SlotAdvanced Slot = "advanced"
	// SlotEmbedding 向量化档位：知识库索引与向量检索使用
	SlotEmbedding Slot = "embedding"
)

// AllSlots 是全部固定档位的清单，声明顺序即前端展示顺序：三档对话档位在前，向量化槽位在后。
// 档位枚举的成员集合只在此处声明，视图层与 ai 运行时一律引用本清单，不自行列举档位。
var AllSlots = []Slot{SlotFlash, SlotStandard, SlotAdvanced, SlotEmbedding}

// ParseSlot 解析并校验档位取值（忽略首尾空白；不区分大小写）：四个固定档位均为合法取值，
// 其余取值返回英文错误。档位清单与取值规范化只在本包声明，调用方不再各自校验并拼错误文案。
func ParseSlot(value Slot) (Slot, error) {
	slot := Slot(strings.ToLower(strings.TrimSpace(string(value))))
	for _, known := range AllSlots {
		if known == slot {
			return slot, nil
		}
	}
	return "", errors.Newf("invalid slot %q: expected one of %s", value, slotNames(AllSlots))
}

// chatSlots 是允许作为对话档位的槽位清单（embedding 不在其列），由 AllSlots 按档位所要求的
// 模型类型过滤得出：档位成员仍只声明在 AllSlots 一处，顺序与其一致。
var chatSlots = func() []Slot {
	slots := make([]Slot, 0, len(AllSlots))
	for _, slot := range AllSlots {
		if SlotKind(slot) == ModelKindChat {
			slots = append(slots, slot)
		}
	}
	return slots
}()

// slotNames 把档位清单渲染为错误文案中的枚举串（如 "flash, standard, advanced"）：文案与
// 档位清单同源，新增档位时无需再同步修改任何错误消息。
func slotNames(slots []Slot) string {
	names := make([]string, len(slots))
	for i, slot := range slots {
		names[i] = string(slot)
	}
	return strings.Join(names, ", ")
}

// InvalidChatSlotError 返回「对话档位取值不合法」的英文错误，枚举文案与档位清单同源；供
// 视图层在 ParseChatSlot 返回 false 时直接使用，各处不再重复拼写档位枚举。
func InvalidChatSlotError(value Slot) error {
	return chatSlotError("invalid chat slot", value)
}

// chatSlotError 拼装「对话档位取值不合法」的英文错误：subject 是错误主语，用于区分取值
// 来源（显式指定 / 调用点声明 / 注册表默认），枚举文案取自 chatSlots。
func chatSlotError(subject string, value Slot) error {
	return errors.Newf("%s %q: expected one of %s", subject, value, slotNames(chatSlots))
}

// ParseChatSlot 解析并校验对话档位取值（忽略首尾空白；不区分大小写）：
// 仅 flash / standard / advanced 合法，embedding 或未知取值返回 ok=false。
func ParseChatSlot(value Slot) (Slot, bool) {
	slot := Slot(strings.ToLower(strings.TrimSpace(string(value))))
	for _, known := range chatSlots {
		if known == slot {
			return slot, true
		}
	}
	return "", false
}

// SlotKind 返回档位要求的模型类型：embedding 档位要求向量模型，其余对话档位要求对话模型。
func SlotKind(slot Slot) ModelKind {
	if slot == SlotEmbedding {
		return ModelKindEmbedding
	}
	return ModelKindChat
}

// ResolveSlot 按「显式指定 → 调用点声明 → 注册表默认」的优先级解析本次调用使用的对话档位：
// requested 为调用方显式指定（如请求体字段），declared 为调用点声明的建议档位（如视图层
// 按业务场景推荐），fallback 为参数注册表 agent.defaultSlot 的默认档位。三者按序取第一个
// 非空值；任一非空取值不合法（embedding 或未知档位）或全部为空时返回英文错误。
func ResolveSlot(requested, declared, fallback Slot) (Slot, error) {
	if requested != "" {
		slot, ok := ParseChatSlot(requested)
		if !ok {
			return "", InvalidChatSlotError(requested)
		}
		return slot, nil
	}
	if declared != "" {
		slot, ok := ParseChatSlot(declared)
		if !ok {
			return "", chatSlotError("invalid declared chat slot", declared)
		}
		return slot, nil
	}
	slot, ok := ParseChatSlot(fallback)
	if !ok {
		return "", chatSlotError("invalid default chat slot", fallback)
	}
	return slot, nil
}

// ModelConfig 是一个模型端点的运行时配置快照，由 ai 运行时从模型注册表（llm_models）
// 与档位绑定（llm_bindings）组装而来，是 LLM 接入配置的唯一事实来源（不再来自配置文件）。
type ModelConfig struct {
	// Name 模型在注册表中的本地标识（llm_models.name），用于运维识别与展示
	Name string
	// Kind 模型类型（chat / embedding）
	Kind ModelKind
	// BaseURL 模型端点的 OpenAI 兼容接口 Base URL
	BaseURL string
	// APIKey 访问模型端点的明文凭据（落库为密文，此处为解密后的明文）
	APIKey string
	// Model 供应商侧的模型名（如 gpt-4o-mini、text-embedding-3-small）
	Model string
	// Timeout 单次请求的超时；未声明（0）时由探活经 defaults.Set 回填 default 标签声明的 15s，
	// 构造正式客户端时 0 仍交由底层客户端默认值
	Timeout time.Duration `default:"15s"`
	// Temperature 采样温度，仅 chat 类模型有意义
	Temperature float64
	// MaxTokens 单次生成的最大 token 数，仅 chat 类模型有意义
	MaxTokens int
	// Dimensions 向量维度，仅 embedding 类模型有意义（须与知识库向量列一致）
	Dimensions int
}

// Validate 校验模型端点的字段不变量：端点三要素非空、超时与最大 token 数为正数、
// 温度落在 [0, 2]。Kind 与 Name 不在校验范围内：Kind 由 ParseModelKind 在入口解析为合法取值，
// Name 的注册表内唯一性属于视图层语义。向量维度同样不校验：它不再由前端提交，
// 「索引维度是运维独占的 schema 属性」由运维经 migrate ai-embedding 显式指定，模型的已验证
// 维度则由后端在绑定 embedding 档位时经真实探测写入（见 VerifyEmbeddingCompat）。
func (c ModelConfig) Validate() error {
	switch {
	case strings.TrimSpace(c.BaseURL) == "":
		return errors.New("baseURL must be non-empty")
	case strings.TrimSpace(c.APIKey) == "":
		return errors.New("apiKey must be non-empty")
	case strings.TrimSpace(c.Model) == "":
		return errors.New("model must be non-empty")
	case c.Timeout <= 0:
		return errors.New("timeoutSeconds must be a positive integer")
	case c.MaxTokens <= 0:
		return errors.New("maxTokens must be a positive integer")
	case c.Temperature < 0 || c.Temperature > 2:
		return errors.New("temperature must be within [0, 2]")
	default:
		return nil
	}
}
