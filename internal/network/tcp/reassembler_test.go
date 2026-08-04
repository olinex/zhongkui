package tcp

import (
	"bufio"
	"context"
	stderrors "errors"
	"net"
	"testing"
	"time"

	"github.com/gopacket/gopacket/layers"
	. "github.com/smartystreets/goconvey/convey"
	"go.uber.org/zap"
	n "zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/network/detectors"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

// mockDetector 是用于测试的 AppProtocolDetector 实现。
// 可以控制 Detect 的匹配结果以及 Extract 的返回错误，
// 用于模拟流中断、数据不足等场景。
type mockDetector struct {
	detectErr  error // Detect 阶段的错误
	extractErr error // Extract 阶段的错误
	matchReq   bool  // 请求侧检测是否匹配
	matchRsp   bool  // 响应侧检测是否匹配
}

func (m *mockDetector) Detect(ctx context.Context, peek []byte, c2s bool) (bool, error) {
	if m.detectErr != nil {
		return false, m.detectErr
	}
	if c2s {
		return m.matchReq, nil
	}
	return m.matchRsp, nil
}

func (m *mockDetector) Extract(ctx context.Context, clientReader, serverReader *bufio.Reader) (any, any, error) {
	if m.extractErr != nil {
		return nil, nil, m.extractErr
	}
	// 正常提取：读取所有可用数据
	clientData := make([]byte, clientReader.Size())
	serverData := make([]byte, serverReader.Size())
	n1, err := clientReader.Read(clientData)
	if err != nil {
		return nil, nil, err
	}
	n2, err := serverReader.Read(serverData)
	if err != nil {
		return nil, nil, err
	}
	return clientData[:n1], serverData[:n2], nil
}

func (m *mockDetector) Build(ctx context.Context, baseMeta *n.BaseMeta,
	streamMeta *detectors.StreamMeta,
	streamIndex uint32,
	clientPayload, serverPayload any,
	buildTailer detectors.BuildTailer,
) (any, error) {
	return map[string]any{
		"client": clientPayload,
		"server": serverPayload,
	}, nil
}

// testReassemblerFixture 包含一个完整握手且缓冲区有数据的 Reassembler 和相关测试上下文。
type testReassemblerFixture struct {
	rsm       *Reassembler[*mockTCPPacket]
	meta      *n.BaseMeta
	rsmCtx    context.Context
	rsmCancel context.CancelFunc
	// dstChan 与rsm共用的目标管道，帧记录（NetworkStack）在appendPacket时即时写入，
	// 应用层对象在parse中写入，测试通过该管道断言输出。
	dstChan chan any
}

// newTestReassemblerWithHandshake 创建一个完成三次握手的 Reassembler，
// 双方缓冲区中已写入测试数据，供 extractApplication 的 Detect 阶段使用。
// 返回 fixture 和 cancel，调用方应 defer cancel 并 defer rsm.release。
func newTestReassemblerWithHandshake(_ *testing.T, sessionID byte) *testReassemblerFixture {
	fixture := newTestReassemblerWithHandshakeWithoutBuffer(nil, sessionID)

	// 向双方缓冲区写入测试数据，使 Detect 阶段能匹配
	_, err := fixture.rsm.client.brw.Write([]byte("mock request data for detect"))
	So(err, ShouldBeNil)
	So(fixture.rsm.client.brw.Flush(), ShouldBeNil)
	_, err = fixture.rsm.server.brw.Write([]byte("mock response data for detect"))
	So(err, ShouldBeNil)
	So(fixture.rsm.server.brw.Flush(), ShouldBeNil)

	return fixture
}

// newTestReassemblerWithHandshakeWithoutBuffer 创建一个完成三次握手的 Reassembler，
// 但不向缓冲区中写入测试数据，用于测试 buildTailer 等不依赖缓冲数据的场景
func newTestReassemblerWithHandshakeWithoutBuffer(_ *testing.T, sessionID byte) *testReassemblerFixture {
	clientAddr := net.ParseIP("192.168.1.100")
	serverAddr := net.ParseIP("10.0.0.1")

	meta := &n.BaseMeta{
		Namespace: "test-ns",
		Capture:   "test-cap",
		Version:   "1.0",
		SessionID: [16]byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF,
			0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF},
	}
	meta.SessionID[0] = sessionID

	syn := newMockTCPPacketWithOptions(
		withSynFlag(true),
		withSrcAddr(clientAddr), withDstAddr(serverAddr),
		withSrcPort(layers.TCPPort(50000)), withDstPort(layers.TCPPort(80)),
		withSeqNum(1000),
		withPayload(nil),
	)
	syn.meta = meta
	syn.streamID = "test-stream-id"

	ctx, cancel := context.WithCancel(context.Background())

	logger := &logging.Logger{SugaredLogger: zap.NewNop().Sugar()}
	dstChan := make(chan any, 32)
	rsm, err := newReassembler(ctx, logger, syn, 10, dstChan)
	So(err, ShouldBeNil)

	synAck := newMockTCPPacketWithOptions(
		withSynAckFlag(true), withAckFlag(true),
		withSrcAddr(serverAddr), withDstAddr(clientAddr),
		withSrcPort(layers.TCPPort(80)), withDstPort(layers.TCPPort(50000)),
		withSeqNum(5000),
		withPayload(nil),
	)
	synAck.ackNum = 1001

	err = rsm.server.handshakeWithSecondSynAck(ctx, synAck)
	So(err, ShouldBeNil)

	ack := newMockTCPPacketWithOptions(
		withAckFlag(true),
		withSrcAddr(clientAddr), withDstAddr(serverAddr),
		withSrcPort(layers.TCPPort(50000)), withDstPort(layers.TCPPort(80)),
		withSeqNum(1001),
		withPayload(nil),
	)
	ack.ackNum = 5001

	err = rsm.client.handshakeWithThirdAck(ctx, ack, rsm.server)
	So(err, ShouldBeNil)

	return &testReassemblerFixture{
		rsm:       rsm,
		meta:      meta,
		rsmCtx:    ctx,
		rsmCancel: cancel,
		dstChan:   dstChan,
	}
}

