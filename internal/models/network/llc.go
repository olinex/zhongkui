package network

import (
	"encoding/hex"

	"github.com/gopacket/gopacket/layers"
)

// LLCHeaders 表示 IEEE 802.2 LLC 链路层子层头部信息
// LLC 常见于 802.11 数据帧，不单独落表，随数据包的 layer_stack 一并持久化。
type LLCHeaders struct {
	// DSAP 目的服务访问点
	DSAP uint8 `json:"dsap"`
	// IG 组地址指示位
	IG bool `json:"ig"`
	// SSAP 源服务访问点
	SSAP uint8 `json:"ssap"`
	// CR 命令/响应指示位
	CR bool `json:"cr"`
	// Control 控制字段
	Control uint16 `json:"control"`
}

// SNAPHeaders 表示 LLC/SNAP 扩展头部信息
// SNAP 在 LLC 之上承载以太网类型语义，不单独落表，随数据包的 layer_stack 一并持久化。
type SNAPHeaders struct {
	// OrganizationalCode 组织代码，使用十六进制字符串表示
	OrganizationalCode string `json:"organizationalCode"`
	// Type 内层以太网类型
	Type layers.EthernetType `json:"type"`
}

// NewLLCHeaders 从 gopacket LLC 层创建 LLCHeaders
func NewLLCHeaders(llc *layers.LLC) *LLCHeaders {
	return &LLCHeaders{
		DSAP:    llc.DSAP,
		IG:      llc.IG,
		SSAP:    llc.SSAP,
		CR:      llc.CR,
		Control: llc.Control,
	}
}

// NewSNAPHeaders 从 gopacket SNAP 层创建 SNAPHeaders
func NewSNAPHeaders(snap *layers.SNAP) *SNAPHeaders {
	return &SNAPHeaders{
		OrganizationalCode: hex.EncodeToString(snap.OrganizationalCode),
		Type:               snap.Type,
	}
}
