package pcap

import (
	"context"
	"time"

	"github.com/pterm/pterm"
	"zhongkui.com/zhongkui/internal/pkg/pipeline"
	"zhongkui.com/zhongkui/internal/tasks/workers"
)

// pcapTaskWorker 包装 BatchTaskWorker，在任务执行前后注入统计和打印统计信息
type pcapTaskWorker struct {
	worker *workers.BatchTaskWorker
}

// Do 执行 PCAP 导入任务，在任务执行期间收集统计信息并在结束后通过 pterm.Debug 打印
func (w *pcapTaskWorker) Do(ctx context.Context, task *pipeline.Task) error {
	stats := NewPcapStats()
	ctx = WithPcapStats(ctx, stats)
	err := w.worker.Do(ctx, task)
	pterm.Debug.Printfln("[pcap import stats] %s", stats.Snapshot())
	return err
}

// NewPcapBatchTaskWorker 创建一个用于处理PCAP文件导入的批量任务 worker。
// 该 worker 从 S3 获取 PCAP 文件，解析数据包，转换为模型并加载到数据库。
// 任务执行结束后通过 pterm.Debug 打印流量包导入统计信息。
func NewPcapBatchTaskWorker() *pcapTaskWorker {
	return &pcapTaskWorker{
		worker: workers.NewBatchTaskWorker(
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
		),
	}
}
