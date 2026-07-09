package provider

import (
	"github.com/samber/do/v2"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/otel"
	"zhongkui.com/zhongkui/internal/pkg/s3"
)

func newS3Provider() do.Provider[*s3.S3Client] {
	return func(i do.Injector) (*s3.S3Client, error) {
		viper, err := InvokeViper(i)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		cfg, err := s3.ExtractConfig(viper)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		kd, err := InvokeKeyDerivation(i)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		// 如果 OTEL 已启用，为 S3 HTTP 调用添加链路追踪
		client, err := s3.NewS3Client(cfg, kd, otel.S3ClientOptions()...)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return client, nil
	}
}

func NewS3() do.Provider[*s3.S3Client] {
	return newS3Provider()
}

func InjectS3(i do.Injector) {
	do.Provide(i, NewS3())
}

func InvokeS3(i do.Injector) (*s3.S3Client, error) {
	return do.Invoke[*s3.S3Client](i)
}

func DescribeS3Config() config.ConfigDescriptions {
	return s3.DescribeConfig()
}
