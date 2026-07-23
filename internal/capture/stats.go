package capture

import (
	"fmt"
	"sync/atomic"
	"time"
)

// CaptureStats 抓包统计收集器，使用原子操作保证并发安全
// 用于在命令行调试模式下实时追踪抓包的关键指标：
// - 累计捕获的数据包数量
// - 累计捕获的字节数
// - 累计完成的数据段（segment）数量
// - 抓包启动时间，用于计算速率（pps、mbps）
type CaptureStats struct {
	packets   atomic.Int64
	bytes     atomic.Int64
	segments  atomic.Int64
	startTime time.Time
}

// NewCaptureStats 创建新的统计收集器实例，记录当前时间为启动时间
func NewCaptureStats() *CaptureStats {
	return &CaptureStats{
		startTime: time.Now(),
	}
}

// RecordSegment 记录一次 dumpOne 的结果，原子性地累加 segment 计数
// 该方法由 dump goroutine 在每次完成 segment 写入后调用
func (s *CaptureStats) RecordSegment() {
	s.segments.Add(1)
}

// RecordPacket 逐包记录捕获到的一个数据包，原子性地累加包计数和字节数
// 该方法由 dump goroutine 在抓包循环内部调用，保证定时打印能看到实时变化的数据
// bytes 为该数据包的原始字节数
func (s *CaptureStats) RecordPacket(bytes int) {
	s.packets.Add(1)
	s.bytes.Add(int64(bytes))
}

// Snapshot 获取当前统计数据的原子快照，返回一份不可变的副本
// 用于定时打印线程安全地读取统计数据，避免数据竞争
func (s *CaptureStats) Snapshot() CaptureStatsSnapshot {
	return CaptureStatsSnapshot{
		Packets:  s.packets.Load(),
		Bytes:    s.bytes.Load(),
		Segments: s.segments.Load(),
		Elapsed:  time.Since(s.startTime),
	}
}

// CaptureStatsSnapshot 统计数据的不可变快照
type CaptureStatsSnapshot struct {
	Packets  int64
	Bytes    int64
	Segments int64
	Elapsed  time.Duration
}

// String 格式化统计快照为人类可读的单行输出
// 输出格式：packets: N, bytes: X.XX MB, segments: N, pps: N.N, mbps: N.NN
func (s CaptureStatsSnapshot) String() string {
	elapsedSec := s.Elapsed.Seconds()
	pps := float64(0)
	mbps := float64(0)
	if elapsedSec > 0 {
		pps = float64(s.Packets) / elapsedSec
		// 8 bits per byte, convert to megabits per second
		mbps = float64(s.Bytes) * 8 / elapsedSec / 1_000_000
	}
	mb := float64(s.Bytes) / 1_000_000
	return fmt.Sprintf(
		"packets: %d, bytes: %.2f MB, segments: %d, pps: %.1f, mbps: %.2f",
		s.Packets, mb, s.Segments, pps, mbps,
	)
}
