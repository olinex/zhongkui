package ai

import (
	// 标准库
	"slices"
	"strings"

	// 项目内部
	"zhongkui.com/zhongkui/internal/models/ai"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/utils"
)

// ParamKind 是参数值的类型标识，决定生效值的解析方式与前端表单控件类型。
type ParamKind string

// 参数类型常量：string（单行文本）、text（多行文本，如 prompt 原文）、int（整数）、
// float（浮点数）、bool（布尔开关）。
const (
	// ParamKindString 单行字符串（分节标题、融合策略名等）
	ParamKindString ParamKind = "string"
	// ParamKindText 多行文本（prompt 原文）
	ParamKindText ParamKind = "text"
	// ParamKindInt 整数（topK、截断长度、各类上限等）
	ParamKindInt ParamKind = "int"
	// ParamKindFloat 浮点数（融合权重、温度等）
	ParamKindFloat ParamKind = "float"
	// ParamKindBool 布尔开关
	ParamKindBool ParamKind = "bool"
)

// 融合策略 retrieval.fusion 的封闭取值：注册表以此声明可选项，AI 运行时的策略判定复用
// 同一份字面量，避免两处各写一遍字符串。
const (
	// FusionRRF 倒数排名融合（Reciprocal Rank Fusion），也是未匹配到其他策略时的兜底策略
	FusionRRF = "rrf"
	// FusionWeighted 加权融合，权重取自 retrieval.weightVector / retrieval.weightKeyword
	FusionWeighted = "weighted"
)

// ParamSpec 是单个可调参数的元信息，由本包注册表唯一声明：本包只持有键、类型、分组与
// 封闭取值清单，既不含任何出厂值（含提示词正文），也不含任何面向用户的文案。出厂值由
// migrate fixture 注入 agent_params.default_value，当前生效值存于同表 value 列，数据库是
// 该类值的唯一来源；展示名与说明一律经后端 i18n 提供（param.<Key>.label /
// param.<Key>.comment，Key 全局唯一因而直接复用为文案键的一部分），故本包内不出现任何
// 可翻译文案。
type ParamSpec struct {
	// Key 点号分层参数键（如 agent.systemPrompt），注册表内唯一，并复用为 i18n 文案键的一部分
	Key string
	// Group 前端展示分组（如 agent、retrieval、session、chunking、eval、sanitize）
	Group string
	// Kind 参数值类型，决定生效值的解析方式
	Kind ParamKind
	// Options 封闭取值清单（仅 string 类型可有）：非空时该参数只能取清单中的一个值，
	// 前端据此渲染下拉框而非自由文本框，启动期参数强校验与写入接口据此拒绝清单外的值；
	// 为空表示自由文本。清单内的字面量即库内存储值，不做任何本地化。
	Options []string
	// EnumType 取值所属的枚举类名（可选，只与 Options 同时出现）：非空时 Options 内的字面量即
	// 该类条目的 value，消费方据此经枚举注册表的 label 键（enum.<类名>.<key>）渲染展示文案，
	// 与全站枚举展示同源；为空时字面量本身即展示文案（如 retrieval.fusion）。
	// 类名仅为引用，取值空间仍唯一来自枚举注册表；两者是否一致由视图层守卫测试兜底。
	EnumType string
}

