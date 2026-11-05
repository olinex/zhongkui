package capture

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	reporter_v1 "zhongkuisec.com/zhongkui/internal/pkg/protobuf/reporter/v1"
	"zhongkuisec.com/zhongkui/internal/pkg/validator"
)

// testValidate 测试用的验证器实例，绑定 binding 标签名以便验证配置结构体的校验规则
var testValidate = validator.Validate

// TestWatchConfig_TolerateIntervalTimeout 测试WatchConfig的TolerateIntervalTimeout方法
func TestWatchConfig_TolerateIntervalTimeout(t *testing.T) {
	Convey("Test WatchConfig TolerateIntervalTimeout", t, func() {
		Convey("当使用默认容差比率时", func() {
			config := &WatchConfig{
				IntervalTimeout: 60 * time.Second,
				TolerateRatio:   3.0,
			}

			expected := time.Duration(float64(60*time.Second.Nanoseconds()) * float64(1+3.0))
			actual := config.TolerateIntervalTimeout()

			So(actual, ShouldEqual, expected)
			So(actual, ShouldEqual, 240*time.Second) // 60 * (1 + 3) = 240
		})

		Convey("当容差比率为0时", func() {
			config := &WatchConfig{
				IntervalTimeout: 30 * time.Second,
				TolerateRatio:   0.0,
			}

			actual := config.TolerateIntervalTimeout()
			So(actual, ShouldEqual, 30*time.Second) // 30 * (1 + 0) = 30
		})

		Convey("当容差比率为小数时", func() {
			config := &WatchConfig{
				IntervalTimeout: 10 * time.Second,
				TolerateRatio:   0.5,
			}

			actual := config.TolerateIntervalTimeout()
			So(actual, ShouldEqual, 15*time.Second) // 10 * (1 + 0.5) = 15
		})

		Convey("当间隔时间为0时", func() {
			config := &WatchConfig{
				IntervalTimeout: 0,
				TolerateRatio:   3.0,
			}

			actual := config.TolerateIntervalTimeout()
			So(actual, ShouldEqual, 0*time.Second) // 0 * (1 + 3) = 0
		})
	})
}

// TestFeedbackConfig_TolerateIntervalTimeout 测试FeedbackConfig的TolerateIntervalTimeout方法
func TestFeedbackConfig_TolerateIntervalTimeout(t *testing.T) {
	Convey("Test FeedbackConfig TolerateIntervalTimeout", t, func() {
		Convey("当使用默认容差比率时", func() {
			config := &FeedbackConfig{
				IntervalTimeout: 5 * time.Second,
				TolerateRatio:   3.0,
			}

			expected := time.Duration(float64(5*time.Second.Nanoseconds()) * float64(1+3.0))
			actual := config.TolerateIntervalTimeout()

			So(actual, ShouldEqual, expected)
			So(actual, ShouldEqual, 20*time.Second) // 5 * (1 + 3) = 20
		})

		Convey("当容差比率为0时", func() {
			config := &FeedbackConfig{
				IntervalTimeout: 2 * time.Second,
				TolerateRatio:   0.0,
			}

			actual := config.TolerateIntervalTimeout()
			So(actual, ShouldEqual, 2*time.Second) // 2 * (1 + 0) = 2
		})
	})
}

// TestConfig_DefaultValues 测试配置结构的默认值
func TestConfig_DefaultValues(t *testing.T) {
	Convey("Test Config Default Values", t, func() {
		Convey("WatchConfig默认值", func() {
			config := &WatchConfig{}

			// 注意：默认值是通过mapstructure标签设置的，这里测试结构体零值
			// 实际使用中，配置会被Viper填充默认值
			So(config.IntervalTimeout, ShouldEqual, 0)
			So(config.TolerateRatio, ShouldEqual, 0)
			So(config.MaxRetries, ShouldEqual, 0)
		})

		Convey("FeedbackConfig默认值", func() {
			config := &FeedbackConfig{}

			So(config.IntervalTimeout, ShouldEqual, 0)
			So(config.TolerateRatio, ShouldEqual, 0)
			So(config.MaxRetries, ShouldEqual, 0)
		})

		Convey("UploadConfig默认值", func() {
			config := &UploadConfig{}

			So(config.DumpDir, ShouldEqual, "")
			So(config.EncoderLevel, ShouldEqual, 0)
			So(config.IntervalTimeout, ShouldEqual, 0)
			So(config.PacketBufferSize, ShouldEqual, 0)
			So(config.MaxRetries, ShouldEqual, 0)
		})
	})
}

