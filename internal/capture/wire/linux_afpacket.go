//go:build linux

package wire

import (
	"reflect"
	"time"
	"unsafe"

	"github.com/gopacket/gopacket/afpacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcap"
	"golang.org/x/net/bpf"
	"golang.org/x/sys/unix"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	reporter_v1 "zhongkui.com/zhongkui/internal/pkg/protobuf/reporter/v1"
)

const (
	afpacketPlatformWireName = "afpacket"
	// InterfaceBufferMB 单张网卡 AF_PACKET 接收环缓冲区大小（MB），用于计算接收块数量
	InterfaceBufferMB = 64
)

func convertToRawBpfInstructions(pcapInsts []pcap.BPFInstruction) []bpf.RawInstruction {
	bpfInsts := make([]bpf.RawInstruction, len(pcapInsts))
	for i, ins := range pcapInsts {
		bpfInsts[i] = bpf.RawInstruction{
			Op: ins.Code,
			Jt: ins.Jt,
			Jf: ins.Jf,
			K:  ins.K,
		}
	}
	return bpfInsts
}

type afpacketWire struct {
	snaplen uint32
	live    *afpacket.TPacket
}

// NewAfpacketWire 基于 AF_PACKET（TPACKET_V2）创建抓包实现，按配置设置 snaplen、
// 轮询超时、混杂模式与接收环缓冲区，并开启 PACKET_IGNORE_OUTGOING。
func NewAfpacketWire(config *reporter_v1.Config) (Wire, error) {
	snaplen := int(config.SnapLen)
	// TPACKET_V2 要求 frameSize 必须 TPACKET_ALIGN(sizeof(tpacket2_hdr) + snaplen)
	// TPACKET_ALIGNMENT = 16 on x86_64, tpacket2_hdr = 32 字节
	const tpacketAlignment = 16
	const tpacket2HdrLen = 32
	frameSize := ((tpacket2HdrLen + snaplen) + tpacketAlignment - 1) / tpacketAlignment * tpacketAlignment
	blockSize := frameSize * 128
	numBlocks := (InterfaceBufferMB * 1024 * 1024) / blockSize
	if numBlocks == 0 {
		numBlocks = 1
	}

	opts := []any{
		afpacket.OptFrameSize(frameSize),
		afpacket.OptPollTimeout(time.Duration(config.Timeout) * time.Millisecond),
		afpacket.OptAddVLANHeader(true),
		afpacket.OptBlockSize(blockSize),
		afpacket.OptNumBlocks(numBlocks),
		afpacket.OptBlockTimeout(afpacket.DefaultBlockTimeout),
		afpacket.OptTPacketVersion(afpacket.TPacketVersionHighestAvailable),
		afpacket.SocketRaw,
	}
	if config.Device != "any" {
		opts = append(opts, afpacket.OptInterface(config.Device))
	}
	live, err := afpacket.NewTPacket(opts...)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	err = live.SetPromiscuous(config.Promisc)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// 在 loopback 上 AF_PACKET 会同时捕获 TX 和 RX 两个方向的包，导致每发一包被计两次。
	// 设置 PACKET_IGNORE_OUTGOING 只捕获 incoming 方向，与 libpcap 行为一致。
	// PACKET_IGNORE_OUTGOING = 23 (include/uapi/linux/if_packet.h)
	if err := setIgnoreOutgoing(live); err != nil {
		return nil, errors.WithStack(err)
	}

	return &afpacketWire{
		snaplen: config.SnapLen,
		live:    live,
	}, nil
}

// Name 返回抓包实现名。
func (a *afpacketWire) Name() string {
	return afpacketPlatformWireName
}

// LinkType 返回链路层类型，AF_PACKET 统一按以太网处理。
func (a *afpacketWire) LinkType() layers.LinkType {
	return layers.LinkTypeEthernet
}

// SetBPFFilter 编译并下发 BPF 过滤表达式。
func (a *afpacketWire) SetBPFFilter(expr string) error {
	bpf, err := pcap.CompileBPFFilter(a.LinkType(), int(a.snaplen), expr)
	if err != nil {
		return errors.WithStack(err)
	}
	bpfIns := convertToRawBpfInstructions(bpf)
	if err := a.live.SetBPF(bpfIns); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// ZeroCopyReadPacketData 零拷贝读取一个数据包，返回原始数据、捕获信息与错误。
func (a *afpacketWire) ZeroCopyReadPacketData() *RawPacket {
	data, ci, err := a.live.ZeroCopyReadPacketData()
	return &RawPacket{
		Data:        data,
		CaptureInfo: ci,
		Err:         err,
	}
}

// Close 关闭底层 AF_PACKET 抓包句柄。
func (a *afpacketWire) Close() {
	a.live.Close()
}

// setIgnoreOutgoing 设置 AF_PACKET socket 的 PACKET_IGNORE_OUTGOING 选项，
// 避免 loopback 上同时捕获 TX/RX 两个方向的包导致重复计数。
// PACKET_IGNORE_OUTGOING = 23 (include/uapi/linux/if_packet.h)
func setIgnoreOutgoing(tp *afpacket.TPacket) error {
	v := reflect.ValueOf(tp).Elem()
	fdField := v.FieldByName("fd")
	if !fdField.IsValid() {
		return errors.New("cannot access afpacket fd field")
	}
	fd := *(*int)(unsafe.Pointer(fdField.UnsafeAddr()))
	const packetIgnoreOutgoing = 0x17 // 23
	return unix.SetsockoptInt(fd, unix.SOL_PACKET, packetIgnoreOutgoing, 1)
}
