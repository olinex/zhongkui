// Package flow 实现基于 TCP 流的分布式抓包处理。
// 包含从 ClickHouse 轮询数据、TCP 流重组、批量输出到 ClickHouse 的完整流水线。
package flow

import (
	"context"
	"fmt"

	"github.com/samber/do/v2"
	"zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/network/tcp"
	"zhongkui.com/zhongkui/internal/pkg/distributed"
	"zhongkui.com/zhongkui/internal/pkg/errgroupx"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/etl"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/provider"
)

const (
	ProcIpv4Tcp = "ipv4-tcp"
	ProcIpv6Tcp = "ipv6-tcp"
)

// PacketProcessor 管理 TCP 流处理的完整生命周期。
// 包含 BatchWriter 和分布式编排器，协调从 ClickHouse 轮询 → TCP 流重组 → 批量写入的完整流水线。
type PacketProcessor[P tcp.TCPPacket] struct {
	logger *logging.Logger
	writer *BatchWriter
	orch   *distributed.Orchestrator[*flowClusterHandler[P], P]
}

// Start 启动处理管道，阻塞直到 ctx 取消。
func (p *PacketProcessor[P]) Start(ctx context.Context, name string) error {
	p.logger.Ctx(ctx).Infof("packet processor %s starting", name)

	wg, ctx := errgroupx.WithContext(ctx)
	wg.GoNamed(fmt.Sprintf("%s-output-flush", name), func() error {
		p.logger.Ctx(ctx).Debugf("packet processor %s writer goroutine starting", name)
		return p.writer.Start(ctx)
	})
	wg.GoNamed(fmt.Sprintf("%s-input-route", name), func() error {
		p.logger.Ctx(ctx).Debugf("packet processor %s orchestrator goroutine starting", name)
		return p.orch.Start(ctx)
	})
	if err := wg.Wait(); err != nil {
		p.logger.Ctx(ctx).Errorf("packet processor %s exited with error: %v", name, err)
		return errors.WithStack(err)
	}
	p.logger.Ctx(ctx).Infof("packet processor %s stopped", name)
	return nil
}

// Shutdown 优雅关闭处理器。
func (p *PacketProcessor[P]) Shutdown() {
	p.logger.Infof("packet processor shutting down")
	p.orch.Shutdown()
}

// NewIPv4PacketProcessor 创建 IPv4 TCP 处理器
func NewIPv4PacketProcessorFactory(
	injector do.Injector,
) etl.ProcessorFactory {
	return func(ctx context.Context) (etl.Processor, error) {
		return NewPacketProcessor[*network.PacketIPv4TCP](ctx, injector, ProcIpv4Tcp)
	}
}

// NewIPv6PacketProcessor 创建 IPv6 TCP 处理器
func NewIPv6PacketProcessorFactory(
	injector do.Injector,
) etl.ProcessorFactory {
	return func(ctx context.Context) (etl.Processor, error) {
		return NewPacketProcessor[*network.PacketIPv6TCP](ctx, injector, ProcIpv6Tcp)
	}
}

// NewPacketProcessor 创建 TCP 流处理器。
func NewPacketProcessor[P tcp.TCPPacket](
	ctx context.Context,
	injector do.Injector,
	name string,
) (etl.Processor, error) {
	if client, err := provider.InvokeEtcdClient(injector); err != nil {
		return nil, errors.WithStack(err)
	} else if db, err := provider.InvokeRawDatabase(injector); err != nil {
		return nil, errors.WithStack(err)
	} else if logger, err := provider.InvokePipelineLogger(injector); err != nil {
		return nil, errors.WithStack(err)
	} else if param, err := etl.GetProcParam[Param](ctx); err != nil {
		return nil, errors.WithStack(err)
	} else {
		clusterCfg := &distributed.ClusterConfig{
			VNodeCount:   param.VNodeCount,
			ChanSize:     param.BatchSize,
			PollInterval: param.PollInterval,
			SaveInterval: param.SaveInterval,
			ReplayWindow: param.ReplayWindow,
		}
		writer := NewBatchWriter(db, logger, param)
		handler := newFlowClusterHandler[P](db, logger, writer, param)
		return &PacketProcessor[P]{
			logger: logger,
			writer: writer,
			orch: distributed.NewOrchestrator(
				client,
				logger,
				clusterCfg,
				handler,
				name,
			),
		}, nil
	}
}
