// Command huginn_smoke 是 Huginn Net FFI 集成的一次性冒烟验证程序：
// 构造已知 p0f 签名的 Linux IPv4 SYN 包，调用 AnalyzeTCPIPv4 验证端到端指纹识别。
// 使用方式: go run -tags huginn cmd/huginn_smoke/main.go
package main

import (
	"fmt"

	"zhongkui.com/zhongkui/internal/network/huginn"
)

// buildLinuxSynPacket 构造典型的 Linux 3.1-3.10 IPv4 SYN 数据包：
// TTL=64、MSS=1460、窗口=MSS*10（14600）、WS=6、选项 mss,sok,ts,nop,ws、DF 置位。
// 对应 p0f.fp 签名 `*:64:0:*:mss*10,6:mss,sok,ts,nop,ws:df,id+:0` → Linux 3.1-3.10
func buildLinuxSynPacket() []byte {
	// TCP 头 20 字节 + 选项 20 字节（mss,sok,ts,nop,ws 恰好 4 字节对齐）
	tcp := []byte{
		0x30, 0x39, // src port 12345
		0x00, 0x50, // dst port 80
		0x01, 0x02, 0x03, 0x04, // seq
		0x00, 0x00, 0x00, 0x00, // ack
		0xA0, 0x02, // data offset=10, SYN
		0x39, 0x08, // window 14600
		0x00, 0x00, // checksum（指纹识别不校验）
		0x00, 0x00, // urgent
		// options
		0x02, 0x04, 0x05, 0xB4, // MSS 1460
		0x04, 0x02, // SACK permitted
		0x08, 0x0A, 0x01, 0x02, 0x03, 0x04, 0x00, 0x00, 0x00, 0x00, // timestamp
		0x01,       // NOP
		0x03, 0x03, 0x06, // window scale 6
	}
	// IPv4 头 20 字节 + TCP
	ip := []byte{
		0x45,             // version=4, IHL=5
		0x00,             // DSCP/ECN
		0x00, 0x3C,       // total length 60
		0x12, 0x34,       // identification
		0x40, 0x00,       // DF
		0x40,             // TTL 64
		0x06,             // TCP
		0x00, 0x00,       // checksum
		192, 168, 1, 1,   // src
		10, 0, 0, 1,      // dst
	}
	return append(ip, tcp...)
}

func main() {
	raw := buildLinuxSynPacket()
	fp, err := huginn.AnalyzeTCPIPv4(raw)
	if err != nil {
		fmt.Printf("AnalyzeTCPIPv4 error: %v\n", err)
		return
	}
	if fp == nil {
		fmt.Println("AnalyzeTCPIPv4 returned nil（未匹配到签名）")
		return
	}
	fmt.Printf("os=%q os_quality=%.3f sig=%q mtu=%d\n", fp.OS, fp.OSQuality, fp.Sig, fp.MTU)
}
