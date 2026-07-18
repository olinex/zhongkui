package provider

import (
	"github.com/samber/do/v2"
	"zhongkui.com/zhongkui/internal/capture"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	reporter_v1 "zhongkui.com/zhongkui/internal/pkg/protobuf/reporter/v1"
)

var (
	_ do.ShutdownerWithError = (*capture.CaptureClient)(nil)
)

func newCaptureClientProvider(i do.Injector) (*capture.CaptureClient, error) {
	if viper, err := InvokeViper(i); err != nil {
		return nil, errors.WithStack(err)
	} else if cfg, err := capture.ExtractConfig(viper); err != nil {
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

func NewCaptureClient() do.Provider[*capture.CaptureClient] {
	return newCaptureClientProvider
}

func InjectCaptureClient(i do.Injector) {
	do.Provide(i, NewCaptureClient())
}

func InvokeCaptureClient(i do.Injector) (*capture.CaptureClient, error) {
	return do.Invoke[*capture.CaptureClient](i)
}

func MustInvokeCaptureClient(i do.Injector) *capture.CaptureClient {
	return do.MustInvoke[*capture.CaptureClient](i)
}
