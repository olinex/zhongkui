package llm

import (
	// stdlib
	"slices"
	"strings"

	// internal

	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/utils"
)

// ParamKind 是参数值的类型标识，决定生效值的解析方式与前端表单控件类型。
// ParamKind is the type identifier of a parameter value; it decides how an effective value
// is parsed and which form control the frontend renders.
type ParamKind string

// 参数类型常量：string（单行文本）、text（多行文本，如 prompt 原文）、int（整数）、
// float（浮点数）、bool（布尔开关）。
// Parameter kind constants: string (single-line text), text (multi-line text such as a
// prompt body), int (integer), float (floating point), bool (boolean switch).
const (
	// ParamKindString 单行字符串（分节标题、融合策略名等）
	// ParamKindString a single-line string (a section title, a fusion strategy name, ...)
	ParamKindString ParamKind = "string"
	// ParamKindText 多行文本（prompt 原文）
	// ParamKindText a multi-line text (a prompt body)
	ParamKindText ParamKind = "text"
	// ParamKindInt 整数（topK、截断长度、各类上限等）
	// ParamKindInt an integer (topK, a truncation length, any bound, ...)
	ParamKindInt ParamKind = "int"
	// ParamKindFloat 浮点数（融合权重、温度等）
	// ParamKindFloat a floating-point number (a fusion weight, a temperature, ...)
	ParamKindFloat ParamKind = "float"
	// ParamKindBool 布尔开关
	// ParamKindBool a boolean switch
	ParamKindBool ParamKind = "bool"
)

// 融合策略 retrieval.fusion 的封闭取值：注册表以此声明可选项，AI 运行时的策略判定复用
// 同一份字面量，避免两处各写一遍字符串。
// The closed value space of the fusion strategy retrieval.fusion: the registry declares the
// options from it and the AI runtime reuses the very same literals when deciding the strategy,
// so the string is never spelled twice.
const (
	// FusionRRF 倒数排名融合（Reciprocal Rank Fusion），也是未匹配到其他策略时的兜底策略
	// FusionRRF reciprocal rank fusion, also the fallback when no other strategy matches
	FusionRRF = "rrf"
	// FusionWeighted 加权融合，权重取自 retrieval.weightVector / retrieval.weightKeyword
	// FusionWeighted weighted fusion, weighted by retrieval.weightVector / retrieval.weightKeyword
	FusionWeighted = "weighted"
)

// ParamSpec 是单个可调参数的元信息，由本包注册表唯一声明：本包只持有键、类型、分组与
// 封闭取值清单，既不含任何出厂值（含提示词正文），也不含任何面向用户的文案。出厂值由
// migrate fixture 注入 agent_params.default_value，当前生效值存于同表 value 列，数据库是
// 该类值的唯一来源；展示名与说明一律经后端 i18n 提供（param.<Key>.label /
// param.<Key>.comment，Key 全局唯一因而直接复用为文案键的一部分），故本包内不出现任何
// 可翻译文案。
// ParamSpec is the metadata of a single tunable parameter, declared solely by the registry of
// this package: the package holds only the key, kind, group and closed value space — never any
// factory value (including prompt bodies) and never any user-facing copy. Factory values are
// injected by the migrate fixtures into agent_params.default_value and the effective values
// live in the value column of the same table, making the database the single source of such
// values; the display name and description always come from the backend i18n bundle
// (param.<Key>.label / param.<Key>.comment — Key is globally unique and therefore reused as
// part of the copy key), so no translatable copy ever appears inside this package.
type ParamSpec struct {
	// Key 点号分层参数键（如 agent.systemPrompt），注册表内唯一，并复用为 i18n 文案键的一部分
	// Key the dotted hierarchical parameter key (e.g. agent.systemPrompt), unique in the
	// registry and reused as part of the i18n copy key
	Key string
	// Group 前端展示分组（如 agent、retrieval、session、chunking、eval、sanitize）
	// Group the frontend display group (e.g. agent, retrieval, session, chunking, eval, sanitize)
	Group string
	// Kind 参数值类型，决定生效值的解析方式
	// Kind the parameter value kind, deciding how an effective value is parsed
	Kind ParamKind
	// Options 封闭取值清单（仅 string 类型可有）：非空时该参数只能取清单中的一个值，
	// 前端据此渲染下拉框而非自由文本框，启动期参数强校验与写入接口据此拒绝清单外的值；
	// 为空表示自由文本。清单内的字面量即库内存储值，不做任何本地化。
	// Options the closed value space (only the string kind may carry one): when non-empty the
	// parameter can only take one of the listed values, the frontend renders a select instead of
	// a free text field, and the startup parameter check as well as the write API reject values
	// outside the list; empty means free text. The listed literals are exactly the values stored
	// in the database and are never localized.
	Options []string
	// EnumType 取值所属的枚举类名（可选，只与 Options 同时出现）：非空时 Options 内的字面量即
	// 该类条目的 value，消费方据此经枚举注册表的 label 键（enum.<类名>.<key>）渲染展示文案，
	// 与全站枚举展示同源；为空时字面量本身即展示文案（如 retrieval.fusion）。
	// 类名仅为引用，取值空间仍唯一来自枚举注册表；两者是否一致由视图层守卫测试兜底。
	// EnumType the name of the enum class the values belong to (optional, only alongside
	// Options): when non-empty the literals in Options are the values of that class and consumers
	// render the display copy through the registry label key (enum.<className>.<key>), keeping
	// enum presentation single-sourced; empty means the literal itself is the display copy (as
	// with retrieval.fusion). The class name is only a reference — the value space still comes
	// solely from the enum registry — and a view-layer guard test keeps the two in step.
	EnumType string
}

