package network

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gopacket/gopacket/layers"
	. "github.com/smartystreets/goconvey/convey"
)

// TestVXLANChannel 测试VXLANChannel相关功能
func TestVXLANChannel(t *testing.T) {
	Convey("Test VXLANChannel functions", t, func() {
		Convey("NewVXLANChannel should create valid VXLANChannel", func() {
			vxlan := &layers.VXLAN{
				ValidIDFlag:      true,
				VNI:              123456,
				GBPExtension:     true,
				GBPDontLearn:     false,
				GBPApplied:       true,
				GBPGroupPolicyID: 1001,
			}

			channel := NewVXLANChannel(vxlan)

			So(channel, ShouldNotBeNil)
			So(channel.ValidIDFlag, ShouldBeTrue)
			So(channel.VNI, ShouldEqual, 123456)
			So(channel.GBPExtension, ShouldBeTrue)
			So(channel.GBPDontLearn, ShouldBeFalse)
			So(channel.GBPApplied, ShouldBeTrue)
			So(channel.GBPGroupPolicyID, ShouldEqual, 1001)
		})

		Convey("NewVXLANChannel should handle default values", func() {
			vxlan := &layers.VXLAN{}

			channel := NewVXLANChannel(vxlan)

			So(channel, ShouldNotBeNil)
			So(channel.ValidIDFlag, ShouldBeFalse)
			So(channel.VNI, ShouldEqual, 0)
			So(channel.GBPExtension, ShouldBeFalse)
			So(channel.GBPDontLearn, ShouldBeFalse)
			So(channel.GBPApplied, ShouldBeFalse)
			So(channel.GBPGroupPolicyID, ShouldEqual, 0)
		})
	})
}

// TestGRERouting 测试GRERouting相关功能
func TestGRERouting(t *testing.T) {
	Convey("Test GRERouting functions", t, func() {
		Convey("NewGRERouting should create valid GRERouting", func() {
			rawRouting := &layers.GRERouting{
				AddressFamily:      2, // IPv4 地址族
				SRELength:          4,
				RoutingInformation: []byte{192, 168, 1, 1, 192, 168, 1, 2},
				Next: &layers.GRERouting{
					AddressFamily:      2,
					SRELength:          4,
					RoutingInformation: []byte{10, 0, 0, 1, 10, 0, 0, 2},
					Next:               nil,
				},
			}

			routing := NewGRERouting(rawRouting)

			So(routing, ShouldNotBeNil)
			So(routing.AddressFamily, ShouldEqual, 2)
			So(routing.SRELength, ShouldEqual, 4)
			So(routing.RoutingInformation, ShouldResemble, []byte{192, 168, 1, 1, 192, 168, 1, 2})
			So(routing.Next, ShouldNotBeNil)
			So(routing.Next.AddressFamily, ShouldEqual, 2)
			So(routing.Next.SRELength, ShouldEqual, 4)
			So(routing.Next.RoutingInformation, ShouldResemble, []byte{10, 0, 0, 1, 10, 0, 0, 2})
			So(routing.Next.Next, ShouldBeNil)
		})

		Convey("NewGRERouting should return nil for nil input", func() {
			routing := NewGRERouting(nil)
			So(routing, ShouldBeNil)
		})

		Convey("GRERouting Value should serialize to JSON", func() {
			routing := &GRERouting{
				AddressFamily:      2,
				SRELength:          4,
				RoutingInformation: []byte{192, 168, 1, 1},
			}

			value, err := routing.Value()
			So(err, ShouldBeNil)

			// Value() 可以返回 []byte 或 string
			var jsonBytes []byte
			switch v := value.(type) {
			case string:
				jsonBytes = []byte(v)
			case []byte:
				jsonBytes = v
			default:
				So(fmt.Sprintf("unexpected type: %T", value), ShouldBeNil)
			}

			var decoded GRERouting
			err = json.Unmarshal(jsonBytes, &decoded)
			So(err, ShouldBeNil)
			So(decoded.AddressFamily, ShouldEqual, 2)
			So(decoded.SRELength, ShouldEqual, 4)
			So(decoded.RoutingInformation, ShouldResemble, []byte{192, 168, 1, 1})
		})

		Convey("GRERouting Scan should deserialize from string", func() {
			jsonData := `{"addressFamily":2,"sreLength":4,"routingInformation":"wKgB","next":null}`
			var routing GRERouting

			err := routing.Scan(jsonData)
			So(err, ShouldBeNil)
			So(routing.AddressFamily, ShouldEqual, 2)
			So(routing.SRELength, ShouldEqual, 4)
		})

		Convey("GRERouting Scan should deserialize from []byte", func() {
			jsonData := []byte(`{"addressFamily":2,"sreLength":4,"routingInformation":"wKgB","next":null}`)
			var routing GRERouting

			err := routing.Scan(jsonData)
			So(err, ShouldBeNil)
			So(routing.AddressFamily, ShouldEqual, 2)
			So(routing.SRELength, ShouldEqual, 4)
		})

		Convey("GRERouting Scan should handle nil value", func() {
			var routing *GRERouting
			err := routing.Scan(nil)
			So(err, ShouldBeNil)
			So(routing, ShouldBeNil)
		})

		Convey("GRERouting Scan should return error for invalid type", func() {
			var routing GRERouting
			err := routing.Scan(123)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "Routing scan failed")
		})

		Convey("GRERouting Scan should handle empty string", func() {
			var routing GRERouting
			err := routing.Scan("")
			So(err, ShouldBeNil)
			// 空值扫描后结构体应为零值
			So(routing.AddressFamily, ShouldEqual, 0)
		})

		Convey("GRERouting Scan should handle empty bytes", func() {
			var routing GRERouting
			err := routing.Scan([]byte{})
			So(err, ShouldBeNil)
			// 空值扫描后结构体应为零值
			So(routing.AddressFamily, ShouldEqual, 0)
		})
	})
}

