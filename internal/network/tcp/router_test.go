package tcp

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/gopacket/gopacket/layers"
	. "github.com/smartystreets/goconvey/convey"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	n "zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

// TestRouterJSONMarshalUnmarshal_Empty 测试空路由器的JSON序列化和反序列化。
// 验证：
//  1. 空路由器可以正确序列化为JSON
//  2. JSON可以正确反序列化为新的路由器实例
//  3. 序列化前后的JSON保持一致（round-trip）
func TestRouterJSONMarshalUnmarshal_Empty(t *testing.T) {
	Convey("Test Router JSON Marshal/Unmarshal (Empty)", t, func() {
		Convey("Empty router round-trip should produce identical JSON", func() {
			r := NewRouter[*mockTCPPacket](100, 50, time.Second)

			data, err := json.Marshal(r)
			So(err, ShouldBeNil)
			So(data, ShouldNotBeNil)

			var r2 Router[*mockTCPPacket]
			err = json.Unmarshal(data, &r2)
			So(err, ShouldBeNil)

			data2, err := json.Marshal(r2)
			So(err, ShouldBeNil)
			So(string(data), ShouldEqual, string(data2))
		})

		Convey("MarshalJSON should produce valid JSON structure", func() {
			r := NewRouter[*mockTCPPacket](100, 50, 5*time.Second)

			data, err := r.MarshalJSON()
			So(err, ShouldBeNil)

			var raw map[string]interface{}
			err = json.Unmarshal(data, &raw)
			So(err, ShouldBeNil)

			// 验证 RouterAux 的字段存在
			So(raw, ShouldContainKey, "metas")
			So(raw, ShouldContainKey, "nodes")
			So(raw, ShouldContainKey, "max_reassembler_size")
			So(raw, ShouldContainKey, "max_out_order_packet_size")
			So(raw, ShouldContainKey, "notice_timeout")

			So(raw["max_reassembler_size"], ShouldEqual, 100)
			So(raw["max_out_order_packet_size"], ShouldEqual, 50)

			// 空路由器的 metas 和 nodes 应为空数组
			metas := raw["metas"].([]interface{})
			nodes := raw["nodes"].([]interface{})
			So(len(metas), ShouldEqual, 0)
			So(len(nodes), ShouldEqual, 0)
		})

		Convey("UnmarshalJSON should restore empty router correctly", func() {
			r1 := NewRouter[*mockTCPPacket](100, 50, time.Second)
			data, _ := r1.MarshalJSON()

			var r2 Router[*mockTCPPacket]
			err := r2.UnmarshalJSON(data)
			So(err, ShouldBeNil)

			// 序列化后再比较，确保配置参数一致
			data2, err := r2.MarshalJSON()
			So(err, ShouldBeNil)
			So(string(data), ShouldEqual, string(data2))
		})
	})
}

