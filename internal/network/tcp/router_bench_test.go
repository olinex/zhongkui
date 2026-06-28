package tcp

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/gopacket/gopacket/layers"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	n "zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/pkg/test"
)

func withStreamID(id string) func(*mockTCPPacket) {
	return func(m *mockTCPPacket) {
		m.streamID = id
	}
}

func withAckNum(ack uint32) func(*mockTCPPacket) {
	return func(m *mockTCPPacket) {
		m.ackNum = ack
	}
}

func BenchmarkRouterSingleFlow(b *testing.B) {
	logger := &logging.Logger{SugaredLogger: zap.NewNop().Sugar()}

	clientAddr := net.ParseIP("192.168.1.100")
	serverAddr := net.ParseIP("10.0.0.1")
	clientPort := layers.TCPPort(50000)
	serverPort := layers.TCPPort(80)

	sessionID := serializer.HexUUID{0xDE, 0xAD, 0xBE, 0xEF, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xAA, 0xBB}
	meta := &n.BaseMeta{
		Namespace: "bench",
		Capture:   "bench",
		Version:   "1.0",
		SessionID: sessionID,
	}

	syn := newMockTCPPacketWithOptions(
		withSynFlag(true),
		withSrcAddr(clientAddr), withDstAddr(serverAddr),
		withSrcPort(clientPort), withDstPort(serverPort),
		withSeqNum(1000),
	)
	syn.meta = meta
	syn.ackNum = 0

	synAck := newMockTCPPacketWithOptions(
		withSynAckFlag(true), withAckFlag(true),
		withSrcAddr(serverAddr), withDstAddr(clientAddr),
		withSrcPort(serverPort), withDstPort(clientPort),
		withSeqNum(2000),
	)
	synAck.meta = meta
	synAck.ackNum = 1001

	ack := newMockTCPPacketWithOptions(
		withAckFlag(true),
		withSrcAddr(clientAddr), withDstAddr(serverAddr),
		withSrcPort(clientPort), withDstPort(serverPort),
		withSeqNum(1001),
	)
	ack.meta = meta
	ack.ackNum = 2001

	payloadData := []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")
	data1 := newMockTCPPacketWithOptions(
		withAckFlag(true),
		withSrcAddr(clientAddr), withDstAddr(serverAddr),
		withSrcPort(clientPort), withDstPort(serverPort),
		withSeqNum(1002),
		withPayload(payloadData),
	)
	data1.meta = meta
	data1.ackNum = 2001

	data2 := newMockTCPPacketWithOptions(
		withAckFlag(true),
		withSrcAddr(clientAddr), withDstAddr(serverAddr),
		withSrcPort(clientPort), withDstPort(serverPort),
		withSeqNum(1002+uint32(len(payloadData))),
		withPayload(payloadData),
	)
	data2.meta = meta
	data2.ackNum = 2001

	data3 := newMockTCPPacketWithOptions(
		withAckFlag(true),
		withSrcAddr(clientAddr), withDstAddr(serverAddr),
		withSrcPort(clientPort), withDstPort(serverPort),
		withSeqNum(1002+2*uint32(len(payloadData))),
		withPayload(payloadData),
	)
	data3.meta = meta
	data3.ackNum = 2001

	finSeq := 1002 + 3*uint32(len(payloadData))
	fin := newMockTCPPacketWithOptions(
		withFinFlag(true), withAckFlag(true),
		withSrcAddr(clientAddr), withDstAddr(serverAddr),
		withSrcPort(clientPort), withDstPort(serverPort),
		withSeqNum(finSeq),
	)
	fin.meta = meta
	fin.ackNum = 2001

	lastAck := newMockTCPPacketWithOptions(
		withAckFlag(true),
		withSrcAddr(serverAddr), withDstAddr(clientAddr),
		withSrcPort(serverPort), withDstPort(clientPort),
		withSeqNum(2001),
	)
	lastAck.meta = meta
	lastAck.ackNum = finSeq + 1

	packets := []*mockTCPPacket{syn, synAck, ack, data1, data2, data3, fin, lastAck}
	numPacketsPerIter := len(packets)

	b.ResetTimer()
	start := time.Now()

	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		wg, ctx := errgroup.WithContext(ctx)

		router := NewRouter[*mockTCPPacket](100, 100, time.Second)
		dstChan := make(chan any, 100)

		for _, pkt := range packets {
			router.addPacket(ctx, logger, pkt, wg, dstChan)
		}

		cancel()
		wg.Wait()
	}

	duration := time.Since(start)
	b.ReportMetric(float64(b.N*numPacketsPerIter)/duration.Seconds(), "pkts/sec")

	test.Tracer.Report()
}

