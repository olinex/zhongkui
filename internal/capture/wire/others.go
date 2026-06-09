//go:build !linux && !darwin

package wire

import (
	"github.com/gopacket/gopacket/layers"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	reporter_v1 "zhongkui.com/zhongkui/internal/pkg/protobuf/reporter/v1"
)

const (
	PlatformWireName = "others"
)

type PlatformWire struct{}

func NewPlatformWire(config *reporter_v1.Config) (*PlatformWire, error) {
	return nil, errors.New("current platform has no native wire implementation")
}

func (p *PlatformWire) Name() string {
	return PlatformWireName
}

func (p *PlatformWire) LinkType() layers.LinkType {
	return layers.LinkTypeEthernet
}

func (p *PlatformWire) SetBPFFilter(expr string) error {
	return nil
}

func (p *PlatformWire) ZeroCopyReadPacketData() *RawPacket {
	return nil
}

func (p *PlatformWire) Close() {
}
