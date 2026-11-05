package periodic

import (
	"testing"
)

// TestParseReportEnvelope 验证统一信封契约的解析与校验：合法信封通过，各类不合规输出给出纠错原因。
func TestParseReportEnvelope(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantOK  bool
		content string
	}{
		{
			name:    "合法信封",
			in:      `{"content":"# 报告\n\n正文","draft":{"key":"value"}}`,
			wantOK:  true,
			content: "# 报告\n\n正文",
		},
		{
			name:    "draft 省略",
			in:      `{"content":"# 报告"}`,
			wantOK:  true,
			content: "# 报告",
		},
		{
			name:    "draft 为 null",
			in:      `{"content":"# 报告","draft":null}`,
			wantOK:  true,
			content: "# 报告",
		},
		{
			name:   "纯 Markdown 被拒绝",
			in:     "# 报告\n\n正文",
			wantOK: false,
		},
		{
			name:   "代码块围栏被拒绝",
			in:     "```json\n{\"content\":\"# 报告\"}\n```",
			wantOK: false,
		},
		{
			name:   "content 为空被拒绝",
			in:     `{"content":"   ","draft":{}}`,
			wantOK: false,
		},
		{
			name:   "draft 非对象被拒绝",
			in:     `{"content":"# 报告","draft":"text"}`,
			wantOK: false,
		},
		{
			name:   "非法 JSON 被拒绝",
			in:     `{"content":`,
			wantOK: false,
		},
		{
			name:   "空输出被拒绝",
			in:     "  ",
			wantOK: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			envelope, err := ParseReportEnvelope(c.in)
			if c.wantOK {
				if err != nil {
					t.Fatalf("ParseReportEnvelope(%q) error = %v, want nil", c.in, err)
				}
				if envelope.Content != c.content {
					t.Fatalf("ParseReportEnvelope(%q) content = %q, want %q", c.in, envelope.Content, c.content)
				}
				return
			}
			if err == nil {
				t.Fatalf("ParseReportEnvelope(%q) error = nil, want rejection", c.in)
			}
			if err.Error() == "" {
				t.Fatalf("ParseReportEnvelope(%q) rejection cause is empty", c.in)
			}
		})
	}
}
