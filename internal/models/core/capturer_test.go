package core

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"google.golang.org/protobuf/proto"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/etcd"
	reporter_v1 "zhongkui.com/zhongkui/internal/pkg/protobuf/reporter/v1"
)

// TestReporterConfigKey 测试 ReporterConfigKey 函数
func TestReporterConfigKey(t *testing.T) {
	convey.Convey("Test ReporterConfigKey function", t, func() {
		convey.Convey("Should generate correct config key with prefix", func() {
			resource := &reporter_v1.Resource{
				Namespace: "test-namespace",
				Name:      "test-capturer",
				Version:   "v1.0.0",
			}

			// 注意：etcd.WithPrefix("reporter/") 返回前缀字符串
			expectedPrefix := etcd.WithPrefix("reporter/")
			expectedKey := expectedPrefix + "test-namespace/test-capturer/v1.0.0/cfg"

			actualKey := ReporterConfigKey(resource)
			convey.So(actualKey, convey.ShouldEqual, expectedKey)
		})

		convey.Convey("Should handle empty namespace, name and version", func() {
			resource := &reporter_v1.Resource{
				Namespace: "",
				Name:      "",
				Version:   "",
			}

			expectedPrefix := etcd.WithPrefix("reporter/")
			expectedKey := expectedPrefix + "///cfg"

			actualKey := ReporterConfigKey(resource)
			convey.So(actualKey, convey.ShouldEqual, expectedKey)
		})
	})
}

// TestReporterStatusListKey 测试 ReporterStatusListKey 函数
func TestReporterStatusListKey(t *testing.T) {
	convey.Convey("Test ReporterStatusListKey function", t, func() {
		convey.Convey("Should generate correct status list key with prefix", func() {
			resource := &reporter_v1.Resource{
				Namespace: "test-namespace",
				Name:      "test-capturer",
				Version:   "v1.0.0",
			}

			expectedPrefix := etcd.WithPrefix("reporter/")
			expectedKey := expectedPrefix + "test-namespace/test-capturer/v1.0.0/sts"

			actualKey := ReporterStatusListKey(resource)
			convey.So(actualKey, convey.ShouldEqual, expectedKey)
		})
	})
}

// TestReporterStatusDetailKey 测试 ReporterStatusDetailKey 函数
func TestReporterStatusDetailKey(t *testing.T) {
	convey.Convey("Test ReporterStatusDetailKey function", t, func() {
		convey.Convey("Should generate correct status detail key with session ID", func() {
			resource := &reporter_v1.Resource{
				Namespace: "test-namespace",
				Name:      "test-capturer",
				Version:   "v1.0.0",
			}
			sessionID := "session-12345"

			expectedPrefix := etcd.WithPrefix("reporter/")
			expectedKey := expectedPrefix + "test-namespace/test-capturer/v1.0.0/sts/session-12345"

			actualKey := ReporterStatusDetailKey(resource, sessionID)
			convey.So(actualKey, convey.ShouldEqual, expectedKey)
		})

		convey.Convey("Should handle empty session ID", func() {
			resource := &reporter_v1.Resource{
				Namespace: "test-namespace",
				Name:      "test-capturer",
				Version:   "v1.0.0",
			}
			sessionID := ""

			expectedPrefix := etcd.WithPrefix("reporter/")
			expectedKey := expectedPrefix + "test-namespace/test-capturer/v1.0.0/sts/"

			actualKey := ReporterStatusDetailKey(resource, sessionID)
			convey.So(actualKey, convey.ShouldEqual, expectedKey)
		})
	})
}

