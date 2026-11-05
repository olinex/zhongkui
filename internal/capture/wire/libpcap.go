package wire

import (
	"time"

	"github.com/gopacket/gopacket/pcap"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	reporter_v1 "zhongkuisec.com/zhongkui/internal/pkg/protobuf/reporter/v1"
)

// libpcap 抓包实现相关常量。
const (
	// LibpcapWireName libpcap 抓包实现的名称标识
	LibpcapWireName = "libpcap"

	// libpcapBufferSize libpcap 抓包缓冲区大小（32MB）
	libpcapBufferSize = 32 * 1024 * 1024
)

// LibpcapWire 基于 libpcap 的抓包实现，封装 pcap.Handle。
type LibpcapWire struct {
	*pcap.Handle
}

// NewLibpcapWire 根据配置创建 libpcap 抓包实例，设置缓冲区、立即模式、快照长度与混杂模式等参数。
func NewLibpcapWire(config *reporter_v1.Config) (Wire, error) {
	if config == nil {
		return nil, errors.New("config is nil")
	}

	inactive, err := pcap.NewInactiveHandle(config.Device)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer inactive.CleanUp()

	if err := inactive.SetBufferSize(libpcapBufferSize); err != nil {
		return nil, errors.WithStack(err)
	}

	if err := inactive.SetImmediateMode(true); err != nil {
		return nil, errors.WithStack(err)
	}

	if err := inactive.SetSnapLen(int(config.SnapLen)); err != nil {
		return nil, errors.WithStack(err)
	}

	if err := inactive.SetPromisc(config.Promisc); err != nil {
		return nil, errors.WithStack(err)
	}
	if err := inactive.SetTimeout(time.Duration(config.Timeout) * time.Millisecond); err != nil {
		return nil, errors.WithStack(err)
	}

	handler, err := inactive.Activate()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &LibpcapWire{
		Handle: handler,
	}, nil
}

// Name 返回抓包实现的名称。
func (l *LibpcapWire) Name() string {
	return LibpcapWireName
}

// ZeroCopyReadPacketData 零拷贝读取一个数据包，返回原始包数据与捕获信息。
func (l *LibpcapWire) ZeroCopyReadPacketData() *RawPacket {
	data, ci, err := l.Handle.ZeroCopyReadPacketData()
	return &RawPacket{
		Data:        data,
		CaptureInfo: ci,
		Err:         err,
	}
}
