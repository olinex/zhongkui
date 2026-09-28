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

// NewS3 返回 S3 客户端的依赖注入 provider 工厂。
func NewS3() do.Provider[*s3.S3Client] {
	return newS3Provider()
}

// InjectS3 向依赖注入器注册 S3 客户端 provider。
func InjectS3(i do.Injector) {
	do.Provide(i, NewS3())
}

// InvokeS3 从依赖注入器解析 S3 客户端，错误由调用方处理。
func InvokeS3(i do.Injector) (*s3.S3Client, error) {
	return do.Invoke[*s3.S3Client](i)
}

// MustInvokeS3 从依赖注入器解析 S3 客户端，失败时 panic（仅用于启动关键依赖）。
func MustInvokeS3(i do.Injector) *s3.S3Client {
	client, err := InvokeS3(i)
	if err != nil {
		panic(err)
	}
	return client
}

// DescribeS3Config 返回 S3 配置项的元描述列表。
func DescribeS3Config() config.ConfigDescriptions {
	return s3.DescribeConfig()
}
