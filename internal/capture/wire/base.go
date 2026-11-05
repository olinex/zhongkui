package wire

import (
	"context"
	"io"
	"net"
	"strings"
	"syscall"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
	reporter_v1 "zhongkuisec.com/zhongkui/internal/pkg/protobuf/reporter/v1"
)

// RawPacket 单个原始抓包数据包，携带数据、捕获信息与读取错误。
type RawPacket struct {
	// Data 数据包原始字节。
	Data []byte
	// CaptureInfo 数据包捕获信息（长度、时间戳等）。
	CaptureInfo gopacket.CaptureInfo
	// Err 读取数据包时的错误，nil 表示读取成功。
	Err error
}

// 模仿 gopacket 的 packetsToChannel
func (r *RawPacket) HasNext() bool {
	if r.Err == nil {
		return true
	}

	if nerr, ok := r.Err.(net.Error); ok && nerr.Temporary() {
		return true
	}

	if r.Err == syscall.EAGAIN {
		return true
	}

	// gopacket/afpacket 的 poll 超时错误不实现 net.Error，需要按字符串判断
	if strings.Contains(strings.ToLower(r.Err.Error()), "timeout") {
		return true
	}

	if r.Err == io.EOF ||
		r.Err == io.ErrUnexpectedEOF ||
		r.Err == io.ErrNoProgress ||
		r.Err == io.ErrClosedPipe ||
		r.Err == io.ErrShortBuffer ||
		r.Err == syscall.EBADF ||
		strings.Contains(r.Err.Error(), "use of closed file") {
		return false
	}
	return false
}

// Wire 定义底层抓包设备的统一接口，屏蔽各平台实现差异。
type Wire interface {
	// Name 返回网卡名称。
	Name() string
	// LinkType 返回链路层类型。
	LinkType() layers.LinkType
	// SetBPFFilter 设置 BPF 过滤表达式。
	SetBPFFilter(expr string) error
	// ZeroCopyReadPacketData 零拷贝读取一个数据包。
	ZeroCopyReadPacketData() *RawPacket
	// Close 关闭抓包设备。
	Close()
}

// NewWire 创建平台专用抓包设备，平台实现不可用时回退到 libpcap 实现。
func NewWire(ctx context.Context, config *reporter_v1.Config, logger *logging.Logger) (Wire, error) {
	if config == nil {
		return nil, errors.New("config is nil")
	}
	pWire, err := NewPlatformWire(config)
	if err != nil {
		logger.Ctx(ctx).Errorf(`new platform wire failed: %v`, err)
		return NewLibpcapWire(config)
	}
	return pWire, nil
}
