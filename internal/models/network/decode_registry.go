package network

import (
	"reflect"
	"sync"

	"github.com/gopacket/gopacket"
	"zhongkuisec.com/zhongkui/internal/pkg/enum"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// DecodeKind 表示层解码器对解析流程的推进方式
// 编排器根据该值决定继续解析下一层、递归解析内层数据包或终止解析；
// 该类型注册为枚举类 decodeKind，取值空间为 continue / recurse / stop
type DecodeKind string

var (
	// layerDecoderMu 保护解码注册表，注册阶段写入，解析阶段读取
	layerDecoderMu sync.RWMutex
	// layerDecoders 协议类型到解码器的映射
	layerDecoders = map[gopacket.LayerType]LayerDecoder{}
	// layerDecoderOrder 解码器注册顺序，供稳定遍历
	layerDecoderOrder []LayerDecoder
)

// 层解码器的解析推进方式
const (
	// DecodeContinue 表示当前层解析完成后继续解析下一层
	DecodeContinue DecodeKind = "continue"
	// DecodeRecurse 表示当前层是封装协议，需要递归解析内层数据包
	DecodeRecurse DecodeKind = "recurse"
	// DecodeStop 表示解析在当前层终止，不再解析后续层
	DecodeStop DecodeKind = "stop"
)

// decodeKindValues 是 decodeKind 枚举类的取值空间，声明顺序即枚举类条目顺序
var decodeKindValues = []DecodeKind{DecodeContinue, DecodeRecurse, DecodeStop}

// DecodeContext 表示解码单层协议时可见的层级上下文
type DecodeContext struct {
	// CurrentLevel 当前层级，供不推进层级的协议（如 IPv6 扩展头）使用
	CurrentLevel LayerLevel
	// NextLevel 下一层级，供常规头部与解码失败层使用
	NextLevel LayerLevel
}

// DecodeResult 表示单层协议的解码结果
// 编排器据此追加层信息、推进层级、递归解析内层数据包或终止解析
type DecodeResult struct {
	// Info 需要进入层堆栈的层信息，nil 表示该层不进入堆栈
	Info *LayerInfo
	// Kind 解析推进方式
	Kind DecodeKind
	// Inner 递归解析的内层数据包，仅 Kind 为 DecodeRecurse 时使用
	Inner gopacket.Packet
	// InnerBase 内层递归解析的起始层级，仅 Kind 为 DecodeRecurse 时使用
	InnerBase LayerLevel
	// Payload 终止解析时返回的载荷数据，仅 Kind 为 DecodeStop 时使用
	Payload []byte
}

// LayerDecoder 表示单个 gopacket 协议层的解码器
// 新增协议只需实现该接口或通过 RegisterTypedDecoder 注册解码函数
type LayerDecoder interface {
	// LayerType 返回解码器负责的协议类型
	LayerType() gopacket.LayerType
	// Decode 解码一层协议并给出解析推进方式
	Decode(DecodeContext, gopacket.Layer) (DecodeResult, error)
}

// typedLayerDecoder 把具体 Layer 类型的解码函数适配为 LayerDecoder
type typedLayerDecoder[L gopacket.Layer] struct {
	// layerType 解码器负责的协议类型，由 L 推导
	layerType gopacket.LayerType
	// decode 具体类型的解码函数
	decode func(DecodeContext, L) (DecodeResult, error)
}

// LayerType 返回解码器负责的协议类型
func (d *typedLayerDecoder[L]) LayerType() gopacket.LayerType {
	return d.layerType
}

// Decode 校验层类型后调用具体类型的解码函数
func (d *typedLayerDecoder[L]) Decode(ctx DecodeContext, layer gopacket.Layer) (DecodeResult, error) {
	typed, ok := layer.(L)
	if !ok {
		return DecodeResult{}, errors.Newf("layer %s decoder received unexpected type %T", d.layerType, layer)
	}
	return d.decode(ctx, typed)
}

// RegisterTypedDecoder 以具体 Layer 类型注册解码器
// 协议类型由 L 的 LayerType 方法推导，注册点无需手写 any 类型断言；
// 同一协议类型重复注册会在启动期 panic
func RegisterTypedDecoder[L gopacket.Layer](decode func(DecodeContext, L) (DecodeResult, error)) {
	registerLayerDecoder(&typedLayerDecoder[L]{
		layerType: deriveLayerType[L](),
		decode:    decode,
	})
}

// deriveLayerType 推导具体 Layer 类型对应的协议类型
// 指针类型先分配非 nil 零值，兼容 LayerType 为值接收者的类型（如 gopacket.Payload）
func deriveLayerType[L gopacket.Layer]() gopacket.LayerType {
	layerType := reflect.TypeOf((*L)(nil)).Elem()
	if layerType.Kind() == reflect.Pointer {
		return reflect.New(layerType.Elem()).Interface().(L).LayerType()
	}
	var zero L
	return zero.LayerType()
}

// registerLayerDecoder 注册解码器，重复注册同一协议类型时 panic
func registerLayerDecoder(decoder LayerDecoder) {
	layerDecoderMu.Lock()
	defer layerDecoderMu.Unlock()
	layerType := decoder.LayerType()
	if _, ok := layerDecoders[layerType]; ok {
		panic(errors.Newf("network: duplicate layer decoder for %s", layerType))
	}
	layerDecoders[layerType] = decoder
	layerDecoderOrder = append(layerDecoderOrder, decoder)
}

// lookupLayerDecoder 按协议类型查找解码器，未注册返回 false
func lookupLayerDecoder(layerType gopacket.LayerType) (LayerDecoder, bool) {
	layerDecoderMu.RLock()
	defer layerDecoderMu.RUnlock()
	decoder, ok := layerDecoders[layerType]
	return decoder, ok
}

// LayerDecoders 返回按注册顺序排列的解码器快照
// 输出顺序稳定，可用于检查当前已注册的协议类型
func LayerDecoders() []LayerDecoder {
	layerDecoderMu.RLock()
	defer layerDecoderMu.RUnlock()
	decoders := make([]LayerDecoder, len(layerDecoderOrder))
	copy(decoders, layerDecoderOrder)
	return decoders
}

// init 注册枚举类 decodeKind（取值空间固定，条目在注册时快照）
func init() {
	enum.Register(enum.Type{Name: "decodeKind", Entries: enum.EntriesOf(decodeKindValues)})
}
