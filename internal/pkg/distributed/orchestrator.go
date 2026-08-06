package distributed

import (
	"context"
	"fmt"
	"hash/fnv"
	"time"

	"zhongkui.com/zhongkui/internal/pkg/errgroupx"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/etcd"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

// HashKeyToShard 使用 FNV-1a 32 位哈希算法计算 key 对应的分片索引。
// 对 key 取 FNV-1a 32-bit hash，然后 mod shardCount 得到 [0, shardCount) 范围内的分片索引。
// 返回的索引可用于定位数据项应当归属的虚拟节点分片。
func HashKeyToShard(key string, shardCount int) (int, error) {
	h := fnv.New32a()
	if count, err := h.Write([]byte(key)); err != nil {
		return 0, errors.WithStack(err)
	} else if count != len(key) {
		return 0, errors.New("hash key length not match")
	}
	return int(h.Sum32() % uint32(shardCount)), nil
}

// Orchestrator 分布式数据处理编排器。
// 协调集群成员之间的分布式数据处理流程，使用 etcd 进行成员注册与发现，
// 并持久化各成员的处理水位（watermark）。每个编排器实例代表集群中的一个成员，
// 通过 ClusterHandler 接口与具体业务逻辑解耦。
//
// 类型参数:
//   - Handler: 实现 ClusterHandler[Item] 接口的业务处理器
//   - Item: 业务数据项的类型
type Orchestrator[Handler ClusterHandler[Item], Item any] struct {
	client   *etcd.EtcdClient
	logger   *logging.Logger
	cluster  *etcd.Cluster
	config   *ClusterConfig
	handler  Handler
	name     string
	memberID string
}

// NewOrchestrator 创建分布式编排器实例。
//
// 参数:
//   - client: etcd 客户端，用于成员注册和水位持久化
//   - logger: 结构化日志实例
//   - config: 集群运行时配置（虚拟节点数、通道大小、轮询及持久化间隔等）
//   - handler: 业务处理器实现，负责 Poll（拉取数据）和 Start（消费数据）
//   - name: 集群名称，同一集群的实例共享命名空间
func NewOrchestrator[Handler ClusterHandler[Item], Item any](
	client *etcd.EtcdClient,
	logger *logging.Logger,
	config *ClusterConfig,
	handler Handler,
	name string,
) *Orchestrator[Handler, Item] {
	cluster := etcd.NewCluster(client, name)
	return &Orchestrator[Handler, Item]{
		client:   client,
		logger:   logger,
		cluster:  cluster,
		config:   config,
		handler:  handler,
		name:     name,
		memberID: cluster.MemberID(),
	}
}

func (o *Orchestrator[Handler, Item]) memberIDPrefix() string {
	return fmt.Sprintf(`%s%s/%s/`, etcd.ClusterEtcdPrefix, o.name, o.memberID)
}

func (o *Orchestrator[Handler, Item]) watermarkKey() string {
	return fmt.Sprintf("%swatermark", o.memberIDPrefix())
}

func (o *Orchestrator[Handler, Item]) saveWatermark(ctx context.Context, watermark time.Time) error {
	key := o.watermarkKey()
	if _, err := o.client.Put(ctx, key, watermark.Format(time.RFC3339Nano)); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (o *Orchestrator[Handler, Item]) getWatermark(ctx context.Context) (time.Time, error) {
	key := o.watermarkKey()
	resp, err := o.client.Get(ctx, key)
	minWatermark := time.Now().Add(-o.config.ReplayWindow)
	if err != nil {
		return minWatermark, errors.WithStack(err)
	}
	if len(resp.Kvs) == 0 {
		return minWatermark, nil
	}
	raw := resp.Kvs[0].Value
	if len(raw) == 0 {
		return minWatermark, nil
	}
	if wm, err := time.Parse(time.RFC3339Nano, string(raw)); err != nil {
		return minWatermark, errors.WithStack(err)
	} else {
		return wm, nil
	}
}

func (o *Orchestrator[Handler, Item]) getWatermarkRange(watermark time.Time) (time.Time, time.Time) {
	now := time.Now()
	minWatermark := now.Add(-o.config.ReplayWindow)
	maxWatermark := now.Add(-o.config.PollOffset)
	if watermark.After(minWatermark) && watermark.Before(maxWatermark) {
		return watermark, maxWatermark
	}
	return minWatermark, maxWatermark
}

// GetMemberID 返回当前编排器实例在 etcd 集群中的唯一成员标识。
// 成员 ID 由 etcd.Cluster 在实例创建时分配，用于标识该实例在集群中的身份。
func (o *Orchestrator[Handler, Item]) GetMemberID() string {
	return o.memberID
}

// Start 运行分布式处理管道。
//
// 编排器负责集群成员管理、水位持久化及主循环调度，不进行分片操作。
// 数据分片由 ClusterHandler 实现方在 Poll 方法中通过 vnodeIndices
// 自行处理，Start 方法中由实现方自行决定处理逻辑。
//
// 执行流程:
//  1. 加入集群（etcd lease 注册 + Watch 成员变化）
//  2. 创建 items 通道，启动 handler.Start() goroutine
//  3. 主循环:
//     a. 从 etcd 加载上次水位
//     b. 按 pollInterval 定时调用 handler.Poll() 拉取数据
//     c. 将拉取到的数据项转发到 handler.Start 的 items 通道
//     d. 按 saveInterval 定时持久化水位
//  4. ctx 取消时:
//     a. 最终持久化水位
//     b. cluster.Leave() 注销成员
//     c. 关闭 items 通道，等待 handler.Start goroutine 退出
func (o *Orchestrator[Handler, Item]) Start(ctx context.Context) error {
	return o.cluster.Transaction(ctx, func(ctx context.Context) error {
		o.logger.Ctx(ctx).Infof(
			"orchestrator %s started, memberID=%s, clusterSize=%d",
			o.name,
			o.memberID,
			o.cluster.MemberCount(),
		)

		// 创建 items 通道，连接 Poll（生产者）和 Start（消费者）
		itemChan := make(chan Item, o.config.ChanSize)
		defer close(itemChan)

		// 启动 handler.Start 消费者 goroutine
		wg, innerCtx := errgroupx.WithContext(ctx)
		wg.GoNamed(
			fmt.Sprintf("%s-%s", o.name, o.memberID),
			func() error {
				o.logger.Ctx(innerCtx).Infof("orchestrator %s handler started, memberID=%s", o.name, o.memberID)
				if err := o.handler.Start(innerCtx, itemChan); err != nil {
					o.logger.Ctx(innerCtx).Errorf("orchestrator %s handler exited with error, memberID=%s, err=%v", o.name, o.memberID, err)
					return errors.WithStack(err)
				} else {
					o.logger.Ctx(innerCtx).Infof("orchestrator %s handler exited cleanly, memberID=%s", o.name, o.memberID)
					return nil
				}
			},
		)

		// 主循环：定时 Poll 拉取数据 + 定时持久化水位
		pollTicker := time.NewTicker(o.config.PollInterval)
		defer pollTicker.Stop()

		saveTicker := time.NewTicker(o.config.SaveInterval)
		defer saveTicker.Stop()

		watermark, err := o.getWatermark(ctx)
		if err != nil {
			o.logger.Ctx(innerCtx).Errorf("orchestrator %s get initial watermark failed: %v", o.name, err)
			return errors.WithStack(err)
		}
		o.logger.Ctx(innerCtx).Infof(
			"orchestrator %s initial watermark=%s, memberID=%s",
			o.name,
			watermark.Format(time.RFC3339Nano),
			o.memberID,
		)

		for {
			select {
			case <-innerCtx.Done():
				// 区分外部关闭与 handler 异常退出：外部 ctx 未取消说明是 handler 触发 errgroup 取消
				if ctx.Err() != nil {
					o.logger.Ctx(innerCtx).Infof(
						"orchestrator %s shutting down (external signal), memberID=%s, finalWatermark=%s",
						o.name, o.memberID, watermark.Format(time.RFC3339Nano))
				} else {
					o.logger.Ctx(innerCtx).Errorf(
						"orchestrator %s shutting down (handler exited), memberID=%s, finalWatermark=%s",
						o.name, o.memberID, watermark.Format(time.RFC3339Nano))
				}
				return nil

			case <-pollTicker.C:
				pollStart := time.Now()
				dispatchChan := make(chan Item, o.config.ChanSize)
				pollWg, pollCtx := errgroupx.WithContext(innerCtx)
				pollWg.GoNamed(
					fmt.Sprintf(`%s-%s-poll`, o.name, o.memberID),
					func() error {
						defer close(dispatchChan)
						prevWatermark, curWatermark := o.getWatermarkRange(watermark)
						o.logger.Ctx(pollCtx).Debugf(
							"orchestrator %s poll started, memberID=%s, watermarkRange=(%s,%s], vnodeIndices=%v",
							o.name,
							o.memberID,
							prevWatermark.Format(time.RFC3339Nano),
							curWatermark.Format(time.RFC3339Nano),
							o.cluster.VNodeIndicesOf(o.memberID),
						)
						err := o.handler.Poll(
							pollCtx,
							prevWatermark,
							curWatermark,
							o.cluster.VNodeIndicesOf(o.memberID),
							dispatchChan,
						)
						o.logger.Ctx(pollCtx).Debugf(
							"orchestrator %s poll finished, memberID=%s, duration=%s, err=%v",
							o.name,
							o.memberID,
							time.Since(pollStart),
							err,
						)
						return err
					},
				)
				pollWg.GoNamed(
					fmt.Sprintf(`%s-%s-dispatch`, o.name, o.memberID),
					func() error {
						wm := watermark
						total := 0
						dispatched := 0
						dropped := 0
						for item := range dispatchChan {
							total++
							select {
							case itemChan <- item:
								wm = o.handler.Watermark(item)
								dispatched++
							default:
								dropped++
								// 通道满，丢弃当前项（下次 poll 重新拉取）
							}
						}
						o.logger.Ctx(pollCtx).Infof(
							"orchestrator %s poll result, memberID=%s, total=%d, dispatched=%d, dropped=%d, watermark=%s",
							o.name,
							o.memberID,
							total,
							dispatched,
							dropped,
							wm.Format(time.RFC3339Nano),
						)
						if total > 0 {
							watermark = wm
						}
						return nil
					},
				)
				if err := pollWg.Wait(); err != nil {
					o.logger.Ctx(innerCtx).Errorf(
						"orchestrator %s poll failed, memberID=%s, err=%v",
						o.name,
						o.memberID,
						err,
					)
					return errors.WithStack(err)
				}

			case <-saveTicker.C:
				if !watermark.IsZero() {
					if err := o.saveWatermark(ctx, watermark); err != nil {
						o.logger.Ctx(ctx).Errorf(
							"orchestrator %s save watermark failed, memberID=%s, watermark=%s, err=%v",
							o.name,
							o.memberID,
							watermark.Format(time.RFC3339Nano),
							err,
						)
					} else {
						o.logger.Ctx(ctx).Debugf(
							"orchestrator %s watermark saved, memberID=%s, watermark=%s",
							o.name,
							o.memberID,
							watermark.Format(time.RFC3339Nano),
						)
					}
				}
			}
		}
	})
}

// Shutdown 优雅关闭编排器。
// 委托 handler.Shutdown() 执行业务层清理（如关闭数据库连接、等待处理中的任务完成等）。
// 该方法不会关闭 etcd 连接或移除集群成员，仅负责业务处理器的资源释放。
func (o *Orchestrator[Handler, Item]) Shutdown() {
	o.handler.Shutdown()
}
