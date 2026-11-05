// Package knowledge 提供知识库（RAG）文档导入的 asynq 任务处理器。
// ParseHandler 消费投递侧经 knowledge.ParseTask（ai:doc:parse）接口投递的任务：解析文档 ID
// 载荷后，拉取并解密 S3 原件、抽取文本、写入 content 并切块向量化，最终把文档状态收敛为
// ready 或 failed。
package knowledge

import (
	// 标准库
	"encoding/json"

	// 第三方
	"github.com/hibiken/asynq"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/validator"
)

const (
	// ParseTask 知识文档导入解析任务类型，载荷为 ParsePayload（文档 ID）。由 web 侧在落库并
	// 加密上传原件后投递，仅供 worker 进程消费：worker 拉取解密原件、抽取文本、切块向量化并
	// 收敛文档状态。投递使用 asynq.MaxRetry(0)，解析失败不自动重试，由用户经「重新解析」重跑。
	ParseTask = "ai:doc:parse"
)

// ParsePayload 知识文档解析任务载荷，仅携带目标文档 ID。
type ParsePayload struct {
	// DocumentID 待解析的知识文档 ID，必填
	DocumentID uint `json:"documentID" binding:"required"`
}

// NewParseTask 把文档 ID 编码为 ai:doc:parse 异步任务；ID 缺失（为零值）返回错误。
func NewParseTask(documentID uint) (*asynq.Task, error) {
	payload := ParsePayload{DocumentID: documentID}
	if err := validateParsePayload(payload); err != nil {
		return nil, errors.WithStack(err)
	}
	data, err := json.Marshal(&payload)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return asynq.NewTask(ParseTask, data), nil
}

// validateParsePayload 校验解析载荷的必填字段：文档 ID。校验规则声明在 ParsePayload 的
// binding 标签上，经全局 validator 引擎统一执行（validator.Check），错误为
// validator.ValidationErrors。
func validateParsePayload(in ParsePayload) error {
	return errors.WithStack(validator.Check(&in))
}