// Specs 是全部可调参数的注册表，声明顺序即前端展示顺序。新增参数必须同时补齐三处，
// 缺一处都会被守卫拦下：在此登记（缺则经 Params 读取时 panic，编程缺陷 fail fast）、
// 在 migrate fixture 中注入出厂值（缺则启动期强校验中止，守卫测试更早暴露）、
// 在两份语言文件中补齐 param.<Key>.label 与 param.<Key>.comment 文案（缺则由视图层的
// i18n 键完整性测试暴露）。
var Specs = []ParamSpec{
	// ---- agent：Agent 对话的提示词、消息分节标题与上下文截断 ----
	{
		Key:   "agent.systemPrompt",
		Group: "agent",
		Kind:  ParamKindText,
	},
	{
		Key:   "agent.businessSectionTitle",
		Group: "agent",
		Kind:  ParamKindString,
	},
	{
		Key:   "agent.questionSectionTitle",
		Group: "agent",
		Kind:  ParamKindString,
	},
	{
		Key:   "agent.retrievedDocSnippetLimit",
		Group: "agent",
		Kind:  ParamKindInt,
	},
	{
		Key:      "agent.defaultSlot",
		Group:    "agent",
		Kind:     ParamKindString,
		Options:  ai.ChatSlotValues(),
		EnumType: "llmSlot",
	},
	// agent.maxToolRounds 工具调用循环的最大轮数：单轮对话内「生成 -> 执行工具 -> 再生成」
	// 的循环上限，达到上限仍存在未完成的工具调用时以该轮文本作为答案。
	{
		Key:   "agent.maxToolRounds",
		Group: "agent",
		Kind:  ParamKindInt,
	},
	// agent.maxToolResultBytes 单次工具结果注入模型上下文前的字节上限：超出部分被截断，
	// 用于限制工具结果对上下文的占用。
	{
		Key:   "agent.maxToolResultBytes",
		Group: "agent",
		Kind:  ParamKindInt,
	},

	// ---- retrieval：混合检索融合与 LLM 重排 ----
	{
		Key:   "retrieval.topK",
		Group: "retrieval",
		Kind:  ParamKindInt,
	},
	{
		Key:   "retrieval.candidates",
		Group: "retrieval",
		Kind:  ParamKindInt,
	},
	{
		Key:     "retrieval.fusion",
		Group:   "retrieval",
		Kind:    ParamKindString,
		Options: []string{FusionRRF, FusionWeighted},
	},
	{
		Key:   "retrieval.weightVector",
		Group: "retrieval",
		Kind:  ParamKindFloat,
	},
	{
		Key:   "retrieval.weightKeyword",
		Group: "retrieval",
		Kind:  ParamKindFloat,
	},
	{
		Key:   "retrieval.rerankSystemPrompt",
		Group: "retrieval",
		Kind:  ParamKindText,
	},
	{
		Key:   "retrieval.rerankSnippetLimit",
		Group: "retrieval",
		Kind:  ParamKindInt,
	},
	{
		Key:   "retrieval.rerankTemperature",
		Group: "retrieval",
		Kind:  ParamKindFloat,
	},

	// ---- session：会话上下文注入上限 ----
	{
		Key:   "session.maxContextMessages",
		Group: "session",
		Kind:  ParamKindInt,
	},
	{
		Key:   "session.maxContextBytes",
		Group: "session",
		Kind:  ParamKindInt,
	},
	// session.topicSystemPrompt 会话短主题生成的系统提示词。
	{
		Key:   "session.topicSystemPrompt",
		Group: "session",
		Kind:  ParamKindText,
	},

	// ---- memory：长期记忆与会话摘要 ----
	// memory.enabled 长期记忆总开关：关闭时跳过记忆检索与注入，且不报错。
	{
		Key:   "memory.enabled",
		Group: "memory",
		Kind:  ParamKindBool,
	},
	// memory.sectionTitle 记忆注入时使用的分节标题。
	{
		Key:   "memory.sectionTitle",
		Group: "memory",
		Kind:  ParamKindString,
	},
	// memory.topK 注入提示词的记忆条数。
	{
		Key:   "memory.topK",
		Group: "memory",
		Kind:  ParamKindInt,
	},
	// memory.candidates 向量路与关键词路各自的候选条数。
	{
		Key:   "memory.candidates",
		Group: "memory",
		Kind:  ParamKindInt,
	},
	// memory.maxItems 注入条数的硬上限。
	{
		Key:   "memory.maxItems",
		Group: "memory",
		Kind:  ParamKindInt,
	},
	// memory.maxItemBytes 单条记忆注入的字节上限，超出部分被截断。
	{
		Key:   "memory.maxItemBytes",
		Group: "memory",
		Kind:  ParamKindInt,
	},
	// memory.dedupeThreshold 语义去重/合并阈值（余弦相似度）。
	{
		Key:   "memory.dedupeThreshold",
		Group: "memory",
		Kind:  ParamKindFloat,
	},
	// memory.summarySystemPrompt 会话滚动摘要的系统提示词。
	{
		Key:   "memory.summarySystemPrompt",
		Group: "memory",
		Kind:  ParamKindText,
	},
	// memory.summaryMinEvictedMessages 触发滚动摘要的最少未摘要消息条数。
	{
		Key:   "memory.summaryMinEvictedMessages",
		Group: "memory",
		Kind:  ParamKindInt,
	},
	// memory.extractSystemPrompt 压缩时抽取长期记忆的系统提示词。
	{
		Key:   "memory.extractSystemPrompt",
		Group: "memory",
		Kind:  ParamKindText,
	},
	// memory.extractMaxItems 单次压缩最多抽取的记忆条数。
	{
		Key:   "memory.extractMaxItems",
		Group: "memory",
		Kind:  ParamKindInt,
	},

	// ---- chunking：Markdown 结构感知切片 ----
	{
		Key:   "chunking.size",
		Group: "chunking",
		Kind:  ParamKindInt,
	},
	{
		Key:   "chunking.overlap",
		Group: "chunking",
		Kind:  ParamKindInt,
	},

	// ---- eval：RAG 评估 judge 提示词与参数 ----
	{
		Key:   "eval.answerSystemPrompt",
		Group: "eval",
		Kind:  ParamKindText,
	},
	{
		Key:   "eval.chunkRelevanceSystemPrompt",
		Group: "eval",
		Kind:  ParamKindText,
	},
	{
		Key:   "eval.claimsSystemPrompt",
		Group: "eval",
		Kind:  ParamKindText,
	},
	{
		Key:   "eval.reverseQuestionsSystemPrompt",
		Group: "eval",
		Kind:  ParamKindText,
	},
	// eval.suggestionsSystemPrompt 生成检索试运行的改进建议（覆盖不足 / 排序调参 / 生成质量等）。
	{
		Key:   "eval.suggestionsSystemPrompt",
		Group: "eval",
		Kind:  ParamKindText,
	},
	{
		Key:   "eval.reverseQuestionCount",
		Group: "eval",
		Kind:  ParamKindInt,
	},
	{
		Key:   "eval.snippetLimit",
		Group: "eval",
		Kind:  ParamKindInt,
	},

	// ---- sanitize：业务数据注入前的清洗限界 ----
	{
		Key:   "sanitize.maxItems",
		Group: "sanitize",
		Kind:  ParamKindInt,
	},
	{
		Key:   "sanitize.maxBytes",
		Group: "sanitize",
		Kind:  ParamKindInt,
	},
}

