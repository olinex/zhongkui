//go:build linux

package bench

import (
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"zhongkuisec.com/zhongkui/internal/capture/wire"
	reporter_v1 "zhongkuisec.com/zhongkui/internal/pkg/protobuf/reporter/v1"
)

// 物理网卡 XDP 测试 (双网卡方案)
//
// 要求：两台物理有线网卡互联 (直连网线或同一交换机)
// - NIC1: XDP 抓包端 (capture)
// - NIC2: 原始帧发包端 (send)
//
// 最小硬件配置:
//   - 双口网卡机器，两口直连
//   - 或: 单口 + USB 以太网适配器
//   - 或: 两台机器，一台发包一台抓包 (需额外脚本配合)
//
// 注意: 不支持单网卡机器 — XDP 只能捕获物理网卡 INGRESS 方向的包，
// 单机无法向自己的物理网卡注入 INGRESS 流量。
//
// 如果找不到足够的 XDP 可用物理网卡，benchmark 会自动 skip。

// runXdpPhysNicBaseline 使用双物理网卡进行 XDP 基线测试。
// nicCap: XDP 抓包端网卡名
// nicSend: 发包端网卡名 (需要与 nicCap 互联)
func runXdpPhysNicBaseline(b *testing.B, wireName string, level FlowLevel, nicCap, nicSend string) {
	if testing.Short() && level != LevelSmoke {
		b.Skip("skipping non-smoke test in short mode")
	}

	packets := loadFlowPackets(b, level)

	cfg := &reporter_v1.Config{
		Device:  nicCap,
		SnapLen: 65536,
		Promisc: true,
		Timeout: 1,
	}

	w, err := wire.NewXdpWire(cfg)
	if err != nil {
		b.Skipf("XDP setup on %s failed: %v", nicCap, err)
	}
	b.Logf("XDP wire created on %s (check log for mode: Native/Generic)", nicCap)

	// 打开发包端
	sendHandle, err := OpenInterface(nicSend)
	if err != nil {
		b.Skipf("failed to open send NIC %s: %v", nicSend, err)
	}
	defer sendHandle.Close()

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

	// warmup + 连通性验证
	for i := 0; i < 5; i++ {
		if err := sendHandle.WritePacketData(packets[0].Data); err != nil {
			b.Skipf("warmup send on %s failed: %v (check that %s and %s are connected)", nicSend, err, nicCap, nicSend)
		}
		time.Sleep(5 * time.Millisecond)
	}

	var warmupCount int
	warmupDeadline := time.After(3 * time.Second)
	for warmupCount < 3 {
		select {
		case _, ok := <-captureCh:
			if !ok {
				b.Fatalf("capture channel closed before receiving warmup packets on %s", nicCap)
			}
			warmupCount++
		case <-warmupDeadline:
			b.Skipf("warmup timeout (%d/3) on %s: check that %s and %s are physically connected (crossover cable or same switch)", warmupCount, nicCap, nicCap, nicSend)
		}
	}

	// 排空 settle
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

	result, err := ReplayToLoopback(sendHandle, packets, true)
	if err != nil {
		b.Skipf("replay on %s failed: %v", nicSend, err)
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

// runXdpPhysNicRawBaseline 物理网卡 XDP raw capture（skip copy）基线
func runXdpPhysNicRawBaseline(b *testing.B, wireName string, level FlowLevel, nicCap, nicSend string) {
	if testing.Short() && level != LevelSmoke {
		b.Skip("skipping non-smoke test in short mode")
	}

	packets := loadFlowPackets(b, level)

	cfg := &reporter_v1.Config{
		Device:  nicCap,
		SnapLen: 65536,
		Promisc: true,
		Timeout: 1,
	}

	w, err := wire.NewXdpWire(cfg)
	if err != nil {
		b.Skipf("XDP setup on %s failed: %v", nicCap, err)
	}

	sendHandle, err := OpenInterface(nicSend)
	if err != nil {
		b.Skipf("failed to open send NIC %s: %v", nicSend, err)
	}
	defer sendHandle.Close()

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

	// warmup + 连通性验证
	for i := 0; i < 5; i++ {
		if err := sendHandle.WritePacketData(packets[0].Data); err != nil {
			b.Skipf("warmup send on %s failed: %v", nicSend, err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	warmupDeadline := time.After(3 * time.Second)
	for captured.Load() < 3 {
		select {
		case <-warmupDeadline:
			b.Skipf("warmup timeout (%d/3) on %s: check connectivity between %s and %s", captured.Load(), nicCap, nicCap, nicSend)
		default:
		}
		time.Sleep(5 * time.Millisecond)
	}

	captured.Store(0)
	capturedBytes.Store(0)
	runtime.GC()
	b.ResetTimer()

	result, err := ReplayToLoopback(sendHandle, packets, true)
	if err != nil {
		b.Skipf("replay on %s failed: %v", nicSend, err)
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

// getXdpNicPair 获取一对 XDP 可用的物理网卡。不足则 skip。
func getXdpNicPair(b *testing.B) (nicCap, nicSend string) {
	physNics := FindPhysicalEthernetNics()
	xdpNics := FindXdpCapableNics(2)

	if len(physNics) == 0 {
		b.Skip("no physical Ethernet NIC found")
	}

	if len(physNics) == 1 && len(xdpNics) == 0 {
		b.Skipf("found physical NIC %s but it does not support AF_XDP (driver/hardware limitation); "+
			"upgrade kernel >= 5.4 or use a NIC with XDP driver support (e.g., Intel i40e, mlx5, nfp)", physNics[0])
	}

	if len(physNics) >= 2 && len(xdpNics) < 2 {
		b.Skipf("found %d physical NIC(s) but only %d support AF_XDP; "+
			"need 2 XDP-capable NICs; connect two compatible NICs with a crossover cable", len(physNics), len(xdpNics))
	}

	return xdpNics[0], xdpNics[1]
}

// === 物理网卡 XDP 基线 (需 2 张物理网卡互联) ===

// BenchmarkXdpNativeSmoke 物理网卡 XDP 原生模式冒烟基线。
func BenchmarkXdpNativeSmoke(b *testing.B) {
	nicCap, nicSend := getXdpNicPair(b)
	runXdpPhysNicBaseline(b, "xdp-native-smoke", LevelSmoke, nicCap, nicSend)
}

// BenchmarkXdpNativeSmallFlows 物理网卡 XDP 原生模式小流量基线。
func BenchmarkXdpNativeSmallFlows(b *testing.B) {
	nicCap, nicSend := getXdpNicPair(b)
	runXdpPhysNicBaseline(b, "xdp-native-smallflows", LevelSmallFlows, nicCap, nicSend)
}

// BenchmarkXdpNativeBigFlows 物理网卡 XDP 原生模式大流量基线。
func BenchmarkXdpNativeBigFlows(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping bigflows in short mode")
	}
	nicCap, nicSend := getXdpNicPair(b)
	runXdpPhysNicBaseline(b, "xdp-native-bigflows", LevelBigFlows, nicCap, nicSend)
}

// === 物理网卡 XDP raw capture（skip copy）基线 ===

// BenchmarkRawXdpNativeSmoke 物理网卡 XDP 原生模式纯计数（skip copy）冒烟基线。
func BenchmarkRawXdpNativeSmoke(b *testing.B) {
	nicCap, nicSend := getXdpNicPair(b)
	runXdpPhysNicRawBaseline(b, "raw-xdp-native-smoke", LevelSmoke, nicCap, nicSend)
}

// BenchmarkRawXdpNativeSmallFlows 物理网卡 XDP 原生模式纯计数（skip copy）小流量基线。
func BenchmarkRawXdpNativeSmallFlows(b *testing.B) {
	nicCap, nicSend := getXdpNicPair(b)
	runXdpPhysNicRawBaseline(b, "raw-xdp-native-smallflows", LevelSmallFlows, nicCap, nicSend)
}

// BenchmarkRawXdpNativeBigFlows 物理网卡 XDP 原生模式纯计数（skip copy）大流量基线。
func BenchmarkRawXdpNativeBigFlows(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping bigflows in short mode")
	}
	nicCap, nicSend := getXdpNicPair(b)
	runXdpPhysNicRawBaseline(b, "raw-xdp-native-bigflows", LevelBigFlows, nicCap, nicSend)
}
