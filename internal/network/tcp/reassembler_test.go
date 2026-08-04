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
}

// newTestReassemblerWithHandshake 创建一个完成三次握手的 Reassembler，
// 双方缓冲区中已写入测试数据，供 extractApplication 的 Detect 阶段使用。
// 返回 fixture 和 cancel，调用方应 defer cancel 并 defer rsm.release。
func newTestReassemblerWithHandshake(_ *testing.T, sessionID byte) *testReassemblerFixture {
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
	rsm, err := newReassembler(ctx, logger, syn, 10)
	So(err, ShouldBeNil)

	// 模拟三次握手完成
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

	// 向双方缓冲区写入测试数据，使 Detect 阶段能匹配
	_, err = rsm.client.brw.Write([]byte("mock request data for detect"))
	So(err, ShouldBeNil)
	So(rsm.client.brw.Flush(), ShouldBeNil)
	_, err = rsm.server.brw.Write([]byte("mock response data for detect"))
	So(err, ShouldBeNil)
	So(rsm.server.brw.Flush(), ShouldBeNil)

	return &testReassemblerFixture{
		rsm:       rsm,
		meta:      meta,
		rsmCtx:    ctx,
		rsmCancel: cancel,
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
			dstChan := make(chan any, 10)

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
			dstChan := make(chan any, 10)

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
			dstChan := make(chan any, 10)

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
