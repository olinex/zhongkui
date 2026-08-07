// Package runtime 提供统一的服务生命周期编排。
// 该包定义统一的 Service 接口，将任务队列 Server、etcd 选举 Scheduler 与
// 分布式常驻 Processor 的生命周期抽象为 Start/Shutdown 两个阶段，
// 并通过 Group 并行启动、优雅关闭全部服务。
package runtime

import (
	"context"

	"zhongkui.com/zhongkui/internal/pkg/etcd"
	"zhongkui.com/zhongkui/internal/pkg/etl"
	"zhongkui.com/zhongkui/internal/pkg/pipeline"
)

// Service 统一的服务生命周期接口。
// Start 阻塞运行服务主体逻辑，直到 ctx 取消或服务内部错误返回；
// Shutdown 在 Start 返回后由编排器调用，执行优雅关闭。
//
// 现有三类服务通过本包的适配构造器统一到此接口：
//   - pipeline.Server：asynq 事件驱动任务队列服务器
//   - etcd.Election：基于 etcd 分布式选举的调度器
//   - etl.Processor：分布式流式常驻处理器
type Service interface {
	Start(ctx context.Context) error
	Shutdown(ctx context.Context) error
}

// Named 可选接口，为服务提供名称，用于日志与错误上下文标识。
type Named interface {
	Service
	Name() string
}

// pipelineService 将 pipeline.Server 适配为 Service。
type pipelineService struct {
	server *pipeline.Server
}

// NewPipelineService 包装任务队列服务器为统一 Service。
// 适配 pipeline.Server 的无参 Shutdown 语义，Start 返回后由编排器触发关闭。
func NewPipelineService(server *pipeline.Server) Service {
	return &pipelineService{server: server}
}

func (s *pipelineService) Name() string { return "server" }

func (s *pipelineService) Start(ctx context.Context) error {
	return s.server.Start(ctx)
}

func (s *pipelineService) Shutdown(_ context.Context) error {
	s.server.Shutdown()
	return nil
}

// electionService 将基于 etcd 选举的调度器适配为 Service。
type electionService struct {
	election *etcd.Election
}

// NewElectionService 包装基于 etcd 选举的调度器为统一 Service。
// 适配 etcd.Election 的无参 Shutdown 语义，选举退出后由编排器触发关闭。
func NewElectionService(election *etcd.Election) Service {
	return &electionService{election: election}
}

func (e *electionService) Name() string { return "scheduler" }

func (e *electionService) Start(ctx context.Context) error {
	return e.election.Start(ctx)
}

func (e *electionService) Shutdown(_ context.Context) error {
	e.election.Shutdown()
	return nil
}

// processorService 将分布式常驻处理器适配为 Service。
type processorService struct {
	name      string
	processor etl.Processor
}

// NewProcessorService 包装分布式常驻处理器为统一 Service。
// name 为处理器注册名，Start 时透传给 processor.Start 作为处理器标识。
func NewProcessorService(name string, processor etl.Processor) Service {
	return &processorService{name: name, processor: processor}
}

func (p *processorService) Name() string { return p.name }

func (p *processorService) Start(ctx context.Context) error {
	return p.processor.Start(ctx, p.name)
}

func (p *processorService) Shutdown(_ context.Context) error {
	p.processor.Shutdown()
	return nil
}
