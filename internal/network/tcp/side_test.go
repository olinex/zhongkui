package tcp

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/gopacket/gopacket/layers"
	. "github.com/smartystreets/goconvey/convey"
	n "zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

// ---------------------------------------------------------------
// findIndex 单元测试
// ---------------------------------------------------------------

// TestSideFindIndex 测试 Side.findIndex 的数据包归属语义
// findIndex 返回包含字节 offset 的数据包索引，即第一个右边界严格大于 offset 的包。
// 包 i 的载荷区间为 [PayloadOffsets[i-1], PayloadOffsets[i])，第 0 个包的左边界为 0；
// 空载荷包（重复边界值）对应空区间，不包含任何字节。
// 当 offset 超出流尾（最后一个边界值）时，返回 -1 表示未找到。
func TestSideFindIndex(t *testing.T) {
	Convey("Test findIndex packet-ownership semantics", t, func() {
		Convey("With offsets [3, 5, 7, 9]", func() {
			side := &Side[*mockTCPPacket]{
				PayloadOffsets: []uint64{3, 5, 7, 9},
			}

			Convey("offset=0 should fall in [0, 3), return 0", func() {
				So(side.findIndex(0), ShouldEqual, 0)
			})

			Convey("offset=2 should fall in [0, 3), return 0", func() {
				So(side.findIndex(2), ShouldEqual, 0)
			})

			Convey("offset=3 is the first byte of [3, 5), return 1", func() {
				So(side.findIndex(3), ShouldEqual, 1)
			})

			Convey("offset=4 should fall in [3, 5), return 1", func() {
				So(side.findIndex(4), ShouldEqual, 1)
			})

			Convey("offset=5 is the first byte of [5, 7), return 2", func() {
				So(side.findIndex(5), ShouldEqual, 2)
			})

			Convey("offset=7 is the first byte of [7, 9), return 3", func() {
				So(side.findIndex(7), ShouldEqual, 3)
			})

			Convey("offset=8 should fall in [7, 9), return 3", func() {
				So(side.findIndex(8), ShouldEqual, 3)
			})

			Convey("offset=9 at stream tail should return the last packet 3", func() {
				So(side.findIndex(9), ShouldEqual, 3)
			})

			Convey("offset=11 should be beyond rightmost, return -1", func() {
				So(side.findIndex(11), ShouldEqual, -1)
			})

			Convey("offset=100 should be far beyond rightmost, return -1", func() {
				So(side.findIndex(100), ShouldEqual, -1)
			})
		})

		Convey("With single element [5]", func() {
			side := &Side[*mockTCPPacket]{
				PayloadOffsets: []uint64{5},
			}

			Convey("offset=0 should return 0", func() {
				So(side.findIndex(0), ShouldEqual, 0)
			})

			Convey("offset=5 should return 0", func() {
				So(side.findIndex(5), ShouldEqual, 0)
			})

			Convey("offset=6 should return -1", func() {
				So(side.findIndex(6), ShouldEqual, -1)
			})
		})

		Convey("With empty offsets, should return -1", func() {
			side := &Side[*mockTCPPacket]{
				PayloadOffsets: []uint64{},
			}

			So(side.findIndex(0), ShouldEqual, -1)
			So(side.findIndex(100), ShouldEqual, -1)
		})

		Convey("With consecutive monotonic offsets [10, 20, 30, 40]", func() {
			side := &Side[*mockTCPPacket]{
				PayloadOffsets: []uint64{10, 20, 30, 40},
			}

			Convey("offset=0 returns 0", func() {
				So(side.findIndex(0), ShouldEqual, 0)
			})

			Convey("offset=15 returns 1", func() {
				So(side.findIndex(15), ShouldEqual, 1)
			})

			Convey("offset=30 is the first byte of [30, 40), return 3", func() {
				So(side.findIndex(30), ShouldEqual, 3)
			})

			Convey("offset=40 returns 3", func() {
				So(side.findIndex(40), ShouldEqual, 3)
			})

			Convey("offset=41 returns -1", func() {
				So(side.findIndex(41), ShouldEqual, -1)
			})
		})

		Convey("With large offsets to test binary search correctness", func() {
			offsets := make([]uint64, 1000)
			for i := range offsets {
				offsets[i] = uint64((i + 1) * 100) // 100, 200, 300, ...
			}
			side := &Side[*mockTCPPacket]{
				PayloadOffsets: offsets,
			}

			Convey("offset=50 should return 0", func() {
				So(side.findIndex(50), ShouldEqual, 0)
			})

			Convey("offset=50000 should be the first byte of the packet ending at 50100, return 500", func() {
				So(side.findIndex(50000), ShouldEqual, 500)
			})

			Convey("offset=100001 should be beyond, return -1", func() {
				So(side.findIndex(100001), ShouldEqual, -1)
			})
		})

		Convey("With duplicate offsets (handshake empty packets) [0, 0, 5, 11, 13]", func() {
			side := &Side[*mockTCPPacket]{
				PayloadOffsets: []uint64{0, 0, 5, 11, 13},
			}

			Convey("offset=0 is the first byte of the first data packet [0, 5), return 2", func() {
				So(side.findIndex(0), ShouldEqual, 2)
			})

			Convey("offset=3 should fall in [0, 5), return 2", func() {
				So(side.findIndex(3), ShouldEqual, 2)
			})

			Convey("offset=5 is the first byte of [5, 11), return 3", func() {
				So(side.findIndex(5), ShouldEqual, 3)
			})

			Convey("offset=8 should fall in [5, 11), return 3", func() {
				So(side.findIndex(8), ShouldEqual, 3)
			})

			Convey("offset=13 at stream tail should return the last packet 4", func() {
				So(side.findIndex(13), ShouldEqual, 4)
			})

			Convey("offset=14 should be beyond, return -1", func() {
				So(side.findIndex(14), ShouldEqual, -1)
			})
		})

		Convey("With mid-stream empty payload packet [0, 0, 5, 5, 11]", func() {
			side := &Side[*mockTCPPacket]{
				PayloadOffsets: []uint64{0, 0, 5, 5, 11},
			}

			Convey("offset=4 should fall in [0, 5), return 2", func() {
				So(side.findIndex(4), ShouldEqual, 2)
			})

			Convey("offset=5 should skip the empty packet and fall in [5, 11), return 4", func() {
				So(side.findIndex(5), ShouldEqual, 4)
			})
		})
	})
}

