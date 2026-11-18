package network

import (
	"github.com/gopacket/gopacket/layers"
)

// PPPoEHeaders 表示 PPPoE 链路层封装头部信息
// PPPoE 位于以太网与 PPP 之间，不单独落表，随数据包的 layer_stack 一并持久化。
type PPPoEHeaders struct {
	// Version PPPoE 版本号
	Version uint8 `json:"version"`
	// Type PPPoE 类型
	Type uint8 `json:"type"`
	// Code PPPoE 消息码，0 表示会话数据，其余为发现阶段报文
	Code layers.PPPoECode `json:"code"`
	// SessionID PPPoE 会话标识
	SessionID uint16 `json:"sessionID"`
	// Length PPPoE 载荷长度
	Length uint16 `json:"length"`
}

// NewPPPoEHeaders 从 gopacket PPPoE 层创建 PPPoEHeaders
func NewPPPoEHeaders(pppoe *layers.PPPoE) *PPPoEHeaders {
	return &PPPoEHeaders{
		Version:   pppoe.Version,
		Type:      pppoe.Type,
		Code:      pppoe.Code,
		SessionID: pppoe.SessionId,
		Length:    pppoe.Length,
	}
}