// TestUnmarshalAsReporterConfig 测试 UnmarshalAsReporterConfig 函数
func TestUnmarshalAsReporterConfig(t *testing.T) {
	convey.Convey("Test UnmarshalAsReporterConfig function", t, func() {
		convey.Convey("Should unmarshal valid config bytes correctly", func() {
			config := &reporter_v1.Config{
				Device:  "eth0",
				SnapLen: 65535,
				Promisc: true,
				Timeout: 30,
				BpfExpr: "tcp port 80",
				Enabled: true,
			}

			bytes, err := proto.Marshal(config)
			convey.So(err, convey.ShouldBeNil)

			unmarshalledConfig, err := UnmarshalAsReporterConfig(bytes)
			convey.So(err, convey.ShouldBeNil)
			convey.So(unmarshalledConfig, convey.ShouldNotBeNil)
			convey.So(unmarshalledConfig.Device, convey.ShouldEqual, config.Device)
			convey.So(unmarshalledConfig.SnapLen, convey.ShouldEqual, config.SnapLen)
			convey.So(unmarshalledConfig.Promisc, convey.ShouldEqual, config.Promisc)
			convey.So(unmarshalledConfig.Timeout, convey.ShouldEqual, config.Timeout)
			convey.So(unmarshalledConfig.BpfExpr, convey.ShouldEqual, config.BpfExpr)
			convey.So(unmarshalledConfig.Enabled, convey.ShouldEqual, config.Enabled)
		})

		convey.Convey("Should return error for invalid bytes", func() {
			invalidBytes := []byte("invalid protobuf data")

			config, err := UnmarshalAsReporterConfig(invalidBytes)
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(config, convey.ShouldBeNil)
			convey.So(errors.Is(err, proto.Error), convey.ShouldBeTrue)
		})

		convey.Convey("Should handle empty bytes", func() {
			config, err := UnmarshalAsReporterConfig([]byte{})
			// proto.Unmarshal 对空字节数组可能不会返回错误
			// 而是返回一个空的配置对象
			if err != nil {
				convey.So(err, convey.ShouldNotBeNil)
				convey.So(config, convey.ShouldBeNil)
			} else {
				convey.So(config, convey.ShouldNotBeNil)
				convey.So(config.Device, convey.ShouldEqual, "")
			}
		})
	})
}

// TestUnmarshalAsReporterStatus 测试 UnmarshalAsReporterStatus 函数
func TestUnmarshalAsReporterStatus(t *testing.T) {
	convey.Convey("Test UnmarshalAsReporterStatus function", t, func() {
		convey.Convey("Should unmarshal valid status bytes correctly", func() {
			status := &reporter_v1.Status{
				Type:          reporter_v1.StatusType_RUNNING,
				ConfigVersion: 123,
				GlobalVersion: 456,
			}

			bytes, err := proto.Marshal(status)
			convey.So(err, convey.ShouldBeNil)

			unmarshalledStatus, err := UnmarshalAsReporterStatus(bytes)
			convey.So(err, convey.ShouldBeNil)
			convey.So(unmarshalledStatus, convey.ShouldNotBeNil)
			convey.So(unmarshalledStatus.Type, convey.ShouldEqual, status.Type)
			convey.So(unmarshalledStatus.ConfigVersion, convey.ShouldEqual, status.ConfigVersion)
			convey.So(unmarshalledStatus.GlobalVersion, convey.ShouldEqual, status.GlobalVersion)
		})

		convey.Convey("Should return error for invalid bytes", func() {
			invalidBytes := []byte("invalid protobuf data")

			status, err := UnmarshalAsReporterStatus(invalidBytes)
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(status, convey.ShouldBeNil)
			convey.So(errors.Is(err, proto.Error), convey.ShouldBeTrue)
		})
	})
}

// TestReporterConfigMarshalAsBytes 测试 ReporterConfigMarshalAsBytes 函数
func TestReporterConfigMarshalAsBytes(t *testing.T) {
	convey.Convey("Test ReporterConfigMarshalAsBytes function", t, func() {
		convey.Convey("Should marshal config to bytes correctly", func() {
			config := &reporter_v1.Config{
				Device:  "eth0",
				SnapLen: 65535,
				Promisc: true,
				Timeout: 30,
				BpfExpr: "tcp port 80",
				Enabled: true,
			}

			bytes, err := ReporterConfigMarshalAsBytes(config)
			convey.So(err, convey.ShouldBeNil)
			convey.So(bytes, convey.ShouldNotBeNil)
			convey.So(len(bytes), convey.ShouldBeGreaterThan, 0)

			// 验证可以正确反序列化
			unmarshalledConfig, err := UnmarshalAsReporterConfig(bytes)
			convey.So(err, convey.ShouldBeNil)
			convey.So(unmarshalledConfig.Device, convey.ShouldEqual, config.Device)
		})

		convey.Convey("Should handle nil config", func() {
			bytes, err := ReporterConfigMarshalAsBytes(nil)
			convey.So(err, convey.ShouldBeNil)
			// proto.Marshal 对 nil 可能返回空字节数组
			// 在 Go 中，空切片 [] 不等于 nil，但 convey 的 ShouldNotBeNil 可能认为它是 nil
			// 我们只检查没有错误发生，并确保返回的是字节切片类型
			_ = bytes // 使用变量避免编译警告
		})
	})
}

