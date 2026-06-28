package otel

import (
	"context"

	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// NewSampler 根据环境配置创建对应的 Sampler。
//
//   - always_on: 全量采样（默认），等价于 sdktrace.AlwaysSample
//   - head:      头部概率采样，使用 ParentBased + TraceIDRatioBased，
//     尊重父 Span 的采样决策，仅对新根 Span 按 SamplerRatio 概率采样
//   - tail:      尾部过滤，Span 创建时全量采样，由 SpanProcessor 在 Span 结束时过滤
func NewSampler() sdktrace.Sampler {
	switch Env.SamplerType {
	case SAMPLER_HEAD:
		return sdktrace.ParentBased(
			sdktrace.TraceIDRatioBased(Env.SamplerRatio),
		)
	case SAMPLER_TAIL:
		return sdktrace.AlwaysSample()
	default:
		return sdktrace.AlwaysSample()
	}
}

// NewFilterSpanProcessor 创建一个尾部过滤 SpanProcessor。
// 当 sampler type 非 tail 时返回 nil。
//
// next 是下游的 SpanProcessor（通常为 BatchSpanProcessor），
// 只有通过过滤条件的 Span 才会被转发给 next。
func NewFilterSpanProcessor(next sdktrace.SpanProcessor) sdktrace.SpanProcessor {
	if Env.SamplerType != SAMPLER_TAIL {
		return nil
	}
	return &filterSpanProcessor{
		next: next,
		filter: func(span sdktrace.ReadOnlySpan) bool {
			return tailFilter(span)
		},
	}
}

// tailFilter 尾部过滤逻辑：仅保留状态为 Error 的 Span。
func tailFilter(span sdktrace.ReadOnlySpan) bool {
	if Env.TailOnError {
		return span.Status().Code == codes.Error
	}
	return true
}

type filterSpanProcessor struct {
	next   sdktrace.SpanProcessor
	filter func(sdktrace.ReadOnlySpan) bool
}

func (p *filterSpanProcessor) OnStart(ctx context.Context, s sdktrace.ReadWriteSpan) {
	p.next.OnStart(ctx, s)
}

func (p *filterSpanProcessor) OnEnd(s sdktrace.ReadOnlySpan) {
	if p.filter(s) {
		p.next.OnEnd(s)
	}
}

func (p *filterSpanProcessor) Shutdown(ctx context.Context) error {
	return p.next.Shutdown(ctx)
}

func (p *filterSpanProcessor) ForceFlush(ctx context.Context) error {
	return p.next.ForceFlush(ctx)
}
