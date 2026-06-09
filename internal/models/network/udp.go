package network

import (
	"github.com/gopacket/gopacket/layers"
)

// UDPHeaders 表示UDP协议头部信息
// UDP（User Datagram Protocol）是一种无连接的传输层协议，提供简单的不可靠数据报服务
// 该结构体封装了UDP协议头部的所有字段，用于存储、查询和分析UDP数据包
//
// 字段说明:
//   - UDPSrcPort:  源端口号，16位无符号整数，标识发送应用程序的端口
//   - UDPDstPort:  目的端口号，16位无符号整数，标识接收应用程序的端口
//   - UDPLength:   UDP数据包总长度，16位无符号整数，包括头部和数据（以字节为单位）
//   - UDPChecksum: 校验和，16位无符号整数，用于错误检测（可选，IPv4中可为0）
//
// 使用场景:
//   - 存储和查询UDP网络数据包
//   - 分析DNS、DHCP、NTP等基于UDP的协议
//   - 监控网络流量和性能分析
type UDPHeaders struct {
	UDPSrcPort  layers.UDPPort `gorm:"type:UInt16 CODEC(ZSTD(3))" json:"src_port"`
	UDPDstPort  layers.UDPPort `gorm:"type:UInt16 CODEC(ZSTD(3))" json:"dst_port"`
	UDPLength   uint16         `gorm:"type:UInt16 CODEC(ZSTD(3))" json:"length"`
	UDPChecksum uint16         `gorm:"type:UInt16 CODEC(ZSTD(3))" json:"checksum"`
}

// NewUDPHeaders 从gopacket UDP层创建UDPHeaders
// 将底层gopacket库解析的UDP头部信息转换为应用层的UDPHeaders结构
//
// 参数:
//   - udp: gopacket库解析的UDP层对象，包含原始UDP头部信息
//
// 返回值:
//   - *UDPHeaders: 新创建的UDPHeaders指针，包含所有UDP头部字段
//
// 转换过程:
//  1. 复制源端口和目的端口
//  2. 复制UDP数据包长度
//  3. 复制校验和字段
func NewUDPHeaders(udp *layers.UDP) *UDPHeaders {
	return &UDPHeaders{
		UDPSrcPort:  udp.SrcPort,  // 源端口
		UDPDstPort:  udp.DstPort,  // 目的端口
		UDPLength:   udp.Length,   // UDP数据包长度
		UDPChecksum: udp.Checksum, // 校验和
	}
}

// PacketIPv4UDP 表示IPv4 UDP数据包
// 组合了PacketIPv4和UDPHeaders，表示完整的IPv4 UDP数据包信息
// 包含以太网层、IPv4层和UDP层的所有头部信息
//
// 使用场景:
//   - 存储和查询IPv4 UDP网络数据包
//   - 分析基于IPv4 UDP的应用程序协议
//   - 网络流量监控和安全分析
type PacketIPv4UDP struct {
	PacketIPv4 // 嵌入IPv4数据包基础结构
	UDPHeaders // 嵌入UDP头部信息
}

// PacketIPv6UDP 表示IPv6 UDP数据包
// 组合了PacketIPv6和UDPHeaders，表示完整的IPv6 UDP数据包信息
// 包含以太网层、IPv6层和UDP层的所有头部信息
//
// 使用场景:
//   - 存储和查询IPv6 UDP网络数据包
//   - 分析基于IPv6 UDP的应用程序协议（如DNS over IPv6）
//   - IPv6网络流量监控和性能分析
type PacketIPv6UDP struct {
	PacketIPv6 // 嵌入IPv6数据包基础结构
	UDPHeaders // 嵌入UDP头部信息
}




