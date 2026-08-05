// Package worker 提供分布式任务处理引擎。
// 包含 worker 注册、流水线处理器工厂和基于 etcd 的分布式任务协调。
package worker

import (
	"context"

	"github.com/samber/do/v2"
	"zhongkui.com/zhongkui/internal/pkg/pipeline"
	"zhongkui.com/zhongkui/internal/worker/handlers/pcap"
	"zhongkui.com/zhongkui/internal/worker/processors/flow"
)

// Registry 注册所有 worker 处理器到流水线框架。
// 目前注册两种类型的处理器：
//   - pcap 导入任务的 batch handler（pcap.NewPcapBatchHandler）
//   - 分布式 TCP 流处理器工厂（IPv4 和 IPv6）
func Registry(ctx context.Context, injector do.Injector) {
	// 注册 pcap 导入任务的 worker
	pipeline.HandlerRegister(pcap.PcapImport, pcap.NewPcapBatchHandler())

	// 注册分布式 TCP 流处理器
	pipeline.RegisterProcessorFactory(flow.ProcIpv4Tcp, flow.NewIPv4PacketProcessorFactory(injector))
	pipeline.RegisterProcessorFactory(flow.ProcIpv6Tcp, flow.NewIPv6PacketProcessorFactory(injector))
}