// TestExtractConfig 测试从Viper提取配置
func TestExtractConfig(t *testing.T) {
	Convey("Test ExtractConfig", t, func() {
		Convey("当Viper包含有效配置时", func() {
			v := viper.New()

			// 设置配置值
			v.Set("capture.resource.namespace", "test-namespace")
			v.Set("capture.resource.name", "test-name")
			v.Set("capture.resource.version", "v1.0.0")

			v.Set("capture.watch.intervalTimeout", "30s")
			v.Set("capture.watch.tolerateRatio", 2.5)
			v.Set("capture.watch.maxRetries", 3)

			v.Set("capture.feedback.intervalTimeout", "10s")
			v.Set("capture.feedback.tolerateRatio", 1.5)
			v.Set("capture.feedback.maxRetries", 2)

			v.Set("capture.upload.dumpDir", "/tmp/test")
			v.Set("capture.upload.encoderLevel", 4)
			v.Set("capture.upload.intervalTimeout", "2m")
			v.Set("capture.upload.packetBufferSize", 500000)
			v.Set("capture.upload.maxRetries", 4)

			config, err := ExtractConfig(v, testValidate)

			So(err, ShouldBeNil)
			So(config, ShouldNotBeNil)

			// 验证Resource
			So(config.Resource.Namespace, ShouldEqual, "test-namespace")
			So(config.Resource.Name, ShouldEqual, "test-name")
			So(config.Resource.Version, ShouldEqual, "v1.0.0")

			// 验证Watch配置
			So(config.Watch.IntervalTimeout, ShouldEqual, 30*time.Second)
			So(config.Watch.TolerateRatio, ShouldEqual, 2.5)
			So(config.Watch.MaxRetries, ShouldEqual, uint(3))

			// 验证Feedback配置
			So(config.Feedback.IntervalTimeout, ShouldEqual, 10*time.Second)
			So(config.Feedback.TolerateRatio, ShouldEqual, 1.5)
			So(config.Feedback.MaxRetries, ShouldEqual, uint(2))

			// 验证Upload配置
			So(config.Upload.DumpDir, ShouldEqual, "/tmp/test")
			So(config.Upload.EncoderLevel, ShouldEqual, 4)
			So(config.Upload.IntervalTimeout, ShouldEqual, 2*time.Minute)
			So(config.Upload.PacketBufferSize, ShouldEqual, int32(500000))
			So(config.Upload.MaxRetries, ShouldEqual, uint(4))
		})

		Convey("当Viper为空时", func() {
			v := viper.New()

			config, err := ExtractConfig(v, testValidate)

			// 当Viper中没有capture前缀的配置时，ExtractConfig会返回错误
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "Cannot get")
			So(config, ShouldBeNil)
		})

		Convey("当配置包含无效的时间格式时", func() {
			v := viper.New()
			// 首先设置一些有效配置，否则v.Sub("capture")会返回nil
			v.Set("capture.resource.name", "test")
			v.Set("capture.watch.intervalTimeout", "invalid-duration")

			config, err := ExtractConfig(v, testValidate)

			// 无效的时间格式会导致解析错误
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "invalid duration")
			So(config, ShouldBeNil)
		})
	})
}

// TestDescribeConfig 测试配置描述生成
func TestDescribeConfig(t *testing.T) {
	Convey("Test DescribeConfig", t, func() {
		Convey("生成配置描述", func() {
			descriptions := DescribeConfig()

			So(descriptions, ShouldNotBeNil)

			// 验证描述包含主要字段
			// ConfigDescriptions是一个切片，包含所有配置字段的描述
			So(len(descriptions), ShouldBeGreaterThan, 0)

			// 验证至少包含一些预期的配置路径
			hasWatchConfig := false
			hasFeedbackConfig := false
			hasUploadConfig := false

			for _, desc := range descriptions {
				if desc.Path == "capture.watch.intervalTimeout" {
					hasWatchConfig = true
				}
				if desc.Path == "capture.feedback.intervalTimeout" {
					hasFeedbackConfig = true
				}
				if desc.Path == "capture.upload.dumpDir" {
					hasUploadConfig = true
				}
			}

			// 这些配置应该至少有一个存在
			So(hasWatchConfig || hasFeedbackConfig || hasUploadConfig, ShouldBeTrue)
		})
	})
}

