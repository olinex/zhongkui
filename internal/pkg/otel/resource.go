// Package otel（本文件）负责构建 OpenTelemetry 资源信息，汇聚 SDK、进程、
// 系统、容器、主机及部署环境等属性。
package otel

import (
	"context"

	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/version"
)

// NewResource 构建 OpenTelemetry 资源，自动采集环境、SDK、进程与主机等属性。
func NewResource(ctx context.Context) (*sdkresource.Resource, error) {
	res, err := sdkresource.New(
		ctx,
		sdkresource.WithFromEnv(),      // 从环境变量 OTEL_RESOURCE_ATTRIBUTES 和 OTEL_SERVICE_NAME 中发现并提供属性。
		sdkresource.WithTelemetrySDK(), // 发现并提供所使用的 OpenTelemetry SDK 信息。
		sdkresource.WithProcess(),      // 发现并提供当前进程信息。
		sdkresource.WithOS(),           // 发现并提供操作系统信息。
		sdkresource.WithContainer(),    // 发现并提供容器信息。
		sdkresource.WithHost(),         // 发现并提供主机信息。
		sdkresource.WithAttributes(
			semconv.DeploymentEnvironment(base.Env.Environment),
			semconv.ServiceVersion(version.Short()),
		),
	)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return res, nil
}
