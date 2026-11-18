package network

import (
	"reflect"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

// fakeUnregisteredLayerType 测试用链路层协议类型
// 仅注册到 gopacket（用于解码数据包），不注册到 network 层解码表
const fakeUnregisteredLayerType gopacket.LayerType = 16000

// init 把测试用链路层注册到 gopacket，供未注册层回退用例构造数据包
func init() {
	gopacket.RegisterLayerType(int(fakeUnregisteredLayerType), gopacket.LayerTypeMetadata{
		Name:    "FakeUnregisteredLink",
		Decoder: gopacket.DecodeFunc(decodeFakeUnregisteredLinkLayer),
	})
}

// stubLayerDecoder 注册表测试用的桩解码器
type stubLayerDecoder struct {
	layerType gopacket.LayerType
}

// LayerType 返回桩解码器负责的协议类型
func (d *stubLayerDecoder) LayerType() gopacket.LayerType {
	return d.layerType
}

// Decode 桩实现，直接终止解析
func (d *stubLayerDecoder) Decode(DecodeContext, gopacket.Layer) (DecodeResult, error) {
	return DecodeResult{Kind: DecodeStop}, nil
}

// TestLayerDecoderRegistryLookup 验证内置协议的解码器均已注册并可查找
func TestLayerDecoderRegistryLookup(t *testing.T) {
	layerTypes := []gopacket.LayerType{
		layers.LayerTypeEthernet,
		layers.LayerTypeLoopback,
		layers.LayerTypeIPv4,
		layers.LayerTypeIPv6,
		layers.LayerTypeTCP,
		layers.LayerTypeUDP,
	}
	for _, layerType := range layerTypes {
		decoder, ok := lookupLayerDecoder(layerType)
		if !ok {
			t.Errorf("lookupLayerDecoder(%s) not found", layerType)
			continue
		}
		if decoder == nil {
			t.Errorf("lookupLayerDecoder(%s) returned nil decoder", layerType)
			continue
		}
		if got := decoder.LayerType(); got != layerType {
			t.Errorf("lookupLayerDecoder(%s) returned decoder for %s", layerType, got)
		}
	}
}

// TestLayerDecodersOrderStableAndUnique 验证 LayerDecoders 快照顺序稳定且无重复
func TestLayerDecodersOrderStableAndUnique(t *testing.T) {
	first := LayerDecoders()
	second := LayerDecoders()

	if len(first) != len(second) {
		t.Fatalf("LayerDecoders() length changed between calls: %d vs %d", len(first), len(second))
	}
	seen := make(map[gopacket.LayerType]struct{}, len(first))
	for i := range first {
		firstType := first[i].LayerType()
		if second[i].LayerType() != firstType {
			t.Errorf("LayerDecoders() order not stable at index %d: %s vs %s", i, firstType, second[i].LayerType())
		}
		if _, ok := seen[firstType]; ok {
			t.Errorf("LayerDecoders() contains duplicate decoder for %s", firstType)
			continue
		}
		seen[firstType] = struct{}{}
	}
	if len(seen) != len(first) {
		t.Errorf("LayerDecoders() unique protocol count %d, want %d", len(seen), len(first))
	}
}

// TestLayerDecoderRegistryDuplicatePanics 验证同一协议类型重复注册解码器时 panic
func TestLayerDecoderRegistryDuplicatePanics(t *testing.T) {
	const stubType gopacket.LayerType = 17001
	stub := &stubLayerDecoder{layerType: stubType}

	t.Cleanup(func() {
		layerDecoderMu.Lock()
		defer layerDecoderMu.Unlock()
		delete(layerDecoders, stubType)
		for i, decoder := range layerDecoderOrder {
			if decoder.LayerType() == stubType {
				layerDecoderOrder = append(layerDecoderOrder[:i], layerDecoderOrder[i+1:]...)
				break
			}
		}
	})

	registerLayerDecoder(stub)

	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatalf("expected duplicate layer decoder registration to panic")
		}
	}()
	registerLayerDecoder(stub)
}

// TestExtractLayerInfosUnregisteredLayerFallback 验证未注册到 network 解码表的层走默认回退分支
func TestExtractLayerInfosUnregisteredLayerFallback(t *testing.T) {
	data := []byte{0xab, 0xcd}
	data = append(data, []byte("unregistered payload")...)
	packet := gopacket.NewPacket(data, fakeUnregisteredLayerType, gopacket.Default)

	var (
		level   LayerLevel
		stack   LayerStack
		payload []byte
	)
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Fatalf("ExtractLayerInfos panicked for unregistered layer: %v", recovered)
			}
		}()
		level, stack, payload = ExtractLayerInfos(packet, LayerLevelLink)
	}()

	if level != LayerLevelLink {
		t.Errorf("level = %d, want %d", level, LayerLevelLink)
	}
	if len(stack) != 1 {
		t.Fatalf("stack length = %d, want 1", len(stack))
	}
	if stack[0].Level != LayerLevelLink {
		t.Errorf("stack[0].Level = %d, want %d", stack[0].Level, LayerLevelLink)
	}
	if stack[0].Proto != fakeUnregisteredLayerType {
		t.Errorf("stack[0].Proto = %s, want %s", stack[0].Proto, fakeUnregisteredLayerType)
	}
	if stack[0].Headers != nil {
		t.Errorf("stack[0].Headers = %v, want nil", stack[0].Headers)
	}
	if string(payload) != "unregistered payload" {
		t.Errorf("payload = %q, want %q", payload, "unregistered payload")
	}
}

// TestPacketModelPrototypes 验证已注册模型原型按预期顺序返回且无重复
func TestPacketModelPrototypes(t *testing.T) {
	prototypes := PacketModelPrototypes()
	expected := []any{
		&PacketEthernet{},
		&PacketIPv4{},
		&PacketIPv6{},
		&PacketIPv4TCP{},
		&PacketIPv6TCP{},
		&PacketIPv4UDP{},
		&PacketIPv6UDP{},
	}
	if len(prototypes) != len(expected) {
		t.Fatalf("PacketModelPrototypes() returned %d prototypes, want %d: %v", len(prototypes), len(expected), prototypeTypes(prototypes))
	}
	seen := make(map[reflect.Type]struct{}, len(prototypes))
	for i, prototype := range prototypes {
		gotType := reflect.TypeOf(prototype)
		wantType := reflect.TypeOf(expected[i])
		if gotType != wantType {
			t.Errorf("prototype[%d] = %v, want %v", i, gotType, wantType)
		}
		if _, ok := seen[gotType]; ok {
			t.Errorf("duplicate prototype %v at index %d", gotType, i)
			continue
		}
		seen[gotType] = struct{}{}
	}
}

// prototypeTypes 返回原型运行时类型列表，用于失败信息
func prototypeTypes(prototypes []any) []reflect.Type {
	types := make([]reflect.Type, 0, len(prototypes))
	for _, prototype := range prototypes {
		types = append(types, reflect.TypeOf(prototype))
	}
	return types
}
