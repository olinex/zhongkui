//go:build !linux && !darwin

package wire

import (
	"github.com/gopacket/gopacket/layers"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	reporter_v1 "zhongkuisec.com/zhongkui/internal/pkg/protobuf/reporter/v1"
)

// 其他平台（非 Linux / Darwin）的抓包实现相关常量。
const (
	// PlatformWireName 其他平台抓包实现的名称标识
	PlatformWireName = "others"
)

// PlatformWire 其他平台的抓包实现，当前平台不支持原生抓包时使用。
type PlatformWire struct{}

// NewPlatformWire 创建其他平台的抓包实现，当前平台无原生抓包能力，总是返回错误。
func NewPlatformWire(config *reporter_v1.Config) (*PlatformWire, error) {
	return nil, errors.New("current platform has no native wire implementation")
}

// Name 返回抓包实现的名称。
func (p *PlatformWire) Name() string {
	return PlatformWireName
}

// LinkType 返回链路层类型（以太网）。
func (p *PlatformWire) LinkType() layers.LinkType {
	return layers.LinkTypeEthernet
}

// SetBPFFilter 设置 BPF 过滤表达式，当前实现为空操作。
func (p *PlatformWire) SetBPFFilter(expr string) error {
	return nil
}

// ZeroCopyReadPacketData 零拷贝读取数据包，当前实现不支持，返回 nil。
func (p *PlatformWire) ZeroCopyReadPacketData() *RawPacket {
	return nil
}

// Close 关闭抓包实例，当前实现为空操作。
func (p *PlatformWire) Close() {
}
