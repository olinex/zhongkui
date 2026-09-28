// Package status 提供外部依赖状态自省注册表。
// 它是「外部依赖状态自省机制」的地基：允许各模块注册自己的连通性探测项，
// 由注册表统一并行执行，并最终以统一的 Status 结构体与表格形式对外呈现。
//
// 典型用法：
//
//	reg := status.New()
//	reg.Register(status.Spec{Name: "redis", Category: "cache", Probe: probeRedis})
//	reg.SetPerProbeTimeout(3 * time.Second)
//	results := reg.Run(ctx)
//	_ = reg.Render(nil)
package status

import (
	"context"
	"time"
)

// Status 单个外部依赖的健康状态。
// 字段名/类型固定；Err 与 Latency 带 omitempty。
type Status struct {
	Name       string        `json:"name"`              // 依赖名称
	Category   string        `json:"category"`          // 依赖类别
	Configured bool          `json:"configured"`        // 是否已配置
	Reachable  bool          `json:"reachable"`         // 是否可达
	Err        string        `json:"message,omitempty"` // 不可达原因
	Latency    time.Duration `json:"latency,omitempty"` // 探测耗时
}

// State 返回稳定枚举值，用于展示与 json 输出。
// 取值：ok / unreachable / disabled。
func (s Status) State() string {
	switch {
	case !s.Configured:
		return "disabled"
	case !s.Reachable:
		return "unreachable"
	default:
		return "ok"
	}
}

// Probe 一次依赖连通性探测。
// 实现必须自行 recover 内部 panic 并返回 Status（绝不抛出）。
type Probe func(ctx context.Context) Status

// Spec 一个待注册的依赖探测项。
type Spec struct {
	Name     string // 依赖名称
	Category string // 依赖类别
	Probe    Probe  // 连通性探测函数
}
