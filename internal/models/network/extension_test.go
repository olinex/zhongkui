package network

import (
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	reporter_v1 "zhongkuisec.com/zhongkui/internal/pkg/protobuf/reporter/v1"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// fakeLinkLayerType 测试用可扩展链路层协议类型（编号落在 gopacket 非常用区间）
const fakeLinkLayerType gopacket.LayerType = 16001

// init 把测试用可扩展链路层注册到 gopacket，供构造数据包
func init() {
	gopacket.RegisterLayerType(int(fakeLinkLayerType), gopacket.LayerTypeMetadata{
		Name:    "FakeLinkLayer",
		Decoder: gopacket.DecodeFunc(decodeFakeLinkLayer),
	})
}

// fakeLinkLayerBase 测试用链路层的公共字段：前 2 字节为 magic，其余为内层载荷
type fakeLinkLayerBase struct {
	magic    uint16
	contents []byte
	payload  []byte
}

// LayerContents 返回层头部字节
func (l *fakeLinkLayerBase) LayerContents() []byte {
	return l.contents
}

// LayerPayload 返回层载荷
func (l *fakeLinkLayerBase) LayerPayload() []byte {
	return l.payload
}

// fakeLinkLayer 测试用可扩展链路层
type fakeLinkLayer struct {
	fakeLinkLayerBase
}

// LayerType 返回层协议类型（零值即可推导，供 RegisterTypedDecoder 使用）
func (l *fakeLinkLayer) LayerType() gopacket.LayerType {
	return fakeLinkLayerType
}

// fakeUnregisteredLinkLayer 测试用未注册到 network 解码表的链路层
type fakeUnregisteredLinkLayer struct {
	fakeLinkLayerBase
}

// LayerType 返回层协议类型
func (l *fakeUnregisteredLinkLayer) LayerType() gopacket.LayerType {
	return fakeUnregisteredLayerType
}

// decodeFakeLinkBase 解析测试用链路层公共头部
func decodeFakeLinkBase(data []byte) (fakeLinkLayerBase, error) {
	if len(data) < 2 {
		return fakeLinkLayerBase{}, fmt.Errorf("fake link layer: need at least 2 bytes, got %d", len(data))
	}
	return fakeLinkLayerBase{
		magic:    binary.BigEndian.Uint16(data[:2]),
		contents: data[:2],
		payload:  data[2:],
	}, nil
}

// decodeFakeLinkLayer 解码测试用可扩展链路层
func decodeFakeLinkLayer(data []byte, p gopacket.PacketBuilder) error {
	base, err := decodeFakeLinkBase(data)
	if err != nil {
		return err
	}
	p.AddLayer(&fakeLinkLayer{fakeLinkLayerBase: base})
	return p.NextDecoder(gopacket.DecodePayload)
}

// decodeFakeUnregisteredLinkLayer 解码测试用未注册链路层
func decodeFakeUnregisteredLinkLayer(data []byte, p gopacket.PacketBuilder) error {
	base, err := decodeFakeLinkBase(data)
	if err != nil {
		return err
	}
	p.AddLayer(&fakeUnregisteredLinkLayer{fakeLinkLayerBase: base})
	return p.NextDecoder(gopacket.DecodePayload)
}

// fakeLinkHeaders 测试用链路层头部
type fakeLinkHeaders struct {
	Magic uint16
}

// fakePacketModel 测试用链路层持久化模型
type fakePacketModel struct {
	AsPacket
	RowMeta
	fakeLinkHeaders
	Payload []byte
}

// TestExtensibleLinkLayerRegistration 验证新增链路层只需注册解码器与装配器，
// 无需修改 ExtractLayerInfos / BuildAsPacketModel 即可解析并产出模型
func TestExtensibleLinkLayerRegistration(t *testing.T) {
	const payload = "fake link payload"
	data := []byte{0x12, 0x34}
	data = append(data, payload...)

	RegisterTypedDecoder(func(ctx DecodeContext, layer *fakeLinkLayer) (DecodeResult, error) {
		return DecodeResult{
			Info: &LayerInfo{
				Level:   ctx.NextLevel,
				Proto:   layer.LayerType(),
				Headers: &fakeLinkHeaders{Magic: layer.magic},
			},
			Kind: DecodeContinue,
		}, nil
	})
	RegisterTypedAssembler(
		LayerLevelLink,
		fakeLinkLayerType,
		func(stack LayerStack) (*fakeLinkHeaders, bool) {
			return firstHeaderOf[*fakeLinkHeaders](stack, LayerLevelLink)
		},
		func(b *ModelBuilder, headers *fakeLinkHeaders) any {
			return &fakePacketModel{
				AsPacket: AsPacket{
					BaseMeta: BaseMeta{
						SessionID: b.SessionID,
						Namespace: b.Source.Namespace,
						Capture:   b.Source.Name,
						Version:   b.Source.Version,
					},
					TimestampMeta: TimestampMeta{
						Timestamp: b.Timestamp,
						CreatedAt: time.Now(),
					},
					CaptureMeta: CaptureMeta{
						Truncated:     b.Truncated,
						CaptureLength: b.CaptureLength,
					},
					PacketMeta: PacketMeta{
						LayerStack: b.Stack,
					},
				},
				RowMeta:         RowMeta{ID: serializer.NewHexUUIDv4()},
				fakeLinkHeaders: *headers,
				Payload:         append([]byte(nil), b.Payload...),
			}
		},
		func() []any {
			return []any{&fakePacketModel{}}
		},
	)
	t.Cleanup(func() {
		layerDecoderMu.Lock()
		delete(layerDecoders, fakeLinkLayerType)
		for i, decoder := range layerDecoderOrder {
			if decoder.LayerType() == fakeLinkLayerType {
				layerDecoderOrder = append(layerDecoderOrder[:i], layerDecoderOrder[i+1:]...)
				break
			}
		}
		layerDecoderMu.Unlock()

		assemblerMu.Lock()
		delete(modelAssemblers, assemblerKey{level: LayerLevelLink, layerType: fakeLinkLayerType})
		for i, assembler := range assemblerOrder {
			if assembler.Level() == LayerLevelLink && assembler.LayerType() == fakeLinkLayerType {
				assemblerOrder = append(assemblerOrder[:i], assemblerOrder[i+1:]...)
				break
			}
		}
		assemblerMu.Unlock()
	})

	packet := gopacket.NewPacket(data, fakeLinkLayerType, gopacket.Default)
	sessionID := serializer.NewHexUUIDv4()
	source := reporter_v1.ResourceRaw{
		Namespace: "fake-namespace",
		Name:      "fake-capturer",
		Version:   "v1.0.0",
	}

	builder := NewPacketModelBuilderWithLinkType(packet, sessionID, source, layers.LinkTypeEthernet)
	if builder.Level != LayerLevelLink {
		t.Fatalf("builder.Level = %d, want %d", builder.Level, LayerLevelLink)
	}
	if len(builder.Stack) != 1 {
		t.Fatalf("stack length = %d, want 1", len(builder.Stack))
	}
	info := builder.Stack[0]
	if info.Level != LayerLevelLink {
		t.Errorf("stack[0].Level = %d, want %d", info.Level, LayerLevelLink)
	}
	if info.Proto != fakeLinkLayerType {
		t.Errorf("stack[0].Proto = %s, want %s", info.Proto, fakeLinkLayerType)
	}
	headers, ok := info.Headers.(*fakeLinkHeaders)
	if !ok {
		t.Fatalf("stack[0].Headers = %T, want *fakeLinkHeaders", info.Headers)
	}
	if headers.Magic != 0x1234 {
		t.Errorf("headers.Magic = %#x, want %#x", headers.Magic, 0x1234)
	}
	if string(builder.Payload) != payload {
		t.Errorf("builder.Payload = %q, want %q", builder.Payload, payload)
	}

	model := builder.BuildAsPacketModel()
	fake, ok := model.(*fakePacketModel)
	if !ok {
		t.Fatalf("BuildAsPacketModel() = %T, want *fakePacketModel", model)
	}
	if fake.Magic != 0x1234 {
		t.Errorf("model.Magic = %#x, want %#x", fake.Magic, 0x1234)
	}
	if string(fake.Payload) != payload {
		t.Errorf("model.Payload = %q, want %q", fake.Payload, payload)
	}
	if fake.Namespace != source.Namespace || fake.Capture != source.Name || fake.Version != source.Version {
		t.Errorf("model metadata = %+v, want namespace=%s capture=%s version=%s", fake.BaseMeta, source.Namespace, source.Name, source.Version)
	}
	if fake.SessionID != sessionID {
		t.Errorf("model.SessionID = %v, want %v", fake.SessionID, sessionID)
	}
}
