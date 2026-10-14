// Package s3 提供与S3兼容对象存储服务交互的客户端封装
// 基于minio-go库实现，支持Amazon S3、MinIO等兼容S3协议的对象存储服务
package s3

import (
	"fmt"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"zhongkuisec.com/zhongkui/internal/pkg/base"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// otelRoundTripper 是实现了 http.RoundTripper 接口的包装结构体，
// 为每个 S3 HTTP 请求创建 OpenTelemetry span，实现链路追踪。
type otelRoundTripper struct {
	next http.RoundTripper
}

// NewOtelRoundTripper 创建一个支持 OpenTelemetry 追踪的 HTTP RoundTripper。
//
// 参数:
//   - next: 底层的 http.RoundTripper 实例，通常为 http.DefaultTransport
//
// 返回:
//   - 包装后的 http.RoundTripper，在为每个请求创建追踪 span
func NewOtelRoundTripper(next http.RoundTripper) http.RoundTripper {
	return &otelRoundTripper{next: next}
}

// RoundTrip 实现 http.RoundTripper 接口。
// 为每个 HTTP 请求创建一个追踪 span，记录请求和响应的关键属性。
func (t *otelRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// 从请求中提取传播的上下文（如果有的话）
	ctx := otel.GetTextMapPropagator().Extract(req.Context(), propagation.HeaderCarrier(req.Header))

	// 从 URL 路径中解析 bucket 和 object key
	bucket, objectKey := parseS3Path(req.URL.Path)

	spanName := fmt.Sprintf(`%s.%s/%s`, Prefix, req.Method, req.URL.Host)

	ctx, span := otel.Tracer(fmt.Sprintf(`%s-%s`, base.GlobalPrefix, Prefix)).Start(ctx, spanName)
	defer span.End()

	span.SetAttributes(
		semconv.HTTPRequestMethodKey.String(req.Method),
		semconv.URLFullKey.String(req.URL.String()),
		semconv.ServerAddressKey.String(req.URL.Host),
	)
	if bucket != "" {
		span.SetAttributes(attribute.String(fmt.Sprintf(`%s.bucket`, Prefix), bucket))
	}
	if objectKey != "" {
		span.SetAttributes(attribute.String(fmt.Sprintf(`%s.object_key`, Prefix), objectKey))
	}

	// 将上下文注入到请求中，确保传播
	req = req.WithContext(ctx)
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

	resp, err := t.next.RoundTrip(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return resp, errors.WithStack(err)
	}

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))
	if resp.StatusCode >= 400 {
		span.SetStatus(codes.Error, http.StatusText(resp.StatusCode))
	}

	return resp, nil
}

// parseS3Path 从 S3 URL 路径中解析存储桶名称和对象键。
// S3 路径格式通常为: /bucket-name/object/key/path
func parseS3Path(path string) (bucket, objectKey string) {
	path = strings.TrimPrefix(path, "/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		return "", ""
	}
	bucket = parts[0]
	if len(parts) > 1 {
		objectKey = parts[1]
	}
	return bucket, objectKey
}
