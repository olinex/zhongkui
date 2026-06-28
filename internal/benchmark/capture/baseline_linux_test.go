//go:build linux

package bench

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
	"zhongkui.com/zhongkui/internal/capture/wire"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	reporter_v1 "zhongkui.com/zhongkui/internal/pkg/protobuf/reporter/v1"
)

func newBenchLogger() *logging.Logger {
	return &logging.Logger{SugaredLogger: zap.NewNop().Sugar()}
}

// === Platform 基线 (loopback) ===

func BenchmarkPlatformSmoke(b *testing.B) {
	logger := newBenchLogger()
	runBaseline(b, "platform-smoke", LevelSmoke, func(cfg *reporter_v1.Config) (wire.Wire, error) {
		return wire.NewWire(context.Background(), cfg, logger)
	})
}

func BenchmarkPlatformSmallFlows(b *testing.B) {
	logger := newBenchLogger()
	runBaseline(b, "platform-smallflows", LevelSmallFlows, func(cfg *reporter_v1.Config) (wire.Wire, error) {
		return wire.NewWire(context.Background(), cfg, logger)
	})
}

func BenchmarkPlatformBigFlows(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping bigflows in short mode")
	}
	logger := newBenchLogger()
	runBaseline(b, "platform-bigflows", LevelBigFlows, func(cfg *reporter_v1.Config) (wire.Wire, error) {
		return wire.NewWire(context.Background(), cfg, logger)
	})
}

func BenchmarkPlatformSlowSmoke(b *testing.B) {
	logger := newBenchLogger()
	runSlowBaseline(b, "platform-slow-smoke", LevelSmoke, func(cfg *reporter_v1.Config) (wire.Wire, error) {
		return wire.NewWire(context.Background(), cfg, logger)
	}, 1*time.Millisecond)
}

func BenchmarkPlatformSlowSmallFlows(b *testing.B) {
	logger := newBenchLogger()
	runSlowBaseline(b, "platform-slow-smallflows", LevelSmallFlows, func(cfg *reporter_v1.Config) (wire.Wire, error) {
		return wire.NewWire(context.Background(), cfg, logger)
	}, 1*time.Millisecond)
}

// === XDP 基线 (veth pair) ===