// TestGREChannel 测试GREChannel相关功能
func TestGREChannel(t *testing.T) {
	Convey("Test GREChannel functions", t, func() {
		Convey("NewGREChannel should create valid GREChannel", func() {
			gre := &layers.GRE{
				ChecksumPresent:   true,
				RoutingPresent:    true,
				KeyPresent:        true,
				SeqPresent:        true,
				StrictSourceRoute: false,
				AckPresent:        true,
				RecursionControl:  0,
				Flags:             0,
				Version:           0,
				Protocol:          layers.EthernetTypeIPv4,
				Checksum:          0x1234,
				Offset:            0x5678,
				Key:               0xABCDEF01,
				Seq:               0x98765432,
				Ack:               0xFEDCBA09,
				GRERouting: &layers.GRERouting{
					AddressFamily:      2,
					SRELength:          4,
					RoutingInformation: []byte{192, 168, 1, 1},
				},
			}

			channel := NewGREChannel(gre)

			So(channel, ShouldNotBeNil)
			So(channel.ChecksumPresent, ShouldBeTrue)
			So(channel.RoutingPresent, ShouldBeTrue)
			So(channel.KeyPresent, ShouldBeTrue)
			So(channel.SeqPresent, ShouldBeTrue)
			So(channel.StrictSourceRoute, ShouldBeFalse)
			So(channel.AckPresent, ShouldBeTrue)
			So(channel.CRecursionControl, ShouldEqual, 0)
			So(channel.Flags, ShouldEqual, 0)
			So(channel.Version, ShouldEqual, 0)
			So(channel.Protocol, ShouldEqual, layers.EthernetTypeIPv4)
			So(channel.Checksum, ShouldEqual, 0x1234)
			So(channel.Offset, ShouldEqual, 0x5678)
			So(channel.Key, ShouldEqual, 0xABCDEF01)
			So(channel.Seq, ShouldEqual, 0x98765432)
			So(channel.Ack, ShouldEqual, 0xFEDCBA09)
			So(channel.Routing, ShouldNotBeNil)
			So(channel.Routing.AddressFamily, ShouldEqual, 2)
		})

		Convey("NewGREChannel should handle nil routing", func() {
			gre := &layers.GRE{
				ChecksumPresent: false,
				RoutingPresent:  false,
				KeyPresent:      false,
				SeqPresent:      false,
				AckPresent:      false,
				Protocol:        layers.EthernetTypeIPv6,
				GRERouting:      nil,
			}

			channel := NewGREChannel(gre)

			So(channel, ShouldNotBeNil)
			So(channel.RoutingPresent, ShouldBeFalse)
			So(channel.Routing, ShouldBeNil)
			So(channel.Protocol, ShouldEqual, layers.EthernetTypeIPv6)
		})
	})
}

