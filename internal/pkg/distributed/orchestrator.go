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

var (
	clusterPrefix = etcd.WithPrefix("clusters/")
)

// hashKeyToShard 计算 key 对应的分片索引。
func HashKeyToShard(key string, shardCount int) (int, error) {
	h := fnv.New32a()
	if count, err := h.Write([]byte(key)); err != nil {
		return 0, errors.WithStack(err)
	} else if count != len(key) {
		return 0, errors.New("hash key length not match")
	}
	return int(h.Sum32() % uint32(shardCount)), nil
}

type Orchestrator[Handler ClusterHandler[Item], Item any] struct {
	client   *etcd.EtcdClient
	logger   *logging.Logger
	cluster  *etcd.Cluster
	config   *ClusterConfig
	handler  Handler
	name     string
	memberID string
}

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

func (o *Orchestrator[Handler, Item]) clusterPrefix() string {
	return fmt.Sprintf(`%s%s/%s/`, clusterPrefix, o.name, o.memberID)
}

func (o *Orchestrator[Handler, Item]) watermarkKey() string {
	return fmt.Sprintf("%swatermark", o.clusterPrefix())
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
	defaultWatermark := time.Now().Add(-o.config.ReplayWindow)
	if err != nil {
		return defaultWatermark, errors.WithStack(err)
	}
	if len(resp.Kvs) == 0 {
		return defaultWatermark, nil
	}
	raw := resp.Kvs[0].Value
	if len(raw) == 0 {
		return defaultWatermark, nil
	}
	if wm, err := time.Parse(time.RFC3339Nano, string(raw)); err != nil {
		return defaultWatermark, errors.WithStack(err)
	} else if wm.After(defaultWatermark) {
		return wm, nil
	}
	return defaultWatermark, nil
}

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
		// 3. 创建通道
		itemChan := make(chan Item, o.config.ChanSize)
		defer close(itemChan)

		// 4. 启动所有分片的处理 goroutine
		wg, innerCtx := errgroupx.WithContext(ctx)
		wg.GoNamed(
			fmt.Sprintf(
				"%s-%s",
				o.name,
				o.memberID,
			),
			func() error {
				return o.handler.Start(innerCtx, itemChan)
			},
		)

		// 5. 主循环
		pollTicker := time.NewTicker(o.config.PollInterval)
		defer pollTicker.Stop()

		saveTicker := time.NewTicker(o.config.SaveInterval)
		defer saveTicker.Stop()

		currentWatermark, err := o.getWatermark(innerCtx)
		if err != nil {
			return errors.WithStack(err)
		}

		for {
			select {
			case <-innerCtx.Done():
				o.logger.Ctx(innerCtx).Info("stream shutting down")
				return nil

			case <-pollTicker.C:
				dispatchChan := make(chan Item, o.config.ChanSize)
				pollWg, pollCtx := errgroupx.WithContext(innerCtx)
				pollWg.GoNamed(
					fmt.Sprintf(`%s-%s-poll`, o.name, o.memberID),
					func() error {
						defer close(dispatchChan)
						return o.handler.Poll(
							pollCtx,
							currentWatermark,
							o.cluster.VNodeIndicesOf(o.memberID),
							dispatchChan,
						)
					},
				)
				pollWg.GoNamed(
					fmt.Sprintf(`%s-%s-dispatch`, o.name, o.memberID),
					func() error {
						var wm time.Time
						total := 0
						dispatched := 0
						for item := range dispatchChan {
							total++
							select {
							case itemChan <- item:
								wm = o.handler.Watermark(item)
								dispatched++
							default:
								// 分片通道满，跳过（下次 poll 重新拉取）
							}
						}
						if total > 0 {
							o.logger.Ctx(pollCtx).Debugf("polled %d items, dispatched %d, watermark=%s",
								total,
								dispatched,
								wm.Format(time.RFC3339),
							)
							currentWatermark = wm
						}
						return nil
					},
				)
				if err := pollWg.Wait(); err != nil {
					return errors.WithStack(err)
				}

			case <-saveTicker.C:
				if !currentWatermark.IsZero() {
					if err := o.saveWatermark(ctx, currentWatermark); err != nil {
						o.logger.Ctx(ctx).Errorf("save watermark error: %v", err)
					}
				}
			}
		}
	})
}

func (o *Orchestrator[Handler, Item]) Shutdown() {
	o.handler.Shutdown()
}
