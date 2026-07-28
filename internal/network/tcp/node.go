package tcp

import (
	"context"
	"fmt"

	n "zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/pkg/errgroupx"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

// ReassemblerNode 表示重组器节点，用于在双向链表中管理TCP流的重组器实例。
// 每个节点对应一个TCP连接流，包含重组器实例和LRU链表指针。
// 节点使用双向链表结构实现LRU（最近最少使用）缓存策略，提高流查找效率。
//
// 类型参数:
//   - P: 实现了TCPPacket接口的数据包类型
type ReassemblerNode[P TCPPacket] struct {
	ID          string          // TCP流的唯一标识符
	Reassembler *Reassembler[P] // TCP重组器实例
	prev        *ReassemblerNode[P]
	next        *ReassemblerNode[P]
}

// run 启动重组器节点的重组器实例。
// 该方法初始化重组器控制器，并在新的goroutine中启动重组器的运行。
// 如果节点已经启动，则返回错误。
//
// 参数:
//   - wg: errgroup.Group实例，用于管理goroutine的错误和等待
//   - dstChan: 目标通道，用于输出重组后的应用层数据
//   - packetChannelSize: 数据包通道的缓冲区大小
//
// 返回值:
//   - error: 如果节点已经启动，返回错误；否则返回nil
//
// 启动流程:
//  1. 检查控制器是否已初始化，避免重复启动
//  2. 创建新的重组器控制器
//  3. 在新的goroutine中启动重组器
//  4. 设置defer确保节点关闭时资源正确释放
func (n *ReassemblerNode[P]) run(
	ctx context.Context,
	logger *logging.Logger,
	meta *n.BaseMeta,
	wg *errgroupx.Group,
	dstChan chan<- any,
) {
	wg.GoNamed(
		fmt.Sprintf(`reassembler-node-%s`, n.ID),
		func() error {
			defer func() {
				if err := recover(); err != nil {
					logger.Ctx(ctx).Errorf(`reassembler node %s panic: %v`, n.ID, err)
				}
			}()
			err := n.Reassembler.parse(
				ctx,
				meta,
				dstChan,
			)
			if err != nil {
				logger.Ctx(ctx).Debugf(`reassembler node %s run error: %v`, n.ID, err)
			}
			return nil
		})
}