// TestGeneveOption 测试GeneveOption相关功能
func TestGeneveOption(t *testing.T) {
	Convey("Test GeneveOption functions", t, func() {
		Convey("NewGeneveOption should create valid GeneveOption", func() {
			geneveOpt := &layers.GeneveOption{
				Class:  0x0102,
				Type:   0x03,
				Flags:  0x04,
				Length: 0x05,
				Data:   []byte{0x06, 0x07, 0x08, 0x09},
			}

			option := NewGeneveOption(geneveOpt)

			So(option, ShouldNotBeNil)
			So(option.Class, ShouldEqual, 0x0102)
			So(option.Type, ShouldEqual, 0x03)
			So(option.Flags, ShouldEqual, 0x04)
			So(option.Length, ShouldEqual, 0x05)
			So(option.Data, ShouldResemble, []byte{0x06, 0x07, 0x08, 0x09})
		})
	})
}

// TestGeneveChannel 测试GeneveChannel相关功能
func TestGeneveChannel(t *testing.T) {
	Convey("Test GeneveChannel functions", t, func() {
		Convey("NewGeneveChannel should create valid GeneveChannel", func() {
			// 创建Geneve选项（注意：gopacket中使用指针切片）
			option1 := &layers.GeneveOption{
				Class:  0x0102,
				Type:   0x03,
				Flags:  0x04,
				Length: 0x05,
				Data:   []byte{0x06, 0x07, 0x08, 0x09},
			}
			option2 := &layers.GeneveOption{
				Class:  0x0A0B,
				Type:   0x0C,
				Flags:  0x0D,
				Length: 0x0E,
				Data:   []byte{0x0F, 0x10, 0x11, 0x12},
			}

			geneve := &layers.Geneve{
				Version:        0,
				OptionsLength:  8,
				OAMPacket:      true,
				CriticalOption: false,
				Protocol:       layers.EthernetTypeIPv4,
				VNI:            123456,
				Options:        []*layers.GeneveOption{option1, option2},
			}

			channel := NewGeneveChannel(geneve)

			So(channel, ShouldNotBeNil)
			So(channel.Version, ShouldEqual, 0)
			So(channel.OptionsLength, ShouldEqual, 8)
			So(channel.OAMPacket, ShouldBeTrue)
			So(channel.CriticalOption, ShouldBeFalse)
			So(channel.Protocol, ShouldEqual, layers.EthernetTypeIPv4)
			So(channel.VNI, ShouldEqual, 123456)
			So(len(channel.Options), ShouldEqual, 2)
			So(channel.Options[0].Class, ShouldEqual, 0x0102)
			So(channel.Options[0].Type, ShouldEqual, 0x03)
			So(channel.Options[1].Class, ShouldEqual, 0x0A0B)
			So(channel.Options[1].Type, ShouldEqual, 0x0C)
		})

		Convey("NewGeneveChannel should handle empty options", func() {
			geneve := &layers.Geneve{
				Version:        0,
				OptionsLength:  0,
				OAMPacket:      false,
				CriticalOption: false,
				Protocol:       layers.EthernetTypeIPv6,
				VNI:            789012,
				Options:        []*layers.GeneveOption{},
			}

			channel := NewGeneveChannel(geneve)

			So(channel, ShouldNotBeNil)
			So(channel.OptionsLength, ShouldEqual, 0)
			So(len(channel.Options), ShouldEqual, 0)
			So(channel.Protocol, ShouldEqual, layers.EthernetTypeIPv6)
			So(channel.VNI, ShouldEqual, 789012)
		})
	})
}

// TestMPLSChannel 测试MPLSChannel相关功能
func TestMPLSChannel(t *testing.T) {
	Convey("Test MPLSChannel functions", t, func() {
		Convey("NewMPLSChannel should create valid MPLSChannel", func() {
			mpls := &layers.MPLS{
				Label:        0x12345, // 20位标签
				TrafficClass: 0x5,     // 3位流量类别
				StackBottom:  true,
				TTL:          64,
			}

			channel := NewMPLSChannel(mpls)

			So(channel, ShouldNotBeNil)
			So(channel.Label, ShouldEqual, 0x12345)
			So(channel.TrafficClass, ShouldEqual, 0x5)
			So(channel.StackBottom, ShouldBeTrue)
			So(channel.TTL, ShouldEqual, 64)
		})

		Convey("NewMPLSChannel should handle middle label (not stack bottom)", func() {
			mpls := &layers.MPLS{
				Label:        0xABCDE,
				TrafficClass: 0x7,
				StackBottom:  false,
				TTL:          128,
			}

			channel := NewMPLSChannel(mpls)

			So(channel, ShouldNotBeNil)
			So(channel.Label, ShouldEqual, 0xABCDE)
			So(channel.TrafficClass, ShouldEqual, 0x7)
			So(channel.StackBottom, ShouldBeFalse)
			So(channel.TTL, ShouldEqual, 128)
		})
	})
}

