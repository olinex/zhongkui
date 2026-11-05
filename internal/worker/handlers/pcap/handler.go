package pcap

import (
	"context"
	"time"

	"github.com/pterm/pterm"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/etl"
	"zhongkuisec.com/zhongkui/internal/pkg/translate"
	"zhongkuisec.com/zhongkui/internal/worker/handlers/base"
)

// pcapHandler 包装 BatchHandler，在任务执行前后注入统计和打印统计信息
type pcapHandler struct {
	handler *base.BatchHandler
}

// Do 执行 PCAP 导入任务，在任务执行期间收集统计信息并在结束后通过 pterm.Debug 打印
func (w *pcapHandler) Do(ctx context.Context, task etl.Task) error {
	stats := NewPcapStats()
	ctx = WithPcapStats(ctx, stats)
	err := w.handler.Do(ctx, task)
	pterm.Debug.Println(translate.T("cli.pcap.import-stats", map[string]any{"Stats": stats.Snapshot()}))
	return errors.WithStack(err)
}

// NewPcapBatchHandler 创建一个用于处理PCAP文件导入的批量任务处理器。
// 该处理器从 S3 获取 PCAP 文件，解析数据包，转换为模型并加载到数据库。
// 任务执行结束后通过 pterm.Debug 打印流量包导入统计信息。
func NewPcapBatchHandler() *pcapHandler {
	return &pcapHandler{
		handler: base.NewBatchHandler(
			100000,
			time.Second,
			ExtractPacketFromPcap,
			[]etl.TaskTransformFunc[any]{
				TransFromPacketToModel,
			},
			[]etl.TaskLoadFunc[any]{
				LoadPacketModelToTable,
			},
			nil,
			nil,
		),
	}
}
