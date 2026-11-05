package bench

import (
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"zhongkuisec.com/zhongkui/internal/capture/wire"
	reporter_v1 "zhongkuisec.com/zhongkui/internal/pkg/protobuf/reporter/v1"
)

// FlowLevel 流量档位，控制基准测试加载的 pcap 数据量。
type FlowLevel int

// 流量档位取值。
const (
	// LevelSmoke 冒烟档位，加载最小数据集。
	LevelSmoke FlowLevel = iota
	// LevelSmallFlows 小流量档位。
	LevelSmallFlows
	// LevelBigFlows 大流量档位。
	LevelBigFlows
)

func loadFlowPackets(tb testing.TB, level FlowLevel) []PcapPacket {
	switch level {
	case LevelSmoke:
		path := filepath.Join("testdata", "fallback.pcap")
		if _, err := os.Stat(path); os.IsNotExist(err) {
			packets, err := LoadPcapFromBytes(TestPcapData)
			if err != nil {
				tb.Fatalf("failed to load embedded test.pcap: %v", err)
			}
			return packets
		}
		packets, err := LoadPcap(path)
		if err != nil {
			tb.Fatalf("failed to load fallback.pcap: %v", err)
		}
		return packets
	case LevelSmallFlows:
		path := filepath.Join("testdata", "smallFlows.pcap")
		if _, err := os.Stat(path); os.IsNotExist(err) {
			path = filepath.Join("testdata", "fallback.pcap")
			if _, err := os.Stat(path); os.IsNotExist(err) {
				tb.Skipf("smallFlows.pcap not found, run 'make bench-prepare' to download")
			}
		}
		packets, err := LoadPcap(path)
		if err != nil {
			tb.Fatalf("failed to load pcap: %v", err)
		}
		return packets
	case LevelBigFlows:
		path := filepath.Join("testdata", "bigFlows.pcap")
		if _, err := os.Stat(path); os.IsNotExist(err) {
			tb.Skipf("bigFlows.pcap not found, run 'make bench-prepare' to download")
		}
		packets, err := LoadPcap(path)
		if err != nil {
			tb.Fatalf("failed to load pcap: %v", err)
		}
		return packets
	}
	return nil
}

func runBaseline(b *testing.B, wireName string, level FlowLevel, newWire func(*reporter_v1.Config) (wire.Wire, error)) {
	if testing.Short() && level != LevelSmoke {
		b.Skip("skipping non-smoke test in short mode")
	}

	packets := loadFlowPackets(b, level)

	dev := LoopbackName()
	if dev == "" {
		b.Skip("no loopback device found")
	}

	cfg := &reporter_v1.Config{
		Device:  dev,
		SnapLen: 65536,
		Promisc: true,
		Timeout: 1,
	}

	w, err := newWire(cfg)
	if err != nil {
		b.Skipf("failed to create %s wire: %v (try running with sudo)", wireName, err)
	}

	handle, err := OpenLoopback()
	if err != nil {
		b.Skipf("failed to open loopback for replay: %v (try running with sudo)", err)
	}
	defer handle.Close()

	stopCh := make(chan struct{})
	captureCh := make(chan wire.RawPacket, len(packets)*2)
	go func() {
		defer close(captureCh)
		for {
			select {
			case <-stopCh:
				return
			default:
			}
			pkt := w.ZeroCopyReadPacketData()
			if !pkt.HasNext() {
				return
			}
			if pkt.Err != nil {
				continue
			}
			cp := wire.RawPacket{
				Data:        make([]byte, len(pkt.Data)),
				CaptureInfo: pkt.CaptureInfo,
			}
			copy(cp.Data, pkt.Data)
			captureCh <- cp
		}
	}()

	for i := 0; i < 5; i++ {
		if err := handle.WritePacketData(packets[0].Data); err != nil {
			b.Skipf("warmup send failed: %v (try running with sudo)", err)
		}
		time.Sleep(5 * time.Millisecond)
	}

	var warmupCount int
	warmupDeadline := time.After(3 * time.Second)
	for warmupCount < 3 {
		select {
		case _, ok := <-captureCh:
			if !ok {
				b.Fatal("capture channel closed before receiving warmup packets")
			}
			warmupCount++
		case <-warmupDeadline:
			b.Skipf("capture did not receive warmup packets within timeout (%d/3), %s may not work on loopback (try running with sudo)", warmupCount, wireName)
		}
	}

	settleDeadline := time.After(200 * time.Millisecond)
drainSettle:
	for {
		select {
		case <-captureCh:
		case <-settleDeadline:
			break drainSettle
		}
	}

	// 回收之前 wire 创建可能产生的大内存分配（如 XDP umem 16MB），
	// 避免 GC 在计时期间触发影响结果
	runtime.GC()
	b.ResetTimer()

	result, err := ReplayToLoopback(handle, packets, true)
	if err != nil {
		b.Skipf("replay failed: %v (try running with sudo)", err)
	}

	replayDuration := result.EndTime.Sub(result.StartTime)
	drainDeadline := time.After(replayDuration*2 + 500*time.Millisecond)

	var capturedPackets []wire.RawPacket
drainLoop:
	for {
		select {
		case pkt, ok := <-captureCh:
			if !ok {
				break drainLoop
			}
			capturedPackets = append(capturedPackets, pkt)
		case <-drainDeadline:
			break drainLoop
		}
	}

	// 通过 stop channel 通知 goroutine 退出，避免 gopacket Close() munmap 竞态导致 SIGSEGV
	close(stopCh)
	for range captureCh {
	}
	w.Close()

	b.StopTimer()

	var totalBytes int64
	for _, pkt := range capturedPackets {
		totalBytes += int64(len(pkt.Data))
	}

	metrics := &CaptureMetrics{
		Name:          wireName,
		SentCount:     result.SentCount,
		CapturedCount: len(capturedPackets),
		TotalBytes:    totalBytes,
		Duration:      replayDuration,
	}
	metrics.Compute()
	metrics.ReportToBenchmark(b)
}

