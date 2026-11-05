// 本文件定义 AI 技能模型（AgentSkill）：每条技能由名称、明文描述与正文（技能内容）组成，
// 正文经 crypto 序列化器加密存储（列类型 BYTEA），避免明文落库；同一命名空间内未删除的技能
// 名称唯一。技能分两类来源共用同一张表：命名空间级自定义技能由用户维护、正文必填；代码声明的
// 内置技能固定落在 base.SystemNamespace 命名空间，注册信息（名称 / 明文描述 / 启停）由 fixture
// 注入，正文为空、由 internal/ai/skill 的代码注册表按名提供，跨命名空间恒可用。
package ai

import (
	// 标准库
	"encoding/json"
	"strings"
	"time"

	// 第三方
	"gorm.io/datatypes"
	"gorm.io/gorm"

	// 项目内部
	coremodel "zhongkuisec.com/zhongkui/internal/models/core"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
	"zhongkuisec.com/zhongkui/internal/pkg/validator"
)

// AgentSkill 表示一条 AI 技能定义，记录技能名称、明文描述与加密正文，并提供启停开关。
// 对应表为 agent_skills：自定义技能按命名空间隔离；内置技能固定位于 base.SystemNamespace，
// 正文为空（Content 为 nil），由代码注册表按名提供。
type AgentSkill struct {
	gorm.Model // 内嵌 gorm.Model，包含 ID、CreatedAt、UpdatedAt、DeletedAt 字段

	// Namespace 所属的命名空间实体，用于 GORM 关联（foreignKey:NamespaceName;references:Name；级联删除）
	Namespace coremodel.Namespace `gorm:"foreignKey:NamespaceName;references:Name;onDelete:CASCADE"`
	// NamespaceName 所属命名空间名称，多租户隔离字段；内置技能固定为 base.SystemNamespace
	NamespaceName string `gorm:"type:VARCHAR(32);index:idx_agent_skills_namespace"`
	// Source 技能来源：custom | builtin，取值见 SkillSource（枚举类 agentSkillSource）
	Source SkillSource `gorm:"type:VARCHAR(16);not null;default:'custom';index:idx_agent_skills_source"`
	// Name 技能名称，同一命名空间内未删除的技能名称唯一；内置技能同时是代码注册表的对齐键
	Name string `gorm:"type:VARCHAR(64)"`
	// Description 技能描述（明文存储），便于在技能列表中识别技能用途
	Description string `gorm:"type:VARCHAR(256)"`
	// Content 技能正文内容，经 crypto 序列化器加密存储；内置技能不落库（nil），正文取自代码注册表
	Content *serializer.Crypto `gorm:"type:BYTEA;serializer:crypto" secure:"mode=\"aes-gcm\",offset=12823,length=32"`
	// Enabled 技能是否启用，默认 true；停用的技能不参与运行时装配
	Enabled bool `gorm:"type:BOOLEAN;not null;default:true"`
	// PeriodicCapable 技能是否可周期执行（由用户声明）：为 true 时应按固定周期、固定时间窗口
	// 取数并输出报告，可作为周期性报告计划的技能来源
	PeriodicCapable bool `gorm:"type:BOOLEAN;not null;default:false"`
	// InputFields 技能输入字段的结构化描述（由用户填写，JSON 数组），供报告计划渲染输入表单；
	// 无输入字段时为空数组
	InputFields datatypes.JSON `gorm:"type:JSONB;not null;default:'[]'"`
	// FixedFields 技能固定参数（由用户填写，JSON 对象），键为参数名、值为任意 JSON 值，
	// 保留原始 JSON 类型；周期技能的 timeOffset / timeWindow 为 Go 时长字符串；无固定参数时为空对象
	FixedFields datatypes.JSON `gorm:"type:JSONB;not null;default:'{}'"`
}

// SkillInputField 表示技能的一个输入字段的结构化描述：由用户填写，是报告计划渲染输入表单的
// JSON 协议载体（技能行 input_fields 列的 JSON 形状）。
type SkillInputField struct {
	// Name 字段名称，作为报告计划 inputValues 中的键
	Name string `json:"name"`
	// Type 字段类型，取值见 SkillInputFieldType
	Type SkillInputFieldType `json:"type"`
	// Required 该字段是否必填
	Required bool `json:"required"`
	// Description 字段用途说明，可为空
	Description string `json:"description"`
}

