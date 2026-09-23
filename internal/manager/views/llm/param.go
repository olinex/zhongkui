package llm

import (
	// stdlib
	"fmt"
	"strings"

	// third-party
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	// internal

	"zhongkui.com/zhongkui/internal/ai"
	"zhongkui.com/zhongkui/internal/manager/middlewares"
	"zhongkui.com/zhongkui/internal/manager/response"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	pkgllm "zhongkui.com/zhongkui/internal/pkg/llm"
	"zhongkui.com/zhongkui/internal/pkg/translate"
	"zhongkui.com/zhongkui/internal/pkg/utils"
)

// LlmParamItemOut 定义单个 Agent 参数的元信息、出厂值、当前生效值与偏离态。
// Key / Group / Kind 来自代码内参数注册表（唯一事实来源）；Default 为库内出厂值
// （agent_params.default_value，只由 migrate fixture 维护）；Value 为库内当前生效值
// （agent_params.value）；Label / Comment 跟随请求语言。
type LlmParamItemOut struct {
	// Key 点号分层参数键（如 agent.systemPrompt），注册表内唯一
	Key string `json:"key"`
	// Group 展示分组（agent / retrieval / session / chunking / eval / sanitize）
	Group string `json:"group"`
	// Kind 参数值类型：string | text | int | float | bool
	Kind string `json:"kind"`
	// Default 出厂值（库内 default_value，按 Kind 解析为对应 JSON 类型，避免前端二次解析）
	Default any `json:"default"`
	// Value 当前生效值（库内 value，按 Kind 解析为对应 JSON 类型）
	Value any `json:"value"`
	// Overridden 当前生效值是否已偏离出厂值（value != default_value）
	Overridden bool `json:"overridden"`
	// Label 参数展示名称（i18n 键 param.<Key>.label，未命中回退参数短名）
	Label string `json:"label"`
	// Comment 参数说明（i18n 键 param.<Key>.comment，语言文件为唯一来源，未命中回退键本身）
	Comment string `json:"comment"`
	// Options 封闭取值清单（仅 string 类型参数可能非空）：非空时前端渲染下拉框，
	// 清单内字面量即库内存储值、不做本地化；为空表示自由文本
	Options []string `json:"options"`
	// EnumType 取值所属的枚举类名（仅与 Options 同时非空）：前端据此经枚举注册表的
	// label 键（enum.<类名>.<key>）渲染选项文案，与全站枚举展示同源；为空时字面量即文案
	EnumType string `json:"enumType"`
}

// LlmParamListOut 定义 Agent 参数列表的输出结构。
// Items 按注册表声明顺序（即前端展示顺序）返回全部可调参数。
type LlmParamListOut struct {
	// Items 全部可调参数的元信息与当前值
	Items []LlmParamItemOut `json:"items"`
}

// LlmParamSetItemIn 定义单条参数输入：Value 为 JSON 原生类型（字符串 / 数值 / 布尔）。
// Value 为 null 或空字符串表示恢复出厂值（把当前值写回 default_value）。
type LlmParamSetItemIn struct {
	// Key 参数键，必须是注册表已声明的键
	Key string `json:"key"`
	// Value 当前生效值，按注册表声明的类型校验；null 或空串表示恢复出厂值
	Value any `json:"value"`
}

// LlmParamSetIn 定义批量写入 Agent 参数的请求输入结构。
type LlmParamSetIn struct {
	// Items 待写入的参数项列表
	Items []LlmParamSetItemIn `json:"items"`
}

// LlmParamResetIn 定义恢复 Agent 参数出厂值的请求输入结构。
// Group 与 Keys 二选一：Group 优先（恢复该分组全部参数），两者皆空时拒绝。
type LlmParamResetIn struct {
	// Keys 待恢复出厂值的参数键列表（Group 为空时生效）
	Keys []string `json:"keys"`
	// Group 待恢复出厂值的展示分组，恢复该分组下的全部参数
	Group string `json:"group"`
}

