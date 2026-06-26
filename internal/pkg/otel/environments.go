package otel

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"zhongkui.com/zhongkui/internal/pkg/base"
)

var (
	once sync.Once
	Env  OtelEnv
)

type OtelEnv struct {
	ServiceName  string
	OtlpEndpoint string
	OtlpInsecure bool
	OtlpProtocol string
}

// ExtractConfig 从 viper 实例中提取 OTEL 配置，同时支持环境变量覆盖
//
// 配置优先级：环境变量 > 配置文件 > 默认值
//
// 支持的环境变量（viper AutomaticEnv 自动绑定，环境变量前缀为 ZKI）：
//   - ZKI_OTEL_SERVICE_NAME     -> otel.service_name
//   - ZKI_OTEL_TRACES_EXPORTER  -> otel.traces_exporter
//   - ZKI_OTEL_METRICS_EXPORTER -> otel.metrics_exporter
//   - ZKI_OTEL_LOGS_EXPORTER    -> otel.logs_exporter
//   - ZKI_OTEL_OTLP_ENDPOINT    -> otel.otlp_endpoint
//   - ZKI_OTEL_OTLP_INSECURE    -> otel.otlp_insecure
//
// 同时支持标准 OTEL 环境变量覆盖：
//   - OTEL_SERVICE_NAME           -> 覆盖 service_name
//   - OTEL_TRACES_EXPORTER        -> 覆盖 traces_exporter
//   - OTEL_METRICS_EXPORTER       -> 覆盖 metrics_exporter
//   - OTEL_LOGS_EXPORTER          -> 覆盖 logs_exporter
//   - OTEL_EXPORTER_OTLP_ENDPOINT -> 覆盖 otlp_endpoint
//   - OTEL_EXPORTER_OTLP_INSECURE -> 覆盖 otlp_insecure
//   - OTEL_EXPORTER_OTLP_PROTOCOL -> 覆盖 otlp_protocol

const (
	OTLP_PROTOCOL_GRPC = "gRPC"
	OTLP_PROTOCOL_HTTP = "HTTP"
)

var (
	EXPORTER_OTLP_PROTOCOL_ENV string = OTLP_PROTOCOL_GRPC
	EXPORTER_OTLP_ENDPOINT_ENV string = "http://localhost:4317"
	EXPORTER_OTLP_INSECURE_ENV bool   = false
)

func init() {
	// 环境变量
	tmp := os.Getenv(fmt.Sprintf(`%s_OTEL_EXPORTER_OTLP_PROTOCOL`, strings.ToUpper(base.GLOBAL_PREFIX)))

	if tmp != "" {
		EXPORTER_OTLP_PROTOCOL_ENV = tmp
	}
}
