package tasks

import (
	"zhongkui.com/zhongkui/internal/pkg/pipeline"
	"zhongkui.com/zhongkui/internal/tasks/flow"
	"zhongkui.com/zhongkui/internal/tasks/pcap"
)

func init() {
	// 注册 pcap 导入任务的 worker
	pipeline.WorkerRegister(pcap.PCAP_IMPORT, pcap.NewPcapBatchTaskWorker())

	// 注册 TCP 流重组任务的 worker
	pipeline.WorkerRegister(flow.PERIODIC_IPV4_REASSEMBLE, flow.NewIPv4FlowVersionTaskWorker())
	pipeline.WorkerRegister(flow.PERIODIC_IPV6_REASSEMBLE, flow.NewIPv6FlowVersionTaskWorker())
}
