//go:build darwin

package wire

import (
	reporter_v1 "zhongkui.com/zhongkui/internal/pkg/protobuf/reporter/v1"
)

func NewPlatformWire(config *reporter_v1.Config) (Wire, error) {
	return NewLibpcapWire(config)
}
