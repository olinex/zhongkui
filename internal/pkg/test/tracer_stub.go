//go:build !profile
// +build !profile

// Package test 提供测试与基准测试的通用辅助设施：阶段耗时追踪（StageTracer）
// 与基准运行、性能剖析工具。
package test

// Tracer 全局阶段耗时追踪器实例（非 profile 构建下的空实现）。
var Tracer = NewStageTracer()

// StageTracer 阶段耗时追踪器（非 profile 构建下的空实现，各方法均为空操作）。
type StageTracer struct{}

// NewStageTracer 创建一个 StageTracer 实例（非 profile 构建下的空实现）。
func NewStageTracer() *StageTracer { return &StageTracer{} }

// Begin 开始一个命名阶段（非 profile 构建下的空操作）。
func (t *StageTracer) Begin(name string) {}

// End 结束最近一次 Begin 开启的阶段（非 profile 构建下的空操作）。
func (t *StageTracer) End() {}

// Report 输出阶段统计报告（非 profile 构建下的空操作）。
func (t *StageTracer) Report() {}