// TestDatabaseSerialization 测试数据库序列化相关功能
func TestDatabaseSerialization(t *testing.T) {
	Convey("Test database serialization for channel types", t, func() {
		Convey("GRERouting should implement driver.Valuer and sql.Scanner", func() {
			// 测试driver.Valuer接口
			routing := &GRERouting{
				AddressFamily:      2,
				SRELength:          4,
				RoutingInformation: []byte{192, 168, 1, 1},
			}

			var valuer driver.Valuer = routing
			value, err := valuer.Value()
			So(err, ShouldBeNil)
			So(value, ShouldNotBeNil)

			// 测试sql.Scanner接口
			var scanner interface {
				Scan(src any) error
			} = routing

			jsonData, _ := json.Marshal(routing)
			err = scanner.Scan(jsonData)
			So(err, ShouldBeNil)
		})

		Convey("GRERouting Value should handle empty routing information", func() {
			routing := &GRERouting{
				AddressFamily:      2,
				SRELength:          0,
				RoutingInformation: []byte{},
			}

			value, err := routing.Value()
			So(err, ShouldBeNil)

			// Value() 可以返回 []byte 或 string
			var jsonBytes []byte
			switch v := value.(type) {
			case string:
				jsonBytes = []byte(v)
			case []byte:
				jsonBytes = v
			default:
				So(fmt.Sprintf("unexpected type: %T", value), ShouldBeNil)
			}

			var decoded GRERouting
			err = json.Unmarshal(jsonBytes, &decoded)
			So(err, ShouldBeNil)
			So(decoded.SRELength, ShouldEqual, 0)
			So(len(decoded.RoutingInformation), ShouldEqual, 0)
		})

		Convey("GRERouting Scan should handle invalid JSON", func() {
			var routing GRERouting
			err := routing.Scan("{invalid json")
			So(err, ShouldNotBeNil)
		})
	})
}

// BenchmarkChannelCreation 性能测试
func BenchmarkChannelCreation(b *testing.B) {
	// 准备测试数据
	vxlan := &layers.VXLAN{
		ValidIDFlag:      true,
		VNI:              123456,
		GBPExtension:     true,
		GBPDontLearn:     false,
		GBPApplied:       true,
		GBPGroupPolicyID: 1001,
	}

	gre := &layers.GRE{
		ChecksumPresent:   true,
		RoutingPresent:    true,
		KeyPresent:        true,
		SeqPresent:        true,
		StrictSourceRoute: false,
		AckPresent:        true,
		RecursionControl:  0,
		Flags:             0,
		Version:           0,
		Protocol:          layers.EthernetTypeIPv4,
		Checksum:          0x1234,
		Offset:            0x5678,
		Key:               0xABCDEF01,
		Seq:               0x98765432,
		Ack:               0xFEDCBA09,
	}

	geneve := &layers.Geneve{
		Version:        0,
		OptionsLength:  8,
		OAMPacket:      true,
		CriticalOption: false,
		Protocol:       layers.EthernetTypeIPv4,
		VNI:            123456,
		Options: []*layers.GeneveOption{
			{
				Class:  0x0102,
				Type:   0x03,
				Flags:  0x04,
				Length: 0x05,
				Data:   []byte{0x06, 0x07, 0x08, 0x09},
			},
		},
	}

	mpls := &layers.MPLS{
		Label:        0x12345,
		TrafficClass: 0x5,
		StackBottom:  true,
		TTL:          64,
	}

	b.ResetTimer()

	b.Run("VXLANChannel", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			NewVXLANChannel(vxlan)
		}
	})

	b.Run("GREChannel", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			NewGREChannel(gre)
		}
	})

	b.Run("GeneveChannel", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			NewGeneveChannel(geneve)
		}
	})

	b.Run("MPLSChannel", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			NewMPLSChannel(mpls)
		}
	})
}

// BenchmarkGRERoutingSerialization 性能测试
func BenchmarkGRERoutingSerialization(b *testing.B) {
	routing := &GRERouting{
		AddressFamily:      2,
		SRELength:          4,
		RoutingInformation: []byte{192, 168, 1, 1, 192, 168, 1, 2},
		Next: &GRERouting{
			AddressFamily:      2,
			SRELength:          4,
			RoutingInformation: []byte{10, 0, 0, 1, 10, 0, 0, 2},
			Next:               nil,
		},
	}

	b.ResetTimer()

	b.Run("Value", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			routing.Value()
		}
	})

	b.Run("Scan", func(b *testing.B) {
		jsonData, _ := json.Marshal(routing)
		var dest GRERouting

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			dest.Scan(jsonData)
		}
	})
}
