package config

import (
	// 标准库
	"slices"
	"strings"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/utils"
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
	// agent.generalSystemPrompt 通用对话 Agent（general 类型）的系统提示词，要求直接输出 Markdown 正文。
	{
		Key:   "agent.generalSystemPrompt",
		Group: "agent",
		Kind:  ParamKindText,
	},
	// agent.auditLevelSystemPrompt 客户端工具调用「审核重要级」评估器的系统提示词：在 client 工具
	// 挂起前对本次全部待执行调用做一次批量分类，产出四档重要级，要求仅输出 JSON。
	{
		Key:   "agent.auditLevelSystemPrompt",
		Group: "agent",
		Kind:  ParamKindText,
	},
	// agent.promptOptimizeSystemPrompt 提示词优化（把用户诉求改写为更清晰提问）的系统提示词：
	// 单轮调用，要求仅输出改写后的提问正文本身。
	{
		Key:   "agent.promptOptimizeSystemPrompt",
		Group: "agent",
		Kind:  ParamKindText,
	},
	// agent.nextStepsSystemPrompt 回答完成后生成「推荐的下一步」的系统提示词：基于整体上下文给出
	// 1-3 条按推荐程度排序的可直接发送诉求，要求仅输出 JSON。
	{
		Key:   "agent.nextStepsSystemPrompt",
		Group: "agent",
		Kind:  ParamKindText,
	},
	// agent.nextStepsEnabled 是否在每条回答完成后自动生成「推荐的下一步」；关闭后不生成、不展示。
	{
		Key:   "agent.nextStepsEnabled",
		Group: "agent",
		Kind:  ParamKindBool,
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
	// agent.maxToolRounds 工具调用循环的最大轮数：单次自主运行（生成 -> 执行工具 -> 再生成）的
	// 循环上限；人工审批 / 问卷挂起后的续跑重新计数，故同一轮对话内多次交互不会耗尽额度。
	// 达到上限仍存在未完成的工具调用时以该轮文本作为答案。
	{
		Key:   "agent.maxToolRounds",
		Group: "agent",
		Kind:  ParamKindInt,
	},
	// agent.maxToolResultBytes 单次工具结果注入模型上下文前的字节上限：超出部分被截断，
	// 用于限制工具结果对上下文的占用。默认值须容纳「整篇展开的知识文档 + 其余命中」，
	// 与 retrieval.expandDocLimit 保持量级一致（见该参数说明）。
	{
		Key:   "agent.maxToolResultBytes",
		Group: "agent",
		Kind:  ParamKindInt,
	},
	// agent.runDetachGraceSeconds 驻留运行退订宽限秒数：某 running 运行的最后一名订阅者退订后，
	// 经过该宽限期仍无订阅者的运行被自动取消（避免无人观看仍持续烧 token）；<=0 表示关闭自动取消。
	{
		Key:   "agent.runDetachGraceSeconds",
		Group: "agent",
		Kind:  ParamKindInt,
	},
	// agent.llmTimeoutRetryEnabled 非流式模型调用（记忆抽取、审核分级等 Generate 链路）
	// 是否在超时时自动重试；流式对话不施加整请求超时，不受该开关影响。
	{
		Key:   "agent.llmTimeoutRetryEnabled",
		Group: "agent",
		Kind:  ParamKindBool,
	},
	// agent.llmTimeoutRetryMaxRetries 超时重试的最大次数（不含首次请求）；每次重试的截止时间
	// 按 agent.llmTimeoutRetryMultiplier 逐次放大。
	{
		Key:   "agent.llmTimeoutRetryMaxRetries",
		Group: "agent",
		Kind:  ParamKindInt,
	},
	// agent.llmTimeoutRetryMultiplier 超时重试的截止时间倍数：首次沿用模型单次请求超时，
	// 第 n 次重试使用「模型超时 × 倍数^n」；<=0 时按 2 处理。
	{
		Key:   "agent.llmTimeoutRetryMultiplier",
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
	// retrieval.expandDocLimit 一次检索中整篇展开（small-to-big，父块 = 整篇文档）的累计长度上限
	// （rune 数）：按结果顺序逐篇展开，单篇超过该值、或展开后累计超过该值的文档降级保持原分块结果
	// （绝不截断整篇），使一次检索注入模型的整篇内容有确定上界；非正数表示关闭整篇展开。
	// 默认值须覆盖代码生成的接口知识文档规模，且与 agent.maxToolResultBytes 匹配。
	{
		Key:   "retrieval.expandDocLimit",
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
	// session.compactThresholdRatio 触发自动压缩的占用比例阈值（占用 token / 档位容量）。
	{
		Key:   "session.compactThresholdRatio",
		Group: "session",
		Kind:  ParamKindFloat,
	},
	// session.topicSystemPrompt 会话短主题生成的系统提示词。
	{
		Key:   "session.topicSystemPrompt",
		Group: "session",
		Kind:  ParamKindText,
	},
	// session.maxUserTurns 会话的交互轮次上限：有效轮次 = 会话内当前存在的用户提问条数（role=user，
	// 被回滚删除的不计入），达到该上限即自动锁定、拒绝新提问；回滚使轮次降到上限以下即自动解锁。
	// <=0 视为不设上限（永不锁定）。锁定是 StateOf 的全函数推导结果，不写任何状态列。
	{
		Key:   "session.maxUserTurns",
		Group: "session",
		Kind:  ParamKindInt,
	},

	// ---- memory：长期记忆与会话摘要 ----
	// memory.enabled 长期记忆总开关：关闭时跳过记忆检索与注入，且不报错。
	{
		Key:   "memory.enabled",
		Group: "memory",
		Kind:  ParamKindBool,
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
	// memory.evictedMessageByteLimit 滚动摘要与记忆抽取提示词中单条被折叠消息正文的字节上限。
	{
		Key:   "memory.evictedMessageByteLimit",
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

// ValidateRelations 校验跨键的参数关系约束：单键类型与封闭取值由 ValidateValue 保证，本函数
// 只处理单个键无法表达的约束，供启动期强校验在构造读取器前调用。缺失的键跳过（部分键的测试
// 场景），存在但不可解析的键仍返回错误。当前覆盖：
//   - retrieval.weightVector / retrieval.weightKeyword 均非负且不同时为零：weighted 融合按
//     两者加权，负权重会让排序失去意义，双零会使所有融合分为零；
//   - agent.maxToolResultBytes 与 retrieval.expandDocLimit 同时为正时，前者不得小于后者：
//     整篇展开按 rune 计数、工具结果按字节截断，前者至少须容纳「1 字节 / rune」的下界，
//     否则整篇展开的结果必然在回灌前被截断成非法 JSON。
func ValidateRelations(values map[string]string) error {
	vector, hasVector, err := floatValue(values, "retrieval.weightVector")
	if err != nil {
		return errors.WithStack(err)
	}
	keyword, hasKeyword, err := floatValue(values, "retrieval.weightKeyword")
	if err != nil {
		return errors.WithStack(err)
	}
	if hasVector && hasKeyword {
		if vector < 0 || keyword < 0 {
			return errors.Newf("ai: retrieval fusion weights must be non-negative: weightVector=%g, weightKeyword=%g", vector, keyword)
		}
		if vector+keyword <= 0 {
			return errors.New("ai: retrieval fusion weights must not both be zero")
		}
	}
	maxToolBytes, hasMaxToolBytes, err := intValue(values, "agent.maxToolResultBytes")
	if err != nil {
		return errors.WithStack(err)
	}
	expandRunes, hasExpandRunes, err := intValue(values, "retrieval.expandDocLimit")
	if err != nil {
		return errors.WithStack(err)
	}
	if hasMaxToolBytes && hasExpandRunes && maxToolBytes > 0 && expandRunes > 0 && maxToolBytes < expandRunes {
		return errors.Newf("ai: agent.maxToolResultBytes (%d) must be at least retrieval.expandDocLimit (%d) when both are positive", maxToolBytes, expandRunes)
	}
	return nil
}

// floatValue 读取并解析 float 参数字面量；键缺失或为空时 present=false。
func floatValue(values map[string]string, key string) (float64, bool, error) {
	literal, ok := values[key]
	if !ok || literal == "" {
		return 0, false, nil
	}
	parsed, err := utils.ParseScalar(utils.ScalarKindFloat, literal)
	if err != nil {
		return 0, false, errors.Newf("ai: parameter %q is not a valid float: %q", key, literal)
	}
	return parsed.(float64), true, nil
}

// intValue 读取并解析 int 参数字面量；键缺失或为空时 present=false。
func intValue(values map[string]string, key string) (int, bool, error) {
	literal, ok := values[key]
	if !ok || literal == "" {
		return 0, false, nil
	}
	parsed, err := utils.ParseScalar(utils.ScalarKindInt, literal)
	if err != nil {
		return 0, false, errors.Newf("ai: parameter %q is not a valid int: %q", key, literal)
	}
	return parsed.(int), true, nil
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
