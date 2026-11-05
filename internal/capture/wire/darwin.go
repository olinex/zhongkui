//go:build darwin

package wire

import (
	reporter_v1 "zhongkuisec.com/zhongkui/internal/pkg/protobuf/reporter/v1"
)

// NewPlatformWire 在 Darwin 平台创建基于 libpcap 的抓包实例。
func NewPlatformWire(config *reporter_v1.Config) (Wire, error) {
	return NewLibpcapWire(config)
}