// BenchmarkLibpcapSmoke libpcap 常规基线冒烟。
func BenchmarkLibpcapSmoke(b *testing.B) {
	runBaseline(b, "libpcap-smoke", LevelSmoke, wire.NewLibpcapWire)
}

// BenchmarkLibpcapSmallFlows libpcap 常规基线小流量。
func BenchmarkLibpcapSmallFlows(b *testing.B) {
	runBaseline(b, "libpcap-smallflows", LevelSmallFlows, wire.NewLibpcapWire)
}

// BenchmarkLibpcapBigFlows libpcap 常规基线大流量。
func BenchmarkLibpcapBigFlows(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping bigflows in short mode")
	}
	runBaseline(b, "libpcap-bigflows", LevelBigFlows, wire.NewLibpcapWire)
}

func runSlowBaseline(b *testing.B, wireName string, level FlowLevel, newWire func(*reporter_v1.Config) (wire.Wire, error), interval time.Duration) {
	if testing.Short() && level != LevelSmoke {
		b.Skip("skipping non-smoke test in short mode")
	}

	packets := loadFlowPackets(b, level)

	dev := LoopbackName()
	if dev == "" {
		b.Skip("no loopback device found")
	}

	cfg := &reporter_v1.Config{
		Device:  dev,
		SnapLen: 65536,
		Promisc: true,
		Timeout: 1,
	}

	w, err := newWire(cfg)
	if err != nil {
		b.Skipf("failed to create %s wire: %v (try running with sudo)", wireName, err)
	}

	handle, err := OpenLoopback()
	if err != nil {
		b.Skipf("failed to open loopback for replay: %v (try running with sudo)", err)
	}
	defer handle.Close()

	stopCh := make(chan struct{})
	captureCh := make(chan wire.RawPacket, len(packets)*2)
	go func() {
		defer close(captureCh)
		for {
			select {
			case <-stopCh:
				return
			default:
			}
			pkt := w.ZeroCopyReadPacketData()
			if !pkt.HasNext() {
				return
			}
			if pkt.Err != nil {
				continue
			}
			cp := wire.RawPacket{
				Data:        make([]byte, len(pkt.Data)),
				CaptureInfo: pkt.CaptureInfo,
			}
			copy(cp.Data, pkt.Data)
			captureCh <- cp
		}
	}()

	for i := 0; i < 5; i++ {
		if err := handle.WritePacketData(packets[0].Data); err != nil {
			b.Skipf("warmup send failed: %v (try running with sudo)", err)
		}
		time.Sleep(5 * time.Millisecond)
	}

	var warmupCount int
	warmupDeadline := time.After(3 * time.Second)
	for warmupCount < 3 {
		select {
		case _, ok := <-captureCh:
			if !ok {
				b.Fatal("capture channel closed before receiving warmup packets")
			}
			warmupCount++
		case <-warmupDeadline:
			b.Skipf("capture did not receive warmup packets within timeout (%d/3), %s may not work on loopback (try running with sudo)", warmupCount, wireName)
		}
	}

	settleDeadline := time.After(200 * time.Millisecond)
drainSettle:
	for {
		select {
		case <-captureCh:
		case <-settleDeadline:
			break drainSettle
		}
	}

	runtime.GC()
	b.ResetTimer()

	result, err := SlowReplayToLoopback(handle, packets, interval)
	if err != nil {
		b.Skipf("slow replay failed: %v (try running with sudo)", err)
	}

	replayDuration := result.EndTime.Sub(result.StartTime)
	drainDeadline := time.After(replayDuration*2 + 500*time.Millisecond)

	var capturedPackets []wire.RawPacket
drainLoop:
	for {
		select {
		case pkt, ok := <-captureCh:
			if !ok {
				break drainLoop
			}
			capturedPackets = append(capturedPackets, pkt)
		case <-drainDeadline:
			break drainLoop
		}
	}

	// 通过 stop channel 通知 goroutine 退出，避免 gopacket Close() munmap 竞态导致 SIGSEGV
	close(stopCh)
	for range captureCh {
	}
	w.Close()

	b.StopTimer()

	var totalBytes int64
	for _, pkt := range capturedPackets {
		totalBytes += int64(len(pkt.Data))
	}

	metrics := &CaptureMetrics{
		Name:          wireName,
		SentCount:     result.SentCount,
		CapturedCount: len(capturedPackets),
		TotalBytes:    totalBytes,
		Duration:      replayDuration,
	}
	metrics.Compute()
	metrics.ReportToBenchmark(b)
}