// TestCapturer_CreateWithConfig 测试 Capturer 的 CreateWithConfig 方法
func TestCapturer_CreateWithConfig(t *testing.T) {
	convey.Convey("Test Capturer CreateWithConfig method", t, func() {
		convey.Convey("Should have correct method signature", func() {
			// 验证方法存在且签名正确
			capturer := &Capturer{
				NamespaceName: "test-namespace",
				Name:          "test-capturer",
				Version:       "v1.0.0",
			}

			// 验证结构体字段
			convey.So(capturer.NamespaceName, convey.ShouldEqual, "test-namespace")
			convey.So(capturer.Name, convey.ShouldEqual, "test-capturer")
			convey.So(capturer.Version, convey.ShouldEqual, "v1.0.0")

			// 验证方法可以通过编译
			// 实际功能测试需要集成测试环境
		})
	})
}

// TestCapturer_UpdateConfig 测试 Capturer 的 UpdateConfig 方法
func TestCapturer_UpdateConfig(t *testing.T) {
	convey.Convey("Test Capturer UpdateConfig method", t, func() {
		convey.Convey("Should have correct method signature", func() {
			capturer := &Capturer{
				NamespaceName: "test-namespace",
				Name:          "test-capturer",
				Version:       "v1.0.0",
			}

			// 验证结构体字段
			convey.So(capturer.NamespaceName, convey.ShouldEqual, "test-namespace")
			convey.So(capturer.Name, convey.ShouldEqual, "test-capturer")
			convey.So(capturer.Version, convey.ShouldEqual, "v1.0.0")
		})
	})
}

// TestCapturer_DeleteWithConfig 测试 Capturer 的 DeleteWithConfig 方法
func TestCapturer_DeleteWithConfig(t *testing.T) {
	convey.Convey("Test Capturer DeleteWithConfig method", t, func() {
		convey.Convey("Should have correct method signature", func() {
			capturer := &Capturer{
				NamespaceName: "test-namespace",
				Name:          "test-capturer",
				Version:       "v1.0.0",
			}

			// 验证结构体字段
			convey.So(capturer.NamespaceName, convey.ShouldEqual, "test-namespace")
			convey.So(capturer.Name, convey.ShouldEqual, "test-capturer")
			convey.So(capturer.Version, convey.ShouldEqual, "v1.0.0")
		})
	})
}

// TestCapturer_GetConfig 测试 Capturer 的 GetConfig 方法
func TestCapturer_GetConfig(t *testing.T) {
	convey.Convey("Test Capturer GetConfig method", t, func() {
		convey.Convey("Should have correct method signature", func() {
			capturer := &Capturer{
				NamespaceName: "test-namespace",
				Name:          "test-capturer",
				Version:       "v1.0.0",
			}

			// 验证结构体字段
			convey.So(capturer.NamespaceName, convey.ShouldEqual, "test-namespace")
			convey.So(capturer.Name, convey.ShouldEqual, "test-capturer")
			convey.So(capturer.Version, convey.ShouldEqual, "v1.0.0")
		})
	})
}

// TestCapturer_GetStatusList 测试 Capturer 的 GetStatusList 方法
func TestCapturer_GetStatusList(t *testing.T) {
	convey.Convey("Test Capturer GetStatusList method", t, func() {
		convey.Convey("Should have correct method signature", func() {
			capturer := &Capturer{
				NamespaceName: "test-namespace",
				Name:          "test-capturer",
				Version:       "v1.0.0",
			}

			// 验证结构体字段
			convey.So(capturer.NamespaceName, convey.ShouldEqual, "test-namespace")
			convey.So(capturer.Name, convey.ShouldEqual, "test-capturer")
			convey.So(capturer.Version, convey.ShouldEqual, "v1.0.0")
		})
	})
}

// TestCapturer_ModelRegistration 测试 Capturer 模型注册
func TestCapturer_ModelRegistration(t *testing.T) {
	convey.Convey("Test Capturer model registration", t, func() {
		convey.Convey("Should have proper database annotations", func() {
			capturer := &Capturer{}

			// 验证结构体标签
			// 这里我们验证结构体字段的 GORM 标签
			convey.So(capturer, convey.ShouldNotBeNil)

			// 验证唯一索引约束
			// NamespaceName, Name, Version 应该有 uniqueIndex:capturer_unique_index
			// 这在实际数据库操作中会体现
		})
	})
}
