package pcap

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"
)

// pcapStatsContextKey 用于在 context 中存储 PcapStats 实例的键类型
type pcapStatsContextKey struct{}

// PcapStats PCAP 导入任务统计收集器，使用原子操作保证并发安全
// 用于追踪每次 PCAP 导入任务的关键指标：
// - 累计提取的数据包数量和字节数
// - 累计加载的各类网络模型数量（Ethernet、IPv4、IPv6、TCP、UDP）
// - 任务启动时间，用于计算处理耗时和速率
type PcapStats struct {
	// packets 累计提取的数据包总数。
	packets atomic.Int64
	// bytes 累计提取的数据包总字节数。
	bytes atomic.Int64
	// ethCount 累计加载的 Ethernet 模型数量。
	ethCount atomic.Int64
	// ipv4Count 累计加载的 IPv4 模型数量。
	ipv4Count atomic.Int64
	// ipv6Count 累计加载的 IPv6 模型数量。
	ipv6Count atomic.Int64
	// ipv4TCPCount 累计加载的 IPv4 TCP 模型数量。
	ipv4TCPCount atomic.Int64
	// ipv6TCPCount 累计加载的 IPv6 TCP 模型数量。
	ipv6TCPCount atomic.Int64
	// ipv4UDPCount 累计加载的 IPv4 UDP 模型数量。
	ipv4UDPCount atomic.Int64
	// ipv6UDPCount 累计加载的 IPv6 UDP 模型数量。
	ipv6UDPCount atomic.Int64
	// fingerprintCount 累计成功的 TCP 被动指纹识别次数。
	fingerprintCount atomic.Int64
	// fingerprintErrCount 累计失败的 TCP 被动指纹识别次数。
	fingerprintErrCount atomic.Int64
	// startTime 统计收集器创建时刻，用于计算处理耗时与速率。
	startTime time.Time
}

// NewPcapStats 创建新的 PCAP 统计收集器实例，记录当前时间为启动时间
func NewPcapStats() *PcapStats {
	return &PcapStats{
		startTime: time.Now(),
	}
}

// WithPcapStats 将 PcapStats 实例注入到 context 中
func WithPcapStats(ctx context.Context, stats *PcapStats) context.Context {
	return context.WithValue(ctx, pcapStatsContextKey{}, stats)
}

// GetPcapStats 从 context 中获取 PcapStats 实例
func GetPcapStats(ctx context.Context) *PcapStats {
	if stats, ok := ctx.Value(pcapStatsContextKey{}).(*PcapStats); ok {
		return stats
	}
	return nil
}

// RecordPacket 记录提取到一个数据包，原子性地累加包计数和字节数
// packetBytes 为该数据包的原始字节数
func (s *PcapStats) RecordPacket(packetBytes int) {
	s.packets.Add(1)
	s.bytes.Add(int64(packetBytes))
}

// RecordEth 记录加载一个 Ethernet 模型
func (s *PcapStats) RecordEth() {
	s.ethCount.Add(1)
}

// RecordIPv4 记录加载一个 IPv4 模型
func (s *PcapStats) RecordIPv4() {
	s.ipv4Count.Add(1)
}

// RecordIPv6 记录加载一个 IPv6 模型
func (s *PcapStats) RecordIPv6() {
	s.ipv6Count.Add(1)
}

// RecordIPv4TCP 记录加载一个 IPv4 TCP 模型
func (s *PcapStats) RecordIPv4TCP() {
	s.ipv4TCPCount.Add(1)
}

// RecordIPv6TCP 记录加载一个 IPv6 TCP 模型
func (s *PcapStats) RecordIPv6TCP() {
	s.ipv6TCPCount.Add(1)
}

// RecordIPv4UDP 记录加载一个 IPv4 UDP 模型
func (s *PcapStats) RecordIPv4UDP() {
	s.ipv4UDPCount.Add(1)
}