// BenchmarkLibpcapSlowSmoke libpcap 慢速回放基线冒烟。
func BenchmarkLibpcapSlowSmoke(b *testing.B) {
	runSlowBaseline(b, "libpcap-slow-smoke", LevelSmoke, wire.NewLibpcapWire, 1*time.Millisecond)
}

// BenchmarkLibpcapSlowSmallFlows libpcap 慢速回放基线小流量。
func BenchmarkLibpcapSlowSmallFlows(b *testing.B) {
	runSlowBaseline(b, "libpcap-slow-smallflows", LevelSmallFlows, wire.NewLibpcapWire, 1*time.Millisecond)
}

// === 纯计数（skip copy）基线：测试 raw capture 吞吐上限 ===

// runRawBaseline 只计数不解包，消除 make+copy+channel 开销，测量裸 capture 能力
func runRawBaseline(b *testing.B, wireName string, level FlowLevel, newWire func(*reporter_v1.Config) (wire.Wire, error)) {
	if testing.Short() && level != LevelSmoke {
		b.Skip("skipping non-smoke test in short mode")
	}

	packets := loadFlowPackets(b, level)

	dev := LoopbackName()
	if dev == "" {
		b.Skip("no loopback device found")
	}

	cfg := &reporter_v1.Config{
		Device:  dev,
		SnapLen: 65536,
		Promisc: true,
		Timeout: 1,
	}

	w, err := newWire(cfg)
	if err != nil {
		b.Skipf("failed to create %s wire: %v (try running with sudo)", wireName, err)
	}

	handle, err := OpenLoopback()
	if err != nil {
		b.Skipf("failed to open loopback for replay: %v (try running with sudo)", err)
	}
	defer handle.Close()

	var captured atomic.Int64
	var capturedBytes atomic.Int64
	stopCh := make(chan struct{})
	go func() {
		for {
			select {
			case <-stopCh:
				return
			default:
			}
			pkt := w.ZeroCopyReadPacketData()
			if !pkt.HasNext() {
				return
			}
			if pkt.Err != nil {
				continue
			}
			captured.Add(1)
			capturedBytes.Add(int64(len(pkt.Data)))
		}
	}()

	// 预热
	for i := 0; i < 5; i++ {
		if err := handle.WritePacketData(packets[0].Data); err != nil {
			b.Skipf("warmup send failed: %v (try running with sudo)", err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	warmupDeadline := time.After(3 * time.Second)
	for captured.Load() < 3 {
		select {
		case <-warmupDeadline:
			b.Skipf("capture did not receive warmup packets within timeout (%d/3), %s may not work on loopback (try running with sudo)", captured.Load(), wireName)
		default:
		}
		time.Sleep(5 * time.Millisecond)
	}

	// 清零，开始计时
	captured.Store(0)
	runtime.GC()
	b.ResetTimer()

	result, err := ReplayToLoopback(handle, packets, true)
	if err != nil {
		b.Skipf("replay failed: %v (try running with sudo)", err)
	}
	replayDuration := result.EndTime.Sub(result.StartTime)
	time.Sleep(replayDuration + 100*time.Millisecond)

	b.StopTimer()
	close(stopCh)
	w.Close()

	metrics := &CaptureMetrics{
		Name:          wireName,
		SentCount:     result.SentCount,
		CapturedCount: int(captured.Load()),
		TotalBytes:    capturedBytes.Load(),
		Duration:      replayDuration,
	}
	metrics.Compute()
	metrics.ReportToBenchmark(b)
}

// BenchmarkRawLibpcapSmoke libpcap 纯计数（skip copy）基线冒烟。
func BenchmarkRawLibpcapSmoke(b *testing.B) {
	runRawBaseline(b, "raw-libpcap-smoke", LevelSmoke, wire.NewLibpcapWire)
}

// BenchmarkRawLibpcapSmallFlows libpcap 纯计数（skip copy）基线小流量。
func BenchmarkRawLibpcapSmallFlows(b *testing.B) {
	runRawBaseline(b, "raw-libpcap-smallflows", LevelSmallFlows, wire.NewLibpcapWire)
}

// BenchmarkRawLibpcapBigFlows libpcap 纯计数（skip copy）基线大流量。
func BenchmarkRawLibpcapBigFlows(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping bigflows in short mode")
	}
	runRawBaseline(b, "raw-libpcap-bigflows", LevelBigFlows, wire.NewLibpcapWire)
}
