package otel

import (
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"zhongkui.com/zhongkui/internal/pkg/base"
)

func NewTraceExporter() (sdktrace.SpanExporter, error) {
	if base.Env.Debug {
		return stdouttrace.New(
			stdouttrace.WithPrettyPrint(),
		)
	}

}