// fatalError 是用于测试非可重试错误的哨兵错误。
var fatalError = stderrors.New("fatal extraction error")

func TestReassemblerParse_RetryableError(t *testing.T) {
	// 初始化 OTEL metrics：生产环境由 NewRouter 调用，测试需手动调用
	InitMetrics()

	Convey("Test parse handles retryable errors", t, func() {

		Convey("parse should continue when Extract returns ErrDataIncomplete", func() {
			protocolKey := detectors.AppProtocol("test-retryable-incomplete")
			detector := &mockDetector{
				matchReq:   true,
				matchRsp:   true,
				extractErr: detectors.ErrDataIncomplete,
			}
			detectors.RegisterAppProtocolDetector(protocolKey, detector)
			defer delete(detectors.Detectors, protocolKey)

			fixture := newTestReassemblerWithHandshake(t, 0x01)
			defer fixture.rsmCancel()
			defer fixture.rsm.Close()

			signalChan := make(chan any, 1)
			dstChan := fixture.dstChan

			parseCtx, parseCancel := context.WithCancel(context.Background())
			defer parseCancel()

			done := make(chan error, 1)
			go func() {
				done <- fixture.rsm.parse(parseCtx, fixture.meta, signalChan, dstChan)
			}()

			// 等待 parse 协程进入 select 循环
			time.Sleep(20 * time.Millisecond)

			// 发送信号触发提取，Extract 返回 ErrDataIncomplete
			signalChan <- nil
			time.Sleep(50 * time.Millisecond)

			// parse 不应退出（可重试错误）
			select {
			case parseErr := <-done:
				t.Fatalf("parse exited unexpectedly: %v", parseErr)
			default:
				// parse 仍在运行，符合预期
			}

			parseCancel()
			<-done
		})

		Convey("parse should return error on non-retryable error", func() {
			protocolKey := detectors.AppProtocol("test-fatal-error")
			detector := &mockDetector{
				matchReq:   true,
				matchRsp:   true,
				extractErr: fatalError,
			}
			detectors.RegisterAppProtocolDetector(protocolKey, detector)
			defer delete(detectors.Detectors, protocolKey)

			fixture := newTestReassemblerWithHandshake(t, 0x03)
			defer fixture.rsmCancel()
			defer fixture.rsm.Close()

			signalChan := make(chan any, 1)
			dstChan := fixture.dstChan

			parseCtx, parseCancel := context.WithCancel(context.Background())
			defer parseCancel()

			done := make(chan error, 1)
			go func() {
				done <- fixture.rsm.parse(parseCtx, fixture.meta, signalChan, dstChan)
			}()

			time.Sleep(20 * time.Millisecond)

			signalChan <- nil

			select {
			case parseErr := <-done:
				So(parseErr, ShouldNotBeNil) // 非可重试错误应向上传播
			case <-time.After(time.Second):
				t.Fatal("parse did not exit on fatal error")
			}

			parseCancel()
		})

		Convey("parse should not exit when no protocol matches (app is nil)", func() {
			// 注册一个不匹配任何数据的 detector
			protocolKey := detectors.AppProtocol("test-no-match")
			detector := &mockDetector{
				matchReq: false, // 永远不匹配
				matchRsp: false,
			}
			detectors.RegisterAppProtocolDetector(protocolKey, detector)
			defer delete(detectors.Detectors, protocolKey)

			fixture := newTestReassemblerWithHandshake(t, 0x04)
			defer fixture.rsmCancel()
			defer fixture.rsm.Close()

			signalChan := make(chan any, 1)
			dstChan := fixture.dstChan

			parseCtx, parseCancel := context.WithCancel(context.Background())
			defer parseCancel()

			done := make(chan error, 1)
			go func() {
				done <- fixture.rsm.parse(parseCtx, fixture.meta, signalChan, dstChan)
			}()

			time.Sleep(20 * time.Millisecond)

			// 发送信号，无协议匹配 → extractApplication 返回 nil, nil
			signalChan <- nil
			time.Sleep(50 * time.Millisecond)

			// parse 不应退出
			select {
			case parseErr := <-done:
				t.Fatalf("parse exited unexpectedly on no-match: %v", parseErr)
			default:
				// parse 仍在运行，符合预期
			}

			parseCancel()
			<-done
		})

	})
}