// ---------------------------------------------------------------
// UpdateComsumed 单元测试
// ---------------------------------------------------------------

// makeSideWithPayload 创建一条已完成三次握手的 Side，并通过 appendPacket 追加若干带 payload 的包。
// 返回 Side、目标管道 dstChan、ctx cancel 和缓冲数据的总字节数 totalWritten。
// 帧记录（NetworkStack）在 appendPacket 时即时写入 dstChan。
func makeSideWithPayload(sessionID byte, payloads ...[]byte) (*Side[*mockTCPPacket], chan any, context.CancelFunc, int) {
	fixture := newTestReassemblerWithHandshakeWithoutBuffer(nil, sessionID)
	client := fixture.rsm.client
	cancel := fixture.rsmCancel

	totalWritten := 0
	for _, payload := range payloads {
		pkt := newMockTCPPacketWithOptions(
			withPayload(payload),
			withSeqNum(client.expectedSeq),
		)
		if err := client.appendPacket(fixture.rsmCtx, pkt); err != nil {
			panic("appendPacket failed: " + err.Error())
		}
		totalWritten += len(payload)
	}
	// 关闭 server 侧的 writer，避免 ringbuffer goroutine 泄漏；client 侧保持可用
	fixture.rsm.server.bufferClose()
	return client, fixture.dstChan, cancel, totalWritten
}

// collectStacks 从目标管道读取 count 条 NetworkStack 帧记录，非帧记录类型直接忽略。
func collectStacks(t *testing.T, dstChan <-chan any, count int) n.NetworkFrame {
	t.Helper()
	frames := make(n.NetworkFrame, 0, count)
	for len(frames) < count {
		select {
		case item, ok := <-dstChan:
			if !ok {
				t.Fatalf("dstChan closed before collecting %d frames, got %d", count, len(frames))
			}
			if stack, isStack := item.(*n.NetworkStack); isStack {
				frames = append(frames, stack)
			}
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for %d frames, got %d", count, len(frames))
		}
	}
	return frames
}