// LlmParamListGet 处理获取 Agent 参数列表的请求。
// 返回注册表全部参数的元信息、库内出厂值与当前生效值，并标注当前值是否已偏离出厂值；
// label / comment 跟随请求语言（i18n 键 param.<group>.<name>.label / .comment）。
func LlmParamListGet(c *gin.Context) {
	var rows []core.AgentParam
	if err := middlewares.GetCoreDatabase(c).Find(&rows).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 每个注册表键恰有一条记录（由启动期强校验保证）；缺失时按零值展示，不 panic
	byKey := make(map[string]core.AgentParam, len(rows))
	for i := range rows {
		byKey[rows[i].Key] = rows[i]
	}
	lang := middlewares.GetLanguage(c)
	items := make([]LlmParamItemOut, 0, len(pkgllm.Specs))
	for _, spec := range pkgllm.Specs {
		row := byKey[spec.Key]
		items = append(items, LlmParamItemOut{
			Key:        spec.Key,
			Group:      spec.Group,
			Kind:       string(spec.Kind),
			Default:    paramJSONValue(spec, row.DefaultValue),
			Value:      paramJSONValue(spec, row.Value),
			Overridden: row.Value != row.DefaultValue,
			Label:      paramLabel(lang, spec),
			Comment:    paramComment(lang, spec),
			Options:    spec.Options,
			EnumType:   spec.EnumType,
		})
	}
	response.SimpleOK.AbortWithGinW(c, LlmParamListOut{Items: items})
}

// LlmParamSet 处理批量写入 Agent 参数的请求。
// 先全量校验（键必须已声明、值必须符合注册表声明的类型），校验通过后再逐条写入，
// 避免部分写入后因后续条目非法而中断；空值等价于恢复出厂值（把当前值写回
// default_value），与「参数记录恒存在」的启动期强校验保持一致。
func LlmParamSet(c *gin.Context) {
	var in LlmParamSetIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	if len(in.Items) == 0 {
		response.CreateBadBodyError(errors.New("items must not be empty")).AbortWithGinH(c)
		return
	}
	type paramUpdate struct {
		key   string
		value string
	}
	updates := make([]paramUpdate, 0, len(in.Items))
	for _, item := range in.Items {
		spec, ok := pkgllm.Lookup(item.Key)
		if !ok {
			response.CreateBadBodyError(errors.Newf("parameter %q is not declared in the registry", item.Key)).AbortWithGinH(c)
			return
		}
		value, err := normalizeParamValue(spec, item.Value)
		if err != nil {
			response.CreateBadBodyError(err).AbortWithGinH(c)
			return
		}
		updates = append(updates, paramUpdate{key: spec.Key, value: value})
	}
	db := middlewares.GetCoreDatabase(c)
	for _, update := range updates {
		if update.value == "" {
			if err := resetAgentParam(db, []string{update.key}); err != nil {
				response.CreateAccidentError(err).AbortWithGinH(c)
				return
			}
			continue
		}
		if err := upsertAgentParam(db, update.key, update.value); err != nil {
			response.CreateAccidentError(err).AbortWithGinH(c)
			return
		}
	}
	afterConfigWrite(c, ai.ChangeKindParam, fmt.Sprintf("%d parameters", len(updates)))
	response.SimpleOK.AbortWithGinH(c)
}

// LlmParamReset 处理恢复 Agent 参数出厂值的请求。
// 按分组（Group 优先，映射为注册表中该分组下的全部键）或按键列表把当前值写回出厂值，
// 未声明的分组/键直接拒绝；成功后在提交后置动作中重建运行时并广播变更。
func LlmParamReset(c *gin.Context) {
	var in LlmParamResetIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	keys := make([]string, 0, len(in.Keys))
	if group := strings.TrimSpace(in.Group); group != "" {
		for _, spec := range pkgllm.Specs {
			if spec.Group == group {
				keys = append(keys, spec.Key)
			}
		}
		if len(keys) == 0 {
			response.CreateBadBodyError(errors.Newf("parameter group %q is not declared in the registry", group)).AbortWithGinH(c)
			return
		}
	} else if len(in.Keys) > 0 {
		for _, key := range in.Keys {
			if _, ok := pkgllm.Lookup(key); !ok {
				response.CreateBadBodyError(errors.Newf("parameter %q is not declared in the registry", key)).AbortWithGinH(c)
				return
			}
			keys = append(keys, key)
		}
	} else {
		response.CreateBadBodyError(errors.New("either keys or group must be provided")).AbortWithGinH(c)
		return
	}
	if err := resetAgentParam(middlewares.GetCoreDatabase(c), keys); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	afterConfigWrite(c, ai.ChangeKindParam, fmt.Sprintf("%d parameters", len(keys)))
	response.SimpleOK.AbortWithGinH(c)
}

// resetAgentParam 把给定键的当前生效值写回出厂值（value = default_value），不删除记录：
// 参数记录必须恒存在，删行会让下次启动的强校验失败。
func resetAgentParam(db *gorm.DB, keys []string) error {
	return errors.WithStack(db.Model(&core.AgentParam{}).Where("key IN ?", keys).
		Update("value", gorm.Expr("default_value")).Error)
}

