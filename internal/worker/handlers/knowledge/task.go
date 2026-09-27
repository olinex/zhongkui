// Package knowledge 提供知识库（RAG）文档导入的 asynq 任务处理器。
// ParseHandler 消费投递侧经 knowledge.ParseTask（ai:doc:parse）接口投递的任务：解析文档 ID
// 载荷后，拉取并解密 S3 原件、抽取文本、写入 content 并切块向量化，最终把文档状态收敛为
// ready 或 failed。
//
// Package knowledge provides the asynq handlers for knowledge-base (RAG) document import.
// ParseHandler consumes the knowledge.ParseTask (ai:doc:parse) tasks enqueued by the
// producing side: after parsing the document ID payload it materializes and decrypts the S3
// source file, extracts the text, persists the content, chunks and embeds it, and finally
// converges the document status to ready or failed.
package knowledge

import (
	// stdlib
	"encoding/json"

	// third-party
	"github.com/hibiken/asynq"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/validator"
)

const (
	// ParseTask 知识文档导入解析任务类型，载荷为 ParsePayload（文档 ID）。由 web 侧在落库并
	// 加密上传原件后投递，仅供 worker 进程消费：worker 拉取解密原件、抽取文本、切块向量化并
	// 收敛文档状态。投递使用 asynq.MaxRetry(0)，解析失败不自动重试，由用户经「重新解析」重跑。
	// ParseTask is the knowledge document import parse task type carrying a ParsePayload
	// (document ID). The web side enqueues it after persisting the row and encrypting the
	// source file, and only the worker process consumes it: the worker materializes and
	// decrypts the source file, extracts the text, chunks and embeds it and converges the
	// document status. It is enqueued with asynq.MaxRetry(0); a parse failure is not retried
	// automatically and is re-run by the user through "reparse".
	ParseTask = "ai:doc:parse"
)

// ParsePayload 知识文档解析任务载荷，仅携带目标文档 ID。
// ParsePayload is the knowledge document parse task payload, carrying only the target
// document ID.
type ParsePayload struct {
	// DocumentID 待解析的知识文档 ID，必填
	// DocumentID the ID of the knowledge document to parse, required
	DocumentID uint `json:"documentId" binding:"required"`
}

// NewParseTask 把文档 ID 编码为 ai:doc:parse 异步任务；ID 缺失（为零值）返回错误。
// NewParseTask encodes a document ID into an ai:doc:parse async task; a missing ID (zero
// value) returns an error.
func NewParseTask(documentID uint) (*asynq.Task, error) {
	payload := ParsePayload{DocumentID: documentID}
	if err := validateParsePayload(payload); err != nil {
		return nil, err
	}
	data, err := json.Marshal(&payload)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return asynq.NewTask(ParseTask, data), nil
}

// validateParsePayload 校验解析载荷的必填字段：文档 ID。校验规则声明在 ParsePayload 的
// binding 标签上，经全局 validator 引擎统一执行（validator.Validate.Struct），错误为
// validator.ValidationErrors。
// validateParsePayload validates the required fields of a parse payload: the document ID.
// The rules are declared as binding tags on ParsePayload and enforced by the global
// validator engine via validator.Validate.Struct, returning a validator.ValidationErrors on
// failure.
func validateParsePayload(in ParsePayload) error {
	return errors.WithStack(validator.Validate.Struct(&in))
}