// runVethBaseline 使用 veth pair 进行基线测试。
// XDP 需要网卡驱动支持 native 模式，loopback 不支持，因此使用 veth 虚拟网卡。
func runVethBaseline(b *testing.B, wireName string, level FlowLevel, newWire func(*reporter_v1.Config) (wire.Wire, error)) {
	if testing.Short() && level != LevelSmoke {
		b.Skip("skipping non-smoke test in short mode")
	}

	packets := loadFlowPackets(b, level)

	// 创建 veth pair
	capIface, sendIface, vethCleanup, err := SetupVethPair()
	if err != nil {
		b.Skipf("创建 veth pair 失败: %v (需要 sudo)", err)
	}
	defer vethCleanup()

	cfg := &reporter_v1.Config{
		Device:  capIface,
		SnapLen: 65536,
		Promisc: true,
		Timeout: 1,
	}

	w, err := newWire(cfg)
	if err != nil {
		b.Skipf("创建 %s wire 失败: %v", wireName, err)
	}

	handle, err := OpenInterface(sendIface)
	if err != nil {
		b.Skipf("打开发包端 %s 失败: %v (需要 sudo)", sendIface, err)
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

	// warmup: 发送 5 个包，等待 XDP 捕获至少 3 个
	for i := 0; i < 5; i++ {
		if err := handle.WritePacketData(packets[0].Data); err != nil {
			b.Skipf("warmup 发包失败: %v", err)
		}
		time.Sleep(5 * time.Millisecond)
	}

	var warmupCount int
	warmupDeadline := time.After(3 * time.Second)
	for warmupCount < 3 {
		select {
		case _, ok := <-captureCh:
			if !ok {
				b.Fatal("capture channel 在收到 warmup 包前关闭")
			}
			warmupCount++
		case <-warmupDeadline:
			b.Skipf("warmup 超时 (%d/3)，%s 可能未收到数据包", warmupCount, wireName)
		}
	}

	// 排空残留数据
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

	result, err := ReplayToLoopback(handle, packets, true)
	if err != nil {
		b.Skipf("replay 失败: %v", err)
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

func BenchmarkXdpSmoke(b *testing.B) {
	runVethBaseline(b, "xdp-smoke", LevelSmoke, wire.NewXdpWire)
}

func BenchmarkXdpSmallFlows(b *testing.B) {
	runVethBaseline(b, "xdp-smallflows", LevelSmallFlows, wire.NewXdpWire)
}

func BenchmarkXdpBigFlows(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping bigflows in short mode")
	}
	runVethBaseline(b, "xdp-bigflows", LevelBigFlows, wire.NewXdpWire)
}

// === XDP 慢速基线 (veth pair) ===

func runVethSlowBaseline(b *testing.B, wireName string, level FlowLevel, newWire func(*reporter_v1.Config) (wire.Wire, error), interval time.Duration) {
	if testing.Short() && level != LevelSmoke {
		b.Skip("skipping non-smoke test in short mode")
	}

	packets := loadFlowPackets(b, level)

	capIface, sendIface, vethCleanup, err := SetupVethPair()
	if err != nil {
		b.Skipf("创建 veth pair 失败: %v (需要 sudo)", err)
	}
	defer vethCleanup()

	cfg := &reporter_v1.Config{
		Device:  capIface,
		SnapLen: 65536,
		Promisc: true,
		Timeout: 1,
	}

	w, err := newWire(cfg)
	if err != nil {
		b.Skipf("创建 %s wire 失败: %v", wireName, err)
	}

	handle, err := OpenInterface(sendIface)
	if err != nil {
		b.Skipf("打开发包端 %s 失败: %v (需要 sudo)", sendIface, err)
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
			b.Skipf("warmup 发包失败: %v", err)
		}
		time.Sleep(5 * time.Millisecond)
	}

	var warmupCount int
	warmupDeadline := time.After(3 * time.Second)
	for warmupCount < 3 {
		select {
		case _, ok := <-captureCh:
			if !ok {
				b.Fatal("capture channel 在收到 warmup 包前关闭")
			}
			warmupCount++
		case <-warmupDeadline:
			b.Skipf("warmup 超时 (%d/3)，%s 可能未收到数据包", warmupCount, wireName)
		}
	}

	settleDeadline := time.After(200 * time.Millisecond)
drainSettleSlow:
	for {
		select {
		case <-captureCh:
		case <-settleDeadline:
			break drainSettleSlow
		}
	}

	runtime.GC()
	b.ResetTimer()

	result, err := SlowReplayToLoopback(handle, packets, interval)
	if err != nil {
		b.Skipf("slow replay 失败: %v", err)
	}

	replayDuration := result.EndTime.Sub(result.StartTime)
	drainDeadline := time.After(replayDuration*2 + 500*time.Millisecond)

	var capturedPackets []wire.RawPacket
drainLoopSlow:
	for {
		select {
		case pkt, ok := <-captureCh:
			if !ok {
				break drainLoopSlow
			}
			capturedPackets = append(capturedPackets, pkt)
		case <-drainDeadline:
			break drainLoopSlow
		}
	}

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

func BenchmarkXdpSlowSmoke(b *testing.B) {
	runVethSlowBaseline(b, "xdp-slow-smoke", LevelSmoke, wire.NewXdpWire, 1*time.Millisecond)
}

func BenchmarkXdpSlowSmallFlows(b *testing.B) {
	runVethSlowBaseline(b, "xdp-slow-smallflows", LevelSmallFlows, wire.NewXdpWire, 1*time.Millisecond)
}

// === Afpacket 独立基线 (loopback) ===

func BenchmarkAfpacketSmoke(b *testing.B) {
	runBaseline(b, "afpacket-smoke", LevelSmoke, wire.NewAfpacketWire)
}

func BenchmarkAfpacketSmallFlows(b *testing.B) {
	runBaseline(b, "afpacket-smallflows", LevelSmallFlows, wire.NewAfpacketWire)
}

func BenchmarkAfpacketBigFlows(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping bigflows in short mode")
	}
	runBaseline(b, "afpacket-bigflows", LevelBigFlows, wire.NewAfpacketWire)
}

func BenchmarkAfpacketSlowSmoke(b *testing.B) {
	runSlowBaseline(b, "afpacket-slow-smoke", LevelSmoke, wire.NewAfpacketWire, 1*time.Millisecond)
}

func BenchmarkAfpacketSlowSmallFlows(b *testing.B) {
	runSlowBaseline(b, "afpacket-slow-smallflows", LevelSmallFlows, wire.NewAfpacketWire, 1*time.Millisecond)
}

// === Linux 平台 raw capture（skip copy）基线 ===

func BenchmarkRawPlatformSmoke(b *testing.B) {
	logger := newBenchLogger()
	runRawBaseline(b, "raw-platform-smoke", LevelSmoke, func(cfg *reporter_v1.Config) (wire.Wire, error) {
		return wire.NewWire(context.Background(), cfg, logger)
	})
}

func BenchmarkRawPlatformSmallFlows(b *testing.B) {
	logger := newBenchLogger()
	runRawBaseline(b, "raw-platform-smallflows", LevelSmallFlows, func(cfg *reporter_v1.Config) (wire.Wire, error) {
		return wire.NewWire(context.Background(), cfg, logger)
	})
}

func BenchmarkRawPlatformBigFlows(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping bigflows in short mode")
	}
	logger := newBenchLogger()
	runRawBaseline(b, "raw-platform-bigflows", LevelBigFlows, func(cfg *reporter_v1.Config) (wire.Wire, error) {
		return wire.NewWire(context.Background(), cfg, logger)
	})
}

