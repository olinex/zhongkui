// 本文件承载 client 来源工具的 schema 注册表：client 工具由前端在用户自身凭据下执行，后端不
// 构建本地执行器、不拨号任何远端，只把这里的 *schema.ToolInfo 并入工具装配的 schema 清单（见
// store_tools.go 的 appendClientTool），供模型选择调用。与「元信息注册表（internal/ai 的
// ToolSpecs）+ 出厂行（002_fixtures.sql）」共同构成 client 工具的完整链路：元信息与出厂行决定
// 工具是否存在，本注册表决定工具的入参契约。它是 builtin 工具实现注册表（internal/ai/tools 的
// Register）在 client 侧的等价物——故守卫测试 internal/ai/tools_fixture_test.go 要求 source=client
// 的工具必须在此有对应条目。当前注册表登记了问卷类只读交互工具 ask_user。

package ai

import (
	// 标准库
	"encoding/json"
	"strings"

	// 第三方
	"github.com/cloudwego/eino/schema"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// askUserToolName 是问卷类 client 工具 ask_user 的工具名单点声明（禁止在别处写该字面量）。
const askUserToolName = "ask_user"

// apiInvokeToolName 是只读接口调用 client 工具 api_invoke 的工具名单点声明。
const apiInvokeToolName = "api_invoke"

// maxQuestionnaireOptions 是单个提问允许的候选答案条数上限（与工具 schema 的
// 「1~3 条」约束一致）。
const maxQuestionnaireOptions = 3

// clientToolInfos 是 client 工具名到 eino schema 的注册表，仅供本包注册与解析使用。
var clientToolInfos = map[string]*schema.ToolInfo{}

// registerClientToolInfo 登记一个 client 工具的 schema，应由工具声明所在的 init 期调用。工具名
// 为空、schema 为 nil、schema 名与工具名不一致或同名重复登记均视为编程缺陷，直接 panic（fail
// fast），避免非法注册表在生产中以残缺形态运行（风格与 internal/ai/tools 的 Register 一致）。
func registerClientToolInfo(name string, info *schema.ToolInfo) {
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

// clientToolInfo 按名取回 client 工具的 schema，未登记时返回 ok=false（装配层据此记英文告警并跳过）。
func clientToolInfo(name string) (*schema.ToolInfo, bool) {
	info, ok := clientToolInfos[name]
	return info, ok
}

// isClientTool 判断工具名是否为已登记的 client 工具：是则本次调用须挂起等待前端审批与执行
// （实现不落在本地工具索引中），否则按本地工具派发。
func isClientTool(name string) bool {
	_, ok := clientToolInfos[name]
	return ok
}

// isQuestionnaireTool 判断工具名是否为问卷类 client 工具（当前仅 ask_user）：此类调用只向用户
// 提问、无副作用，供上层据此将其豁免人工审核。
func isQuestionnaireTool(name string) bool {
	return name == askUserToolName
}

// newAskUserToolInfo 构造 ask_user 的工具元信息与参数 JSON schema，供模型决定何时调用。参数经
// eino 的 ParameterInfo 形式描述（questions 必填，每题含 id / question / options）。
func newAskUserToolInfo() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: askUserToolName,
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

// newApiInvokeToolInfo 构造 api_invoke 的工具元信息与参数 JSON schema：模型只给能力 ID 与请求体，
// 不能指定任意 URL；HTTP 方法与路径由后端按能力快照白名单解析并写入挂起现场，前端只执行该
// 已解析的只读调用。
func newApiInvokeToolInfo() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: apiInvokeToolName,
		Desc: "在用户自身凭据下调用一个已登记的只读平台能力，并返回接口响应。能力清单可通过 " +
			"knowledge_search 并传 format=capability 查询。只允许调用平台能力中" +
			"登记且标记为可调用的只读能力；不要用它执行写入、删除或任意 URL 请求。调用前可先用 knowledge_search " +
			"并传 format=native_api 确认入参字段与路径参数。pathParams 用于填充路径中的 :param 占位符，params 为请求体对象。" +
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

// QuestionnaireOption 是 ask_user 问卷中的一个候选答案：Text 为候选答案文本，Detail 为其可选说明。
type QuestionnaireOption struct {
	// Text 候选答案文本
	Text string `json:"text"`
	// Detail 候选答案的可选说明，用于补充其含义或后果
	Detail string `json:"detail,omitempty"`
}

// QuestionnaireQuestion 是 ask_user 问卷中的一个提问：ID 为本次问卷内的唯一标识（回传答案据它定位
// 对应问题），Question 为题面，Options 为 1~3 条候选答案。
type QuestionnaireQuestion struct {
	// ID 本次问卷内的唯一标识，用于在回传答案中定位对应问题
	ID string `json:"id"`
	// Question 要向用户提出的问题文本
	Question string `json:"question"`
	// Options 该问题的候选答案列表（1~3 条）
	Options []QuestionnaireOption `json:"options"`
}

// QuestionnaireAnswer 是用户对某题的作答：Answered=false 表示未作答（Answer 为空）；Custom=true
// 表示用户选择了「其他」并自填文本，此时 Answer 为用户自定义文本。
type QuestionnaireAnswer struct {
	// QuestionID 对应的提问 ID（QuestionnaireQuestion.ID）
	QuestionID string `json:"questionID"`
	// Answered 该题是否已作答
	Answered bool `json:"answered"`
	// Answer 作答文本：候选项文本或用户自定义文本（未作答时为空）
	Answer string `json:"answer"`
	// Custom 该作答是否为「其他」自定义输入（false 表示选择了某个候选项）
	Custom bool `json:"custom"`
}

// QuestionnairePayload 是「一轮」ask_user 问卷的完整载荷：Questions 为该轮的题目，Answers 为该轮用户的
// 作答（首次挂起时为空，续跑终态化后承载答案，供历史回显与回退重答）。同一回合内模型可多轮提问（拿到上一轮
// 答案后再次调用 ask_user），每一轮各自成一个 questionnaire 块、按 block_seq 先后**并列**存储，前端据此把
// 各轮渲染成各自独立的问答框并把最新一轮沉在底部。它既是 run 现场的回落依据，也是回退（重新回答）接口的出参形态。
type QuestionnairePayload struct {
	// Questions 本次问卷的题目列表
	Questions []QuestionnaireQuestion `json:"questions"`
	// Answers 用户作答列表；缺项的题目按未作答处理
	Answers []QuestionnaireAnswer `json:"answers,omitempty"`
}

// askUserArgs 是 ask_user 工具入参的解析目标：只取问卷题目（答案由前端另经 tool-result 回传）。
type askUserArgs struct {
	Questions []QuestionnaireQuestion `json:"questions"`
}

// parseQuestionnaireArgs 把模型给出的 ask_user 调用入参解析为问卷载荷并做基本校验（至少 1 题，每题需
// 有非空 id / question 与 1~3 条非空 text 的候选项）。解析或校验失败返回英文错误，调用方据此按工具
// 失败回灌、让模型修正后重试，而不是挂起一个空问卷。
func parseQuestionnaireArgs(arguments string) (*QuestionnairePayload, error) {
	var args askUserArgs
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return nil, errors.WithStack(err)
	}
	if len(args.Questions) == 0 {
		return nil, errors.New("ask_user requires at least one question")
	}
	for _, q := range args.Questions {
		if strings.TrimSpace(q.ID) == "" {
			return nil, errors.New("each ask_user question requires a non-empty id")
		}
		if strings.TrimSpace(q.Question) == "" {
			return nil, errors.New("each ask_user question requires a non-empty question text")
		}
		if len(q.Options) == 0 || len(q.Options) > maxQuestionnaireOptions {
			return nil, errors.Newf("ask_user question %q requires 1 to %d options", q.ID, maxQuestionnaireOptions)
		}
		for _, o := range q.Options {
			if strings.TrimSpace(o.Text) == "" {
				return nil, errors.Newf("ask_user question %q has an option without text", q.ID)
			}
		}
	}
	return &QuestionnairePayload{Questions: args.Questions}, nil
}

// markQuestionIDs 把一组题目 id 记入 seen，并返回其中首个重复的 id（与 seen 或本组内自身重复；无重复
// 返回空串）。一轮内可有多条 ask_user 调用，其题目会合并为同一张问卷；答案按题目 id 归属，故 id 重复
// 时调用方应拒绝该条调用并按工具失败回灌，避免合并后的答案无法拆分。检测到重复即不写入 seen（整条
// 调用被拒，其题目不应占用 id）。
func markQuestionIDs(seen map[string]struct{}, questions []QuestionnaireQuestion) string {
	pending := make(map[string]struct{}, len(questions))
	for _, q := range questions {
		if _, ok := seen[q.ID]; ok {
			return q.ID
		}
		if _, ok := pending[q.ID]; ok {
			return q.ID
		}
		pending[q.ID] = struct{}{}
	}
	for id := range pending {
		seen[id] = struct{}{}
	}
	return ""
}

// appendQuestionnaireRound 把本轮挂起的问卷载荷作为一轮追加到锚点行已累计的各轮之后。非问卷挂起（本轮
// 载荷为空或无题目）时返回原列表，使锚点行上已累计的各轮不被抹掉；纯代码侧无需按题目 id 去重——各轮
// 并列存储、答案按轮归属，跨轮重名不再影响答案拆分（同轮内的重复 id 仍由 markQuestionIDs 拦下并按工具
// 失败回灌）。
func appendQuestionnaireRound(rounds []QuestionnairePayload, next *QuestionnairePayload) []QuestionnairePayload {
	if next == nil || len(next.Questions) == 0 {
		return rounds
	}
	return append(rounds, *next)
}

// attachRoundAnswers 把本轮回传的答案并入**最后一轮**（本次挂起新追加的那一轮）：按题目 id 归并（同 id 以
// 新答案为准并保持其原位置），新题目 id 追加在末尾。每次续跑只回传本轮题目的答案，故须并入末轮；只并入
// 末轮使各轮答案互不覆盖——此前各轮的答案各自留在自己的载荷里。
func attachRoundAnswers(rounds []QuestionnairePayload, answers []QuestionnaireAnswer) []QuestionnairePayload {
	if len(rounds) == 0 || len(answers) == 0 {
		return rounds
	}
	last := &rounds[len(rounds)-1]
	merged := make([]QuestionnaireAnswer, 0, len(last.Answers)+len(answers))
	at := make(map[string]int, len(last.Answers))
	for _, a := range last.Answers {
		at[a.QuestionID] = len(merged)
		merged = append(merged, a)
	}
	for _, a := range answers {
		if i, ok := at[a.QuestionID]; ok {
			merged[i] = a
			continue
		}
		at[a.QuestionID] = len(merged)
		merged = append(merged, a)
	}
	last.Answers = merged
	return rounds
}

// clearLastRoundAnswers 清空**末轮**（本次挂起对应的那一轮）的用户答案。用于「取消回答」：末轮因回退重开
// 可能仍带着上次作答的答案（供预填重开），取消后须回到未作答——不清则终态化写回的仍是旧答案，刷新 /
// 重进会把它当作本次作答展示。返回新的切片（不原地改动入参），空列表时原样返回。
func clearLastRoundAnswers(rounds []QuestionnairePayload) []QuestionnairePayload {
	if len(rounds) == 0 {
		return rounds
	}
	out := append([]QuestionnairePayload(nil), rounds...)
	out[len(out)-1].Answers = nil
	return out
}

// FullyAnswered 报告**单轮**载荷中的题目是否全部已作答：逐题按 id 取**首个**匹配答案（与前端
// questionnaireAnswerTextOf 的判定一致），答案存在且 Answered=true 视为已作答。多轮提问的各轮各自成一份
// 载荷，故视图层只需对**末轮**调用它即可判定 awaiting：末轮仍存在未作答的题目等价于「该轮仍在等用户作答」；
// 非问卷挂起（末轮在此之前已作答）时它恒为 true、不误判。
func (p QuestionnairePayload) FullyAnswered() bool {
	if len(p.Questions) == 0 {
		return false
	}
	answered := make(map[string]bool, len(p.Answers))
	for _, a := range p.Answers {
		if _, dup := answered[a.QuestionID]; dup {
			continue
		}
		answered[a.QuestionID] = a.Answered
	}
	for _, q := range p.Questions {
		if !answered[q.ID] {
			return false
		}
	}
	return true
}

// init 在启动期经 client schema 注册表登记 ask_user 与 api_invoke；client 工具只登记入参契约，
// 执行落于前端（api_invoke 的接口白名单解析与只读校验由后端在挂起前完成）。
func init() {
	registerClientToolInfo(askUserToolName, newAskUserToolInfo())
	registerClientToolInfo(apiInvokeToolName, newApiInvokeToolInfo())
}
