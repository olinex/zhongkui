package pcap

import (
	"time"

	"zhongkui.com/zhongkui/internal/pkg/pipeline"
	"zhongkui.com/zhongkui/internal/tasks/workers"
)

// NewPcapBatchTaskWorker 创建一个用于处理PCAP文件导入的批量任务 worker。
// 该 worker 从 S3 获取 PCAP 文件，解析数据包，转换为模型并加载到数据库。
func NewPcapBatchTaskWorker() *workers.BatchTaskWorker {
	return workers.NewBatchTaskWorker(
		100000,
		time.Second,
		ExtractPacketFromPcap,
		[]pipeline.TransformFunc{
			TransFromPacketToModel,
		},
		[]pipeline.LoadFunc{
			LoadPacketModelToTable,
		},
		nil,
		nil,
	)
}