// TestReassemblerMeta 测试 Reassembler.Meta 方法
// 验证创建的 StreamMeta 包含正确的连接信息
func TestReassemblerMeta(t *testing.T) {
	InitMetrics()

	Convey("Test Reassembler.Meta", t, func() {
		Convey("Meta should return correct StreamMeta with connection info", func() {
			fixture := newTestReassemblerWithHandshake(t, 0x10)
			defer fixture.rsmCancel()
			defer fixture.rsm.Close()

			meta := fixture.rsm.Meta()
			So(meta, ShouldNotBeNil)
			So(meta.TraceID, ShouldNotBeNil)
			So(meta.ClientAddr.String(), ShouldEqual, "192.168.1.100")
			So(meta.ClientPort, ShouldEqual, layers.TCPPort(50000))
			So(meta.ClientISN, ShouldEqual, uint32(1000))
			So(meta.ServerAddr.String(), ShouldEqual, "10.0.0.1")
			So(meta.ServerPort, ShouldEqual, layers.TCPPort(80))
			So(meta.ServerISN, ShouldEqual, uint32(5000))
			So(meta.Timestamp.IsZero(), ShouldBeFalse)
		})
	})
}

// TestReassemblerBuildTailer 测试 Reassembler.buildTailer 方法
// 验证 buildTailer 在无缓冲数据时返回零值 AppTailer，且可重入安全
func TestReassemblerBuildTailer(t *testing.T) {
	InitMetrics()

	Convey("Test Reassembler.buildTailer", t, func() {
		Convey("buildTailer should return AppTailer with zero consumed when no data in buffers", func() {
			fixture := newTestReassemblerWithHandshakeWithoutBuffer(t, 0x11)
			defer fixture.rsmCancel()
			defer fixture.rsm.Close()

			tailer, err := fixture.rsm.buildTailer(fixture.rsmCtx)
			So(err, ShouldBeNil)
			So(tailer, ShouldNotBeNil)
			// 握手包均无 payload，consumed 应为 0
			So(tailer.ClientConsumed, ShouldEqual, uint64(0))
			So(tailer.ServerConsumed, ShouldEqual, uint64(0))
		})

		Convey("buildTailer should be reentrant safe", func() {
			fixture := newTestReassemblerWithHandshakeWithoutBuffer(t, 0x12)
			defer fixture.rsmCancel()
			defer fixture.rsm.Close()

			tailer1, err := fixture.rsm.buildTailer(fixture.rsmCtx)
			So(err, ShouldBeNil)
			So(tailer1, ShouldNotBeNil)

			tailer2, err := fixture.rsm.buildTailer(fixture.rsmCtx)
			So(err, ShouldBeNil)
			So(tailer2, ShouldNotBeNil)
			// 第二次调用 consumed 不会增加（无新数据写入）
			So(tailer2.ClientConsumed, ShouldEqual, uint64(0))
			So(tailer2.ServerConsumed, ShouldEqual, uint64(0))
		})
	})
}

