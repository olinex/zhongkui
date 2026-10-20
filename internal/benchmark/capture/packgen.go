package bench

import (
	"encoding/binary"
	"net"
	"os"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// GenerateFallbackPcap 生成包含指定数量 TCP 数据包的兜底 pcap 文件。
func GenerateFallbackPcap(outputPath string, numPackets int) error {
	f, err := os.Create(outputPath)
	if err != nil {
		return errors.WithStack(err)
	}
	defer f.Close()

	writer := pcapgo.NewWriter(f)
	if err := writer.WriteFileHeader(65536, layers.LinkTypeEthernet); err != nil {
		return errors.WithStack(err)
	}

	broadcastMAC := net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}

	opts := gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}

	for i := range numPackets {
		eth := &layers.Ethernet{
			SrcMAC:       broadcastMAC,
			DstMAC:       broadcastMAC,
			EthernetType: layers.EthernetTypeIPv4,
		}

		ip := &layers.IPv4{
			SrcIP:    net.IP{10, 0, 0, 1},
			DstIP:    net.IP{10, 0, 0, 2},
			Protocol: layers.IPProtocolTCP,
		}

		tcp := &layers.TCP{
			SrcPort: 12345,
			DstPort: 80,
			SYN:     true,
		}
		tcp.SetNetworkLayerForChecksum(ip)

		payload := make([]byte, 512)
		binary.BigEndian.PutUint64(payload[:8], uint64(i))
		for j := 8; j < 512; j++ {
			payload[j] = 'A'
		}

		buf := gopacket.NewSerializeBuffer()
		if err := gopacket.SerializeLayers(buf, opts, eth, ip, tcp, gopacket.Payload(payload)); err != nil {
			return errors.WithStack(err)
		}

		ci := gopacket.CaptureInfo{
			Length:        len(buf.Bytes()),
			CaptureLength: len(buf.Bytes()),
		}

		if err := writer.WritePacket(ci, buf.Bytes()); err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}
