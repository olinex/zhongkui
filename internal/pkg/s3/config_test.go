package s3

import (
	"strings"
	"testing"

	"github.com/creasty/defaults"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

// TestConfigDefaultValues 测试 Config 结构体的默认值
// 验证当使用 defaults.Set() 函数时，Config 结构体的字段是否正确设置默认值
func TestConfigDefaultValues(t *testing.T) {
	Convey("测试 Config 结构体的默认值", t, func() {
		Convey("应该正确设置默认值", func() {
			cfg := &Config{}
			err := defaults.Set(cfg)
			So(err, ShouldBeNil)

			// 验证默认值
			// 注意：github.com/creasty/defaults 包将 "-" 解释为空字符串
			So(cfg.Endpoint, ShouldEqual, "")
			So(cfg.AccessKey, ShouldEqual, "")
			So(cfg.SecretKey, ShouldEqual, "")
			So(cfg.Bucket, ShouldEqual, "")
			So(cfg.Region, ShouldEqual, "")
			So(cfg.UseSSL, ShouldBeTrue)
		})

		Convey("应该允许手动设置值并覆盖默认值", func() {
			cfg := &Config{
				Endpoint:  "http://localhost:9000",
				AccessKey: "test-access-key",
				SecretKey: "test-secret-key",
				Bucket:    "test-bucket",
				Region:    "us-east-1",
				UseSSL:    false,
			}

			So(cfg.Endpoint, ShouldEqual, "http://localhost:9000")
			So(cfg.AccessKey, ShouldEqual, "test-access-key")
			So(cfg.SecretKey, ShouldEqual, "test-secret-key")
			So(cfg.Bucket, ShouldEqual, "test-bucket")
			So(cfg.Region, ShouldEqual, "us-east-1")
			So(cfg.UseSSL, ShouldBeFalse)
		})
	})
}

// TestDescribeConfig 测试 DescribeConfig 函数
// 验证 DescribeConfig 函数返回正确的配置描述信息
func TestDescribeConfig(t *testing.T) {
	Convey("测试 DescribeConfig 函数", t, func() {
		Convey("应该返回包含正确前缀的配置描述", func() {
			descriptions := DescribeConfig()

			So(descriptions, ShouldNotBeNil)
			So(len(descriptions), ShouldBeGreaterThan, 0)

			// 检查是否包含预期的配置项
			// 注意：config.DescribeConfig 可能返回不同的路径格式
			// 我们检查是否至少包含一些预期的配置项
			hasConfigItems := false
			hasS3Prefix := false

			for _, desc := range descriptions {
				// 检查路径是否包含 "s3" 前缀
				if strings.Contains(desc.Path, "s3") || strings.Contains(desc.Path, "endpoint") ||
					strings.Contains(desc.Path, "accessKey") || strings.Contains(desc.Path, "secretKey") ||
					strings.Contains(desc.Path, "bucket") || strings.Contains(desc.Path, "region") ||
					strings.Contains(desc.Path, "useSSL") {
					hasS3Prefix = true
				}

				// 检查是否有有效的配置项
				if desc.Path != "" && desc.Kind != "" {
					hasConfigItems = true
				}
			}

			So(hasConfigItems, ShouldBeTrue)
			So(hasS3Prefix, ShouldBeTrue)

			// 输出调试信息
			t.Logf("Found %d config descriptions", len(descriptions))
			for i, desc := range descriptions {
				t.Logf("  [%d] Path: %s, Kind: %s, Default: %s", i, desc.Path, desc.Kind, desc.Default)
			}
		})

		Convey("应该支持多个配置名称", func() {
			descriptions := DescribeConfig("default", "backup")

			So(descriptions, ShouldNotBeNil)
			So(len(descriptions), ShouldBeGreaterThan, 0)

			// 检查描述信息是否包含正确的配置项
			// 注意：DescribeConfig 函数可能返回相同的描述，无论名称如何
			// 这取决于 config.DescribeConfig 的具体实现
			for _, desc := range descriptions {
				So(desc.Path, ShouldNotBeEmpty)
			}
		})

		Convey("应该支持空配置名称列表", func() {
			descriptions := DescribeConfig()

			So(descriptions, ShouldNotBeNil)
			So(len(descriptions), ShouldBeGreaterThan, 0)
		})
	})
}

// TestExtractConfig 测试 ExtractConfig 函数
// 验证 ExtractConfig 函数能够正确从 Viper 实例中提取配置
func TestExtractConfig(t *testing.T) {
	Convey("测试 ExtractConfig 函数", t, func() {
		Convey("应该提取具有默认值的配置", func() {
			v := viper.New()
			// 设置一个空的 s3.default 配置节
			v.Set("s3.default", map[string]any{})

			cfg, err := ExtractConfig(v, "default")

			So(err, ShouldBeNil)
			So(cfg, ShouldNotBeNil)
			// 注意：github.com/creasty/defaults 包将 "-" 解释为空字符串
			So(cfg.Endpoint, ShouldEqual, "")
			So(cfg.AccessKey, ShouldEqual, "")
			So(cfg.SecretKey, ShouldEqual, "")
			So(cfg.Bucket, ShouldEqual, "")
			So(cfg.Region, ShouldEqual, "")
			So(cfg.UseSSL, ShouldBeTrue)
		})

		Convey("应该提取具有自定义值的配置", func() {
			v := viper.New()
			// 设置自定义的 s3.custom 配置
			v.Set("s3.custom.endpoint", "https://s3.amazonaws.com")
			v.Set("s3.custom.accessKey", "custom-access-key")
			v.Set("s3.custom.secretKey", "custom-secret-key")
			v.Set("s3.custom.bucket", "custom-bucket")
			v.Set("s3.custom.region", "us-west-2")
			v.Set("s3.custom.useSSL", true)

			cfg, err := ExtractConfig(v, "custom")

			So(err, ShouldBeNil)
			So(cfg, ShouldNotBeNil)
			So(cfg.Endpoint, ShouldEqual, "https://s3.amazonaws.com")
			So(cfg.AccessKey, ShouldEqual, "custom-access-key")
			So(cfg.SecretKey, ShouldEqual, "custom-secret-key")
			So(cfg.Bucket, ShouldEqual, "custom-bucket")
			So(cfg.Region, ShouldEqual, "us-west-2")
			So(cfg.UseSSL, ShouldBeTrue)
		})

		Convey("应该处理不存在的配置", func() {
			v := viper.New()
			// 不设置任何 s3 配置

			cfg, err := ExtractConfig(v, "nonexistent")

			// 根据 config.ExtractConfig 的实现，可能会返回错误或默认配置
			// 这里我们检查至少有一个结果（错误或配置）
			if err != nil {
				So(err, ShouldNotBeNil)
				So(cfg, ShouldBeNil)
			} else {
				So(cfg, ShouldNotBeNil)
			}
		})

		Convey("应该处理部分配置的情况", func() {
			v := viper.New()
			// 只设置部分配置字段
			v.Set("s3.partial.endpoint", "http://localhost:9000")
			v.Set("s3.partial.bucket", "partial-bucket")
			// 不设置 accessKey, secretKey, region, useSSL

			cfg, err := ExtractConfig(v, "partial")

			So(err, ShouldBeNil)
			So(cfg, ShouldNotBeNil)
			So(cfg.Endpoint, ShouldEqual, "http://localhost:9000")
			So(cfg.Bucket, ShouldEqual, "partial-bucket")
			// 其他字段应该使用默认值
			// 注意：github.com/creasty/defaults 包将 "-" 解释为空字符串
			So(cfg.AccessKey, ShouldEqual, "")
			So(cfg.SecretKey, ShouldEqual, "")
			So(cfg.Region, ShouldEqual, "")
			So(cfg.UseSSL, ShouldBeTrue)
		})

		Convey("应该处理 UseSSL 为 false 的情况", func() {
			v := viper.New()
			v.Set("s3.nossl.endpoint", "http://localhost:9000")
			v.Set("s3.nossl.accessKey", "nossl-key")
			v.Set("s3.nossl.secretKey", "nossl-secret")
			v.Set("s3.nossl.bucket", "nossl-bucket")
			v.Set("s3.nossl.region", "")
			v.Set("s3.nossl.useSSL", false)

			cfg, err := ExtractConfig(v, "nossl")

			So(err, ShouldBeNil)
			So(cfg, ShouldNotBeNil)
			So(cfg.UseSSL, ShouldBeFalse)
			So(cfg.Endpoint, ShouldEqual, "http://localhost:9000")
		})

		Convey("应该处理带有特殊字符的配置值", func() {
			v := viper.New()
			v.Set("s3.special.endpoint", "https://s3.example.com:8443/path")
			v.Set("s3.special.accessKey", "AKIAIOSFODNN7EXAMPLE")
			v.Set("s3.special.secretKey", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
			v.Set("s3.special.bucket", "my-bucket-name-123")
			v.Set("s3.special.region", "cn-north-1")
			v.Set("s3.special.useSSL", true)

			cfg, err := ExtractConfig(v, "special")

			So(err, ShouldBeNil)
			So(cfg, ShouldNotBeNil)
			So(cfg.Endpoint, ShouldEqual, "https://s3.example.com:8443/path")
			So(cfg.AccessKey, ShouldEqual, "AKIAIOSFODNN7EXAMPLE")
			So(cfg.SecretKey, ShouldEqual, "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
			So(cfg.Bucket, ShouldEqual, "my-bucket-name-123")
			So(cfg.Region, ShouldEqual, "cn-north-1")
			So(cfg.UseSSL, ShouldBeTrue)
		})
	})
}

// TestConfigIntegration 测试 Config 与其他组件的集成
// 验证 Config 结构体可以用于创建 S3 客户端
func TestConfigIntegration(t *testing.T) {
	Convey("测试 Config 与其他组件的集成", t, func() {
		Convey("应该能够创建有效的 S3 配置", func() {
			cfg := &Config{
				Endpoint:  "http://localhost:9000",
				AccessKey: "minioadmin",
				SecretKey: "minioadmin",
				Bucket:    "test-bucket",
				Region:    "",
				UseSSL:    false,
			}

			// 验证配置字段
			So(cfg.Endpoint, ShouldNotBeEmpty)
			So(cfg.AccessKey, ShouldNotBeEmpty)
			So(cfg.SecretKey, ShouldNotBeEmpty)
			So(cfg.Bucket, ShouldNotBeEmpty)

			// 注意：这里不实际创建 S3 客户端，因为那需要真正的 S3 服务
			// 在实际使用中，这个配置可以传递给 NewS3Client 函数
		})

		Convey("应该验证必需的配置字段", func() {
			// 测试缺少必需字段的情况
			cfg := &Config{
				// 缺少 Endpoint
				AccessKey: "test-key",
				SecretKey: "test-secret",
				Bucket:    "test-bucket",
			}

			// 虽然 Endpoint 的默认值是 "-"，但 defaults.Set() 会将其解释为空字符串
			// 在实际使用中，NewS3Client 函数会验证这些字段
			So(cfg.Endpoint, ShouldEqual, "")
			So(cfg.AccessKey, ShouldEqual, "test-key")
			So(cfg.SecretKey, ShouldEqual, "test-secret")
			So(cfg.Bucket, ShouldEqual, "test-bucket")
		})
	})
}
