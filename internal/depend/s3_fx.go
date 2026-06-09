package depend

import (
	"fmt"

	"github.com/spf13/viper"
	"go.uber.org/fx"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/s3"
)

const (
	CAPTURE_S3 = "capture"
)

type S3FxIn struct {
	fx.In
	V *viper.Viper
}

type CaptureS3ClientIn struct {
	fx.In
	CaptureS3 *s3.S3Client `name:"capture"`
}

func newS3Client(name string) func(in S3FxIn) *s3.S3Client {
	return func(in S3FxIn) *s3.S3Client {
		if cfg, err := s3.ExtractConfig(in.V, name); err != nil {
			panic(err)
		} else if client, err := s3.NewS3Client(cfg); err != nil {
			panic(err)
		} else {
			return client
		}
	}
}

func s3Annotate(name string) any {
	return fx.Annotate(newS3Client(name), fx.ResultTags(fmt.Sprintf(`name:"%s"`, name)))
}

func CaptureS3Annotate() any {
	return s3Annotate(CAPTURE_S3)
}

func DescribeS3Config() config.ConfigDescriptions {
	return s3.DescribeConfig(CAPTURE_S3)
}
