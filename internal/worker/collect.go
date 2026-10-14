// Package worker 提供异步任务的消费侧编排：把各业务域的任务处理器注册进 etl 的全局注册表，
// 供 worker 进程按任务类型分发执行。本包本身不实现队列与调度——投递、消费与定时触发在
// internal/pkg/pipeline，业务逻辑在各子包；本包是二者的装配点，也是「有哪些任务可被消费」的
// 唯一清单。
//
// ## 两类执行模型
//
// 任务按其生命周期分两类，对应 etl 的两组契约：
//
//   - Handler 处理单个短期任务，执行完立即返回，适用于事件驱动的一次性工作（如解析一份文档、
//     投递一封邮件）。经 etl.HandlerRegister 以「任务类型 → 处理器」登记，由队列消费进程按
//     类型路由。
//   - Processor 是长期运行的后台协程，经 Start 启动循环、Shutdown 优雅停止，适用于分布式常驻
//     处理（如抓包 TCP 流处理）。经 etl.RegisterProcessorFactory 以「处理器名 → 工厂」登记，
//     由外部运行时按需实例化并编排生命周期，不经过队列。
//
// 两类之上是统一的 etl.Task 抽象（Type 与 Payload），使 etl 包不依赖具体队列实现；asynq.Task
// 满足该契约，故队列任务可直接被 Handler 消费。
//
// ## 注册清单
//
// 本包的 Registry 在启动期一次性完成全部登记，覆盖以下任务：
//
//   - pcap.PcapImport：PCAP 抓包文件导入的批量处理（经 BatchHandler）；
//   - `tag_event.TagEventDetectTask` / `TagEventSuppressTask` / `TagEventCleanupTask`：标签事件检测、
//     抑制评估与无效记录清理；
//   - mail.MailSendTask（mail:send）：系统级邮件直发，按系统 smtp 配置直接投递，不经数据库通知
//     渠道与目标；
//   - notify.SendGroupTask / SendTargetTask：通用通知的组发送与单目标直投；
//   - knowledge.ParseTask（ai:doc:parse）：知识文档导入解析，拉取解密 S3 原件、抽取文本、切块
//     向量化并收敛文档状态。
//
// 同时登记两个分布式 TCP 流处理器工厂（flow.ProcIpv4Tcp / flow.ProcIpv6Tcp）。任务类型常量均
// 由其所属子包声明（如 knowledge.ParseTask），本包只引用，保证「投递侧与消费侧」共享同一字面量。
//
// ## 配置驱动的处理函数
//
// etl.HandlerRegistry(params) 把全局注册表快照为一份 HandlerFuncs，并把 params 中命名的处理器
// 包装为「调用前向 ctx 注入该处理器的配置参数」；处理器内部经 etl.GetHandlerParam[T] 取回强类型
// 配置。由此，同一份代码可按配置文件为不同任务注入不同参数，而无需把配置烘进处理器构造。
//
// ## 批量任务与失败语义
//
// 批量任务（如 pcap 导入）由子包 handlers/base 的 BatchHandler 承载：其内核是 etl.Chain，把
// extract → transforms → buffer → loads 用原生 channel + errgroup 编排，并内置前置/后置钩子与
// OTEL 指标。任一阶段出错即取消整条流水线，但已提取的数据在缓冲冲刷后仍会被加载完成，保证
// 「已提取不丢」。消费侧另由 pipeline 在最外层挂 RecoverWithStack 中间件兜底 panic 并把带堆栈的
// 错误回写 asynq（可在任务详情查看），是「坏输入不至于让 worker 进程崩溃」的最后一道防线。
//
// ## 失败与重试
//
// 是否重试由投递侧的任务选项决定，而非消费侧：例如 knowledge.ParseTask 以 asynq.MaxRetry(0)
// 投递，解析失败不自动重试，由用户经「重新解析」显式重跑；处理器需在失败路径上把领域状态（如
// 文档 status）收敛为终态，避免任务既不重试又留下悬空状态。
//
// ## 设计取舍
//
// 把登记集中在一处而非散落各子包，是为了让「worker 进程能消费哪些任务」成为可审阅的单一清单：
// 新增任务必然在此处出现，遗漏与重复一目了然。处理器实现留在各业务子包，本包只做装配，故
// worker 进程的启动路径不承载业务逻辑，业务逻辑也可独立测试。任务类型常量由子包声明、本包引用，
// 保证投递侧与消费侧永远不会因各自复制字面量而失配。
package worker

import (
	"context"

	"github.com/samber/do/v2"
	"zhongkuisec.com/zhongkui/internal/pkg/etl"
	"zhongkuisec.com/zhongkui/internal/worker/handlers/knowledge"
	"zhongkuisec.com/zhongkui/internal/worker/handlers/mail"
	"zhongkuisec.com/zhongkui/internal/worker/handlers/notify"
	"zhongkuisec.com/zhongkui/internal/worker/handlers/pcap"
	"zhongkuisec.com/zhongkui/internal/worker/handlers/tag_event"
	"zhongkuisec.com/zhongkui/internal/worker/processors/flow"
)

// Registry 注册所有 worker 处理器到任务队列与处理器框架。
// 目前注册以下处理器：
//   - pcap 导入任务的 batch handler（pcap.NewPcapBatchHandler）
//   - 标签事件检测（tag:event:detect）与抑制评估（tag:event:suppress）handler
//   - 通用通知发送（notify:send）handler，由标签事件等事件源投递
//   - 知识文档导入解析（ai:doc:parse）handler，由知识库导入接口投递
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

	// 注册知识文档导入解析 worker（ai:doc:parse）：拉取解密 S3 原件、抽取文本、切块向量化并收敛状态
	etl.HandlerRegister(knowledge.ParseTask, knowledge.NewParseHandler(injector))

	// 注册分布式 TCP 流处理器
	etl.RegisterProcessorFactory(flow.ProcIpv4Tcp, flow.NewIPv4PacketProcessorFactory(injector))
	etl.RegisterProcessorFactory(flow.ProcIpv6Tcp, flow.NewIPv6PacketProcessorFactory(injector))
}
