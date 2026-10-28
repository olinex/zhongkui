package contract

import (
	// 第三方
	"github.com/cloudwego/eino/schema"
)

// AskUserToolName 是问卷类 client 工具 ask_user 的工具名单点声明（禁止在别处写该字面量）。
const AskUserToolName = "ask_user"

// ApiInvokeToolName 是只读接口调用 client 工具 api_invoke 的工具名单点声明。
const ApiInvokeToolName = "api_invoke"

// MaxQuestionnaireOptions 是单个提问允许的候选答案条数上限（与工具 schema 的「1~3 条」约束一致）。
const MaxQuestionnaireOptions = 3

// clientToolInfos 是 client 工具名到 eino schema 的注册表，仅供本包注册与解析使用。
var clientToolInfos = map[string]*schema.ToolInfo{}

// RegisterClientToolInfo 登记一个 client 工具的 schema，应由工具声明所在的 init 期调用。工具名
// 为空、schema 为 nil、schema 名与工具名不一致或同名重复登记均视为编程缺陷，直接 panic（fail
// fast），避免非法注册表在生产中以残缺形态运行（风格与 internal/ai/tools 的 Register 一致）。
func RegisterClientToolInfo(name string, info *schema.ToolInfo) {
	if name == "" {
		panic("ai: register a client tool with an empty name")
	}
	if info == nil {
		panic("ai: register client tool " + name + " with a nil schema")
	}
	if info.Name != name {
		panic("ai: register client tool " + name + " with mismatched schema name: " + info.Name)
	}
	if _, dup := clientToolInfos[name]; dup {
		panic("ai: duplicate client tool registration: " + name)
	}
	clientToolInfos[name] = info
}

// ClientToolInfo 按名取回 client 工具的 schema，未登记时返回 ok=false（装配层据此记英文告警并跳过）。
func ClientToolInfo(name string) (*schema.ToolInfo, bool) {
	info, ok := clientToolInfos[name]
	return info, ok
}

// IsClientTool 判断工具名是否为已登记的 client 工具：是则本次调用须挂起等待前端审批与执行
// （实现不落在本地工具索引中），否则按本地工具派发。
func IsClientTool(name string) bool {
	_, ok := clientToolInfos[name]
	return ok
}

// IsQuestionnaireTool 判断工具名是否为问卷类 client 工具（当前仅 ask_user）：此类调用只向用户
// 提问、无副作用，供上层据此将其豁免人工审核。
func IsQuestionnaireTool(name string) bool {
	return name == AskUserToolName
}

// NewAskUserToolInfo 构造 ask_user 的工具元信息与参数 JSON schema，供模型决定何时调用。参数经
// eino 的 ParameterInfo 形式描述（questions 必填，每题含 id / question / options）。
func NewAskUserToolInfo() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: AskUserToolName,
		Desc: "当信息不足、需要用户补充细节，或需要用户在多个方案间抉择时，调用本工具向用户提问，" +
			"并等待其作答后再继续。可一次提出多个问题；每个问题给出 1~3 条候选答案，并为候选答案附上" +
			"可选说明。不要把「其他」作为候选项——界面会为每个问题统一提供「其他」与自定义输入。" +
			"用户作答后结果以 JSON 回传，形如 " +
			`{"answers":[{"questionID":"q1","answered":true,"answer":"用户选择的答案","custom":false}]}：` +
			"answered=false 表示用户未作答（此时 answer 为空），custom=true 表示用户选择了自定义输入而非某个候选项。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"questions": {
				Type:     schema.Array,
				Desc:     "要向用户提出的问题列表，至少 1 条。",
				Required: true,
				ElemInfo: &schema.ParameterInfo{
					Type: schema.Object,
					SubParams: map[string]*schema.ParameterInfo{
						"id": {
							Type:     schema.String,
							Desc:     "问题的唯一标识（如 q1），用于在回传答案中定位对应问题。",
							Required: true,
						},
						"question": {
							Type:     schema.String,
							Desc:     "要向用户提问的问题文本。",
							Required: true,
						},
						"options": {
							Type:     schema.Array,
							Desc:     "该问题的候选答案列表，1~3 条。不要把「其他」作为候选项——界面会统一提供「其他」与自定义输入。",
							Required: true,
							ElemInfo: &schema.ParameterInfo{
								Type: schema.Object,
								SubParams: map[string]*schema.ParameterInfo{
									"text": {
										Type:     schema.String,
										Desc:     "候选答案的文本，必填。",
										Required: true,
									},
									"detail": {
										Type: schema.String,
										Desc: "候选答案的可选说明，用于补充其含义或后果。",
									},
								},
							},
						},
					},
				},
			},
		}),
	}
}

// NewApiInvokeToolInfo 构造 api_invoke 的工具元信息与参数 JSON schema：模型只给能力 ID 与请求体，
// 不能指定任意 URL；HTTP 方法与路径由后端按能力快照白名单解析并写入挂起现场，前端只执行该
// 已解析的只读调用。
func NewApiInvokeToolInfo() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: ApiInvokeToolName,
		Desc: "在用户自身凭据下调用一个已登记的只读平台能力，并返回接口响应。能力清单可通过 " +
			"knowledge_search 并传 format=capability 查询。只允许调用平台能力中" +
			"登记且标记为可调用的只读能力；不要用它执行写入、删除或任意 URL 请求。构造 params 前应先调用 knowledge_search " +
			"并传 format=native_api 核对入参字段名、嵌套结构与路径参数，不要凭字段语义自行拼装。pathParams 用于填充路径中的 :param 占位符，params 为请求体对象。" +
			"该调用需要用户人工审批后才会执行。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"capability": {
				Type:     schema.String,
				Desc:     "能力 ID，必须来自平台能力知识（knowledge_search 的 format=capability），例如 traffic.list。",
				Required: true,
			},
			"params": {
				Type: schema.Object,
				Desc: "请求体 JSON 对象；字段与校验规则可用 knowledge_search 并传 format=native_api 查询。无请求体时可省略或传空对象。",
			},
			"pathParams": {
				Type: schema.Object,
				Desc: "路径参数对象：键为路径中的 :param 名（如 id），值为非空字符串；仅当接口路径含占位符时必填。",
			},
		}),
	}
}

// init 在启动期经 client schema 注册表登记 ask_user 与 api_invoke；client 工具只登记入参契约，
// 执行落于前端（api_invoke 的接口白名单解析与只读校验由后端在挂起前完成）。
func init() {
	RegisterClientToolInfo(AskUserToolName, NewAskUserToolInfo())
	RegisterClientToolInfo(ApiInvokeToolName, NewApiInvokeToolInfo())
}