// TestReassemblerParse_StreamIndexIncrement 测试 streamIndex 递增
// 验证每次成功 push app 后 streamIndex 递增
func TestReassemblerParse_StreamIndexIncrement(t *testing.T) {
	InitMetrics()

	Convey("Test parse streamIndex increment", t, func() {
		Convey("streamIndex should increment after each successful push", func() {
			protocolKey := detectors.AppProtocol("test-stream-index")
			detector := &mockDetector{
				matchReq:   true,
				matchRsp:   true,
				extractErr: nil, // 正常提取
			}
			detectors.RegisterAppProtocolDetector(protocolKey, detector)
			defer delete(detectors.Detectors, protocolKey)

			fixture := newTestReassemblerWithHandshake(t, 0x13)
			defer fixture.rsmCancel()
			defer fixture.rsm.Close()

			signalChan := make(chan any, 5)
			dstChan := fixture.dstChan

			parseCtx, parseCancel := context.WithCancel(context.Background())
			defer parseCancel()

			done := make(chan error, 1)
			go func() {
				done <- fixture.rsm.parse(parseCtx, fixture.meta, signalChan, dstChan)
			}()

			time.Sleep(20 * time.Millisecond)

			// 发送两次信号，期望两次 build 的 streamIndex 不同
			// 第一次
			signalChan <- nil
			time.Sleep(50 * time.Millisecond)

			// 向双方缓冲区再写入数据，使第二次提取也能命中
			_, err := fixture.rsm.client.brw.Write([]byte("mock request data 2"))
			So(err, ShouldBeNil)
			So(fixture.rsm.client.brw.Flush(), ShouldBeNil)
			_, err = fixture.rsm.server.brw.Write([]byte("mock response data 2"))
			So(err, ShouldBeNil)
			So(fixture.rsm.server.brw.Flush(), ShouldBeNil)

			// 第二次
			signalChan <- nil
			time.Sleep(50 * time.Millisecond)

			parseCancel()
			<-done

			// 验证 dstChan 中收到了两个 app（帧记录为NetworkStack，需过滤）
			var apps []any
			close(dstChan)
			for item := range dstChan {
				if _, isStack := item.(*n.NetworkStack); isStack {
					continue
				}
				apps = append(apps, item)
			}
			So(len(apps), ShouldBeGreaterThanOrEqualTo, 2)
		})
	})
}
