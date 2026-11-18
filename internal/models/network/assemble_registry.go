package network

import (
	"reflect"
	"sync"

	"github.com/gopacket/gopacket"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

var (
	// assemblerMu 保护装配注册表，注册阶段写入，构建阶段读取
	assemblerMu sync.RWMutex
	// modelAssemblers 层级与协议类型到装配器的映射
	modelAssemblers = map[assemblerKey]ModelAssembler{}
	// assemblerOrder 装配器注册顺序，供稳定遍历
	assemblerOrder []ModelAssembler
)

// ModelAssembler 表示某一层级协议的持久化模型装配器
// 新增协议只需实现该接口或通过 RegisterTypedAssembler 注册装配函数
type ModelAssembler interface {
	// Level 返回装配器负责的协议层级
	Level() LayerLevel
	// LayerType 返回装配器负责的协议类型
	LayerType() gopacket.LayerType
	// Build 从构建器提取该层头部并装配持久化模型，无法装配时返回 nil
	Build(*ModelBuilder) any
	// ModelPrototypes 返回该装配器声明的持久化模型原型
	ModelPrototypes() []any
}

// typedModelAssembler 把具体 Header 类型的装配函数适配为 ModelAssembler
type typedModelAssembler[H any] struct {
	// level 装配器负责的协议层级
	level LayerLevel
	// layerType 装配器负责的协议类型
	layerType gopacket.LayerType
	// header 从层堆栈提取同层级头部的函数
	header func(LayerStack) (H, bool)
	// build 具体类型的装配函数
	build func(*ModelBuilder, H) any
	// prototypes 模型原型提供函数
	prototypes func() []any
}

// Level 返回装配器负责的协议层级
func (a *typedModelAssembler[H]) Level() LayerLevel {
	return a.level
}

// LayerType 返回装配器负责的协议类型
func (a *typedModelAssembler[H]) LayerType() gopacket.LayerType {
	return a.layerType
}

// Build 提取同层级头部后调用具体装配函数，头部缺失时返回 nil
func (a *typedModelAssembler[H]) Build(builder *ModelBuilder) any {
	header, ok := a.header(builder.Stack)
	if !ok {
		return nil
	}
	return a.build(builder, header)
}

// ModelPrototypes 返回该装配器声明的持久化模型原型
func (a *typedModelAssembler[H]) ModelPrototypes() []any {
	return a.prototypes()
}

// assemblerKey 装配注册表的查找键：协议层级与协议类型
type assemblerKey struct {
	// level 协议层级
	level LayerLevel
	// layerType 协议类型
	layerType gopacket.LayerType
}

// RegisterTypedAssembler 以具体 Header 类型注册模型装配器
// header 负责从层堆栈提取头部，build 在头部提取成功后被调用；
// 同一层级与协议类型重复注册会在启动期 panic
func RegisterTypedAssembler[H any](
	level LayerLevel,
	layerType gopacket.LayerType,
	header func(LayerStack) (H, bool),
	build func(*ModelBuilder, H) any,
	prototypes func() []any,
) {
	registerModelAssembler(&typedModelAssembler[H]{
		level:      level,
		layerType:  layerType,
		header:     header,
		build:      build,
		prototypes: prototypes,
	})
}

// registerModelAssembler 注册装配器，重复注册同一层级与协议类型时 panic
func registerModelAssembler(assembler ModelAssembler) {
	assemblerMu.Lock()
	defer assemblerMu.Unlock()
	key := assemblerKey{level: assembler.Level(), layerType: assembler.LayerType()}
	if _, ok := modelAssemblers[key]; ok {
		panic(errors.Newf("network: duplicate model assembler for level %d protocol %s", key.level, key.layerType))
	}
	modelAssemblers[key] = assembler
	assemblerOrder = append(assemblerOrder, assembler)
}

// lookupModelAssembler 按层级与协议类型查找装配器，未注册返回 false
func lookupModelAssembler(level LayerLevel, layerType gopacket.LayerType) (ModelAssembler, bool) {
	assemblerMu.RLock()
	defer assemblerMu.RUnlock()
	assembler, ok := modelAssemblers[assemblerKey{level: level, layerType: layerType}]
	return assembler, ok
}

// firstHeaderOf 在层堆栈中提取指定层级的首个目标类型头部
// 扫描同层级的所有层，返回首个类型断言成功者；未命中返回 H 的零值与 false
func firstHeaderOf[H any](stack LayerStack, level LayerLevel) (H, bool) {
	for _, info := range stack {
		if info.Level != level {
			continue
		}
		if header, ok := info.Headers.(H); ok {
			return header, true
		}
	}
	var zero H
	return zero, false
}

// lastHeaderOf 在层堆栈中提取指定层级的最后一个目标类型头部
// 用于取传输层头部：传输层模型始终以协议栈最后一层为准
func lastHeaderOf[H any](stack LayerStack, level LayerLevel) (H, bool) {
	for i := len(stack) - 1; i >= 0; i-- {
		info := stack[i]
		if info.Level != level {
			continue
		}
		if header, ok := info.Headers.(H); ok {
			return header, true
		}
	}
	var zero H
	return zero, false
}

// PacketModelPrototypes 汇总所有已注册装配器声明的持久化模型原型
// 输出顺序与装配器注册顺序一致，并按运行时类型去重（如 Ethernet 与 Loopback 共用 PacketEthernet）
func PacketModelPrototypes() []any {
	assemblerMu.RLock()
	defer assemblerMu.RUnlock()
	prototypes := make([]any, 0, len(assemblerOrder))
	seen := make(map[reflect.Type]struct{}, len(assemblerOrder))
	for _, assembler := range assemblerOrder {
		for _, prototype := range assembler.ModelPrototypes() {
			prototypeType := reflect.TypeOf(prototype)
			if prototypeType == nil {
				continue
			}
			if _, ok := seen[prototypeType]; ok {
				continue
			}
			seen[prototypeType] = struct{}{}
			prototypes = append(prototypes, prototype)
		}
	}
	return prototypes
}
