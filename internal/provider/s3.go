package provider

import (
	"github.com/samber/do/v2"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/otel"
	"zhongkui.com/zhongkui/internal/pkg/s3"
	"zhongkui.com/zhongkui/internal/pkg/validator"
)

func newS3Provider() do.Provider[*s3.S3Client] {
	return func(i do.Injector) (*s3.S3Client, error) {
		if viper, err := InvokeViper(i); err != nil {
			return nil, errors.WithStack(err)
		} else if cfg, err := s3.ExtractConfig(viper, validator.Validate); err != nil {
			return nil, errors.WithStack(err)
		} else if kd, err := InvokeKeyDerivation(i); err != nil {
			return nil, errors.WithStack(err)
		} else {
			client, err := s3.NewS3Client(cfg, kd, otel.S3ClientOptions()...)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			return client, nil
		}
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

func MustInvokeS3(i do.Injector) *s3.S3Client {
	client, err := InvokeS3(i)
	if err != nil {
		panic(err)
	}
	return client
}

func DescribeS3Config() config.ConfigDescriptions {
	return s3.DescribeConfig()
}