// TestRouterJSONMarshalUnmarshal_WithFlows 测试包含TCP流的路由器JSON序列化和反序列化。
// 验证：
//  1. 包含活跃TCP流的路由器可以正确序列化
//  2. 反序列化后结构性信息保持一致（节点数量、ID等）
//  3. 配置参数在反序列化后保持不变
func TestRouterJSONMarshalUnmarshal_WithFlows(t *testing.T) {
	Convey("Test Router JSON Marshal/Unmarshal (With Flows)", t, func() {

		Convey("Router with single flow should marshal and unmarshal correctly", func() {
			logger := &logging.Logger{SugaredLogger: zap.NewNop().Sugar()}

			clientAddr := net.ParseIP("192.168.1.100")
			serverAddr := net.ParseIP("10.0.0.1")

			sessionID := serializer.HexUUID{0xDE, 0xAD, 0xBE, 0xEF, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xAA, 0xBB}
			meta := &n.BaseMeta{
				Namespace: "test-ns",
				Capture:   "test-cap",
				Version:   "1.0",
				SessionID: sessionID,
			}

			// 创建 SYN 包用来初始化流
			syn := newMockTCPPacketWithOptions(
				withSynFlag(true),
				withSrcAddr(clientAddr), withDstAddr(serverAddr),
				withSrcPort(layers.TCPPort(50000)), withDstPort(layers.TCPPort(80)),
				withSeqNum(1000),
			)
			syn.meta = meta
			syn.ackNum = 0

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			wg, ctx := errgroup.WithContext(ctx)

			r := NewRouter[*mockTCPPacket](100, 50, time.Second)
			dstChan := make(chan any, 10)

			// 添加一个 SYN 包来创建流
			err := r.addPacket(ctx, logger, syn, wg, dstChan)
			So(err, ShouldBeNil)

			// 序列化
			data, err := json.Marshal(r)
			So(err, ShouldBeNil)

			// 反序列化到新路由器
			var r2 Router[*mockTCPPacket]
			err = json.Unmarshal(data, &r2)
			So(err, ShouldBeNil)

			// 反序列化后的路由器也应该能再次序列化，JSON 结构应一致
			data2, err := json.Marshal(r2)
			So(err, ShouldBeNil)
			So(string(data2), ShouldEqual, string(data))

			// 验证结构完整性：节点数量、配置参数
			var store RouterAux[*mockTCPPacket]
			err = json.Unmarshal(data, &store)
			So(err, ShouldBeNil)
			So(len(store.Metas), ShouldBeGreaterThan, 0)
			So(len(store.Nodes), ShouldBeGreaterThan, 0)
			So(len(store.Nodes[0]), ShouldBeGreaterThan, 0)
			So(store.Nodes[0][0].ID, ShouldNotBeEmpty)
			So(store.MaxReassemblerSize, ShouldEqual, 100)
			So(store.MaxOutOrderPacketSize, ShouldEqual, 50)

			// 清理 goroutine
			cancel()
			wg.Wait()
			close(dstChan)
		})

		Convey("Router with multiple flows across different metas should be preserved", func() {
			logger := &logging.Logger{SugaredLogger: zap.NewNop().Sugar()}
			clientAddr := net.ParseIP("192.168.1.100")
			serverAddr := net.ParseIP("10.0.0.1")

			sessionID1 := serializer.HexUUID{0xDE, 0xAD, 0xBE, 0xEF, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xAA, 0xBB}
			sessionID2 := serializer.HexUUID{0xCA, 0xFE, 0xBA, 0xBE, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xAA, 0xBB}

			meta1 := &n.BaseMeta{
				Namespace: "ns-1",
				Capture:   "cap-1",
				Version:   "1.0",
				SessionID: sessionID1,
			}
			meta2 := &n.BaseMeta{
				Namespace: "ns-2",
				Capture:   "cap-2",
				Version:   "1.0",
				SessionID: sessionID2,
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			wg, ctx := errgroup.WithContext(ctx)

			r := NewRouter[*mockTCPPacket](100, 50, time.Second)
			dstChan := make(chan any, 10)

			// meta1 下创建两个流
			flowIDs := []string{
				"a1b2c3d4e5f678901234567890123450",
				"a1b2c3d4e5f678901234567890123451",
			}
			for i, flowID := range flowIDs {
				syn := newMockTCPPacketWithOptions(
					withSynFlag(true),
					withSrcAddr(clientAddr), withDstAddr(serverAddr),
					withSrcPort(layers.TCPPort(50000+uint16(i))), withDstPort(layers.TCPPort(80)),
					withSeqNum(uint32(1000+i*100)),
					withStreamID(flowID),
				)
				syn.meta = meta1
				syn.ackNum = 0

				err := r.addPacket(ctx, logger, syn, wg, dstChan)
				So(err, ShouldBeNil)
			}

			// meta2 下创建一个流
			syn3 := newMockTCPPacketWithOptions(
				withSynFlag(true),
				withSrcAddr(clientAddr), withDstAddr(serverAddr),
				withSrcPort(layers.TCPPort(60000)), withDstPort(layers.TCPPort(443)),
				withSeqNum(2000),
				withStreamID("b1c2d3e4f5a678901234567890123456"),
			)
			syn3.meta = meta2
			syn3.ackNum = 0

			err := r.addPacket(ctx, logger, syn3, wg, dstChan)
			So(err, ShouldBeNil)

			// 序列化
			data, err := json.Marshal(r)
			So(err, ShouldBeNil)

			// 验证序列化结构
			var store RouterAux[*mockTCPPacket]
			err = json.Unmarshal(data, &store)
			So(err, ShouldBeNil)
			So(len(store.Metas), ShouldEqual, 2)

			// meta1 下应有 2 个节点，meta2 下应有 1 个
			totalNodes := 0
			for _, nodes := range store.Nodes {
				totalNodes += len(nodes)
			}
			So(totalNodes, ShouldEqual, 3)

			// 反序列化并再次序列化，验证一致性
			var r2 Router[*mockTCPPacket]
			err = json.Unmarshal(data, &r2)
			So(err, ShouldBeNil)

			data2, err := json.Marshal(r2)
			So(err, ShouldBeNil)
			So(string(data), ShouldEqual, string(data2))

			// 清理
			cancel()
			wg.Wait()
			close(dstChan)
		})

		Convey("Out-of-order packets should survive Side round-trip", func() {
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
				Namespace: "test", Capture: "test", Version: "1.0",
				SessionID: serializer.HexUUID{0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF, 0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF},
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			clientSide, serverSide, err := newSides(ctx, syn, 50, time.Second)
			So(err, ShouldBeNil)

			// 模拟握手完成
			synAck := newMockTCPPacketWithOptions(
				withSynAckFlag(true), withAckFlag(true),
				withSrcAddr(serverAddr), withDstAddr(clientAddr),
				withSrcPort(layers.TCPPort(80)), withDstPort(layers.TCPPort(50000)),
				withSeqNum(5000), withPayload(nil),
			)
			synAck.ackNum = 1001
			serverSide.handshakeWithSecondSynAck(ctx, synAck)

			ack := newMockTCPPacketWithOptions(
				withAckFlag(true),
				withSrcAddr(clientAddr), withDstAddr(serverAddr),
				withSrcPort(layers.TCPPort(50000)), withDstPort(layers.TCPPort(80)),
				withSeqNum(1001), withPayload(nil),
			)
			ack.ackNum = 5001
			clientSide.handshakeWithThirdAck(ctx, ack, serverSide)

			// 注入乱序包：发送 seq=1003 的包（跳过 seq=1002）
			ooPkt := newMockTCPPacketWithOptions(
				withAckFlag(true),
				withSrcAddr(clientAddr), withDstAddr(serverAddr),
				withSrcPort(layers.TCPPort(50000)), withDstPort(layers.TCPPort(80)),
				withSeqNum(1003),
				withPayload([]byte("hello")),
			)
			ooPkt.ackNum = 5001
			err = clientSide.addPacket(ctx, ooPkt, serverSide)
			So(err, ShouldBeNil)
			So(len(clientSide.outOrderPackets), ShouldEqual, 1)

			// 序列化客户端 Side（含乱序包）
			data, err := json.Marshal(clientSide)
			So(err, ShouldBeNil)

			// 反序列化
			var restored Side[*mockTCPPacket]
			err = json.Unmarshal(data, &restored)
			So(err, ShouldBeNil)

			// 乱序包应被保留
			So(len(restored.outOrderPackets), ShouldEqual, 1)
			restoredPkt := restored.outOrderPackets[1003]
			So(restoredPkt.SeqNum(), ShouldEqual, 1003)
			So(string(restoredPkt.GetPayload()), ShouldEqual, "hello")

			// re-marshal 一致性
			data2, _ := json.Marshal(restored)
			So(string(data), ShouldEqual, string(data2))

			cancel()
		})
	})
}

// TestRouterJSON_NonSYNPacket 测试非SYN包不会创建新节点，
// 因此序列化时不会改变路由器状态。
func TestRouterJSON_NonSYNPacket(t *testing.T) {
	Convey("Test Router JSON with non-SYN packets", t, func() {
		Convey("Non-SYN packet should not create node, router stays empty", func() {
			logger := &logging.Logger{SugaredLogger: zap.NewNop().Sugar()}

			clientAddr := net.ParseIP("192.168.1.100")
			serverAddr := net.ParseIP("10.0.0.1")

			ackPkt := newMockTCPPacketWithOptions(
				withAckFlag(true),
				withSrcAddr(clientAddr), withDstAddr(serverAddr),
				withSrcPort(layers.TCPPort(50000)), withDstPort(layers.TCPPort(80)),
				withSeqNum(1001),
			)
			ackPkt.meta = &n.BaseMeta{
				Namespace: "test",
				Capture:   "test",
				Version:   "1.0",
				SessionID: serializer.HexUUID{0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF, 0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF},
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			wg, ctx := errgroup.WithContext(ctx)

			r := NewRouter[*mockTCPPacket](100, 50, time.Second)
			dstChan := make(chan any, 10)

			err := r.addPacket(ctx, logger, ackPkt, wg, dstChan)
			So(err, ShouldBeNil)

			data, err := json.Marshal(r)
			So(err, ShouldBeNil)

			// 由于没有 SYN，meta 的 group 被创建但内部无节点
			var store RouterAux[*mockTCPPacket]
			err = json.Unmarshal(data, &store)
			So(err, ShouldBeNil)
			So(len(store.Metas), ShouldEqual, 1)
			So(len(store.Nodes), ShouldEqual, 1)
			So(len(store.Nodes[0]), ShouldEqual, 0)

			cancel()
			wg.Wait()
			close(dstChan)
		})
	})
}

// TestRouterJSON_DefaultConfig 测试 Default 方法创建的路由器序列化正确性。
func TestRouterJSON_DefaultConfig(t *testing.T) {
	Convey("Test Router JSON with Default factory", t, func() {
		Convey("Default() should create independent router with same config", func() {
			r := NewRouter[*mockTCPPacket](200, 100, 10*time.Second)
			r2 := r.Default().(Router[*mockTCPPacket])

			data1, err := json.Marshal(r)
			So(err, ShouldBeNil)

			data2, err := json.Marshal(r2)
			So(err, ShouldBeNil)

			// Default() 创建的路由器应与原路由器配置一致
			var store1, store2 RouterAux[*mockTCPPacket]
			json.Unmarshal(data1, &store1)
			json.Unmarshal(data2, &store2)

			So(store1.MaxReassemblerSize, ShouldEqual, store2.MaxReassemblerSize)
			So(store1.MaxOutOrderPacketSize, ShouldEqual, store2.MaxOutOrderPacketSize)
			So(store1.NoticeTimeout, ShouldEqual, store2.NoticeTimeout)
		})
	})
}

// TestSideJSONRoundTrip 测试 Side[P] 的 JSON 序列化/反序列化正确性。
// 验证 TCP 状态字段在 round-trip 后保持不变。
func TestSideJSONRoundTrip(t *testing.T) {
	Convey("Test Side JSON round-trip", t, func() {
		Convey("Side state should survive JSON marshal/unmarshal", func() {
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

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			clientSide, serverSide, err := newSides(ctx, syn, 50, time.Second)
			So(err, ShouldBeNil)
			So(clientSide, ShouldNotBeNil)
			So(serverSide, ShouldNotBeNil)

			// 验证客户端初始状态
			So(clientSide.cli2Srv, ShouldBeTrue)
			So(clientSide.state, ShouldEqual, StateSynSent)
			So(clientSide.isn, ShouldEqual, 1000)
			So(clientSide.expectedSeq, ShouldBeGreaterThan, 1000)
			So(clientSide.maxOutOrderPacketSize, ShouldEqual, 50)

			// 序列化客户端 Side
			data, err := json.Marshal(clientSide)
			So(err, ShouldBeNil)

			// 反序列化到新 Side
			var restored Side[*mockTCPPacket]
			err = json.Unmarshal(data, &restored)
			So(err, ShouldBeNil)

			// 验证状态字段被保留
			So(restored.cli2Srv, ShouldEqual, clientSide.cli2Srv)
			So(restored.state, ShouldEqual, clientSide.state)
			So(restored.isn, ShouldEqual, clientSide.isn)
			So(restored.expectedSeq, ShouldEqual, clientSide.expectedSeq)
			So(restored.maxOutOrderPacketSize, ShouldEqual, clientSide.maxOutOrderPacketSize)

			// 运行时字段（buffer、frameStack）不应被序列化
			So(restored.buffer, ShouldBeNil)
			So(restored.frameStack, ShouldBeNil)
			// outOrderPackets 应被序列化（空 map 时保持 nil）
			So(restored.outOrderPackets, ShouldBeNil)

			cancel()
		})

		Convey("Side with Established state should be preserved", func() {
			clientAddr := net.ParseIP("192.168.1.100")
			serverAddr := net.ParseIP("10.0.0.1")

			syn := newMockTCPPacketWithOptions(
				withSynFlag(true),
				withSrcAddr(clientAddr), withDstAddr(serverAddr),
				withSrcPort(layers.TCPPort(50000)), withDstPort(layers.TCPPort(80)),
				withSeqNum(2000),
				withPayload(nil),
			)
			syn.meta = &n.BaseMeta{
				Namespace: "test",
				Capture:   "test",
				Version:   "1.0",
				SessionID: serializer.HexUUID{0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF, 0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF},
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			clientSide, serverSide, err := newSides(ctx, syn, 50, time.Second)
			So(err, ShouldBeNil)

			// 模拟完成三次握手：SYN-ACK + ACK
			// SYN seq=2000 → client expectedSeq=2001
			synAck := newMockTCPPacketWithOptions(
				withSynAckFlag(true), withAckFlag(true),
				withSrcAddr(serverAddr), withDstAddr(clientAddr),
				withSrcPort(layers.TCPPort(80)), withDstPort(layers.TCPPort(50000)),
				withSeqNum(5000),
				withPayload(nil),
			)
			synAck.ackNum = 2001
			err = serverSide.handshakeWithSecondSynAck(ctx, synAck)
			So(err, ShouldBeNil)

			ack := newMockTCPPacketWithOptions(
				withAckFlag(true),
				withSrcAddr(clientAddr), withDstAddr(serverAddr),
				withSrcPort(layers.TCPPort(50000)), withDstPort(layers.TCPPort(80)),
				withSeqNum(2001),
				withPayload(nil),
			)
			ack.ackNum = 5001
			err = clientSide.handshakeWithThirdAck(ctx, ack, serverSide)
			So(err, ShouldBeNil)

			// 双方都应该在 Established 状态
			So(clientSide.state, ShouldEqual, StateEstablished)
			So(serverSide.state, ShouldEqual, StateEstablished)

			// 序列化和反序列化
			data, _ := json.Marshal(serverSide)
			var restored Side[*mockTCPPacket]
			err = json.Unmarshal(data, &restored)
			So(err, ShouldBeNil)

			So(restored.state, ShouldEqual, StateEstablished)
			So(restored.cli2Srv, ShouldEqual, serverSide.cli2Srv)
			So(restored.isn, ShouldEqual, 5000)

			cancel()
		})
	})
}

// TestReassemblerJSONRoundTrip 测试 Reassembler[P] 的 JSON 序列化/反序列化。
// 验证嵌套的 Rectifier 和 Side 状态在 round-trip 后保持不变，
// 且 signalChan 在反序列化后被正确重建。
func TestReassemblerJSONRoundTrip(t *testing.T) {
	Convey("Test Reassembler JSON round-trip", t, func() {
		Convey("Reassembler with handshake state should survive round-trip", func() {
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

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			rsm, err := newReassembler(ctx, syn, 50, time.Second)
			So(err, ShouldBeNil)
			So(rsm, ShouldNotBeNil)

			// 验证原始状态
			So(rsm.ClientAddr.String(), ShouldEqual, "192.168.1.100")
			So(rsm.ServerAddr.String(), ShouldEqual, "10.0.0.1")
			So(rsm.Client.state, ShouldEqual, StateSynSent)
			So(rsm.Server.state, ShouldEqual, StateListen)
			So(rsm.signalChan, ShouldNotBeNil)

			// 序列化
			data, err := json.Marshal(rsm)
			So(err, ShouldBeNil)

			// 反序列化
			var restored Reassembler[*mockTCPPacket]
			err = json.Unmarshal(data, &restored)
			So(err, ShouldBeNil)

			// 验证 Rectifier 字段被保留
			So(restored.ClientAddr.String(), ShouldEqual, "192.168.1.100")
			So(restored.ClientPort, ShouldEqual, layers.TCPPort(50000))
			So(restored.ServerAddr.String(), ShouldEqual, "10.0.0.1")
			So(restored.ServerPort, ShouldEqual, layers.TCPPort(80))

			// 验证嵌套 Side 状态被保留
			So(restored.Client.cli2Srv, ShouldBeTrue)
			So(restored.Client.state, ShouldEqual, StateSynSent)
			So(restored.Client.isn, ShouldEqual, 1000)
			So(restored.Server.cli2Srv, ShouldBeFalse)
			So(restored.Server.state, ShouldEqual, StateListen)

			// 验证 signalChan 被重新创建（不为 nil）
			So(restored.signalChan, ShouldNotBeNil)
			So(cap(restored.signalChan), ShouldBeGreaterThan, 0)

			// 验证 stop() 不会 panic（close 非 nil channel）
			So(func() { restored.stop() }, ShouldNotPanic)

			cancel()
		})

		Convey("Reassembler should produce identical JSON on re-marshal", func() {
			clientAddr := net.ParseIP("192.168.1.100")
			serverAddr := net.ParseIP("10.0.0.1")

			syn := newMockTCPPacketWithOptions(
				withSynFlag(true),
				withSrcAddr(clientAddr), withDstAddr(serverAddr),
				withSrcPort(layers.TCPPort(50000)), withDstPort(layers.TCPPort(80)),
				withSeqNum(3000),
				withPayload(nil),
			)
			syn.meta = &n.BaseMeta{
				Namespace: "test",
				Capture:   "test",
				Version:   "1.0",
				SessionID: serializer.HexUUID{0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF, 0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF},
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			rsm, _ := newReassembler(ctx, syn, 50, time.Second)

			data1, _ := json.Marshal(rsm)

			var restored Reassembler[*mockTCPPacket]
			json.Unmarshal(data1, &restored)

			data2, err := json.Marshal(restored)
			So(err, ShouldBeNil)
			So(string(data1), ShouldEqual, string(data2))

			cancel()
		})
	})
}

// TestRouterJSON_NestedStatePreservation 测试完整 Router 序列化链中所有嵌套状态的保留。
// 这是最完整的集成测试：创建 Router → 添加流 → 序列化 → 反序列化 → 验证每层状态。
func TestRouterJSON_NestedStatePreservation(t *testing.T) {
	Convey("Test Router JSON preserves nested state", t, func() {
		Convey("Complete TCP handshake state should survive Router round-trip", func() {
			logger := &logging.Logger{SugaredLogger: zap.NewNop().Sugar()}

			clientAddr := net.ParseIP("192.168.1.100")
			serverAddr := net.ParseIP("10.0.0.1")

			meta := &n.BaseMeta{
				Namespace: "test-ns",
				Capture:   "test-cap",
				Version:   "1.0",
				SessionID: serializer.HexUUID{0xDE, 0xAD, 0xBE, 0xEF, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xAA, 0xBB},
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			wg, ctx := errgroup.WithContext(ctx)

			r := NewRouter[*mockTCPPacket](100, 50, time.Second)
			dstChan := make(chan any, 10)

			// SYN 包 → 创建流，客户端 SynSent，服务端 Listen
			syn := newMockTCPPacketWithOptions(
				withSynFlag(true),
				withSrcAddr(clientAddr), withDstAddr(serverAddr),
				withSrcPort(layers.TCPPort(50000)), withDstPort(layers.TCPPort(80)),
				withSeqNum(1000),
				withPayload(nil),
			)
			syn.meta = meta
			syn.ackNum = 0
			err := r.addPacket(ctx, logger, syn, wg, dstChan)
			So(err, ShouldBeNil)

			// SYN-ACK → 服务端 SynReceived
			synAck := newMockTCPPacketWithOptions(
				withSynAckFlag(true), withAckFlag(true),
				withSrcAddr(serverAddr), withDstAddr(clientAddr),
				withSrcPort(layers.TCPPort(80)), withDstPort(layers.TCPPort(50000)),
				withSeqNum(5000),
				withPayload(nil),
			)
			synAck.meta = meta
			synAck.ackNum = 1001
			err = r.addPacket(ctx, logger, synAck, wg, dstChan)
			So(err, ShouldBeNil)

			// ACK → 双方 Established
			ack := newMockTCPPacketWithOptions(
				withAckFlag(true),
				withSrcAddr(clientAddr), withDstAddr(serverAddr),
				withSrcPort(layers.TCPPort(50000)), withDstPort(layers.TCPPort(80)),
				withSeqNum(1001),
				withPayload(nil),
			)
			ack.meta = meta
			ack.ackNum = 5001
			err = r.addPacket(ctx, logger, ack, wg, dstChan)
			So(err, ShouldBeNil)

			// 序列化
			data, err := json.Marshal(r)
			So(err, ShouldBeNil)

			// 反序列化并验证
			var store RouterAux[*mockTCPPacket]
			err = json.Unmarshal(data, &store)
			So(err, ShouldBeNil)
			So(len(store.Nodes), ShouldBeGreaterThan, 0)
			So(len(store.Nodes[0]), ShouldBeGreaterThan, 0)

			node := store.Nodes[0][0]
			So(node.ID, ShouldNotBeEmpty)

			rsm := node.Reassembler
			So(rsm, ShouldNotBeNil)

			// 验证 Rectifier 层
			So(rsm.ClientAddr.String(), ShouldEqual, "192.168.1.100")
			So(rsm.ServerAddr.String(), ShouldEqual, "10.0.0.1")
			So(rsm.ClientPort, ShouldEqual, layers.TCPPort(50000))
			So(rsm.ServerPort, ShouldEqual, layers.TCPPort(80))

			// 验证客户端 Side 状态（已握手完成 → Established）
			So(rsm.Client.cli2Srv, ShouldBeTrue)
			So(rsm.Client.state, ShouldEqual, StateEstablished)
			So(rsm.Client.isn, ShouldEqual, 1000)
			So(rsm.Client.expectedSeq, ShouldEqual, 1001) // isn+1, ACK 无 payload 不推进

			// 验证服务端 Side 状态（已握手完成 → Established）
			So(rsm.Server.cli2Srv, ShouldBeFalse)
			So(rsm.Server.state, ShouldEqual, StateEstablished)
			So(rsm.Server.isn, ShouldEqual, 5000)
			So(rsm.Server.expectedSeq, ShouldEqual, 5001) // isn+1, SYN-ACK 无 payload 不推进

			// 验证 signalChan 被正确重建
			So(rsm.signalChan, ShouldNotBeNil)

			cancel()
			wg.Wait()
			close(dstChan)
		})
	})
}