func BenchmarkRawAfpacketSmoke(b *testing.B) {
	runRawBaseline(b, "raw-afpacket-smoke", LevelSmoke, wire.NewAfpacketWire)
}

func BenchmarkRawAfpacketSmallFlows(b *testing.B) {
	runRawBaseline(b, "raw-afpacket-smallflows", LevelSmallFlows, wire.NewAfpacketWire)
}

func BenchmarkRawAfpacketBigFlows(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping bigflows in short mode")
	}
	runRawBaseline(b, "raw-afpacket-bigflows", LevelBigFlows, wire.NewAfpacketWire)
}

// === XDP raw capture（skip copy, veth pair）===

func runVethRawBaseline(b *testing.B, wireName string, level FlowLevel, newWire func(*reporter_v1.Config) (wire.Wire, error)) {
	if testing.Short() && level != LevelSmoke {
		b.Skip("skipping non-smoke test in short mode")
	}

	packets := loadFlowPackets(b, level)

	capIface, sendIface, vethCleanup, err := SetupVethPair()
	if err != nil {
		b.Skipf("创建 veth pair 失败: %v (需要 sudo)", err)
	}
	defer vethCleanup()

	cfg := &reporter_v1.Config{
		Device:  capIface,
		SnapLen: 65536,
		Promisc: true,
		Timeout: 1,
	}

	w, err := newWire(cfg)
	if err != nil {
		b.Skipf("创建 %s wire 失败: %v", wireName, err)
	}

	handle, err := OpenInterface(sendIface)
	if err != nil {
		b.Skipf("打开发包端 %s 失败: %v (需要 sudo)", sendIface, err)
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

	// warmup
	for i := 0; i < 5; i++ {
		if err := handle.WritePacketData(packets[0].Data); err != nil {
			b.Skipf("warmup 发包失败: %v", err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	warmupDeadline := time.After(3 * time.Second)
	for captured.Load() < 3 {
		select {
		case <-warmupDeadline:
			b.Skipf("warmup 超时 (%d/3)，%s 可能未收到数据包", captured.Load(), wireName)
		default:
		}
		time.Sleep(5 * time.Millisecond)
	}

	captured.Store(0)
	runtime.GC()
	b.ResetTimer()

	result, err := ReplayToLoopback(handle, packets, true)
	if err != nil {
		b.Skipf("replay 失败: %v", err)
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

func BenchmarkRawXdpSmoke(b *testing.B) {
	runVethRawBaseline(b, "raw-xdp-smoke", LevelSmoke, wire.NewXdpWire)
}

func BenchmarkRawXdpSmallFlows(b *testing.B) {
	runVethRawBaseline(b, "raw-xdp-smallflows", LevelSmallFlows, wire.NewXdpWire)
}

func BenchmarkRawXdpBigFlows(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping bigflows in short mode")
	}
	runVethRawBaseline(b, "raw-xdp-bigflows", LevelBigFlows, wire.NewXdpWire)
}
