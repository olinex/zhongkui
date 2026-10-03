package bench

import (
	"bytes"
	"os"
	"time"

	"github.com/gopacket/gopacket/pcap"
	"github.com/gopacket/gopacket/pcapgo"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// PcapPacket 单个 pcap 数据包及其捕获时间戳。
type PcapPacket struct {
	// Data 数据包原始字节。
	Data []byte
	// Timestamp 数据包捕获时间戳。
	Timestamp time.Time
}

// LoadPcap 从指定路径的 pcap 文件读取全部数据包。
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

// LoadPcapFromBytes 从内存字节读取全部 pcap 数据包。
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

// LoopbackName 返回系统中第一个环回网卡名称，未找到时返回空字符串。
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

// ReplayResult 回放结果：数据包列表、起止时间与发送统计。
type ReplayResult struct {
	// Packets 参与回放的数据包列表。
	Packets []PcapPacket
	// StartTime 回放开始时间。
	StartTime time.Time
	// EndTime 回放结束时间。
	EndTime time.Time
	// SentCount 实际发送的数据包数量。
	SentCount int
	// TotalBytes 实际发送的总字节数。
	TotalBytes int64
}

// OpenLoopback 在环回网卡上打开用于发包的 pcap handle。
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

// ReplayToLoopback 将数据包回放到环回网卡，topspeed 为 true 时全速发送，否则按原始时间间隔发送。
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

// SlowReplayToLoopback 按固定间隔将数据包慢速回放到环回网卡。
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
