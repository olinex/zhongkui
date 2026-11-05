package periodic

import (
	// 标准库
	"encoding/json"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/validator"
)

// ReportSystemHint 是周期报告每次生成注入的系统级输出契约（追加在系统提示词末尾）。
// 报告计划复用 knowledge 会话，其系统提示本身也要求 {"content","draft"} 信封；此处给出
// 报告场景下的明确结构与约束，与 agent 层的解析校验（ParseReportEnvelope）共同构成
// 「模型软约束 + 代码硬校验」的双层约束。
const ReportSystemHint = `报告输出契约（系统级约束，必须遵守）：
只输出一个 JSON 对象，结构为 {"content":"<Markdown 报告正文>","draft":{<可选结构化载荷>}}。
- content 必须是完整、非空的 Markdown 报告正文（含标题、结论与必要表格）；
- draft 承载结构化事实，没有内容时输出 {}；
- 不得输出 JSON 之外的任何解释、前后缀文字或代码块围栏。`

// ReportEnvelope 是报告生成的统一输出信封：content 为面向用户的 Markdown 正文，
// draft 为供下游自动化消费的结构化载荷。
type ReportEnvelope struct {
	// Content Markdown 报告正文
	Content string `json:"content"`
	// Draft 结构化载荷，没有内容时为 {} 或省略
	Draft json.RawMessage `json:"draft,omitempty"`
}

// reportEnvelopeView 是报告信封的结构化校验视图：content 必填且去空白后非空；draft 用对象映射
// 表达，缺省与 null 解码为 nil，数组/标量在解码阶段即失败。
type reportEnvelopeView struct {
	// Content Markdown 报告正文
	Content string `json:"content" binding:"required,notBlank"`
	// Draft 结构化载荷，必须是 JSON 对象
	Draft map[string]json.RawMessage `json:"draft"`
}

// ParseReportEnvelope 解析并校验模型输出是否符合统一信封契约。
// 校验失败返回面向模型的纠错原因（会被回传要求重新生成）：回答必须是不带围栏的裸 JSON 对象，
// content 必须为非空字符串，draft 若存在必须是 JSON 对象。
func ParseReportEnvelope(raw string) (ReportEnvelope, error) {
	var envelope ReportEnvelope
	var view reportEnvelopeView
	if err := validator.DecodeObject(raw, &view); err != nil {
		return envelope, err
	}
	if err := validator.Check(&view); err != nil {
		return envelope, errors.Newf("the report output violates the envelope contract: %s", validator.Describe(err))
	}
	envelope.Content = view.Content
	if view.Draft != nil {
		draft, err := json.Marshal(view.Draft)
		if err != nil {
			return envelope, errors.Newf("the report output draft cannot be encoded: %v", err)
		}
		envelope.Draft = draft
	}
	return envelope, nil
}