// Lookup 按键查询参数元信息，未声明时返回 ok=false。
func Lookup(key string) (ParamSpec, bool) {
	for _, spec := range Specs {
		if spec.Key == key {
			return spec, true
		}
	}
	return ParamSpec{}, false
}

// scalarKindOf 把注册表声明的参数类型映射为标量类型内核（string 与 text 同属字符串，
// text 仅表示多行文本语义）；未声明的类型返回 ok=false。
func scalarKindOf(kind ParamKind) (utils.ScalarKind, bool) {
	switch kind {
	case ParamKindString, ParamKindText:
		return utils.ScalarKindString, true
	case ParamKindInt:
		return utils.ScalarKindInt, true
	case ParamKindFloat:
		return utils.ScalarKindFloat, true
	case ParamKindBool:
		return utils.ScalarKindBool, true
	default:
		return "", false
	}
}

// OptionAllowed 判断字面量是否属于参数的封闭取值清单；清单为空表示自由文本，一律通过。
// 参数的封闭语义只有这一处实现，启动期强校验、写入接口与 fixture 守卫测试共用。
func OptionAllowed(spec ParamSpec, literal string) bool {
	if len(spec.Options) == 0 {
		return true
	}
	return slices.Contains(spec.Options, literal)
}

// ValidateValue 校验字面量能否按 spec.Kind 解析、且（当参数声明了封闭取值清单时）是否落在
// 清单内，不满足时返回英文错误（含键、类型或清单与字面量）。供启动期参数强校验与 fixture
// 守卫测试复用。
func ValidateValue(spec ParamSpec, literal string) error {
	kind, ok := scalarKindOf(spec.Kind)
	if !ok {
		return errors.Newf("ai: parameter %q has an unknown kind: %q", spec.Key, string(spec.Kind))
	}
	if _, err := utils.ParseScalar(kind, literal); err != nil {
		return errors.Newf("ai: parameter %q is not a valid %s: %q", spec.Key, string(spec.Kind), literal)
	}
	if !OptionAllowed(spec, literal) {
		return errors.Newf("ai: parameter %q is not one of %s: %q", spec.Key, strings.Join(spec.Options, ", "), literal)
	}
	return nil
}

// Params 是参数生效值的不可变读取器：值全部来自 agent_params.value（出厂值由 fixture
// 注入同表 default_value），本包不持有任何出厂值，因此取不到值即视为数据未按迁移
// 种子化，属异常状态。读取映射在构造时被复制，调用方后续修改入参不影响已构造的读取器。
type Params struct {
	// values 生效值映射（非空字符串才算已设置）
	values map[string]string
}

// NewParams 构造参数读取器：空串值视为「无值」被丢弃，其余键值原样保留。注册表未声明的
// 键不会被主动读取（读取时 panic），保留与否不影响运行。
func NewParams(values map[string]string) *Params {
	cp := make(map[string]string, len(values))
	for key, value := range values {
		if value == "" {
			continue
		}
		cp[key] = value
	}
	return &Params{values: cp}
}