// ParseSkillInputFields 把技能行 input_fields 列的 JSON 解码为输入字段列表：空值（nil 或空串）
// 与 JSON null 返回空切片，非法 JSON 或无法映射的形状返回英文错误。
func ParseSkillInputFields(raw datatypes.JSON) ([]SkillInputField, error) {
	fields := make([]SkillInputField, 0)
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return fields, nil
	}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, errors.WithStack(err)
	}
	if fields == nil {
		fields = make([]SkillInputField, 0)
	}
	return fields, nil
}

// SkillInputFieldsJSON 把输入字段列表编码为技能行 input_fields 列的 JSON：nil 或空切片编码为
// 空 JSON 数组，非法值（如无法序列化的内容）返回英文错误。
func SkillInputFieldsJSON(fields []SkillInputField) (datatypes.JSON, error) {
	if fields == nil {
		fields = make([]SkillInputField, 0)
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return datatypes.JSON(raw), nil
}

// SkillInputFieldType 是技能输入字段的类型标识：由用户填写，决定报告计划输入表单的控件
// 形态与计划 inputValues 中允许承载的 JSON 值类型。取值集合与命令边界 inputFieldRule 的
// oneof 规则保持一致。
type SkillInputFieldType string

// 技能输入字段类型常量。
const (
	// SkillInputFieldTypeString 文本类型，承载 JSON 字符串
	SkillInputFieldTypeString SkillInputFieldType = "string"
	// SkillInputFieldTypeNumber 数字类型，承载 JSON 数字
	SkillInputFieldTypeNumber SkillInputFieldType = "number"
	// SkillInputFieldTypeBoolean 布尔类型，承载 JSON 布尔
	SkillInputFieldTypeBoolean SkillInputFieldType = "boolean"
	// SkillInputFieldTypeEnum 枚举类型，承载 JSON 字符串
	SkillInputFieldTypeEnum SkillInputFieldType = "enum"
	// SkillInputFieldTypeDate 日期类型，承载 JSON 字符串；name=nowTime 的字段必须使用该类型
	SkillInputFieldTypeDate SkillInputFieldType = "date"
)

// allSkillInputFieldTypes 是输入字段类型的取值空间，声明顺序即前端编辑器中的展示顺序。
var allSkillInputFieldTypes = []SkillInputFieldType{
	SkillInputFieldTypeDate,
	SkillInputFieldTypeNumber,
	SkillInputFieldTypeString,
	SkillInputFieldTypeEnum,
	SkillInputFieldTypeBoolean,
}

// AllSkillInputFieldTypes 返回全部输入字段类型的副本，声明顺序即前端编辑器中的展示顺序；供边界校验与
// 测试遍历使用，调用方修改返回值不影响该取值空间。
func AllSkillInputFieldTypes() []SkillInputFieldType {
	types := make([]SkillInputFieldType, len(allSkillInputFieldTypes))
	copy(types, allSkillInputFieldTypes)
	return types
}

// expectedJSONKind 返回该输入字段类型在计划 inputValues 中要求承载的 JSON 值类型；未知类型
// 返回 ok=false。date / enum 本质是字符串（其取值语义由前端控件约束），故与 string 同属
// JSON 字符串。
func (t SkillInputFieldType) expectedJSONKind() (validator.JSONKind, bool) {
	switch t {
	case SkillInputFieldTypeNumber:
		return validator.JSONKindNumber, true
	case SkillInputFieldTypeBoolean:
		return validator.JSONKindBoolean, true
	case SkillInputFieldTypeString, SkillInputFieldTypeEnum, SkillInputFieldTypeDate:
		return validator.JSONKindString, true
	default:
		return "", false
	}
}

// 技能输入字段与固定参数的约定键：nowTime 是运行时注入的输入字段名（其类型必须为 date）；
// timeOffset / timeWindow 是周期技能固定参数中必须以 Go 时长字符串给出的键。
const (
	// SkillInputFieldNameNowTime 运行时注入的当前时间字段名；周期技能的该字段类型必须为 date
	SkillInputFieldNameNowTime = "nowTime"
	// SkillFixedFieldTimeOffset 周期技能固定参数中的时间偏移键，值须为 Go 时长字符串，允许为负
	SkillFixedFieldTimeOffset = "timeOffset"
	// SkillFixedFieldTimeWindow 周期技能固定参数中的时间窗口键，值须为 Go 时长字符串且大于 0
	SkillFixedFieldTimeWindow = "timeWindow"
)

// 运行时注入/计算的时间参数键，禁止出现在技能固定参数中。
const (
	skillRuntimeFieldStartTime = "startTime"
	skillRuntimeFieldEndTime   = "endTime"
)

// ParseSkillFixedFields 把技能行 fixed_fields 列的 JSON 解码为自由形状的参数映射：空值（nil 或
// 空串）与 JSON null 返回空 map；非 JSON object 或非法 JSON 返回英文错误。值以 json.RawMessage
// 承载，保留字符串、数字、布尔、对象与数组的原始 JSON 类型。
func ParseSkillFixedFields(raw datatypes.JSON) (map[string]json.RawMessage, error) {
	fields := make(map[string]json.RawMessage)
	if _, err := validator.DecodeOptionalObject(string(raw), &fields); err != nil {
		return nil, errors.Newf("agent skill fixed fields must be a JSON object: %v", err)
	}
	if fields == nil {
		fields = make(map[string]json.RawMessage)
	}
	return fields, nil
}

// SkillFixedFieldsJSON 把固定参数映射编码为技能行 fixed_fields 列的 JSON：nil 或空 map 编码为
// 空 JSON 对象，非法值（如无法序列化的内容）返回英文错误。
func SkillFixedFieldsJSON(fields map[string]json.RawMessage) (datatypes.JSON, error) {
	if len(fields) == 0 {
		return datatypes.JSON([]byte("{}")), nil
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return datatypes.JSON(raw), nil
}

// SkillPeriodicWindow 表示周期技能从固定参数解析出的时间窗口：TimeOffset 是相对 nowTime 的偏移
// （可为负），TimeWindow 是窗口长度（必须大于 0）。
type SkillPeriodicWindow struct {
	// TimeOffset 相对 nowTime 的偏移，可为负（如 -24h 表示窗口结束于当前时刻）
	TimeOffset time.Duration
	// TimeWindow 窗口长度，必须大于 0
	TimeWindow time.Duration
}

// skillPeriodicWindowFields 是周期固定参数的结构化校验视图：presence 由 required 声明，
// 窗口长度约束由 gt=0 声明，统一经全局 validator 执行，避免在多个调用点手写键判断与数值比较；
// 时长值直接复用 serializer.DurationMS 的 JSON 解析能力（可读时长字符串，兼容整数毫秒）。
type skillPeriodicWindowFields struct {
	// TimeOffset 时间偏移，必须存在；零值与负值均合法
	TimeOffset *serializer.DurationMS `json:"timeOffset" binding:"required"`
	// TimeWindow 时间窗口，必须存在且大于 0
	TimeWindow *serializer.DurationMS `json:"timeWindow" binding:"required,gt=0"`
}

// skillReservedFixedFields 是运行时保留键的结构化视图：键一旦出现即为非空 RawMessage（含 JSON
// null），经 isdefault 规则判为违规；键不存在时为零值切片，规则通过。用结构体集中声明保留键，
// 避免散落的字符串集合判断。
type skillReservedFixedFields struct {
	// NowTime 运行时注入的当前时间，禁止写入固定参数；缺失时为零值切片，isdefault 通过
	NowTime json.RawMessage `json:"nowTime" binding:"isdefault"`
	// StartTime 运行时计算的时间窗口起点，禁止写入固定参数
	StartTime json.RawMessage `json:"startTime" binding:"isdefault"`
	// EndTime 运行时计算的时间窗口终点，禁止写入固定参数
	EndTime json.RawMessage `json:"endTime" binding:"isdefault"`
}

// ParseSkillPeriodicWindow 从技能行 fixed_fields 列的 JSON 解析周期时间窗口：要求存在 timeOffset
// 与 timeWindow，且两者均可被 serializer.DurationMS 解析，timeWindow 须大于 0；缺失、类型错误
// 或解析失败返回英文错误。
func ParseSkillPeriodicWindow(raw datatypes.JSON) (SkillPeriodicWindow, error) {
	fields, err := ParseSkillFixedFields(raw)
	if err != nil {
		return SkillPeriodicWindow{}, err
	}
	return ParsePeriodicWindowFromFields(fields)
}

// ParsePeriodicWindowFromFields 从已解码的固定参数映射解析周期时间窗口：把映射转换为
// skillPeriodicWindowFields 结构体后交由全局 validator 校验（required / gt=0），校验通过再转回
// SkillPeriodicWindow，供创建 / 更新校验与运行期直接复用 map。
func ParsePeriodicWindowFromFields(fields map[string]json.RawMessage) (SkillPeriodicWindow, error) {
	raw, err := json.Marshal(fields)
	if err != nil {
		return SkillPeriodicWindow{}, errors.WithStack(err)
	}
	var view skillPeriodicWindowFields
	if err := json.Unmarshal(raw, &view); err != nil {
		return SkillPeriodicWindow{}, errors.Newf("agent skill fixed fields have invalid duration values: %v", err)
	}
	if err := validator.Check(&view); err != nil {
		return SkillPeriodicWindow{}, errors.Newf("agent skill fixed fields do not satisfy the periodic window contract: %v", err)
	}
	return SkillPeriodicWindow{
		TimeOffset: view.TimeOffset.Std(),
		TimeWindow: view.TimeWindow.Std(),
	}, nil
}

// ValidateSkillFixedFields 校验技能固定参数：按 skillReservedFixedFields 的 isdefault 规则拒绝运行时
// 保留键，并拒绝与输入字段同名的固定字段；periodic 为 true 时进一步按 skillPeriodicWindowFields
// 校验 timeOffset / timeWindow，非周期技能不要求时长键。输入字段声明本身由调用方的校验视图负责。
func ValidateSkillFixedFields(fields map[string]json.RawMessage, periodic bool, inputFields []SkillInputField) error {
	raw, err := json.Marshal(fields)
	if err != nil {
		return errors.WithStack(err)
	}
	var reserved skillReservedFixedFields
	if err := json.Unmarshal(raw, &reserved); err != nil {
		return errors.Newf("agent skill fixed fields are invalid: %v", err)
	}
	if err := validator.Check(&reserved); err != nil {
		return errors.Newf("agent skill fixed fields contain runtime reserved keys: %v", err)
	}
	for _, field := range inputFields {
		if _, ok := fields[field.Name]; ok {
			return errors.Newf("agent skill fixed field %q conflicts with an input field of the same name", field.Name)
		}
	}
	if periodic {
		if _, err := ParsePeriodicWindowFromFields(fields); err != nil {
			return err
		}
	}
	return nil
}

// ValidateSkillPeriodicInputFields 校验周期技能的输入字段：必须且只能有一个 name=nowTime、
// type=date 的字段；非周期技能不校验。字段名重复、类型非法等结构问题由调用方的 binding 负责。
func ValidateSkillPeriodicInputFields(periodic bool, fields []SkillInputField) error {
	if !periodic {
		return nil
	}
	count := 0
	for _, field := range fields {
		if field.Name != SkillInputFieldNameNowTime {
			continue
		}
		if field.Type != SkillInputFieldTypeDate {
			return errors.Newf("agent skill periodic input field %q must have type %q, got %q",
				field.Name, SkillInputFieldTypeDate, field.Type)
		}
		count++
	}
	if count != 1 {
		return errors.Newf("agent skill periodic skills require exactly one %q input field of type %q, got %d",
			SkillInputFieldNameNowTime, SkillInputFieldTypeDate, count)
	}
	return nil
}

// InputFieldAcceptsJSON 判断原始 JSON 值是否符合技能输入字段类型：类型不在取值空间或 JSON
// 非法时返回 false；number / boolean 分别要求 JSON 数字 / 布尔，string / enum / date 均要求
// JSON 字符串。
func InputFieldAcceptsJSON(fieldType SkillInputFieldType, raw json.RawMessage) bool {
	want, ok := fieldType.expectedJSONKind()
	if !ok {
		return false
	}
	kind, err := validator.JSONValueKind(raw)
	return err == nil && kind == want
}

// MergeParameters 按传入顺序合并参数映射，后者同名键覆盖前者，返回新 map 且不修改入参；
// 典型用法为 MergeParameters(fixedFields, planInputs, runtimeInputs)，优先级由右向左递增。
func MergeParameters(sources ...map[string]json.RawMessage) map[string]json.RawMessage {
	merged := make(map[string]json.RawMessage)
	for _, source := range sources {
		for key, value := range source {
			merged[key] = value
		}
	}
	return merged
}
