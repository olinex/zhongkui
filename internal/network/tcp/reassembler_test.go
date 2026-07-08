package tcp

import (
	"context"
	stderrors "errors"
	"net"
	"testing"
	"time"

	"github.com/gopacket/gopacket/layers"
	. "github.com/smartystreets/goconvey/convey"
	n "zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
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

func (m *mockDetector) Extract(ctx context.Context, clientRb, serverRb *serializer.RingBuffer) (any, any, error) {
	if m.extractErr != nil {
		return nil, nil, m.extractErr
	}
	// 正常提取：读取所有可用数据
	clientData := make([]byte, clientRb.Capacity())
	serverData := make([]byte, serverRb.Capacity())
	n1, err := clientRb.Read(clientData)
	if err != nil {
		return nil, nil, err
	}
	n2, err := serverRb.Read(serverData)
	if err != nil {
		return nil, nil, err
	}
	return clientData[:n1], serverData[:n2], nil
}

func (m *mockDetector) Build(ctx context.Context, meta *n.BaseMeta,
	clientAddr, serverAddr net.IP,
	clientPort, serverPort layers.TCPPort,
	clientFrameStack, serverFrameStack n.FrameStack,
	clientPayload, serverPayload any,
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
func newTestReassemblerWithHandshake(t *testing.T, timeout time.Duration, sessionID byte) *testReassemblerFixture {
	clientAddr := net.ParseIP("192.168.1.100")
	serverAddr := net.ParseIP("10.0.0.1")

	meta := &n.BaseMeta{
		Namespace: "test-ns",
		Capture:   "test-cap",
		Version:   "1.0",
		SessionID: serializer.HexUUID{0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF,
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

	rsm, err := newReassembler(ctx, syn, 10, timeout)
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

	err = rsm.Server.handshakeWithSecondSynAck(ctx, synAck)
	So(err, ShouldBeNil)

	ack := newMockTCPPacketWithOptions(
		withAckFlag(true),
		withSrcAddr(clientAddr), withDstAddr(serverAddr),
		withSrcPort(layers.TCPPort(50000)), withDstPort(layers.TCPPort(80)),
		withSeqNum(1001),
		withPayload(nil),
	)
	ack.ackNum = 5001

	err = rsm.Client.handshakeWithThirdAck(ctx, ack, rsm.Server)
	So(err, ShouldBeNil)

	// 向双方缓冲区写入测试数据，使 Detect 阶段能匹配
	_, err = rsm.Client.buffer.Write([]byte("mock request data for detect"))
	So(err, ShouldBeNil)
	_, err = rsm.Server.buffer.Write([]byte("mock response data for detect"))
	So(err, ShouldBeNil)

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
			protocolKey := AppProtocol("test-retryable-incomplete")
			detector := &mockDetector{
				matchReq:   true,
				matchRsp:   true,
				extractErr: ErrDataIncomplete,
			}
			RegisterAppProtocolDetector(protocolKey, detector)
			defer delete(detectors, protocolKey)

			fixture := newTestReassemblerWithHandshake(t, 100*time.Millisecond, 0x01)
			defer fixture.rsmCancel()
			defer fixture.rsm.release()

			fixture.rsm.lastNoticeTime = time.Now()

			dstChan := make(chan any, 10)

			parseCtx, parseCancel := context.WithCancel(context.Background())
			defer parseCancel()

			done := make(chan error, 1)
			go func() {
				done <- fixture.rsm.parse(parseCtx, fixture.meta, dstChan)
			}()

			// 等待 parse 协程进入 select 循环
			time.Sleep(20 * time.Millisecond)

			// 发送信号触发提取，Extract 返回 ErrDataIncomplete
			fixture.rsm.notice(parseCtx)
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

		Convey("parse should exit when flow is stuck (lastNoticeTime exceeded timeout)", func() {
			protocolKey := AppProtocol("test-stuck-flow")
			detector := &mockDetector{
				matchReq:   true,
				matchRsp:   true,
				extractErr: ErrDataIncomplete,
			}
			RegisterAppProtocolDetector(protocolKey, detector)
			defer delete(detectors, protocolKey)

			fixture := newTestReassemblerWithHandshake(t, 50*time.Millisecond, 0x02)
			defer fixture.rsmCancel()
			defer fixture.rsm.release()

			// 将 lastNoticeTime 设置为很久以前，模拟僵死流
			fixture.rsm.lastNoticeTime = time.Now().Add(-time.Hour)

			dstChan := make(chan any, 10)

			parseCtx, parseCancel := context.WithCancel(context.Background())
			defer parseCancel()

			done := make(chan error, 1)
			go func() {
				done <- fixture.rsm.parse(parseCtx, fixture.meta, dstChan)
			}()

			time.Sleep(20 * time.Millisecond)

			// 直接向 signalChan 发送信号（绕过 notice()，避免更新 lastNoticeTime）
			// parse 应检测到 lastNoticeTime 已过期并退出
			fixture.rsm.signalChan <- nil

			select {
			case parseErr := <-done:
				So(parseErr, ShouldBeNil) // 僵死流退出应返回 nil（正常释放）
			case <-time.After(time.Second):
				t.Fatal("parse did not exit when flow should be stuck")
			}

			parseCancel()
		})

		Convey("parse should return error on non-retryable error", func() {
			protocolKey := AppProtocol("test-fatal-error")
			detector := &mockDetector{
				matchReq:   true,
				matchRsp:   true,
				extractErr: fatalError,
			}
			RegisterAppProtocolDetector(protocolKey, detector)
			defer delete(detectors, protocolKey)

			fixture := newTestReassemblerWithHandshake(t, 100*time.Millisecond, 0x03)
			defer fixture.rsmCancel()
			defer fixture.rsm.release()

			fixture.rsm.lastNoticeTime = time.Now()

			dstChan := make(chan any, 10)

			parseCtx, parseCancel := context.WithCancel(context.Background())
			defer parseCancel()

			done := make(chan error, 1)
			go func() {
				done <- fixture.rsm.parse(parseCtx, fixture.meta, dstChan)
			}()

			time.Sleep(20 * time.Millisecond)

			fixture.rsm.notice(parseCtx)

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
			protocolKey := AppProtocol("test-no-match")
			detector := &mockDetector{
				matchReq: false, // 永远不匹配
				matchRsp: false,
			}
			RegisterAppProtocolDetector(protocolKey, detector)
			defer delete(detectors, protocolKey)

			fixture := newTestReassemblerWithHandshake(t, 100*time.Millisecond, 0x04)
			defer fixture.rsmCancel()
			defer fixture.rsm.release()

			fixture.rsm.lastNoticeTime = time.Now()

			dstChan := make(chan any, 10)

			parseCtx, parseCancel := context.WithCancel(context.Background())
			defer parseCancel()

			done := make(chan error, 1)
			go func() {
				done <- fixture.rsm.parse(parseCtx, fixture.meta, dstChan)
			}()

			time.Sleep(20 * time.Millisecond)

			// 发送信号，无协议匹配 → extractApplication 返回 nil, nil
			fixture.rsm.notice(parseCtx)
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
