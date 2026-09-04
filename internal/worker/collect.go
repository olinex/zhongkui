// Package worker 提供分布式任务处理引擎。
// 包含 worker 注册、流水线处理器工厂和基于 etcd 的分布式任务协调。
package worker

import (
	"context"

	"github.com/samber/do/v2"
	"zhongkui.com/zhongkui/internal/pkg/etl"
	"zhongkui.com/zhongkui/internal/worker/handlers/mail"
	"zhongkui.com/zhongkui/internal/worker/handlers/notify"
	"zhongkui.com/zhongkui/internal/worker/handlers/pcap"
	"zhongkui.com/zhongkui/internal/worker/handlers/tag_event"
	"zhongkui.com/zhongkui/internal/worker/processors/flow"
)

// Registry 注册所有 worker 处理器到任务队列与处理器框架。
// 目前注册以下处理器：
//   - pcap 导入任务的 batch handler（pcap.NewPcapBatchHandler）
//   - 标签事件检测（tag:event:detect）与抑制评估（tag:event:suppress）handler
//   - 通用通知发送（notify:send）handler，由标签事件等事件源投递
//   - 分布式 TCP 流处理器工厂（IPv4 和 IPv6）
func Registry(ctx context.Context, injector do.Injector) {
	// 注册 pcap 导入任务的 worker
	etl.HandlerRegister(pcap.PcapImport, pcap.NewPcapBatchHandler())

	// 注册标签事件检测、抑制评估与无效记录清理 worker
	etl.HandlerRegister(tag_event.TagEventDetectTask, tag_event.NewEventDetectHandler(injector))
	etl.HandlerRegister(tag_event.TagEventSuppressTask, tag_event.NewEventSuppressHandler(injector))
	etl.HandlerRegister(tag_event.TagEventCleanupTask, tag_event.NewEventCleanupHandler(injector))

	// 注册系统级邮件直发 worker（mail:send），按系统 smtp 配置直接向指定收件人投递，
	// 不经过数据库通知渠道/目标
	etl.HandlerRegister(mail.MailSendTask, mail.NewMailSendHandler(injector))

	// 注册通用通知组发送与单目标直投 worker
	etl.HandlerRegister(notify.SendGroupTask, notify.NewSendGroupHandler(injector))
	etl.HandlerRegister(notify.SendTargetTask, notify.NewSendTargetHandler(injector))

	// 注册分布式 TCP 流处理器
	etl.RegisterProcessorFactory(flow.ProcIpv4Tcp, flow.NewIPv4PacketProcessorFactory(injector))
	etl.RegisterProcessorFactory(flow.ProcIpv6Tcp, flow.NewIPv6PacketProcessorFactory(injector))
}
