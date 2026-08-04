package network

import "github.com/gopacket/gopacket"

type NetworkFrame []*NetworkStack

type NetworkStack struct {
	BaseMeta
	TraceMeta
	TimestampMeta
	DirectionMeta
	ParentProto gopacket.LayerType `gorm:"type:UInt64 CODEC(ZSTD(3))" json:"parent_proto"` // 顶层协议类型（如Ethernet、IPv4、TCP等）
	ChildProto  gopacket.LayerType `gorm:"type:UInt64 CODEC(ZSTD(3))" json:"child_proto"`  // 当前协议类型（如Ethernet、IPv4、TCP等）
	Seq         uint32             `gorm:"type:UInt32 CODEC(ZSTD(3))" json:"seq"`          // 帧序号，保持帧在 FrameStack 中的顺序
	Level       LayerLevel         `gorm:"type:UInt8 CODEC(ZSTD(3))" json:"level"`         // 协议层级（如链路层、网络层等）
	Headers     string             `gorm:"type:String CODEC(ZSTD(3))" json:"headers"`      // 协议头部信息，具体类型由Proto决定
}
