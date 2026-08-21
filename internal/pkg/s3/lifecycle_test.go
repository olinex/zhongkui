package s3

import (
	"encoding/xml"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

// TestLifecycleConfigToMinioConfig 测试规则清单转换为 minio 生命周期配置
func TestLifecycleConfigToMinioConfig(t *testing.T) {
	Convey("TestLifecycleConfigToMinioConfig", t, func() {
		Convey("合法规则完整转换", func() {
			cfg := &LifecycleConfig{Rules: []LifecycleRuleConfig{
				{ID: "pcap-expire", Status: "Enabled", Prefix: "pcap/", ExpireDays: 1},
				{ID: "body-keep", Prefix: "body/"}, // Status 缺省视为 Enabled
			}}
			config, err := cfg.ToMinioConfig()
			So(err, ShouldBeNil)
			So(len(config.Rules), ShouldEqual, 2)
			So(config.Rules[0].ID, ShouldEqual, "pcap-expire")
			So(config.Rules[0].Status, ShouldEqual, "Enabled")
			So(config.Rules[0].RuleFilter.Prefix, ShouldEqual, "pcap/")
			So(int(config.Rules[0].Expiration.Days), ShouldEqual, 1)
			So(config.Rules[1].Status, ShouldEqual, "Enabled")
			So(config.Rules[1].RuleFilter.Prefix, ShouldEqual, "body/")
			So(int(config.Rules[1].Expiration.Days), ShouldEqual, 0)

			// 回归防护：前缀必须通过 Filter.Prefix 序列化，不得同时输出顶层 <Prefix>
			xmlStr, err := xml.Marshal(config)
			So(err, ShouldBeNil)
			So(strings.Count(string(xmlStr), "<Prefix>"), ShouldEqual, 2)
			So(string(xmlStr), ShouldContainSubstring, "<Filter><Prefix>pcap/</Prefix></Filter>")
		})

		Convey("缺失 ID 时拒绝整份配置", func() {
			cfg := &LifecycleConfig{Rules: []LifecycleRuleConfig{
				{Prefix: "pcap/", ExpireDays: 1},
			}}
			_, err := cfg.ToMinioConfig()
			So(err, ShouldNotBeNil)
		})

		Convey("非法 Status 时拒绝整份配置", func() {
			cfg := &LifecycleConfig{Rules: []LifecycleRuleConfig{
				{ID: "bad-status", Status: "Purge", Prefix: "pcap/"},
			}}
			_, err := cfg.ToMinioConfig()
			So(err, ShouldNotBeNil)
		})
	})
}

// TestLifecyclePrefixes 测试路径前缀枚举与上传路径常量一致，且不重复
func TestLifecyclePrefixes(t *testing.T) {
	Convey("TestLifecyclePrefixes", t, func() {
		Convey("枚举包含全部内置前缀且无重复", func() {
			seen := make(map[string]bool)
			for _, p := range LifecyclePrefixes {
				So(p, ShouldNotBeBlank)
				So(seen[p], ShouldBeFalse)
				seen[p] = true
			}
			So(seen[BodyRequestPrefix], ShouldBeTrue)
			So(seen[BodyResponsePrefix], ShouldBeTrue)
			So(seen[PcapPrefix], ShouldBeTrue)
		})
	})
}