// String 读取 string 类型参数的生效值。
func (p *Params) String(key string) string {
	p.spec(key, ParamKindString)
	return p.value(key)
}

// Text 读取 text 类型参数的生效值（多行文本，如 prompt 原文）。
func (p *Params) Text(key string) string {
	p.spec(key, ParamKindText)
	return p.value(key)
}

// Int 读取 int 类型参数的生效值；值缺失或不可解析时 panic（启动期强校验保证正常路径
// 不可达，此处属数据被绕过接口破坏的异常状态）。
func (p *Params) Int(key string) int {
	p.spec(key, ParamKindInt)
	v := p.value(key)
	parsed, err := utils.ParseScalar(utils.ScalarKindInt, v)
	if err != nil {
		panic("ai: parameter " + key + " is not a valid int: " + v)
	}
	return parsed.(int)
}

// Float 读取 float 类型参数的生效值；值缺失或不可解析时 panic（同 Int）。
func (p *Params) Float(key string) float64 {
	p.spec(key, ParamKindFloat)
	v := p.value(key)
	parsed, err := utils.ParseScalar(utils.ScalarKindFloat, v)
	if err != nil {
		panic("ai: parameter " + key + " is not a valid float: " + v)
	}
	return parsed.(float64)
}

// Bool 读取 bool 类型参数的生效值（接受 true/false/1/0 等 strconv 可解析形式）；
// 值缺失或不可解析时 panic（同 Int）。
func (p *Params) Bool(key string) bool {
	p.spec(key, ParamKindBool)
	v := p.value(key)
	parsed, err := utils.ParseScalar(utils.ScalarKindBool, v)
	if err != nil {
		panic("ai: parameter " + key + " is not a valid bool: " + v)
	}
	return parsed.(bool)
}

// spec 返回键的注册元信息并校验声明类型：键未声明或类型不符均视为编程缺陷，
// 直接 panic（fail fast），避免静默读到错误语义的值。
func (p *Params) spec(key string, kind ParamKind) ParamSpec {
	spec, ok := Lookup(key)
	if !ok {
		panic("ai: undeclared parameter key: " + key)
	}
	if spec.Kind != kind {
		panic("ai: parameter " + key + " is " + string(spec.Kind) + ", not " + string(kind))
	}
	return spec
}

// value 返回键的生效值；缺失或为空串时 panic，因为本包已无出厂值可回落（数据须由
// migrate fixture 种子化，启动期强校验保证每个注册表键都有非空值）。
func (p *Params) value(key string) string {
	if p != nil && p.values != nil {
		if v, ok := p.values[key]; ok {
			return v
		}
	}
	panic("ai: parameter " + key + " has no value in the database")
}

// init 校验注册表自身一致性（fail fast）：键非空且唯一、分组非空、类型合法、封闭取值清单
// 仅出现在 string 类型上且条目非空唯一、枚举类引用必须伴随封闭取值清单。任一项不满足即
// panic，避免非法注册表在生产中以残缺形态运行。出厂值的合法性由启动期强校验（本包运行时
// 装配读取 agent_params 时逐键校验，含清单内校验）与 fixture 守卫测试共同保证。
func init() {
	seen := make(map[string]struct{}, len(Specs))
	for _, spec := range Specs {
		if spec.Key == "" {
			panic("ai: parameter spec with an empty key")
		}
		if spec.Group == "" {
			panic("ai: parameter " + spec.Key + " has an empty group")
		}
		if _, dup := seen[spec.Key]; dup {
			panic("ai: duplicate parameter key: " + spec.Key)
		}
		seen[spec.Key] = struct{}{}
		switch spec.Kind {
		case ParamKindString, ParamKindText, ParamKindInt, ParamKindFloat, ParamKindBool:
		default:
			panic("ai: parameter " + spec.Key + " has an unknown kind: " + string(spec.Kind))
		}
		if len(spec.Options) == 0 {
			if spec.EnumType != "" {
				panic("ai: parameter " + spec.Key + " declares an enum type without options")
			}
			continue
		}
		if spec.Kind != ParamKindString {
			panic("ai: parameter " + spec.Key + " declares options on the non-string kind " + string(spec.Kind))
		}
		options := make(map[string]struct{}, len(spec.Options))
		for _, option := range spec.Options {
			if option == "" {
				panic("ai: parameter " + spec.Key + " has an empty option")
			}
			if _, dup := options[option]; dup {
				panic("ai: parameter " + spec.Key + " has duplicate option: " + option)
			}
			options[option] = struct{}{}
		}
	}
}
