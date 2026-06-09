package wire

import (
	"time"

	"github.com/gopacket/gopacket/pcap"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	reporter_v1 "zhongkui.com/zhongkui/internal/pkg/protobuf/reporter/v1"
)

const (
	LibpcapWireName = "libpcap"

	libpcapBufferSize = 32 * 1024 * 1024
)

type LibpcapWire struct {
	*pcap.Handle
}

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

func (l *LibpcapWire) Name() string {
	return LibpcapWireName
}

func (l *LibpcapWire) ZeroCopyReadPacketData() *RawPacket {
	data, ci, err := l.Handle.ZeroCopyReadPacketData()
	return &RawPacket{
		Data:        data,
		CaptureInfo: ci,
		Err:         err,
	}
}
