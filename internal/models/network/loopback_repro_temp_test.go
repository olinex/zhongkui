package network

import (
	"fmt"
	"net"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

func TestLoopbackReproTemp(t *testing.T) {
	// 构造 loopback(family=2/IPv4) + IPv4 + TCP + payload
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	loop := &layers.Loopback{Family: layers.ProtocolFamilyIPv4}
	ip := &layers.IPv4{
		Version:  4,
		IHL:      5,
		SrcIP:    net.IPv4(127, 0, 0, 1),
		DstIP:    net.IPv4(127, 0, 0, 1),
		Protocol: layers.IPProtocolTCP,
	}
	tcp := &layers.TCP{
		SrcPort: layers.TCPPort(50000),
		DstPort: layers.TCPPort(80),
		SYN:     true,
		Window:  65535,
	}
	tcp.SetNetworkLayerForChecksum(ip)
	if err := gopacket.SerializeLayers(buf, opts, loop, ip, tcp, gopacket.Payload([]byte("GET / HTTP/1.1\r\n\r\n"))); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	// 直接手动构造 loopback 头 + 上述 payload，避免 checksum 序列化问题
	pkt2 := gopacket.NewPacket(append([]byte{2, 0, 0, 0}, data...), layers.LinkTypeNull, gopacket.Default)
	fmt.Printf("== manual loopback header, LinkTypeNull ==\n")
	for i, l := range pkt2.Layers() {
		fmt.Printf("  layer[%d] type=%v(%d)\n", i, l.LayerType(), uint32(l.LayerType()))
	}
	level, stack, _ := ExtractLayerInfos(pkt2, LayerLevelLink)
	fmt.Printf("  level=%v stack=%d\n", level, len(stack))
	for i, info := range stack {
		fmt.Printf("    [%d] level=%v proto=%v(%d)\n", i, info.Level, info.Proto, uint32(info.Proto))
	}
	t.Log("done")
}