// RecordIPv6UDP 记录加载一个 IPv6 UDP 模型
func (s *PcapStats) RecordIPv6UDP() {
	s.ipv6UDPCount.Add(1)
}

// RecordTCPFingerprint 记录一次成功的 TCP 被动指纹识别（SYN / SYN-ACK 包）
func (s *PcapStats) RecordTCPFingerprint() {
	s.fingerprintCount.Add(1)
}

// RecordTCPFingerprintError 记录一次 TCP 被动指纹识别失败（FFI 不可用或解析错误）
func (s *PcapStats) RecordTCPFingerprintError() {
	s.fingerprintErrCount.Add(1)
}

// Snapshot 获取当前统计数据的原子快照，返回一份不可变的副本
func (s *PcapStats) Snapshot() PcapStatsSnapshot {
	return PcapStatsSnapshot{
		Packets:        s.packets.Load(),
		Bytes:          s.bytes.Load(),
		Eth:            s.ethCount.Load(),
		IPv4:           s.ipv4Count.Load(),
		IPv6:           s.ipv6Count.Load(),
		IPv4TCP:        s.ipv4TCPCount.Load(),
		IPv6TCP:        s.ipv6TCPCount.Load(),
		IPv4UDP:        s.ipv4UDPCount.Load(),
		IPv6UDP:        s.ipv6UDPCount.Load(),
		TCPFingerprint: s.fingerprintCount.Load(),
		FingerprintErr: s.fingerprintErrCount.Load(),
		Elapsed:        time.Since(s.startTime),
	}
}

// PcapStatsSnapshot 统计数据的不可变快照
type PcapStatsSnapshot struct {
	// Packets 累计提取的数据包总数
	Packets int64
	// Bytes 累计提取的数据包总字节数
	Bytes int64
	// Eth 累计加载的 Ethernet 模型数量
	Eth int64
	// IPv4 累计加载的 IPv4 模型数量
	IPv4 int64
	// IPv6 累计加载的 IPv6 模型数量
	IPv6 int64
	// IPv4TCP 累计加载的 IPv4 TCP 模型数量
	IPv4TCP int64
	// IPv6TCP 累计加载的 IPv6 TCP 模型数量
	IPv6TCP int64
	// IPv4UDP 累计加载的 IPv4 UDP 模型数量
	IPv4UDP int64
	// IPv6UDP 累计加载的 IPv6 UDP 模型数量
	IPv6UDP int64
	// TCPFingerprint 累计成功的 TCP 被动指纹识别次数
	TCPFingerprint int64
	// FingerprintErr 累计失败的 TCP 被动指纹识别次数
	FingerprintErr int64
	// Elapsed 自统计收集器创建以来经过的耗时
	Elapsed time.Duration
}

// String 格式化统计快照为人类可读的多行输出
// 输出格式包含各类模型数量、总包数、字节数和耗时
func (s PcapStatsSnapshot) String() string {
	elapsedSec := s.Elapsed.Seconds()
	pps := float64(0)
	mbps := float64(0)
	if elapsedSec > 0 {
		pps = float64(s.Packets) / elapsedSec
		mbps = float64(s.Bytes) * 8 / elapsedSec / 1_000_000
	}
	mb := float64(s.Bytes) / 1_000_000
	return fmt.Sprintf(
		"\n[packets: %d, bytes: %.2f MB, elapsed: %v, pps: %.1f, mbps: %.2f]"+
			"\n[eth: %d, ipv4: %d, ipv6: %d, ipv4_tcp: %d, ipv6_tcp: %d, ipv4_udp: %d, ipv6_udp: %d]"+
			"\n[tcp_fingerprint: %d, tcp_fingerprint_err: %d]",
		s.Packets, mb, s.Elapsed.Round(time.Millisecond), pps, mbps,
		s.Eth, s.IPv4, s.IPv6, s.IPv4TCP, s.IPv6TCP, s.IPv4UDP, s.IPv6UDP,
		s.TCPFingerprint, s.FingerprintErr,
	)
}
