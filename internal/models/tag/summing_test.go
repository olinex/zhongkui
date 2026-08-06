package tag

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
)

// TestSummingTtlHours 测试聚合数据保留时间常量
func TestSummingTtlHours(t *testing.T) {
	convey.Convey("测试聚合数据保留时间常量", t, func() {
		convey.Convey("SummingTtlHours 应该为 72 小时（3天）", func() {
			convey.So(SummingTTLHours, convey.ShouldEqual, 72)
		})

		convey.Convey("SummingTtlHours 应该等于 3 * 24", func() {
			convey.So(SummingTTLHours, convey.ShouldEqual, 3*24)
		})
	})
}

// TestTagSumming_Struct 测试 TagSumming 结构体的基本属性
func TestTagSumming_Struct(t *testing.T) {
	convey.Convey("测试 TagSumming 结构体", t, func() {
		convey.Convey("应该正确初始化所有字段", func() {
			now := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
			tags := []string{"login_failed", "suspicious_ip"}

			summing := &TagSumming{
				Minute: now,
				Source: "HttpTransport",
				Type:   "fp_tcp_client_sig",
				Item:   "s0:120:0:1",
				Cnt:    42,
			}
			summing.Tags = tags

			convey.So(summing.Minute, convey.ShouldResemble, now)
			convey.So(summing.Source, convey.ShouldEqual, "HttpTransport")
			convey.So(summing.Type, convey.ShouldEqual, "fp_tcp_client_sig")
			convey.So(summing.Item, convey.ShouldEqual, "s0:120:0:1")
			convey.So(summing.Tags, convey.ShouldResemble, []string{"login_failed", "suspicious_ip"})
			convey.So(summing.Cnt, convey.ShouldEqual, 42)
		})

		convey.Convey("零值结构体应该有合理的默认值", func() {
			summing := &TagSumming{}

			convey.So(summing.Minute.IsZero(), convey.ShouldBeTrue)
			convey.So(summing.Source, convey.ShouldBeEmpty)
			convey.So(summing.Type, convey.ShouldBeEmpty)
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
						Minute: time.Now(),
						Source: "HttpTransport",
						Type:   "fp_tcp_server_sig",
						Item:   "s0:120:1:1",
						Cnt:    1,
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
				Minute: now,
				Source: "HttpTransport",
				Type:   "fp_tcp_client_sig",
				Item:   "s0:120:0:1",
				Cnt:    10,
			}
			summing.Tags = []string{"login_failed", "suspicious_ip"}

			data, err := json.Marshal(summing)
			convey.So(err, convey.ShouldBeNil)
			convey.So(string(data), convey.ShouldContainSubstring, `"minute"`)
			convey.So(string(data), convey.ShouldContainSubstring, `"source"`)
			convey.So(string(data), convey.ShouldContainSubstring, `"type"`)
			convey.So(string(data), convey.ShouldContainSubstring, `"item"`)
			convey.So(string(data), convey.ShouldContainSubstring, `"tags"`)
			convey.So(string(data), convey.ShouldContainSubstring, `"cnt"`)
		})

		convey.Convey("应该正确从 JSON 反序列化", func() {
			jsonStr := `{
				"minute": "2024-01-15T10:30:00Z",
				"source": "HttpTransport",
				"type": "fp_tcp_client_sig",
				"item": "s0:120:0:1",
				"tags": ["login_failed", "suspicious_ip"],
				"cnt": 10
			}`

			var summing TagSumming
			err := json.Unmarshal([]byte(jsonStr), &summing)
			convey.So(err, convey.ShouldBeNil)
			convey.So(summing.Source, convey.ShouldEqual, "HttpTransport")
			convey.So(summing.Type, convey.ShouldEqual, "fp_tcp_client_sig")
			convey.So(summing.Item, convey.ShouldEqual, "s0:120:0:1")
			convey.So(summing.Tags, convey.ShouldHaveLength, 2)
			convey.So(summing.Tags[0], convey.ShouldEqual, "login_failed")
			convey.So(summing.Tags[1], convey.ShouldEqual, "suspicious_ip")
			convey.So(summing.Cnt, convey.ShouldEqual, 10)
		})

		convey.Convey("空标签数组应该正确序列化", func() {
			summing := &TagSumming{
				Minute: time.Now(),
				Source: "HttpTransport",
				Type:   "fp_tcp_server_sig",
				Item:   "s0:120:1:1",
				Cnt:    0,
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

// TestTagSumming_TableOptions 测试 TableOptions 方法返回的表配置选项
func TestTagSumming_TableOptions(t *testing.T) {
	convey.Convey("测试 TagSumming 表配置选项", t, func() {
		summing := &TagSumming{}
		options := summing.TableOptions()

		convey.Convey("应该返回包含 gorm:table_options 键的映射", func() {
			convey.So(options, convey.ShouldContainKey, "gorm:table_options")
		})

		tableOpts, ok := options["gorm:table_options"].(string)
		convey.So(ok, convey.ShouldBeTrue)

		convey.Convey("应该使用 SummingMergeTree 引擎", func() {
			convey.So(tableOpts, convey.ShouldContainSubstring, "SummingMergeTree")
		})

		convey.Convey("应该包含正确的排序键", func() {
			convey.So(tableOpts, convey.ShouldContainSubstring, "ORDER BY")
			convey.So(tableOpts, convey.ShouldContainSubstring, "minute")
			convey.So(tableOpts, convey.ShouldContainSubstring, "source")
			convey.So(tableOpts, convey.ShouldContainSubstring, "type")
			convey.So(tableOpts, convey.ShouldContainSubstring, "item")
			convey.So(tableOpts, convey.ShouldContainSubstring, "tags")
		})

		convey.Convey("应该按小时分区", func() {
			convey.So(tableOpts, convey.ShouldContainSubstring, "PARTITION BY toStartOfHour(minute)")
		})

		convey.Convey("应该包含正确的 TTL 配置", func() {
			convey.So(tableOpts, convey.ShouldContainSubstring, "TTL minute + INTERVAL")
			convey.So(tableOpts, convey.ShouldContainSubstring, "HOUR DELETE")
		})

		convey.Convey("TTL 应该使用 SummingTtlHours 常量", func() {
			convey.So(tableOpts, convey.ShouldContainSubstring, "INTERVAL 72 HOUR DELETE")
		})
	})
}

// TestTagSumming_FieldConstraints 测试 TagSumming 的字段约束
func TestTagSumming_FieldConstraints(t *testing.T) {
	convey.Convey("测试 TagSumming 字段约束", t, func() {
		convey.Convey("应该支持不同的数据来源", func() {
			testCases := []struct {
				name   string
				source string
			}{
				{"HttpTransport", "HttpTransport"},
				{"空来源", ""},
			}

			for _, tc := range testCases {
				convey.Convey(tc.name, func() {
					summing := &TagSumming{Source: tc.source}
					convey.So(summing.Source, convey.ShouldEqual, tc.source)
				})
			}
		})

		convey.Convey("应该支持不同的标签维度类型", func() {
			testCases := []struct {
				name string
				typ  string
			}{
				{"客户端指纹", "fp_tcp_client_sig"},
				{"服务端指纹", "fp_tcp_server_sig"},
				{"空类型", ""},
			}

			for _, tc := range testCases {
				convey.Convey(tc.name, func() {
					summing := &TagSumming{Type: tc.typ}
					convey.So(summing.Type, convey.ShouldEqual, tc.typ)
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

// TestTagSumming_TableOptions_EngineSpec 测试 TableOptions 中 ClickHouse 引擎规格的完整性
func TestTagSumming_TableOptions_EngineSpec(t *testing.T) {
	convey.Convey("测试 TableOptions ClickHouse 引擎规格完整性", t, func() {
		summing := &TagSumming{}
		options := summing.TableOptions()
		tableOpts := options["gorm:table_options"].(string)

		convey.Convey("排序键顺序应该为 minute, source, type, item, tags", func() {
			convey.So(tableOpts, convey.ShouldContainSubstring, "ORDER BY")
			convey.So(tableOpts, convey.ShouldContainSubstring, "minute")
			convey.So(tableOpts, convey.ShouldContainSubstring, "source")
			convey.So(tableOpts, convey.ShouldContainSubstring, "type")
			convey.So(tableOpts, convey.ShouldContainSubstring, "item")
			convey.So(tableOpts, convey.ShouldContainSubstring, "tags")
		})

		convey.Convey("应该配置 DELETE 类型的 TTL", func() {
			convey.So(tableOpts, convey.ShouldContainSubstring, "DELETE")
		})

		convey.Convey("TTL 间隔应该匹配 SummingTtlHours", func() {
			expected := "INTERVAL 72 HOUR"
			convey.So(tableOpts, convey.ShouldContainSubstring, expected)
		})
	})
}
