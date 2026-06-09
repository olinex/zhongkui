package wire

import (
	"io"
	"net"
	"strings"
	"syscall"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	reporter_v1 "zhongkui.com/zhongkui/internal/pkg/protobuf/reporter/v1"
)

type RawPacket struct {
	Data        []byte
	CaptureInfo gopacket.CaptureInfo
	Err         error
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

type Wire interface {
	Name() string
	LinkType() layers.LinkType
	SetBPFFilter(expr string) error
	ZeroCopyReadPacketData() *RawPacket
	Close()
}

func NewWire(config *reporter_v1.Config, logger *logging.Logger) (Wire, error) {
	if config == nil {
		return nil, errors.New("config is nil")
	}
	pWire, err := NewPlatformWire(config)
	if err != nil {
		logger.Warnf(`new platform wire failed: %v`, err)
		return NewLibpcapWire(config)
	}
	return pWire, nil
}