// Specs 是全部可调参数的注册表，声明顺序即前端展示顺序。新增参数必须同时补齐三处，
// 缺一处都会被守卫拦下：在此登记（缺则经 Params 读取时 panic，编程缺陷 fail fast）、
// 在 migrate fixture 中注入出厂值（缺则启动期强校验中止，守卫测试更早暴露）、
// 在两份语言文件中补齐 param.<Key>.label 与 param.<Key>.comment 文案（缺则由本包的
// i18n 键完整性测试暴露）。
// Specs is the registry of every tunable parameter; the declaration order is the frontend
// display order. A new parameter must be completed in three places at once, each guarded on
// its own: registered here (a miss panics while reading through Params — a programming defect,
// fail fast), seeded with its factory value in the migrate fixtures (a miss aborts the service
// at startup and surfaces even earlier through the guard test), and given both
// param.<Key>.label and param.<Key>.comment copy in each locale file (a miss is surfaced by
// the i18n key-completeness test of this package).
var Specs = []ParamSpec{
	// ---- agent：Agent 对话的提示词、消息分节标题与上下文截断 ----
	{
		Key:   "agent.systemPrompt",
		Group: "agent",
		Kind:  ParamKindText,
	},
	{
		Key:   "agent.retrievalSectionTitle",
		Group: "agent",
		Kind:  ParamKindString,
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
		Options:  chatSlotValues(),
		EnumType: "llmSlot",
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

// chatSlotValues 把对话档位清单展开为 agent.defaultSlot 的封闭取值清单（embedding 不在其列，
// 该参数只接受对话档位）。档位成员仍只声明在 AllSlots 一处，清单由此派生，新增档位无需同步
// 修改参数注册表。
// chatSlotValues flattens the chat-slot catalog into the closed value space of
// agent.defaultSlot (embedding is not among them — the parameter only accepts chat slots). The
// slot members stay declared only in AllSlots and the list is derived from it, so adding a slot
// needs no synchronized edit of the parameter registry.
func chatSlotValues() []string {
	values := make([]string, 0, len(chatSlots))
	for _, slot := range chatSlots {
		values = append(values, string(slot))
	}
	return values
}

// Lookup 按键查询参数元信息，未声明时返回 ok=false。
// Lookup returns the parameter metadata of the key; ok is false when it is not declared.
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
// scalarKindOf maps a registry-declared parameter kind to the scalar kind kernel (string and
// text are both strings, text only marking multi-line semantics); ok is false for an
// undeclared kind.
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
// OptionAllowed reports whether the literal belongs to the closed value space of the parameter;
// an empty space means free text and always passes. This is the single implementation of the
// closed-value semantics, shared by the startup check, the write API and the fixture guard test.
func OptionAllowed(spec ParamSpec, literal string) bool {
	if len(spec.Options) == 0 {
		return true
	}
	return slices.Contains(spec.Options, literal)
}

// ValidateValue 校验字面量能否按 spec.Kind 解析、且（当参数声明了封闭取值清单时）是否落在
// 清单内，不满足时返回英文错误（含键、类型或清单与字面量）。供启动期参数强校验与 fixture
// 守卫测试复用。
// ValidateValue verifies that a literal parses as spec.Kind and — when the parameter declares a
// closed value space — falls into that space, returning an English error (carrying the key, the
// kind or the space and the literal) when it does not. It is shared by the startup parameter
// check and the fixture guard test.
func ValidateValue(spec ParamSpec, literal string) error {
	kind, ok := scalarKindOf(spec.Kind)
	if !ok {
		return errors.Newf("llm: parameter %q has an unknown kind: %q", spec.Key, string(spec.Kind))
	}
	if _, err := utils.ParseScalar(kind, literal); err != nil {
		return errors.Newf("llm: parameter %q is not a valid %s: %q", spec.Key, string(spec.Kind), literal)
	}
	if !OptionAllowed(spec, literal) {
		return errors.Newf("llm: parameter %q is not one of %s: %q", spec.Key, strings.Join(spec.Options, ", "), literal)
	}
	return nil
}

// Params 是参数生效值的不可变读取器：值全部来自 agent_params.value（出厂值由 fixture
// 注入同表 default_value），本包不持有任何出厂值，因此取不到值即视为数据未按迁移
// 种子化，属异常状态。读取映射在构造时被复制，调用方后续修改入参不影响已构造的读取器。
// Params is the immutable reader of parameter effective values: every value comes from
// agent_params.value (factory values are injected by the fixtures into default_value of the
// same table) and the package holds no factory value, so a missing value means the data was
// not seeded by the migration, an abnormal state. The value map is copied at construction,
// so later mutations of the input never affect an existing reader.
type Params struct {
	// values 生效值映射（非空字符串才算已设置）
	// values the effective value map (only a non-empty string counts as set)
	values map[string]string
}

// NewParams 构造参数读取器：空串值视为「无值」被丢弃，其余键值原样保留。注册表未声明的
// 键不会被主动读取（读取时 panic），保留与否不影响运行。
// NewParams constructs a parameter reader: empty values are treated as "no value" and
// dropped, every other key-value pair is kept as is. Keys absent from the registry are never
// read proactively (reading one panics), so keeping them is harmless.
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
// String reads the effective value of a string parameter.
func (p *Params) String(key string) string {
	p.spec(key, ParamKindString)
	return p.value(key)
}

// Text 读取 text 类型参数的生效值（多行文本，如 prompt 原文）。
// Text reads the effective value of a text parameter (a multi-line text such as a prompt
// body).
func (p *Params) Text(key string) string {
	p.spec(key, ParamKindText)
	return p.value(key)
}

// Int 读取 int 类型参数的生效值；值缺失或不可解析时 panic（启动期强校验保证正常路径
// 不可达，此处属数据被绕过接口破坏的异常状态）。
// Int reads the effective value of an int parameter; a missing or unparsable value panics
// (the startup check keeps this path unreachable in normal operation, so it indicates data
// corrupted behind the API).
func (p *Params) Int(key string) int {
	p.spec(key, ParamKindInt)
	v := p.value(key)
	parsed, err := utils.ParseScalar(utils.ScalarKindInt, v)
	if err != nil {
		panic("llm: parameter " + key + " is not a valid int: " + v)
	}
	return parsed.(int)
}

// Float 读取 float 类型参数的生效值；值缺失或不可解析时 panic（同 Int）。
// Float reads the effective value of a float parameter; a missing or unparsable value panics
// (as with Int).
func (p *Params) Float(key string) float64 {
	p.spec(key, ParamKindFloat)
	v := p.value(key)
	parsed, err := utils.ParseScalar(utils.ScalarKindFloat, v)
	if err != nil {
		panic("llm: parameter " + key + " is not a valid float: " + v)
	}
	return parsed.(float64)
}

// Bool 读取 bool 类型参数的生效值（接受 true/false/1/0 等 strconv 可解析形式）；
// 值缺失或不可解析时 panic（同 Int）。
// Bool reads the effective value of a bool parameter (accepting any strconv-parsable form
// such as true/false/1/0); a missing or unparsable value panics (as with Int).
func (p *Params) Bool(key string) bool {
	p.spec(key, ParamKindBool)
	v := p.value(key)
	parsed, err := utils.ParseScalar(utils.ScalarKindBool, v)
	if err != nil {
		panic("llm: parameter " + key + " is not a valid bool: " + v)
	}
	return parsed.(bool)
}

// spec 返回键的注册元信息并校验声明类型：键未声明或类型不符均视为编程缺陷，
// 直接 panic（fail fast），避免静默读到错误语义的值。
// spec returns the registered metadata of the key and validates the expected kind: an
// undeclared key or a kind mismatch is a programming defect and panics (fail fast) instead
// of silently reading a value with the wrong semantics.
func (p *Params) spec(key string, kind ParamKind) ParamSpec {
	spec, ok := Lookup(key)
	if !ok {
		panic("llm: undeclared parameter key: " + key)
	}
	if spec.Kind != kind {
		panic("llm: parameter " + key + " is " + string(spec.Kind) + ", not " + string(kind))
	}
	return spec
}

// value 返回键的生效值；缺失或为空串时 panic，因为本包已无出厂值可回落（数据须由
// migrate fixture 种子化，启动期强校验保证每个注册表键都有非空值）。
// value returns the effective value of the key; a missing or empty value panics because this
// package holds no factory value to fall back on (the data must be seeded by the migrate
// fixtures and the startup check guarantees every registry key carries a non-empty value).
func (p *Params) value(key string) string {
	if p != nil && p.values != nil {
		if v, ok := p.values[key]; ok {
			return v
		}
	}
	panic("llm: parameter " + key + " has no value in the database")
}

// init 校验注册表自身一致性（fail fast）：键非空且唯一、分组非空、类型合法、封闭取值清单
// 仅出现在 string 类型上且条目非空唯一、枚举类引用必须伴随封闭取值清单。任一项不满足即
// panic，避免非法注册表在生产中以残缺形态运行。出厂值的合法性由启动期强校验（ai 运行时
// 装配读取 agent_params 时逐键校验，含清单内校验）与 fixture 守卫测试共同保证。
// init validates the internal consistency of the registry (fail fast): keys are non-empty and
// unique, groups are non-empty, kinds are known, a closed value space only appears on the
// string kind with non-empty unique entries, and an enum-class reference always comes with a
// closed value space. Any violation panics so an invalid registry can never run in a crippled
// form in production. The validity of factory values is guaranteed by the startup check (the ai
// runtime validates every key while reading agent_params, membership included) together with the
// fixture guard test.
func init() {
	seen := make(map[string]struct{}, len(Specs))
	for _, spec := range Specs {
		if spec.Key == "" {
			panic("llm: parameter spec with an empty key")
		}
		if spec.Group == "" {
			panic("llm: parameter " + spec.Key + " has an empty group")
		}
		if _, dup := seen[spec.Key]; dup {
			panic("llm: duplicate parameter key: " + spec.Key)
		}
		seen[spec.Key] = struct{}{}
		switch spec.Kind {
		case ParamKindString, ParamKindText, ParamKindInt, ParamKindFloat, ParamKindBool:
		default:
			panic("llm: parameter " + spec.Key + " has an unknown kind: " + string(spec.Kind))
		}
		if len(spec.Options) == 0 {
			if spec.EnumType != "" {
				panic("llm: parameter " + spec.Key + " declares an enum type without options")
			}
			continue
		}
		if spec.Kind != ParamKindString {
			panic("llm: parameter " + spec.Key + " declares options on the non-string kind " + string(spec.Kind))
		}
		options := make(map[string]struct{}, len(spec.Options))
		for _, option := range spec.Options {
			if option == "" {
				panic("llm: parameter " + spec.Key + " has an empty option")
			}
			if _, dup := options[option]; dup {
				panic("llm: parameter " + spec.Key + " has duplicate option: " + option)
			}
			options[option] = struct{}{}
		}
	}
}
