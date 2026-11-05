package provider

import (
	"github.com/samber/do/v2"
	"zhongkuisec.com/zhongkui/internal/capture"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	reporter_v1 "zhongkuisec.com/zhongkui/internal/pkg/protobuf/reporter/v1"
	"zhongkuisec.com/zhongkui/internal/pkg/validator"
)

var (
	_ do.ShutdownerWithError = (*capture.CaptureClient)(nil)
)

func newCaptureClientProvider(i do.Injector) (*capture.CaptureClient, error) {
	if viper, err := InvokeViper(i); err != nil {
		return nil, errors.WithStack(err)
	} else if cfg, err := capture.ExtractConfig(viper, validator.Validate); err != nil {
		return nil, errors.WithStack(err)
	} else if logger, err := InvokeCaptureLogger(i); err != nil {
		return nil, errors.WithStack(err)
	} else if grpcClient, err := InvokeReporterGrpcClient(i); err != nil {
		return nil, errors.WithStack(err)
	} else if s3, err := InvokeS3(i); err != nil {
		return nil, errors.WithStack(err)
	} else {
		return capture.NewCaptureClient(
			logger,
			reporter_v1.NewReporterServiceClient(grpcClient),
			s3,
			cfg,
		), nil
	}
}

// NewCaptureClient 返回构建抓包客户端的 provider 工厂。
func NewCaptureClient() do.Provider[*capture.CaptureClient] {
	return newCaptureClientProvider
}

// InjectCaptureClient 向依赖注入器注册抓包客户端 provider。
func InjectCaptureClient(i do.Injector) {
	do.Provide(i, NewCaptureClient())
}

// InvokeCaptureClient 从依赖注入器解析抓包客户端，错误交由调用方处理。
func InvokeCaptureClient(i do.Injector) (*capture.CaptureClient, error) {
	return do.Invoke[*capture.CaptureClient](i)
}

// MustInvokeCaptureClient 从依赖注入器解析抓包客户端，失败时 panic。
func MustInvokeCaptureClient(i do.Injector) *capture.CaptureClient {
	return do.MustInvoke[*capture.CaptureClient](i)
}
