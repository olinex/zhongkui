package tag

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
)

// TestTagSumming_Struct 测试 TagSumming 结构体的基本属性
func TestTagSumming_Struct(t *testing.T) {
	convey.Convey("测试 TagSumming 结构体", t, func() {
		convey.Convey("应该正确初始化所有字段", func() {
			now := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
			tags := []string{"login_failed", "suspicious_ip"}

			summing := &TagSumming{
				Namespace: "default",
				Minute:    now,
				Model:     "zhongkui.com/zhongkui/internal/models/network.HttpTransport",
				Dimension: "fpTcpClientSig",
				Item:      "s0:120:0:1",
				Cnt:       42,
			}
			summing.Tags = tags

			convey.So(summing.Namespace, convey.ShouldEqual, "default")
			convey.So(summing.Minute, convey.ShouldResemble, now)
			convey.So(summing.Model, convey.ShouldEqual, "zhongkui.com/zhongkui/internal/models/network.HttpTransport")
			convey.So(summing.Dimension, convey.ShouldEqual, "fpTcpClientSig")
			convey.So(summing.Item, convey.ShouldEqual, "s0:120:0:1")
			convey.So(summing.Tags, convey.ShouldResemble, []string{"login_failed", "suspicious_ip"})
			convey.So(summing.Cnt, convey.ShouldEqual, 42)
		})

		convey.Convey("零值结构体应该有合理的默认值", func() {
			summing := &TagSumming{}

			convey.So(summing.Namespace, convey.ShouldBeEmpty)
			convey.So(summing.Minute.IsZero(), convey.ShouldBeTrue)
			convey.So(summing.Model, convey.ShouldBeEmpty)
			convey.So(summing.Dimension, convey.ShouldBeEmpty)
			convey.So(summing.Item, convey.ShouldBeEmpty)
			convey.So(summing.Tags, convey.ShouldBeNil)
			convey.So(summing.Cnt, convey.ShouldEqual, 0)
		})

		convey.Convey("应该支持不同的标签组合", func() {
			testCases := []struct {
				name string
				tags []string
			}{
				{"单个标签", []string{"login_failed"}},
				{"多个标签", []string{"login_failed", "suspicious_ip", "brute_force"}},
				{"空标签列表", []string{}},
			}

			for _, tc := range testCases {
				convey.Convey(tc.name, func() {
					summing := &TagSumming{
						Minute:    time.Now(),
						Model:     "zhongkui.com/zhongkui/internal/models/network.HttpTransport",
						Dimension: "fpTcpServerSig",
						Item:      "s0:120:1:1",
						Cnt:       1,
					}
					summing.Tags = tc.tags
					convey.So(summing.Tags, convey.ShouldResemble, tc.tags)
				})
			}
		})

		convey.Convey("Cnt 字段应该使用 uint 类型", func() {
			summing := &TagSumming{Cnt: 0}
			convey.So(summing.Cnt, convey.ShouldEqual, 0)

			summing.Cnt = 4294967295
			convey.So(summing.Cnt, convey.ShouldEqual, 4294967295)
		})
	})
}

// TestTagSumming_JSONSerialization 测试 TagSumming 的 JSON 序列化和反序列化
func TestTagSumming_JSONSerialization(t *testing.T) {
	convey.Convey("测试 TagSumming JSON 序列化", t, func() {
		convey.Convey("应该正确序列化为 JSON", func() {
			now := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
			summing := &TagSumming{
				Namespace: "default",
				Minute:    now,
				Model:     "zhongkui.com/zhongkui/internal/models/network.HttpTransport",
				Dimension: "fpTcpClientSig",
				Item:      "s0:120:0:1",
				Cnt:       10,
			}
			summing.Tags = []string{"login_failed", "suspicious_ip"}

			data, err := json.Marshal(summing)
			convey.So(err, convey.ShouldBeNil)
			convey.So(string(data), convey.ShouldContainSubstring, `"namespace"`)
			convey.So(string(data), convey.ShouldContainSubstring, `"minute"`)
			convey.So(string(data), convey.ShouldContainSubstring, `"model"`)
			convey.So(string(data), convey.ShouldContainSubstring, `"dimension"`)
			convey.So(string(data), convey.ShouldContainSubstring, `"item"`)
			convey.So(string(data), convey.ShouldContainSubstring, `"tags"`)
			convey.So(string(data), convey.ShouldContainSubstring, `"cnt"`)
		})

		convey.Convey("应该正确从 JSON 反序列化", func() {
			jsonStr := `{
				"namespace": "default",
				"minute": "2024-01-15T10:30:00Z",
				"model": "zhongkui.com/zhongkui/internal/models/network.HttpTransport",
				"dimension": "fpTcpClientSig",
				"item": "s0:120:0:1",
				"tags": ["login_failed", "suspicious_ip"],
				"cnt": 10
			}`

			var summing TagSumming
			err := json.Unmarshal([]byte(jsonStr), &summing)
			convey.So(err, convey.ShouldBeNil)
			convey.So(summing.Namespace, convey.ShouldEqual, "default")
			convey.So(summing.Model, convey.ShouldEqual, "zhongkui.com/zhongkui/internal/models/network.HttpTransport")
			convey.So(summing.Dimension, convey.ShouldEqual, "fpTcpClientSig")
			convey.So(summing.Item, convey.ShouldEqual, "s0:120:0:1")
			convey.So(summing.Tags, convey.ShouldHaveLength, 2)
			convey.So(summing.Tags[0], convey.ShouldEqual, "login_failed")
			convey.So(summing.Tags[1], convey.ShouldEqual, "suspicious_ip")
			convey.So(summing.Cnt, convey.ShouldEqual, 10)
		})

		convey.Convey("空标签数组应该正确序列化", func() {
			summing := &TagSumming{
				Minute:    time.Now(),
				Model:     "zhongkui.com/zhongkui/internal/models/network.HttpTransport",
				Dimension: "fpTcpServerSig",
				Item:      "s0:120:1:1",
				Cnt:       0,
			}
			summing.Tags = []string{}

			data, err := json.Marshal(summing)
			convey.So(err, convey.ShouldBeNil)

			var decoded TagSumming
			err = json.Unmarshal(data, &decoded)
			convey.So(err, convey.ShouldBeNil)
			convey.So(decoded.Tags, convey.ShouldHaveLength, 0)
		})
	})
}

