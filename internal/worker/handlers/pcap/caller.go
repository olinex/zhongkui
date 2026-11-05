// Package pcap 提供 PCAP 抓包文件的导入处理：从 S3 拉取抓包原件、解析数据包、
// 转换为网络模型并批量落库，配套的任务载荷定义见 PcapS3Object。
package pcap

import (
	"encoding/json"
	"time"

	"github.com/hibiken/asynq"
	"zhongkuisec.com/zhongkui/internal/capture"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

const (
	// PcapImport PCAP文件导入任务的类型名称
	PcapImport = "network:pcapimport"
)

// PcapS3Object 表示从 S3 获取的 PCAP 文件的对象信息。
// 包含会话 ID、时间戳、命名空间、名称和版本等元数据。
type PcapS3Object struct {
	SessionID string    `json:"sessionID"` // 会话唯一标识符
	Timestamp time.Time `json:"timestamp"` // 文件时间戳
	Namespace string    `json:"namespace"` // 命名空间
	Name      string    `json:"name"`      // 文件名称
	Version   string    `json:"version"`   // 版本号
	SecretKey string    `json:"secretKey"` // 密钥
}

// GetObjectKey 获取 S3 对象的完整键名。
// 根据命名空间、名称、版本、时间戳和会话 ID 构建对象键。
func (o *PcapS3Object) GetObjectKey() string {
	return capture.BuildObjectKey(
		o.Namespace,
		o.Name,
		o.Version,
		o.Timestamp,
		serializer.MustHexUUIDv4FromString(o.SessionID),
	)
}

// NewPcapS3ObjectTask 创建一个新的 PCAP 导入异步任务。
// 将 PcapS3Object 序列化为 JSON 并包装为 asynq.Task。
func NewPcapS3ObjectTask(value *PcapS3Object) (*asynq.Task, error) {
	bytes, err := json.Marshal(value)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return asynq.NewTask(PcapImport, bytes, asynq.MaxRetry(0)), nil
}
