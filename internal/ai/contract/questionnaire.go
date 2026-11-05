package contract

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
// 答案后再次调用 ask_user），每一轮各自成一个 questionnaire 块、按 block_seq 先后并列存储，前端据此把
// 各轮渲染成各自独立的问答框并把最新一轮沉在底部。它既是 run 现场的回落依据，也是回退（重新回答）接口的出参形态。
type QuestionnairePayload struct {
	// Questions 本次问卷的题目列表
	Questions []QuestionnaireQuestion `json:"questions"`
	// Answers 用户作答列表；缺项的题目按未作答处理
	Answers []QuestionnaireAnswer `json:"answers,omitempty"`
	// Awaiting 该轮问卷是否仍在等待用户作答（挂起时 true，作答 / 取消终态化为 false）。
	Awaiting bool `json:"awaiting,omitempty"`
	// InterruptIDs 该轮 ask_user 调用的根中断 ID 列表（挂起时由工具中断分配）：前端「重新回答」据此
	// 定位本轮交互，回退后亦按这些 ID 映射 ResumeParams.Targets。
	InterruptIDs []string `json:"interruptIDs,omitempty"`
}

// FullyAnswered 报告单轮载荷中的题目是否全部已作答：逐题按 id 取首个匹配答案（与前端
// questionnaireAnswerTextOf 的判定一致），答案存在且 Answered=true 视为已作答。多轮提问的各轮各自成一份
// 载荷，故视图层只需对末轮调用它即可判定 awaiting：末轮仍存在未作答的题目等价于「该轮仍在等用户作答」；
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
