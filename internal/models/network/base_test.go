package network

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/gopacket/gopacket/layers"
	. "github.com/smartystreets/goconvey/convey"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

func TestLayerLevel(t *testing.T) {
	Convey("Test LayerLevel methods", t, func() {
		Convey("Next method should return correct next level", func() {
			So(LayerLevelLink.Next(), ShouldEqual, LayerLevelNetwork)
			So(LayerLevelNetwork.Next(), ShouldEqual, LayerLevelTransport)
			So(LayerLevelTransport.Next(), ShouldEqual, LayerLevelApplication)
		})

		Convey("Next method should panic for application layer", func() {
			So(func() { LayerLevelApplication.Next() }, ShouldPanic)
		})
	})
}

func TestLayerStack(t *testing.T) {
	Convey("Test LayerStack database serialization", t, func() {
		Convey("Value method should serialize to JSON", func() {
			stack := LayerStack{
				{
					Level: LayerLevelLink,
					Proto: layers.LayerTypeEthernet,
					Headers: map[string]any{
						"test": "value",
					},
				},
			}

			value, err := stack.Value()
			So(err, ShouldBeNil)

			// 验证序列化结果
			jsonStr, ok := value.(string)
			So(ok, ShouldBeTrue)

			var decoded LayerStack
			err = json.Unmarshal([]byte(jsonStr), &decoded)
			So(err, ShouldBeNil)
			So(len(decoded), ShouldEqual, 1)
			So(decoded[0].Level, ShouldEqual, LayerLevelLink)
		})

		Convey("Value method should handle empty stack", func() {
			var stack LayerStack
			value, err := stack.Value()
			So(err, ShouldBeNil)

			jsonStr, ok := value.(string)
			So(ok, ShouldBeTrue)
			So(jsonStr, ShouldEqual, "[]")
		})

		Convey("Scan method should deserialize from string", func() {
			jsonData := `[{"level":2,"proto":1,"headers":{"test":"value"}}]`
			var stack LayerStack

			err := stack.Scan(jsonData)
			So(err, ShouldBeNil)
			So(len(stack), ShouldEqual, 1)
			So(stack[0].Level, ShouldEqual, LayerLevelLink)
		})

		Convey("Scan method should deserialize from []byte", func() {
			jsonData := []byte(`[{"level":3,"proto":1,"headers":{"test":"value"}}]`)
			var stack LayerStack

			err := stack.Scan(jsonData)
			So(err, ShouldBeNil)
			So(len(stack), ShouldEqual, 1)
			So(stack[0].Level, ShouldEqual, LayerLevelNetwork)
		})

		Convey("Scan method should handle nil value", func() {
			var stack LayerStack
			err := stack.Scan(nil)
			So(err, ShouldBeNil)
			So(stack, ShouldBeNil)
		})

		Convey("Scan method should return error for invalid type", func() {
			var stack LayerStack
			err := stack.Scan(123)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "Layer_stack scan failed")
		})

		Convey("Scan method should handle invalid JSON", func() {
			var stack LayerStack
			err := stack.Scan("{invalid json")
			So(err, ShouldNotBeNil)
		})

		Convey("Scan method should handle empty string", func() {
			var stack LayerStack
			err := stack.Scan("")
			So(err, ShouldBeNil)
			So(stack, ShouldBeNil)
		})

		Convey("Scan method should handle empty bytes", func() {
			var stack LayerStack
			err := stack.Scan([]byte{})
			So(err, ShouldBeNil)
			So(stack, ShouldBeNil)
		})
	})
}

