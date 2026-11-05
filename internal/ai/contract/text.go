package contract

import (
	// 标准库
	"encoding/json"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/validator"
)

// ParseJSONResponse 去掉首尾 ```json 代码围栏后把 content 解析为 T。label 描述调用点
// （如 "dry-run: cannot parse chunk relevance response"）；解析失败时返回的错误文本为
// "<label> as JSON: <响应前 200 字符>"，供各调用点统一容错文案。
func ParseJSONResponse[T any](content, label string) (T, error) {
	var out T
	body := validator.StripJSONFence(content)
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		return out, errors.Newf("%s as JSON: %q", label, truncateForError(body))
	}
	return out, nil
}

// truncateForError 按 rune 截断错误文案中回显的原始响应，避免超长模型输出污染日志。
func truncateForError(s string) string {
	const limit = 200
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "..."
}
