package llm

import (
	// 标准库
	"encoding/json"
	"strings"
	"time"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
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
	// ContextTokens 上下文窗口容量（token 数），仅 chat 类模型有意义：由用户在创建模型时提供，
	// agent 使用该档位模型时据此计算上下文占用比例；0 表示未配置（不做占用展示与自动压缩）
	ContextTokens int
	// Dimensions 向量维度，仅 embedding 类模型有意义（须与知识库向量列一致）
	Dimensions int
	// ExtraBody 高级请求配置，为该模型每次对话请求自动合并的 JSON 对象；承载供应商
	// 自定义字段（不同供应商字段格式各异、无法穷举，这是通用逃生通道），仅 chat 类模型有意义
	ExtraBody map[string]any
	// ThinkingField 思考开关的请求字段名（如 enable_thinking）；为空表示该模型端点不支持思考开关
	ThinkingField string
	// ThinkingOnValue 开启思考时写入 ThinkingField 的取值（JSON 文本解析结果，如 true）
	ThinkingOnValue any
	// ThinkingOffValue 关闭思考时写入 ThinkingField 的取值（JSON 文本解析结果，如 false）
	ThinkingOffValue any
}

// ParseExtraBody 解析模型端点的高级请求配置（llm_models.extra_body）：入参去掉首尾空白后
// 为空则返回 (nil, nil)；否则必须是 JSON 对象（{}），解析为 map[string]any 返回。非对象或
// 非法 JSON 返回英文错误。
func ParseExtraBody(raw string) (map[string]any, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	var value any
	if err := json.Unmarshal([]byte(trimmed), &value); err != nil {
		return nil, errors.Newf("extraBody must be a valid JSON object: %s", err)
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, errors.Newf("extraBody must be a JSON object, got %q", trimmed)
	}
	return object, nil
}

// ParseJSONValue 解析思考开关的开/关取值（llm_models.thinking_on_value / thinking_off_value）：
// 入参去掉首尾空白后为空则返回 (nil, nil)；否则用 json.Unmarshal 解析为任意 JSON 值
// （bool / object / string / number / array）返回。非法 JSON 返回英文错误。
func ParseJSONValue(raw string) (any, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	var value any
	if err := json.Unmarshal([]byte(trimmed), &value); err != nil {
		return nil, errors.Newf("invalid JSON value %q: %s", raw, err)
	}
	return value, nil
}

// Validate 校验模型端点的字段不变量：端点三要素非空、超时与最大 token 数为正数、
// 温度落在 [0, 2]。Kind 与 Name 不在校验范围内：Kind 由 ParseModelKind 在入口解析为合法取值，
// Name 的注册表内唯一性属于视图层语义。向量维度同样不校验：它不再由前端提交，
// 「索引维度是运维独占的 schema 属性」由运维经 sync embedding-dimensions 显式指定，模型的已验证
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
