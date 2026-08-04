package network

import (
	"encoding/json"
	"time"

	"github.com/gopacket/gopacket"
)

type NetworkFrame []*NetworkStack

// NewNetworkFrame 将数据包的网络层堆栈展平为 NetworkStack 记录列表。
// 一个数据包对应多条NetworkStack记录：所有记录共享相同的TopProto，
// 但 Proto、Level、Headers 各不相同，将传输层下的不同网络层数据平铺存储。
//
// 参数:
//   - stack: 数据包的网络层堆栈信息
//   - meta: 数据包的基础元数据（命名空间、捕获器、版本、会话ID等）
//   - timestamp: 数据包的捕获时间戳
//
// 返回值:
//   - NetworkFrame: 展平后的记录列表，Seq、TraceID 与 Direction 由调用方补充设置
func NewNetworkFrame(stack LayerStack, meta BaseMeta, timestamp time.Time) NetworkFrame {
	frame := make(NetworkFrame, 0, len(stack))
	var topProto gopacket.LayerType
	if len(stack) > 0 {
		// 顶层协议类型取协议栈最高层（最后一个）的协议
		topProto = stack[len(stack)-1].Proto
	}
	for _, info := range stack {
		var headers string
		if info.Headers != nil {
			if bytes, err := json.Marshal(info.Headers); err == nil {
				headers = string(bytes)
			}
		}
		frame = append(frame, &NetworkStack{
			BaseMeta: meta,
			TimestampMeta: TimestampMeta{
				Timestamp: timestamp,
				CreatedAt: time.Now(),
			},
			TopProto: topProto,
			Proto:    info.Proto,
			Level:    info.Level,
			Headers:  headers,
		})
	}
	return frame
}

// NetworkStack 表示网络协议栈中的一个帧，包含协议类型、头部信息等。
type NetworkStack struct {
	BaseMeta
	TraceMeta
	TimestampMeta
	DirectionMeta
	TopProto gopacket.LayerType `gorm:"type:UInt64 CODEC(ZSTD(3))" json:"top_proto"` // 顶层协议类型（如Ethernet、IPv4、TCP等）
	Proto    gopacket.LayerType `gorm:"type:UInt64 CODEC(ZSTD(3))" json:"proto"`     // 当前协议类型（如Ethernet、IPv4、TCP等）
	Seq      uint32             `gorm:"type:UInt32 CODEC(ZSTD(3))" json:"seq"`       // 帧序号，保持帧在 FrameStack 中的顺序
	Level    LayerLevel         `gorm:"type:UInt8 CODEC(ZSTD(3))" json:"level"`      // 协议层级（如链路层、网络层等）
	Headers  string             `gorm:"type:String CODEC(ZSTD(3))" json:"headers"`   // 协议头部信息，具体类型由Proto决定
}