// drainReader 从 bufio.Reader 中读取指定字节数，返回实际读取的字节数。
func drainReader(reader io.Reader, n int) int {
	buf := make([]byte, n)
	total := 0
	for total < n {
		read, err := reader.Read(buf[total:])
		if err != nil {
			break
		}
		total += read
	}
	return total
}

// TestSideUpdateComsumed 测试 UpdateComsumed 在 findIndex 配合下的各种边界情况
func TestSideUpdateComsumed(t *testing.T) {
	InitMetrics()

	Convey("Test UpdateComsumed", t, func() {

		Convey("No data written (only handshake), consumed should be 0", func() {
			fixture := newTestReassemblerWithHandshakeWithoutBuffer(t, 0x30)
			defer fixture.rsmCancel()
			defer fixture.rsm.Close()

			consumed, startOff, endOff, startTs, endTs := fixture.rsm.client.UpdateComsumed()
			So(consumed, ShouldEqual, uint64(0))
			So(startOff, ShouldEqual, uint64(0))
			So(endOff, ShouldEqual, uint64(0))
			So(startTs, ShouldEqual, endTs)
		})

		Convey("Data written but none read, consumed stays 0", func() {
			client, _, cancel, _ := makeSideWithPayload(0x31, []byte("hello world"))
			defer cancel()

			consumed, startOff, endOff, _, _ := client.UpdateComsumed()
			So(consumed, ShouldEqual, uint64(0))
			So(startOff, ShouldEqual, uint64(0))
			// 未读取，startOffset == endOffset，落入同一区间
			So(endOff, ShouldEqual, uint64(0))
		})

		Convey("Data partially read, consumed reflects read bytes", func() {
			client, _, cancel, _ := makeSideWithPayload(0x32, []byte("hello world"))
			defer cancel()

			read := drainReader(client.brw.Reader, 5)
			So(read, ShouldEqual, 5)

			consumed, startOff, endOff, startTs, endTs := client.UpdateComsumed()
			So(consumed, ShouldEqual, uint64(5))
			// 段边界即实际字节偏移：段为 [0, 5)
			So(startOff, ShouldEqual, uint64(0))
			So(endOff, ShouldEqual, uint64(5))
			// 段完全落在单个数据包内，起止时间戳取自同一包，二者相等
			So(startTs.Equal(endTs), ShouldBeTrue)
		})

		Convey("All data consumed, consumed equals total written", func() {
			client, _, cancel, totalWritten := makeSideWithPayload(0x33, []byte("hello"))
			defer cancel()

			// 读取全部数据
			drainReader(client.brw.Reader, totalWritten)

			consumed, _, endOff, _, _ := client.UpdateComsumed()
			So(consumed, ShouldEqual, uint64(totalWritten))
			// endOff 应指向最后写入数据的区间右边界
			So(endOff, ShouldEqual, uint64(totalWritten))
		})

		Convey("Multiple UpdateComsumed calls: 2nd call returns 0 when no new data read", func() {
			client, _, cancel, _ := makeSideWithPayload(0x34, []byte("hello world"))
			defer cancel()

			drainReader(client.brw.Reader, 5)

			consumed1, _, _, _, _ := client.UpdateComsumed()
			So(consumed1, ShouldEqual, uint64(5))

			// 第二次调用，未读取新数据
			consumed2, startOff2, endOff2, startTs2, endTs2 := client.UpdateComsumed()
			So(consumed2, ShouldEqual, uint64(0))
			So(startOff2, ShouldEqual, endOff2)
			So(startTs2, ShouldEqual, endTs2)
		})

		Convey("Multiple packets, consumed crosses interval boundaries", func() {
			// 写入 3 个包：5 + 6 + 2 = 13 字节
			client, _, cancel, totalWritten := makeSideWithPayload(0x35,
				[]byte("hello"),  // 5 bytes, Written → 5
				[]byte(" world"), // 6 bytes, Written → 11
				[]byte("!!"),     // 2 bytes, Written → 13
			)
			defer cancel()

			// PayloadOffsets 预期：[0, 0, 5, 11, 13]（含握手两个 0）
			So(len(client.PayloadOffsets), ShouldEqual, 5)
			So(client.PayloadOffsets[2], ShouldEqual, uint64(5))
			So(client.PayloadOffsets[3], ShouldEqual, uint64(11))
			So(client.PayloadOffsets[4], ShouldEqual, uint64(13))

			// 读取 8 字节，跨越"hello"（5字节）和" world"的前 3 字节
			drainReader(client.brw.Reader, 8)

			consumed, startOff, endOff, startTs, endTs := client.UpdateComsumed()
			So(consumed, ShouldEqual, uint64(8))
			// 段边界即实际字节偏移，与包边界无关：段为 [0, 8)
			So(startOff, ShouldEqual, uint64(0))
			So(endOff, ShouldEqual, uint64(8))
			// 段跨越两个数据包，起止时间戳取自首尾字节各自所在的包
			So(startTs.Before(endTs), ShouldBeTrue)

			// 再读剩余 5 字节
			drainReader(client.brw.Reader, totalWritten-8)

			consumed2, startOff2, endOff2, _, _ := client.UpdateComsumed()
			So(consumed2, ShouldEqual, uint64(5))
			So(startOff2, ShouldEqual, uint64(8))
			So(endOff2, ShouldEqual, uint64(totalWritten))
		})

		Convey("Segment ends exactly at a packet boundary", func() {
			client, _, cancel, _ := makeSideWithPayload(0x36,
				[]byte("aaaaa"), // 5 bytes
				[]byte("bbbbb"), // 5 bytes
			)
			defer cancel()

			// PayloadOffsets: [0, 0, 5, 10]
			// 读 5 字节，段 [0, 5) 恰好消费完第一个数据包，结束偏移等于包边界 5
			drainReader(client.brw.Reader, 5)

			consumed, startOff, endOff, _, _ := client.UpdateComsumed()
			So(consumed, ShouldEqual, uint64(5))
			So(startOff, ShouldEqual, uint64(0))
			So(endOff, ShouldEqual, uint64(5))
		})

		Convey("Large single packet: all consumed at once", func() {
			largePayload := bytes.Repeat([]byte("X"), 1024)
			client, _, cancel, totalWritten := makeSideWithPayload(0x37, largePayload)
			defer cancel()

			drainReader(client.brw.Reader, totalWritten)

			consumed, startOff, endOff, _, _ := client.UpdateComsumed()
			So(consumed, ShouldEqual, uint64(totalWritten))
			So(startOff, ShouldEqual, uint64(0))
			So(endOff, ShouldEqual, uint64(totalWritten))
		})

		Convey("Timestamp progression: spanning packets takes boundary packet timestamps", func() {
			client, _, cancel, _ := makeSideWithPayload(0x38,
				[]byte("first"),  // 5 bytes → [0, 5)
				[]byte("second"), // 6 bytes → [5, 11)
			)
			defer cancel()

			// 读 6 字节，段 [0, 6) 跨越两个包：首字节在"first"，末字节在"second"
			drainReader(client.brw.Reader, 6)
			_, _, _, startTs, endTs := client.UpdateComsumed()
			So(startTs.Before(endTs), ShouldBeTrue)

			// 读剩余 5 字节，段 [6, 11) 完全落在"second"内，起止时间戳相等
			drainReader(client.brw.Reader, 5)
			_, _, _, startTs2, endTs2 := client.UpdateComsumed()
			So(startTs2.Equal(endTs2), ShouldBeTrue)
		})

		Convey("Timestamp at packet boundary: boundary offset belongs to the next packet", func() {
			client, _, cancel, _ := makeSideWithPayload(0x3B,
				[]byte("aaaaa"), // 5 bytes → [0, 5)
				[]byte("bbbbb"), // 5 bytes → [5, 10)
			)
			defer cancel()

			// 段 [0, 5)：起止均落在"aaaaa"包内
			drainReader(client.brw.Reader, 5)
			_, _, _, startTs1, endTs1 := client.UpdateComsumed()
			So(startTs1.Equal(endTs1), ShouldBeTrue)

			// 段 [5, 10)：起始偏移 5 恰为包边界，应归属"bbbbb"包（到达时间晚于"aaaaa"），
			// 而非向上取整到前一个包"aaaaa"。
			drainReader(client.brw.Reader, 5)
			_, _, _, startTs2, endTs2 := client.UpdateComsumed()
			So(startTs2.Equal(endTs2), ShouldBeTrue)
			So(endTs1.Before(startTs2), ShouldBeTrue)
		})

	})
}