func BenchmarkRouterConcurrentFlows(b *testing.B) {
	logger := &logging.Logger{SugaredLogger: zap.NewNop().Sugar()}

	clientAddr := net.ParseIP("192.168.1.100")
	serverAddr := net.ParseIP("10.0.0.1")
	clientPort := layers.TCPPort(50000)
	serverPort := layers.TCPPort(80)

	sessionID := serializer.HexUUID{0xDE, 0xAD, 0xBE, 0xEF, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xAA, 0xBB}
	meta := &n.BaseMeta{
		Namespace: "bench",
		Capture:   "bench",
		Version:   "1.0",
		SessionID: sessionID,
	}

	numFlows := 10
	packets := make([]*mockTCPPacket, 0, numFlows*5)
	payloadData := []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")

	for flowID := 0; flowID < numFlows; flowID++ {
		streamID := "a1b2c3d4e5f678901234567890123456"
		streamIDBytes := []byte(streamID)
		copy(streamIDBytes[24:], []byte{byte('0' + flowID/10), byte('0' + flowID%10)})
		flowStreamID := string(streamIDBytes)

		baseSeq := uint32(1000 + flowID*100)

		syn := newMockTCPPacketWithOptions(
			withSynFlag(true),
			withSrcAddr(clientAddr), withDstAddr(serverAddr),
			withSrcPort(clientPort), withDstPort(serverPort),
			withSeqNum(baseSeq),
			withStreamID(flowStreamID),
		)
		syn.meta = meta
		syn.ackNum = 0
		packets = append(packets, syn)

		synAck := newMockTCPPacketWithOptions(
			withSynAckFlag(true), withAckFlag(true),
			withSrcAddr(serverAddr), withDstAddr(clientAddr),
			withSrcPort(serverPort), withDstPort(clientPort),
			withSeqNum(baseSeq+1000),
			withStreamID(flowStreamID),
		)
		synAck.meta = meta
		synAck.ackNum = baseSeq + 1
		packets = append(packets, synAck)

		ack := newMockTCPPacketWithOptions(
			withAckFlag(true),
			withSrcAddr(clientAddr), withDstAddr(serverAddr),
			withSrcPort(clientPort), withDstPort(serverPort),
			withSeqNum(baseSeq+1),
			withStreamID(flowStreamID),
		)
		ack.meta = meta
		ack.ackNum = baseSeq + 1001
		packets = append(packets, ack)

		data := newMockTCPPacketWithOptions(
			withAckFlag(true),
			withSrcAddr(clientAddr), withDstAddr(serverAddr),
			withSrcPort(clientPort), withDstPort(serverPort),
			withSeqNum(baseSeq+2),
			withPayload(payloadData),
			withStreamID(flowStreamID),
		)
		data.meta = meta
		data.ackNum = baseSeq + 1001
		packets = append(packets, data)

		fin := newMockTCPPacketWithOptions(
			withFinFlag(true), withAckFlag(true),
			withSrcAddr(clientAddr), withDstAddr(serverAddr),
			withSrcPort(clientPort), withDstPort(serverPort),
			withSeqNum(baseSeq+3),
			withStreamID(flowStreamID),
		)
		fin.meta = meta
		fin.ackNum = baseSeq + 1001
		packets = append(packets, fin)
	}

	numPacketsPerIter := len(packets)

	b.ResetTimer()
	start := time.Now()

	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		wg, ctx := errgroup.WithContext(ctx)

		router := NewRouter[*mockTCPPacket](100, 100, time.Second)
		dstChan := make(chan any, 100)

		for _, pkt := range packets {
			router.addPacket(ctx, logger, pkt, wg, dstChan)
		}

		cancel()
		wg.Wait()
	}

	duration := time.Since(start)
	b.ReportMetric(float64(b.N*numPacketsPerIter)/duration.Seconds(), "pkts/sec")

	test.Tracer.Report()
}
