package tagengine_test

import (
	"testing"

	"github.com/go-playground/validator/v10"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"

	"zhongkuisec.com/zhongkui/internal/tagengine"
)

// TestConfig 验证标签引擎配置的默认值与描述输出。
func TestConfig(t *testing.T) {
	Convey("Test tagengine config", t, func() {
		Convey("缺省配置使用默认 S3 正文上限", func() {
			v := viper.New()
			v.Set(tagengine.Prefix, map[string]any{})
			cfg, err := tagengine.ExtractConfig(v, validator.New())
			So(err, ShouldBeNil)
			So(cfg.S3CryptoMaxBytes, ShouldEqual, int64(8388608))
		})

		Convey("描述输出包含 s3cryptoMaxBytes 配置项", func() {
			descs := tagengine.DescribeConfig()
			found := false
			for _, desc := range descs {
				if desc.Path == tagengine.Prefix+".s3cryptoMaxBytes" {
					found = true
					break
				}
			}
			So(found, ShouldBeTrue)
		})
	})
}
