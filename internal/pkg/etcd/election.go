// Package etcd 提供了 etcd 客户端的配置管理与连接创建功能。
// 该包封装了 etcd client/v3 客户端的配置结构，支持 TLS 安全连接，
// 并提供了配置描述和配置提取的便捷方法，方便从 viper 配置中加载 etcd 相关配置。
//
// 此外，该包还基于 etcd concurrency 包实现了分布式领导者选举机制，
// 支持多实例部署场景下的主备切换和高可用调度。
package etcd

import (
	"context"
	"fmt"

	"go.etcd.io/etcd/client/v3/concurrency"
	"golang.org/x/sync/errgroup"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

// electionPrefix 是领导者选举在 etcd 中的键前缀，
// 通过 WithPrefix 函数拼接全局前缀，最终格式为 "<GLOBAL_PREFIX>/leader-election/"。
// 所有参与同一选举的竞争者都将在该前缀下创建各自的选举键。
var (
	electionPrefix = WithPrefix("leader-election/")
)

// ElectionHandler 定义了领导者选举中候选者需要实现的接口。
// 当某个实例通过选举成为领导者后，将调用该接口的 Start 方法启动领导者的业务逻辑；
// 当实例需要停止时，将调用 Shutdown 方法进行优雅关闭。
//
// 该接口的设计遵循"关注点分离"原则：
//   - Election 负责 etcd 选举的底层机制（会话管理、竞选、监听）
//   - ElectionHandler 负责成为领导者后的业务逻辑
//
// 方法说明:
//   - Name: 返回选举名称，用于区分不同的选举场景（如 "scheduler"、"task-dispatcher" 等），
//     同一名称的候选者将竞争同一个领导者位置
//   - CandidateID: 返回当前候选者的唯一标识，用于在选举结果中判断是否自己成为了领导者，
//     通常使用实例的唯一标识（如主机名+端口、UUID 等）
//   - Start: 当该候选者成为领导者时被调用，执行领导者的业务逻辑（如启动定时调度、任务分发等），
//     该方法应阻塞运行直到上下文取消或业务逻辑完成
//   - Shutdown: 优雅关闭领导者的业务逻辑，在 Election.Shutdown 时被调用
type ElectionHandler interface {
	Name() string
	CandidateID() string
	Start(ctx context.Context) error
	Shutdown(ctx context.Context) error
}

// Election 封装了基于 etcd 的分布式领导者选举逻辑。
// 它利用 etcd 的 concurrency 包实现多实例间的领导者竞选，
// 确保同一时刻只有一个实例运行关键业务逻辑（如定时任务调度）。
//
// 工作流程:
//  1. 创建 etcd Session（带 TTL 的租约，自动心跳保活）
//  2. 基于 Session 创建选举实例，参与竞选
//  3. 并发执行两个 goroutine：
//     a) Campaign goroutine：阻塞式参与竞选，直到成为领导者
//     b) Observe goroutine：监听选举结果，当确认自己成为领导者后启动业务逻辑
//  4. 当实例挂掉时，Session 的 TTL 到期后自动释放，其他实例可重新竞选
//
// 字段说明:
//   - ctx: 上下文对象，用于控制选举的生命周期，取消该上下文将停止选举
//   - client: etcd 客户端实例，用于与 etcd 集群通信
//   - logger: 日志记录器，用于记录选举过程中的关键事件
//   - handler: 选举事件处理器，定义了成为领导者后的业务逻辑
type Election struct {
	ctx     context.Context
	client  *EtcdClient
	logger  *logging.Logger
	handler ElectionHandler
}

// NewElection 创建一个新的领导者选举实例。
//
// 该函数仅初始化选举所需的结构体，不会立即参与竞选。
// 竞选过程在调用 Start 方法后才开始。
//
// 参数:
//   - ctx: 上下文对象，用于控制选举的生命周期，取消该上下文将停止竞选
//   - client: etcd 客户端实例，必须已成功连接到 etcd 集群
//   - logger: 日志记录器，用于记录选举过程中的状态变更和错误信息
//   - handler: 选举事件处理器，定义了选举名称、候选者标识及领导者的业务逻辑
//
// 返回值:
//   - *Election: 初始化完成的选举实例
func NewElection(
	ctx context.Context,
	client *EtcdClient,
	logger *logging.Logger,
	handler ElectionHandler,
) *Election {
	return &Election{
		ctx:     ctx,
		client:  client,
		logger:  logger,
		handler: handler,
	}
}

// Start 启动领导者选举流程，该方法会阻塞直到选举上下文被取消或领导者的业务逻辑返回。
//
// 执行流程:
//  1. 创建 etcd Session（TTL 为 30 秒），Session 会在后台自动保持心跳。
//     如果当前实例挂掉，Session 过期后锁自动释放，其他候选者可以重新竞选。
//  2. 基于 Session 创建选举实例（Elector），所有竞争同一领导者位置的实例
//     必须使用相同的选举路径（由 electionPrefix + handler.Name() 组成）。
//  3. 启动监听通道（Observe），用于实时获取选举结果变更。
//  4. 使用 errgroup 并发执行两个 goroutine：
//     a) Campaign goroutine：调用 elector.Campaign 参与竞选，该方法会阻塞直到
//     当前 Session 成为领导者或上下文被取消
//     b) Observe goroutine：持续监听选举结果，当检测到自己成为领导者时，
//     调用 handler.Start 启动领导者的业务逻辑
//
// 选举结果处理逻辑:
//   - 选举通道关闭：返回错误，表示选举过程中出现了异常
//   - 无领导者（KVs 为空）：继续等待，可能选举正在进行中
//   - 多个领导者（KVs > 1）：触发 panic，这是严重的一致性错误
//   - 其他实例成为领导者：记录日志，继续参与下一轮竞选
//   - 自己成为领导者：调用 handler.Start，启动领导者的业务逻辑
//
// 返回值:
//   - error: 选举过程中的错误，可能包括 Session 创建失败、Campaign 失败、
//     选举通道异常关闭或 handler.Start 返回的错误
func (e *Election) Start() error {
	name := e.handler.Name()
	// Session会在后台自动保持心跳。如果此实例挂掉，Session过期，锁自动释放。
	session, err := concurrency.NewSession(
		e.client,
		concurrency.WithContext(e.ctx),
		concurrency.WithTTL(30),
	)
	if err != nil {
		return errors.WithStack(err)
	}
	defer session.Close()
	// 创建选举实例，所有竞争者的选举路径必须相同
	elector := concurrency.NewElection(session, fmt.Sprintf(`%s%s`, electionPrefix, name))
	electResultChan := elector.Observe(e.ctx)
	// 参与选举，非阻塞式
	candidateID := e.handler.CandidateID()
	wg, innerCtx := errgroup.WithContext(e.ctx)
	wg.Go(func() error {
		// Campaign方法会阻塞，直到当前session成为领导者
		return elector.Campaign(innerCtx, candidateID)
	})
	// 监听选举结果（关键：用于知道何时成为领导者）
	wg.Go(func() error {
		for {
			select {
			// 外部上下文结束
			case <-innerCtx.Done():
				e.logger.With("context", e.ctx).Info("stop observe leader election")
				return nil
			// 完成了一次选举过程
			case rsp, open := <-electResultChan:
				if !open {
					return errors.Newf(`Elect result channel %s is closed`, name)
				}
				// 本次选举并没有选出领导者
				if len(rsp.Kvs) == 0 {
					continue
				}
				// 本次选举选出了多名领导者，严重错误
				if len(rsp.Kvs) > 1 {
					panic("get more than one leader from election")
				}
				leaderID := string(rsp.Kvs[0].Value)
				e.logger.With("context", e.ctx).Infof(
					`New leader %s %s has been elected, current candidate id is %s`,
					name,
					leaderID,
					candidateID,
				)
				// 本次选举的领导者不是当前进程，继续参与选举
				if string(rsp.Kvs[0].Value) != candidateID {
					e.logger.With("context", e.ctx).Infof(`Current candidate %s is not leader, continue campaign`, candidateID)
					continue
				}
				// 本次选举成为了领导者，有权运行定时调度
				e.logger.With("context", e.ctx).Infof(`Handler %s is running...`, name)
				return e.handler.Start(e.ctx)
			}
		}
	})
	return wg.Wait()
}

// Shutdown 优雅关闭选举实例，通知当前的选举处理器停止运行。
//
// 该方法会调用 handler.Shutdown，触发领导者的业务逻辑进行优雅关闭。
// 注意：该方法不会主动退出选举，Session 的释放依赖于上下文取消后
// Session TTL 到期自动过期。
//
// 调用时机：
//   - 应用接收到终止信号（如 SIGTERM）
//   - 需要主动放弃领导者角色时
func (e *Election) Shutdown() {
	e.handler.Shutdown(e.ctx)
}