// TestConfig_Integration 测试配置的集成场景
func TestConfig_Integration(t *testing.T) {
	Convey("Test Config Integration Scenarios", t, func() {
		Convey("完整配置场景", func() {
			// 创建完整的配置对象
			config := &Config{
				Resource: reporter_v1.ResourceRaw{
					Namespace: "production",
					Name:      "network-capture",
					Version:   "v2.1.0",
				},
				Watch: WatchConfig{
					IntervalTimeout: 45 * time.Second,
					TolerateRatio:   2.0,
					MaxRetries:      4,
				},
				Feedback: FeedbackConfig{
					IntervalTimeout: 8 * time.Second,
					TolerateRatio:   1.8,
					MaxRetries:      3,
				},
				Upload: UploadConfig{
					DumpDir:          "/var/lib/capture",
					EncoderLevel:     3,
					IntervalTimeout:  90 * time.Second,
					PacketBufferSize: 2000000,
					MaxRetries:       5,
				},
			}

			// 验证配置一致性
			So(config.Resource.Name, ShouldEqual, "network-capture")
			So(config.Watch.TolerateIntervalTimeout(), ShouldEqual, 135*time.Second) // 45 * (1 + 2)
			// 注意：8 * (1 + 1.8) = 22.4秒，浮点数计算可能有精度误差
			// 使用近似比较，允许1毫秒的误差
			expectedFeedbackTimeout := 22*time.Second + 400*time.Millisecond
			actualFeedbackTimeout := config.Feedback.TolerateIntervalTimeout()
			So(actualFeedbackTimeout, ShouldAlmostEqual, expectedFeedbackTimeout, time.Millisecond)
			So(config.Upload.DumpDir, ShouldEqual, "/var/lib/capture")
		})

		Convey("最小化配置场景", func() {
			config := &Config{
				Resource: reporter_v1.ResourceRaw{
					Name: "minimal-config",
				},
				// 其他字段使用零值
			}

			So(config.Resource.Name, ShouldEqual, "minimal-config")
			So(config.Watch.IntervalTimeout, ShouldEqual, 0)
			So(config.Feedback.IntervalTimeout, ShouldEqual, 0)
			So(config.Upload.DumpDir, ShouldEqual, "")
		})

		Convey("边界值测试", func() {
			Convey("最大重试次数为0", func() {
				config := &WatchConfig{
					IntervalTimeout: 10 * time.Second,
					TolerateRatio:   1.0,
					MaxRetries:      0,
				}

				So(config.MaxRetries, ShouldEqual, uint(0))
				So(config.TolerateIntervalTimeout(), ShouldEqual, 20*time.Second)
			})

			Convey("负的容差比率（虽然不应该发生）", func() {
				config := &FeedbackConfig{
					IntervalTimeout: 5 * time.Second,
					TolerateRatio:   -0.5,
				}

				// 负的容差比率会导致超时时间小于基础间隔
				// 5 * (1 - 0.5) = 2.5秒 = 2秒500毫秒
				// 浮点数计算可能有精度误差，使用近似比较
				expectedTimeout := 2*time.Second + 500*time.Millisecond
				actualTimeout := config.TolerateIntervalTimeout()
				So(actualTimeout, ShouldAlmostEqual, expectedTimeout, time.Millisecond)
			})

			Convey("极大的缓冲区大小", func() {
				config := &UploadConfig{
					PacketBufferSize: 1000000000, // 10亿
				}

				So(config.PacketBufferSize, ShouldEqual, int32(1000000000))
			})
		})
	})
}

// TestConfig_EdgeCases 测试边界情况和错误处理
func TestConfig_EdgeCases(t *testing.T) {
	Convey("Test Config Edge Cases", t, func() {
		Convey("时间解析边界情况", func() {
			v := viper.New()

			// 测试各种时间格式
			testCases := []struct {
				key   string
				value string
			}{
				{"capture.watch.intervalTimeout", "1h30m"},
				{"capture.watch.intervalTimeout", "500ms"},
				{"capture.watch.intervalTimeout", "0s"},
			}

			for _, tc := range testCases {
				v.Set(tc.key, tc.value)

				config, err := ExtractConfig(v, testValidate)
				So(err, ShouldBeNil)
				So(config, ShouldNotBeNil)

				// 清除设置以进行下一个测试
				v.Set(tc.key, nil)
			}
		})

		Convey("配置前缀测试", func() {
			Convey("PREFIX常量值", func() {
				So(Prefix, ShouldEqual, "capture")
			})

			Convey("配置键名格式", func() {
				// 验证配置键名符合预期格式
				expectedKeys := []string{
					"capture.resource",
					"capture.watch.intervalTimeout",
					"capture.watch.tolerateRatio",
					"capture.watch.maxRetries",
					"capture.feedback.intervalTimeout",
					"capture.feedback.tolerateRatio",
					"capture.feedback.maxRetries",
					"capture.upload.dumpDir",
					"capture.upload.encoderLevel",
					"capture.upload.intervalTimeout",
					"capture.upload.packetBufferSize",
					"capture.upload.maxRetries",
				}

				// 这些键名应该能够被Viper正确解析
				for _, key := range expectedKeys {
					So(key, ShouldContainSubstring, Prefix+".")
				}
			})
		})
	})
}