// TestTagSumming_FieldConstraints 测试 TagSumming 的字段约束
func TestTagSumming_FieldConstraints(t *testing.T) {
	convey.Convey("测试 TagSumming 字段约束", t, func() {
		convey.Convey("应该支持不同的命名空间", func() {
			testCases := []struct {
				name      string
				namespace string
			}{
				{"默认命名空间", "default"},
				{"自定义命名空间", "tenant-b"},
				{"空命名空间", ""},
			}

			for _, tc := range testCases {
				convey.Convey(tc.name, func() {
					summing := &TagSumming{Namespace: tc.namespace}
					convey.So(summing.Namespace, convey.ShouldEqual, tc.namespace)
				})
			}
		})

		convey.Convey("应该支持不同的目标模型", func() {
			testCases := []struct {
				name  string
				model string
			}{
				{"HttpTransport", "zhongkui.com/zhongkui/internal/models/network.HttpTransport"},
				{"空模型", ""},
			}

			for _, tc := range testCases {
				convey.Convey(tc.name, func() {
					summing := &TagSumming{Model: tc.model}
					convey.So(summing.Model, convey.ShouldEqual, tc.model)
				})
			}
		})

		convey.Convey("应该支持不同的标签维度类型", func() {
			testCases := []struct {
				name string
				dim  string
			}{
				{"客户端指纹", "fpTcpClientSig"},
				{"服务端指纹", "fpTcpServerSig"},
				{"空维度", ""},
			}

			for _, tc := range testCases {
				convey.Convey(tc.name, func() {
					summing := &TagSumming{Dimension: tc.dim}
					convey.So(summing.Dimension, convey.ShouldEqual, tc.dim)
				})
			}
		})

		convey.Convey("应该支持不同的维度值", func() {
			testCases := []struct {
				name string
				item string
			}{
				{"客户端指纹签名", "s0:120:0:1"},
				{"服务端指纹签名", "s0:120:1:1"},
				{"空值", ""},
			}

			for _, tc := range testCases {
				convey.Convey(tc.name, func() {
					summing := &TagSumming{Item: tc.item}
					convey.So(summing.Item, convey.ShouldEqual, tc.item)
				})
			}
		})

		convey.Convey("Tags 字段应该支持不同长度的数组", func() {
			testCases := []struct {
				name string
				tags []string
			}{
				{"空数组", []string{}},
				{"单个标签", []string{"tag1"}},
				{"多个标签", []string{"tag1", "tag2", "tag3", "tag4", "tag5"}},
			}

			for _, tc := range testCases {
				convey.Convey(tc.name, func() {
					summing := &TagSumming{}
					summing.Tags = tc.tags
					convey.So(summing.Tags, convey.ShouldHaveLength, len(tc.tags))
				})
			}
		})

		convey.Convey("Cnt 字段应该支持累加操作", func() {
			summing := &TagSumming{Cnt: 10}
			summing.Cnt += 5
			convey.So(summing.Cnt, convey.ShouldEqual, 15)
		})

		convey.Convey("Minute 字段应该支持 UTC 时间", func() {
			utcTime := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
			summing := &TagSumming{Minute: utcTime}
			convey.So(summing.Minute.Location().String(), convey.ShouldEqual, "UTC")
		})
	})
}
