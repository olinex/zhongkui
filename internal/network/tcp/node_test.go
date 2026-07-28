package tcp

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/gopacket/gopacket/layers"
	. "github.com/smartystreets/goconvey/convey"
	"go.uber.org/zap"
	n "zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/pkg/errgroupx"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

func newTestReassembler(ctx context.Context, t *testing.T) *Reassembler[*mockTCPPacket] {
	clientAddr := net.ParseIP("192.168.1.100")
	serverAddr := net.ParseIP("10.0.0.1")

	syn := newMockTCPPacketWithOptions(
		withSynFlag(true),
		withSrcAddr(clientAddr), withDstAddr(serverAddr),
		withSrcPort(layers.TCPPort(50000)), withDstPort(layers.TCPPort(80)),
		withSeqNum(1000),
		withPayload(nil),
	)
	syn.meta = &n.BaseMeta{
		Namespace: "test",
		Capture:   "test",
		Version:   "1.0",
		SessionID: serializer.HexUUID{0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF, 0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF},
	}

	rsm, err := newReassembler(ctx, syn, 10)
	So(err, ShouldBeNil)
	return rsm
}

func TestReassemblerNode_Run(t *testing.T) {
	Convey("Test ReassemblerNode run lifecycle", t, func() {
		InitMetrics()

		Convey("run starts goroutine that exits on context cancel", func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			wg, ctx := errgroupx.WithContext(ctx)

			rsm := newTestReassembler(ctx, t)
			defer rsm.release()

			logger := &logging.Logger{SugaredLogger: zap.NewNop().Sugar()}
			meta := &n.BaseMeta{
				Namespace: "test",
				Capture:   "test",
				Version:   "1.0",
				SessionID: serializer.HexUUID{0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF, 0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF},
			}

			node := &ReassemblerNode[*mockTCPPacket]{
				ID:          "test-node",
				Reassembler: rsm,
			}

			dstChan := make(chan any, 10)
			node.run(ctx, logger, meta, wg, dstChan)

			// 验证 goroutine 已注册到 errgroup
			// 等待一小段时间让 goroutine 进入 parse 循环
			time.Sleep(20 * time.Millisecond)

			// 取消上下文，parse 循环应退出
			cancel()

			// 等待 errgroup 完成
			err := wg.Wait()
			So(err, ShouldBeNil)
		})

		Convey("run should not block caller", func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			wg, ctx := errgroupx.WithContext(ctx)

			rsm := newTestReassembler(ctx, t)
			defer rsm.release()

			logger := &logging.Logger{SugaredLogger: zap.NewNop().Sugar()}
			meta := &n.BaseMeta{
				Namespace: "test",
				Capture:   "test",
				Version:   "1.0",
				SessionID: serializer.HexUUID{0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF, 0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF},
			}

			node := &ReassemblerNode[*mockTCPPacket]{
				ID:          "test-node-2",
				Reassembler: rsm,
			}

			dstChan := make(chan any, 10)

			// run 应立即返回（非阻塞）
			done := make(chan struct{})
			go func() {
				node.run(ctx, logger, meta, wg, dstChan)
				close(done)
			}()

			select {
			case <-done:
				// run 返回，符合预期
			case <-time.After(100 * time.Millisecond):
				t.Fatal("run blocked unexpectedly")
			}

			cancel()
			wg.Wait()
		})

		Convey("run handles multiple nodes concurrently", func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			wg, ctx := errgroupx.WithContext(ctx)

			logger := &logging.Logger{SugaredLogger: zap.NewNop().Sugar()}
			meta := &n.BaseMeta{
				Namespace: "test",
				Capture:   "test",
				Version:   "1.0",
				SessionID: serializer.HexUUID{0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF, 0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF},
			}

			dstChan := make(chan any, 30)
			var mu sync.Mutex
			rsms := make([]*Reassembler[*mockTCPPacket], 0, 3)

			for i := 0; i < 3; i++ {
				rsm := newTestReassembler(ctx, t)
				mu.Lock()
				rsms = append(rsms, rsm)
				mu.Unlock()
				node := &ReassemblerNode[*mockTCPPacket]{
					ID:          "node-" + string(rune('a'+i)),
					Reassembler: rsm,
				}
				node.run(ctx, logger, meta, wg, dstChan)
			}

			// 所有 goroutine 应正常启动
			time.Sleep(20 * time.Millisecond)

			cancel()
			err := wg.Wait()
			So(err, ShouldBeNil)

			for _, rsm := range rsms {
				rsm.release()
			}
		})
	})
}
