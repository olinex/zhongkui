package depend

import (
	"github.com/spf13/viper"
	"go.uber.org/fx"
	"zhongkui.com/zhongkui/internal/capture"
	reporter_v1 "zhongkui.com/zhongkui/internal/pkg/protobuf/reporter/v1"
)

type CaptureClientFxIn struct {
	fx.In
	CaptureLoggerFxIn
	CaptureS3ClientIn
	ReporterGrpcClientFxIn
	V *viper.Viper
}

func NewCaptureClientFx(p CaptureClientFxIn) *capture.CaptureClient {
	cfg, err := capture.ExtractConfig(p.V)
	if err != nil {
		panic(err)
	}
	return capture.NewCaptureClient(
		p.CaptureLogger,
		reporter_v1.NewReporterServiceClient(p.ReporterGrpcClient),
		p.CaptureS3,
		cfg,
	)
}
