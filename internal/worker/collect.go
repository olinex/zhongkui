package worker

import (
	"context"

	"github.com/samber/do/v2"
	"zhongkui.com/zhongkui/internal/pkg/pipeline"
	"zhongkui.com/zhongkui/internal/worker/handlers/pcap"
	"zhongkui.com/zhongkui/internal/worker/processors/flow"
)

func Registry(ctx context.Context, injector do.Injector) {
	// 注册 pcap 导入任务的 worker
	pipeline.HandlerRegister(pcap.PCAP_IMPORT, pcap.NewPcapBatchHandler())

	// 注册分布式 TCP 流处理器
	pipeline.RegisterProcessorFactory(flow.PROC_IPV4_TCP, flow.NewIPv4PacketProcessorFactory(injector))
	pipeline.RegisterProcessorFactory(flow.PROC_IPV6_TCP, flow.NewIPv6PacketProcessorFactory(injector))
}
