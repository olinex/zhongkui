//go:build linux

package wire

import (
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	reporter_v1 "zhongkuisec.com/zhongkui/internal/pkg/protobuf/reporter/v1"
)

// NewPlatformWire 尝试创建平台原生抓包实现。
// 降级顺序：XDP（AF_XDP + Native → AF_XDP + Generic）→ afpacket。
func NewPlatformWire(config *reporter_v1.Config) (Wire, error) {
	wire, err := NewXdpWire(config)
	if err == nil {
		return wire, nil
	}
	xdpErr := errors.WithStack(err)

	wire, err = NewAfpacketWire(config)
	if err == nil {
		return wire, nil
	}

	return nil, errors.Newf("xdp: %v, afpacket: %v", xdpErr, err)
}
