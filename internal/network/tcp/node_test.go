package tcp

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/gopacket/gopacket/layers"
	. "github.com/smartystreets/goconvey/convey"
	"go.uber.org/zap"
	n "zhongkuisec.com/zhongkui/internal/models/network"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// newTestReassemblerNode 创建一个包含 SYN 包的 ReassemblerNode，用于测试。
func newTestReassemblerNode(ctx context.Context, t *testing.T, streamID string) *ReassemblerNode[*mockTCPPacket] {
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
	syn.streamID = streamID

	node, err := newReassemblerNode(ctx, testLogger(), streamID, syn, 10, time.Second*5, make(chan any, 16))
	So(err, ShouldBeNil)
	return node
}

// testMeta 返回测试用的 BaseMeta。
func testMeta() *n.BaseMeta {
	return &n.BaseMeta{
		Namespace: "test",
		Capture:   "test",
		Version:   "1.0",
		SessionID: serializer.HexUUID{0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF, 0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF},
	}
}

// testLogger 返回测试用的 Logger。
func testLogger() *logging.Logger {
	return &logging.Logger{SugaredLogger: zap.NewNop().Sugar()}
}

// TestReassemblerNode_Run 验证 ReassemblerNode 的启动与停止生命周期。
func TestReassemblerNode_Run(t *testing.T) {
	Convey("Test ReassemblerNode start lifecycle", t, func() {
		InitMetrics()

		Convey("start runs in goroutine and exits on stop", func() {
			ctx := context.Background()
			node := newTestReassemblerNode(ctx, t, "test-node-1")

			logger := testLogger()
			meta := testMeta()
			dstChan := make(chan any, 10)

			// start 是阻塞调用，需要在 goroutine 中运行
			go node.start(logger, meta, dstChan)

			// 等待 goroutine 进入 parse 循环
			time.Sleep(20 * time.Millisecond)
			So(node.running(), ShouldBeTrue)

			// 停止节点，parse 循环应退出
			node.stop(true)
			So(node.completed(), ShouldBeTrue)
		})

		Convey("start should not block caller when called in goroutine", func() {
			ctx := context.Background()
			node := newTestReassemblerNode(ctx, t, "test-node-2")

			logger := testLogger()
			meta := testMeta()
			dstChan := make(chan any, 10)

			// 在 goroutine 中调用 start，调用方应立即返回
			started := make(chan struct{})
			go func() {
				close(started)
				node.start(logger, meta, dstChan)
			}()

			select {
			case <-started:
				// goroutine 已启动，符合预期
			case <-time.After(100 * time.Millisecond):
				t.Fatal("start goroutine did not launch")
			}

			// 确认 start 仍在运行（阻塞在 parse 循环中）
			time.Sleep(20 * time.Millisecond)
			So(node.running(), ShouldBeTrue)

			node.stop(true)
		})

		Convey("start handles multiple nodes concurrently", func() {
			ctx := context.Background()

			logger := testLogger()
			meta := testMeta()
			dstChan := make(chan any, 30)

			nodes := make([]*ReassemblerNode[*mockTCPPacket], 0, 3)
			for i := 0; i < 3; i++ {
				node := newTestReassemblerNode(ctx, t, "node-"+string('a'+rune(i)))
				nodes = append(nodes, node)
				go node.start(logger, meta, dstChan)
			}

			// 所有 goroutine 应正常启动
			time.Sleep(20 * time.Millisecond)
			for _, node := range nodes {
				So(node.running(), ShouldBeTrue)
			}

			// 停止所有节点
			for _, node := range nodes {
				node.stop(true)
				So(node.completed(), ShouldBeTrue)
			}
		})
	})
}