// ---------------------------------------------------------------
// 帧记录即时输出单元测试
// ---------------------------------------------------------------

// TestSideFramesWriteToDstChan 测试帧记录即时写入目标管道的语义
// 验证每个数据包展平后的NetworkStack记录在appendPacket时即写入dstChan（无累积存储），
// 且 Seq/TraceID/Cli2Srv 字段填充正确。
func TestSideFramesWriteToDstChan(t *testing.T) {
	InitMetrics()

	Convey("Test frames written to dstChan on appendPacket", t, func() {
		Convey("Should write records with seq increasing across packets", func() {
			client, dstChan, cancel, _ := makeSideWithPayload(0x39,
				[]byte("hello"),
				[]byte("world"),
			)
			defer cancel()

			// 握手期客户端已追加 2 个零载荷包（SYN、ACK）
			So(len(client.PayloadOffsets), ShouldEqual, 4)

			// 握手3条（client SYN、server SYNACK、client ACK）+ 2个数据包 = 5 条帧记录
			frames := collectStacks(t, dstChan, 5)
			So(frames[0].Seq, ShouldEqual, uint32(0)) // client SYN
			So(frames[1].Seq, ShouldEqual, uint32(0)) // server SYNACK
			So(frames[2].Seq, ShouldEqual, uint32(1)) // client ACK
			So(frames[3].Seq, ShouldEqual, uint32(2)) // hello
			So(frames[4].Seq, ShouldEqual, uint32(3)) // world
			// 方向与TraceID填充正确
			So(frames[0].Cli2Srv, ShouldBeTrue)  // client SYN
			So(frames[1].Cli2Srv, ShouldBeFalse) // server SYNACK
			So(frames[3].Cli2Srv, ShouldBeTrue)  // hello（client侧）
			So(frames[0].TraceID, ShouldResemble, client.traceID)
			So(frames[4].TraceID, ShouldResemble, client.traceID)
			So(frames[0].TraceID, ShouldNotResemble, serializer.HexUUID{})

			// 新追加的数据包产生新记录，可继续从管道接收
			pkt := newMockTCPPacketWithOptions(
				withPayload([]byte("third")),
				withSeqNum(client.expectedSeq),
			)
			So(client.appendPacket(context.Background(), pkt), ShouldBeNil)
			frames = collectStacks(t, dstChan, 1)
			So(frames[0].Seq, ShouldEqual, uint32(4))
		})

		Convey("Should assign same seq to all records of a multi-layer packet", func() {
			client, dstChan, cancel, _ := makeSideWithPayload(0x3A, []byte("first"))
			defer cancel()

			pkt := newMockTCPPacketWithOptions(
				withPayload([]byte("multi-layer")),
				withSeqNum(client.expectedSeq),
			)
			// 构造 Ethernet/IPv4/TCP 三层堆栈
			pkt.frameInfo.LayerStack = n.LayerStack{
				{Level: n.LayerLevelLink, Proto: layers.LayerTypeEthernet},
				{Level: n.LayerLevelNetwork, Proto: layers.LayerTypeIPv4},
				{Level: n.LayerLevelTransport, Proto: layers.LayerTypeTCP},
			}
			So(client.appendPacket(context.Background(), pkt), ShouldBeNil)

			// 握手3条 + first(1条) + multi-layer(3条) = 7 条记录
			frames := collectStacks(t, dstChan, 7)
			// multi-layer 包的最后3条记录共享同一 Seq 与 TopProto，包间 Seq 递增
			So(frames[4].Seq, ShouldEqual, frames[5].Seq)
			So(frames[5].Seq, ShouldEqual, frames[6].Seq)
			So(frames[3].Seq, ShouldNotEqual, frames[4].Seq)
			So(frames[4].TopProto, ShouldEqual, layers.LayerTypeTCP)
			So(frames[5].TopProto, ShouldEqual, layers.LayerTypeTCP)
			So(frames[6].TopProto, ShouldEqual, layers.LayerTypeTCP)
			So(frames[4].Proto, ShouldEqual, layers.LayerTypeEthernet)
			So(frames[5].Proto, ShouldEqual, layers.LayerTypeIPv4)
			So(frames[6].Proto, ShouldEqual, layers.LayerTypeTCP)
		})
	})
}
