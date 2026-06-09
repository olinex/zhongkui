package bench

import (
	"bytes"
	"os"
	"time"

	"github.com/gopacket/gopacket/pcap"
	"github.com/gopacket/gopacket/pcapgo"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

type PcapPacket struct {
	Data      []byte
	Timestamp time.Time
}

func LoadPcap(path string) ([]PcapPacket, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer f.Close()

	r, err := pcapgo.NewReader(f)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var packets []PcapPacket
	for {
		data, ci, err := r.ReadPacketData()
		if err != nil {
			break
		}
		packets = append(packets, PcapPacket{
			Data:      data,
			Timestamp: ci.Timestamp,
		})
	}
	return packets, nil
}

func LoadPcapFromBytes(data []byte) ([]PcapPacket, error) {
	r, err := pcapgo.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var packets []PcapPacket
	for {
		data, ci, err := r.ReadPacketData()
		if err != nil {
			break
		}
		packets = append(packets, PcapPacket{
			Data:      data,
			Timestamp: ci.Timestamp,
		})
	}
	return packets, nil
}

func LoopbackName() string {
	devs, err := pcap.FindAllDevs()
	if err != nil {
		return ""
	}
	for _, dev := range devs {
		for _, addr := range dev.Addresses {
			if addr.IP.IsLoopback() {
				return dev.Name
			}
		}
	}
	return ""
}

type ReplayResult struct {
	Packets    []PcapPacket
	StartTime  time.Time
	EndTime    time.Time
	SentCount  int
	TotalBytes int64
}

func OpenLoopback() (*pcap.Handle, error) {
	dev := LoopbackName()
	if dev == "" {
		return nil, errors.New("no loopback device found")
	}
	return OpenInterface(dev)
}

// OpenInterface 在指定网卡上打开用于发包的 pcap handle。
func OpenInterface(name string) (*pcap.Handle, error) {
	handle, err := pcap.OpenLive(name, 65536, true, pcap.BlockForever)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return handle, nil
}

func ReplayToLoopback(handle *pcap.Handle, packets []PcapPacket, topspeed bool) (*ReplayResult, error) {
	result := &ReplayResult{
		Packets:   packets,
		StartTime: time.Now(),
	}

	if topspeed {
		for _, pkt := range packets {
			if err := handle.WritePacketData(pkt.Data); err != nil {
				return nil, errors.WithStack(err)
			}
			result.SentCount++
			result.TotalBytes += int64(len(pkt.Data))
		}
	} else {
		var lastTimestamp time.Time
		for i, pkt := range packets {
			if i > 0 {
				delay := pkt.Timestamp.Sub(lastTimestamp)
				if delay > 0 {
					time.Sleep(delay)
				}
			}
			if err := handle.WritePacketData(pkt.Data); err != nil {
				return nil, errors.WithStack(err)
			}
			lastTimestamp = pkt.Timestamp
			result.SentCount++
			result.TotalBytes += int64(len(pkt.Data))
		}
	}

	result.EndTime = time.Now()
	return result, nil
}

func SlowReplayToLoopback(handle *pcap.Handle, packets []PcapPacket, interval time.Duration) (*ReplayResult, error) {
	result := &ReplayResult{
		Packets:   packets,
		StartTime: time.Now(),
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for _, pkt := range packets {
		if err := handle.WritePacketData(pkt.Data); err != nil {
			return nil, errors.WithStack(err)
		}
		result.SentCount++
		result.TotalBytes += int64(len(pkt.Data))
		<-ticker.C
	}

	result.EndTime = time.Now()
	return result, nil
}
