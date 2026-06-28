//go:build darwin

package bench

import (
	"context"
	"testing"

	"go.uber.org/zap"
	"zhongkui.com/zhongkui/internal/capture/wire"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	reporter_v1 "zhongkui.com/zhongkui/internal/pkg/protobuf/reporter/v1"
)

// === 常规基线 (darwin 上 platform = libpcap) ===

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

func newBenchLogger() *logging.Logger {
	return &logging.Logger{SugaredLogger: zap.NewNop().Sugar()}
}

// === 纯计数 (skip copy) 基线 ===

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