func TestFrameStack(t *testing.T) {
	Convey("Test FrameStack database serialization", t, func() {
		Convey("Value method should serialize to JSON", func() {
			now := time.Now()
			stack := FrameStack{
				{
					Timestamp: now,
					LayerStack: LayerStack{
						{
							Level:   LayerLevelTransport,
							Proto:   layers.LayerTypeTCP,
							Headers: map[string]any{"port": 80},
						},
					},
				},
			}

			value, err := stack.Value()
			So(err, ShouldBeNil)

			// 验证序列化结果
			jsonStr, ok := value.(string)
			So(ok, ShouldBeTrue)

			var decoded FrameStack
			err = json.Unmarshal([]byte(jsonStr), &decoded)
			So(err, ShouldBeNil)
			So(len(decoded), ShouldEqual, 1)
			So(decoded[0].Timestamp.Unix(), ShouldEqual, now.Unix())
		})

		Convey("Scan method should deserialize correctly", func() {
			jsonData := `[{"timestamp":"2024-01-01T12:00:00Z","layer_stack":[{"level":4,"proto":1,"headers":{"port":443}}]}]`
			var stack FrameStack

			err := stack.Scan(jsonData)
			So(err, ShouldBeNil)
			So(len(stack), ShouldEqual, 1)
			So(stack[0].LayerStack[0].Level, ShouldEqual, LayerLevelTransport)
		})

		Convey("Scan method should handle nil value", func() {
			var stack FrameStack
			err := stack.Scan(nil)
			So(err, ShouldBeNil)
			So(stack, ShouldBeNil)
		})

		Convey("Scan method should handle empty string", func() {
			var stack FrameStack
			err := stack.Scan("")
			So(err, ShouldBeNil)
			So(stack, ShouldBeNil)
		})

		Convey("Scan method should handle empty bytes", func() {
			var stack FrameStack
			err := stack.Scan([]byte{})
			So(err, ShouldBeNil)
			So(stack, ShouldBeNil)
		})
	})
}

func TestBaseMeta(t *testing.T) {
	Convey("Test BaseMeta structure", t, func() {
		Convey("Should create valid BaseMeta", func() {
			sessionID, _ := serializer.HexUUIDv4FromString("12345678-1234-1234-1234-123456789012")
			meta := BaseMeta{
				Namespace: "test-namespace",
				Capture:   "test-capture",
				Version:   "v1.0",
				SessionID: sessionID,
			}

			So(meta.Namespace, ShouldEqual, "test-namespace")
			So(meta.Capture, ShouldEqual, "test-capture")
			So(meta.Version, ShouldEqual, "v1.0")
			So(meta.SessionID.String(), ShouldEqual, "12345678-1234-1234-1234-123456789012")
		})
	})
}

func TestTimestampMeta(t *testing.T) {
	Convey("Test TimestampMeta structure", t, func() {
		Convey("Should store timestamp correctly", func() {
			now := time.Now()
			meta := TimestampMeta{Timestamp: now}

			So(meta.Timestamp.Unix(), ShouldEqual, now.Unix())
		})
	})
}

func TestCaptureMeta(t *testing.T) {
	Convey("Test CaptureMeta structure", t, func() {
		Convey("Should store capture information", func() {
			meta := CaptureMeta{
				Truncated:     true,
				CaptureLength: 1500,
			}

			So(meta.Truncated, ShouldBeTrue)
			So(meta.CaptureLength, ShouldEqual, 1500)
		})
	})
}

func TestPacketMeta(t *testing.T) {
	Convey("Test PacketMeta structure", t, func() {
		Convey("Should store layer stack", func() {
			meta := PacketMeta{
				LayerStack: LayerStack{
					{
						Level:   LayerLevelNetwork,
						Proto:   layers.LayerTypeIPv4,
						Headers: map[string]any{"src": "192.168.1.1"},
					},
				},
			}

			So(len(meta.LayerStack), ShouldEqual, 1)
			So(meta.LayerStack[0].Level, ShouldEqual, LayerLevelNetwork)
		})
	})
}

func TestAsPacket(t *testing.T) {
	Convey("Test AsPacket structure", t, func() {
		Convey("Should compose all metadata correctly", func() {
			sessionID, _ := serializer.HexUUIDv4FromString("12345678-1234-1234-1234-123456789012")
			now := time.Now()

			packet := AsPacket{
				BaseMeta: BaseMeta{
					Namespace: "test",
					Capture:   "eth0",
					Version:   "v1",
					SessionID: sessionID,
				},
				TimestampMeta: TimestampMeta{Timestamp: now},
				CaptureMeta: CaptureMeta{
					Truncated:     false,
					CaptureLength: 1514,
				},
				PacketMeta: PacketMeta{
					LayerStack: LayerStack{
						{
							Level:   LayerLevelLink,
							Proto:   layers.LayerTypeEthernet,
							Headers: map[string]any{"dst": "00:11:22:33:44:55"},
						},
					},
				},
			}

			So(packet.Namespace, ShouldEqual, "test")
			So(packet.Timestamp.Unix(), ShouldEqual, now.Unix())
			So(packet.Truncated, ShouldBeFalse)
			So(packet.CaptureLength, ShouldEqual, 1514)
			So(len(packet.LayerStack), ShouldEqual, 1)
		})
	})
}
