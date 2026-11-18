package network

import (
	"github.com/gopacket/gopacket/layers"
)

// Dot1QHeaders 表示 802.1Q / QinQ VLAN 标签头部信息
// VLAN 标签属于链路层子层，不单独落表，随数据包的 layer_stack 一并持久化。
type Dot1QHeaders struct {
	// Priority 优先级（PCP），3 bit
	Priority uint8 `json:"priority"`
	// DropEligible 丢弃资格指示（DEI）
	DropEligible bool `json:"dropEligible"`
	// VLANIdentifier VLAN 标识（VID），12 bit
	VLANIdentifier uint16 `json:"vlanIdentifier"`
	// Type 内层以太网类型
	Type layers.EthernetType `json:"type"`
}

// NewDot1QHeaders 从 gopacket Dot1Q 层创建 Dot1QHeaders
func NewDot1QHeaders(dot1q *layers.Dot1Q) *Dot1QHeaders {
	return &Dot1QHeaders{
		Priority:       dot1q.Priority,
		DropEligible:   dot1q.DropEligible,
		VLANIdentifier: dot1q.VLANIdentifier,
		Type:           dot1q.Type,
	}
}