// upsertAgentParam 写入单条参数的当前生效值：记录必须已由 fixture 种子化（出厂值只由
// fixture 维护），缺失时返回英文错误而不是新建一条没有出厂值的记录。
func upsertAgentParam(db *gorm.DB, key, value string) error {
	param := core.AgentParam{}
	if err := db.Where("key = ?", key).First(&param).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.Newf("parameter %q has no seeded row; run `migrate up` to seed the agent parameter defaults", key)
		}
		return errors.WithStack(err)
	}
	return errors.WithStack(db.Model(&core.AgentParam{}).Where("key = ?", key).Update("value", value).Error)
}

// paramJSONValue 把库内文本字面量按其类型解析为对应的 JSON 原生类型，供前端直接展示与
// 编辑（免去二次解析）；字面量不可解析时原样返回，保证接口始终可读。
func paramJSONValue(spec pkgllm.ParamSpec, literal string) any {
	value, err := utils.ParseScalar(paramScalarKind(spec.Kind), literal)
	if err != nil {
		return literal
	}
	return value
}

// paramScalarKind 把注册表声明的参数类型映射为标量类型内核；string 与 text 同属字符串，
// text 仅表示多行文本语义。
func paramScalarKind(kind pkgllm.ParamKind) utils.ScalarKind {
	switch kind {
	case pkgllm.ParamKindInt:
		return utils.ScalarKindInt
	case pkgllm.ParamKindFloat:
		return utils.ScalarKindFloat
	case pkgllm.ParamKindBool:
		return utils.ScalarKindBool
	default:
		return utils.ScalarKindString
	}
}

// paramLabel 返回参数展示名称：i18n 键 param.<Key>.label（Key 全局唯一，故直接复用为文案
// 键的一部分），未命中时回退参数短名（键的最后一段），保证前端始终有可展示的文案。
func paramLabel(lang *base.Language, spec pkgllm.ParamSpec) string {
	key := fmt.Sprintf("param.%s.label", spec.Key)
	if label := translate.L(lang, key, nil); label != key {
		return label
	}
	if idx := strings.LastIndex(spec.Key, "."); idx >= 0 {
		return spec.Key[idx+1:]
	}
	return spec.Key
}

// paramComment 返回参数说明：i18n 键 param.<Key>.comment（Key 全局唯一，故直接复用为文案
// 键的一部分）。说明文案只在语言文件中维护，注册表不持有副本，故无可回退的兜底文案；
// 未命中时按 translate 层约定返回键本身，由 param_i18n_test.go 的键完整性测试兜底。
func paramComment(lang *base.Language, spec pkgllm.ParamSpec) string {
	return translate.L(lang, fmt.Sprintf("param.%s.comment", spec.Key), nil)
}

// normalizeParamValue 把请求中的 JSON 原生值按注册表声明的类型归一化为库内文本字面量。
// 返回空串表示「恢复出厂值」，由调用方把当前值写回 default_value：入参为 null，或字符串类
// 参数传入空串时即为该情形；数值与布尔类参数的空串按零值处理（表单清空的既有契约）；
// 类型不符或声明了封闭取值清单却传入清单外的值（如 retrieval.fusion）时返回英文错误，
// 避免非法值落库。
// normalizeParamValue normalizes the JSON native value of a request into the in-database text
// literal according to the registry-declared kind. An empty return means "restore the factory
// value", in which case the caller writes the current value back to default_value: that is the
// case for a null input or an empty string on a string-kind parameter; an empty string on a
// numeric or boolean parameter is treated as the zero value (the established contract of a
// cleared form field). A kind mismatch, or a value outside the declared closed value space
// (such as retrieval.fusion), yields an English error so an illegal value never reaches the
// database.
func normalizeParamValue(spec pkgllm.ParamSpec, value any) (string, error) {
	if value == nil {
		return "", nil
	}
	literal, err := utils.FormatScalar(paramScalarKind(spec.Kind), value)
	if err != nil {
		return "", errors.Newf("parameter %q expects value of type %s: %v", spec.Key, spec.Kind, err)
	}
	if literal != "" && !pkgllm.OptionAllowed(spec, literal) {
		return "", errors.Newf("parameter %q expects one of %s, got %q",
			spec.Key, strings.Join(spec.Options, ", "), literal)
	}
	return literal, nil
}
