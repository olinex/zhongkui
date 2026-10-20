//go:build linux

package wire

import (
	"log"
	"net"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/cilium/ebpf/link"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"golang.org/x/sys/unix"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	reporter_v1 "zhongkuisec.com/zhongkui/internal/pkg/protobuf/reporter/v1"
)

const (
	xdpWireName = "xdp"

	umemFrameSize  = 4096
	umemFrameCount = 4096
	rxRingSize     = 2048
	fillRingSize   = 2048
	compRingSize   = 2048
)

type afxdpRing struct {
	data     []byte
	producer *uint32
	consumer *uint32
	desc     []unix.XDPDesc
	size     int
}

func (r *afxdpRing) enqueue(idx uint32) {
	atomic.StoreUint32(r.producer, idx)
}

func (r *afxdpRing) dequeue() (uint32, bool) {
	prod := atomic.LoadUint32(r.producer)
	cons := atomic.LoadUint32(r.consumer)
	if cons == prod {
		return 0, false
	}
	return cons, true
}

func (r *afxdpRing) release(idx uint32) {
	atomic.StoreUint32(r.consumer, idx)
}

type xdpWire struct {
	snaplen uint32
	device  string
	ifindex int

	xskFd int

	umem     []byte
	umemReg  unix.XDPUmemReg
	freePool []uint32

	fillRing *afxdpRing
	compRing *afxdpRing
	rxRing   *afxdpRing

	objs    *ebpf.Collection
	xdpLink link.Link
	bpfSpec *ebpf.CollectionSpec
}

// NewXdpWire 依据配置创建基于 AF_XDP + XDP 的抓包实现：依次尝试 Native XDP 与
// Generic XDP 两种模式，均失败则返回错误，由调用方降级到其他抓包后端。
func NewXdpWire(config *reporter_v1.Config) (Wire, error) {
	if config.Device == "any" {
		return nil, errors.New("xdp does not support 'any' device")
	}

	iface, err := net.InterfaceByName(config.Device)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	xw := &xdpWire{
		snaplen: config.SnapLen,
		device:  config.Device,
		ifindex: iface.Index,
	}

	if err := xw.setupUmem(); err != nil {
		return nil, errors.Newf("failed to set up UMEM: %v", err)
	}

	if err := xw.setupXdpSocket(); err != nil {
		xw.cleanupRings()
		log.Printf("XDP: AF_XDP socket not available on %s: %v", config.Device, err)
		return nil, errors.Newf("AF_XDP socket setup failed (interface may not support AF_XDP): %v", err)
	}

	xw.bpfSpec = buildDefaultCollectionSpec()

	// 模式 1：AF_XDP + Native XDP（驱动模式，受支持的网卡上为零拷贝）
	log.Printf("XDP: attempting mode 1 — AF_XDP + Native XDP (driver) on %s", config.Device)
	if err := xw.loadAndAttachXdp(link.XDPDriverMode); err == nil {
		log.Printf("XDP: mode 1 succeeded — AF_XDP + Native XDP on %s", config.Device)
		return xw, nil
	} else {
		log.Printf("XDP: mode 1 failed on %s: %v", config.Device, err)
	}

	// 模式 2：AF_XDP + Generic XDP（SKB 模式，基于拷贝）
	log.Printf("XDP: attempting mode 2 — AF_XDP + Generic XDP (SKB) on %s", config.Device)
	if err := xw.loadAndAttachXdp(link.XDPGenericMode); err == nil {
		log.Printf("XDP: mode 2 succeeded — AF_XDP + Generic XDP on %s", config.Device)
		return xw, nil
	} else {
		log.Printf("XDP: mode 2 failed on %s: %v", config.Device, err)
	}

	// 全部 XDP 模式均失败——调用方应降级到 afpacket / libpcap
	xw.Close()
	log.Printf("XDP: all modes exhausted on %s, falling back to afpacket", config.Device)
	return nil, errors.New("XDP: failed to attach eBPF program in both Native and Generic modes")
}

func (x *xdpWire) setupUmem() error {
	umemSize := umemFrameCount * umemFrameSize
	x.umem = make([]byte, umemSize)

	x.umemReg = unix.XDPUmemReg{
		Addr:     uint64(uintptr(unsafe.Pointer(&x.umem[0]))),
		Len:      uint64(umemSize),
		Size:     umemFrameSize,
		Headroom: 0,
		Flags:    0,
	}

	x.freePool = make([]uint32, 0, umemFrameCount)
	for i := uint32(0); i < umemFrameCount; i++ {
		x.freePool = append(x.freePool, i)
	}

	return nil
}

func setsockoptRaw(fd, level, opt int, val unsafe.Pointer, vallen uint32) error {
	_, _, e := unix.Syscall6(
		unix.SYS_SETSOCKOPT,
		uintptr(fd),
		uintptr(level),
		uintptr(opt),
		uintptr(val),
		uintptr(vallen),
		0,
	)
	if e != 0 {
		return errors.WithStack(e)
	}
	return nil
}

func getsockoptRaw(fd, level, opt int, val unsafe.Pointer, vallen *uint32) error {
	_, _, e := unix.Syscall6(
		unix.SYS_GETSOCKOPT,
		uintptr(fd),
		uintptr(level),
		uintptr(opt),
		uintptr(val),
		uintptr(unsafe.Pointer(vallen)),
		0,
	)
	if e != 0 {
		return errors.WithStack(e)
	}
	return nil
}

func (x *xdpWire) setupXdpSocket() error {
	fd, err := unix.Socket(unix.AF_XDP, unix.SOCK_RAW, 0)
	if err != nil {
		return errors.WithStack(err)
	}
	x.xskFd = fd

	if err := setsockoptRaw(fd, unix.SOL_XDP, unix.XDP_UMEM_REG,
		unsafe.Pointer(&x.umemReg), uint32(unsafe.Sizeof(x.umemReg))); err != nil {
		unix.Close(fd)
		return errors.WithStack(err)
	}

	if err := unix.SetsockoptInt(fd, unix.SOL_XDP, unix.XDP_RX_RING, rxRingSize); err != nil {
		unix.Close(fd)
		return errors.WithStack(err)
	}
	if err := unix.SetsockoptInt(fd, unix.SOL_XDP, unix.XDP_UMEM_FILL_RING, fillRingSize); err != nil {
		unix.Close(fd)
		return errors.WithStack(err)
	}
	if err := unix.SetsockoptInt(fd, unix.SOL_XDP, unix.XDP_UMEM_COMPLETION_RING, compRingSize); err != nil {
		unix.Close(fd)
		return errors.WithStack(err)
	}

	var offsets unix.XDPMmapOffsets
	offLen := uint32(unsafe.Sizeof(offsets))
	if err := getsockoptRaw(fd, unix.SOL_XDP, unix.XDP_MMAP_OFFSETS,
		unsafe.Pointer(&offsets), &offLen); err != nil {
		unix.Close(fd)
		return errors.WithStack(err)
	}

	x.fillRing, err = setupAfxdpRing(fillRingSize, offsets.Fr.Desc, fd)
	if err != nil {
		unix.Close(fd)
		return errors.WithStack(err)
	}

	x.compRing, err = setupAfxdpRing(compRingSize, offsets.Cr.Desc, fd)
	if err != nil {
		unix.Munmap(x.fillRing.data)
		unix.Close(fd)
		return errors.WithStack(err)
	}

	x.rxRing, err = setupAfxdpRing(rxRingSize, offsets.Rx.Desc, fd)
	if err != nil {
		unix.Munmap(x.fillRing.data)
		unix.Munmap(x.compRing.data)
		unix.Close(fd)
		return errors.WithStack(err)
	}

	x.fillInitialFrames()

	var options uint32 = 0
	if err := setsockoptRaw(fd, unix.SOL_XDP, unix.XDP_OPTIONS,
		unsafe.Pointer(&options), uint32(unsafe.Sizeof(options))); err != nil {
		unix.Close(fd)
		return errors.WithStack(err)
	}

	sa := unix.SockaddrXDP{
		Ifindex:      uint32(x.ifindex),
		QueueID:      0,
		SharedUmemFD: 0,
	}
	if err := unix.Bind(fd, &sa); err != nil {
		unix.Close(fd)
		return errors.WithStack(err)
	}

	return nil
}

// buildDefaultCollectionSpec 构建默认的 XDP CollectionSpec，
// 将全部数据包重定向到 AF_XDP socket。
func buildDefaultCollectionSpec() *ebpf.CollectionSpec {
	spec := &ebpf.CollectionSpec{
		Maps: map[string]*ebpf.MapSpec{
			"xsk_map": {
				Type:       ebpf.XSKMap,
				KeySize:    4,
				ValueSize:  4,
				MaxEntries: 64,
			},
		},
		Programs: map[string]*ebpf.ProgramSpec{},
	}

	prog := asm.Instructions{
		asm.LoadMapPtr(asm.R1, 0).WithReference("xsk_map"),
		asm.Mov.Imm(asm.R2, 0),
		asm.Mov.Imm(asm.R3, 0),
		asm.FnRedirectMap.Call(),
		// 确保返回值是有效的 XDP action：
		// redirect 成功时返回 R0=4（XDP_REDIRECT），失败时回退到 XDP_PASS
		asm.JGT.Imm(asm.R0, 0, "redirect_success"),
		asm.Mov.Imm(asm.R0, 2), // XDP_PASS
		asm.Return().WithSymbol("redirect_success"),
	}

	spec.Programs["xdp_filter_prog"] = &ebpf.ProgramSpec{
		Type:         ebpf.XDP,
		Instructions: prog,
		License:      "GPL",
	}

	return spec
}

func (x *xdpWire) fillInitialFrames() {
	prod := x.fillRing.producer
	fpSize := uint32(len(x.freePool))
	frSize := uint32(x.fillRing.size)
	if fpSize > frSize {
		fpSize = frSize
	}
	for i := uint32(0); i < fpSize; i++ {
		x.fillRing.desc[*prod] = unix.XDPDesc{Addr: uint64(x.freePool[i]) * umemFrameSize}
		*prod++
	}
	x.fillRing.enqueue(*prod)
	x.freePool = x.freePool[fpSize:]
}

func setupAfxdpRing(size int, descOff uint64, xskFd int) (*afxdpRing, error) {
	descSize := uint64(unsafe.Sizeof(unix.XDPDesc{}))
	ringLen := descSize * uint64(size)
	mmapSize := descOff + ringLen

	data, err := unix.Mmap(xskFd, 0, int(mmapSize), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED|unix.MAP_POPULATE)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	ring := &afxdpRing{
		data: data,
		size: size,
	}
	ring.producer = (*uint32)(unsafe.Pointer(&data[descOff]))
	ring.consumer = (*uint32)(unsafe.Add(unsafe.Pointer(ring.producer), unsafe.Sizeof(uint32(0))))
	descPtr := unsafe.Add(unsafe.Pointer(ring.consumer), unsafe.Sizeof(uint32(0)))
	ring.desc = unsafe.Slice((*unix.XDPDesc)(descPtr), size)

	return ring, nil
}

// loadAndAttachXdp 加载 eBPF 程序，将 AF_XDP socket 的 fd 写入 XSK map，
// 并以给定 flags 将 XDP 程序挂载到网卡。调用方须已创建好 UMEM 与 AF_XDP socket。
//
// 本方法在重新加载前会清理上一次已加载的 eBPF 对象与链接，故可安全地重复调用
// （例如以不同的挂载 flags 重试，或供 SetBPFFilter 重新编程）。
func (x *xdpWire) loadAndAttachXdp(flags link.XDPAttachFlags) error {
	// 清理上一次失败尝试中已加载的对象
	if x.xdpLink != nil {
		x.xdpLink.Close()
		x.xdpLink = nil
	}
	if x.objs != nil {
		x.objs.Close()
		x.objs = nil
	}

	x.objs = &ebpf.Collection{}
	if err := x.bpfSpec.LoadAndAssign(x.objs, &ebpf.CollectionOptions{}); err != nil {
		return errors.WithStack(err)
	}

	xdpProg, ok := x.objs.Programs["xdp_filter_prog"]
	if !ok {
		return errors.New("xdp_filter_prog not found in eBPF object")
	}

	xskMap, ok := x.objs.Maps["xsk_map"]
	if !ok {
		return errors.New("xsk_map not found in eBPF object")
	}

	queueID := uint32(0)
	fd := uint32(x.xskFd)
	if err := xskMap.Put(&queueID, &fd); err != nil {
		return errors.WithStack(err)
	}

	var err error
	x.xdpLink, err = link.AttachXDP(link.XDPOptions{
		Program:   xdpProg,
		Interface: x.ifindex,
		Flags:     flags,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

// Name 返回抓包后端名称。
func (x *xdpWire) Name() string {
	return xdpWireName
}

// LinkType 返回链路层类型（以太网）。
func (x *xdpWire) LinkType() layers.LinkType {
	return layers.LinkTypeEthernet
}

// SetBPFFilter 将过滤表达式编译为 eBPF 程序并重新挂载到网卡，优先使用 Native XDP，
// 失败则回退到 Generic XDP。
func (x *xdpWire) SetBPFFilter(expr string) error {
	spec, err := CompileBPFProgramJIT(expr)
	if err != nil {
		return errors.WithStack(err)
	}

	x.bpfSpec = spec

	// 先尝试 Native XDP，再尝试 Generic
	if err := x.loadAndAttachXdp(link.XDPDriverMode); err == nil {
		return nil
	}
	return x.loadAndAttachXdp(link.XDPGenericMode)
}

// ZeroCopyReadPacketData 从 RX ring 零拷贝读取一个数据包，返回指向 UMEM 的原始数据。
func (x *xdpWire) ZeroCopyReadPacketData() *RawPacket {
	if x.rxRing == nil {
		return &RawPacket{Err: syscall.EBADF}
	}

	idx, ok := x.rxRing.dequeue()
	if !ok {
		x.processCompletions()
		return &RawPacket{Err: syscall.EAGAIN}
	}

	desc := x.rxRing.desc[idx]
	umemOffset := desc.Addr
	pktLen := int(desc.Len)

	if pktLen > int(x.snaplen) {
		pktLen = int(x.snaplen)
	}

	data := x.umem[umemOffset : umemOffset+uint64(pktLen)]

	x.rxRing.release(idx + 1)

	ci := gopacket.CaptureInfo{
		Timestamp:     time.Now(),
		CaptureLength: pktLen,
		Length:        int(desc.Len),
	}

	return &RawPacket{
		Data:        data,
		CaptureInfo: ci,
	}
}

func (x *xdpWire) processCompletions() {
	if x.compRing == nil {
		return
	}

	prod := atomic.LoadUint32(x.compRing.producer)
	cons := atomic.LoadUint32(x.compRing.consumer)
	if cons == prod {
		return
	}

	for cons != prod {
		frameIdx := uint32(x.compRing.desc[cons].Addr / umemFrameSize)
		x.freePool = append(x.freePool, frameIdx)
		cons++
		if cons >= uint32(x.compRing.size) {
			cons = 0
		}
		prod = atomic.LoadUint32(x.compRing.producer)
	}
	x.compRing.release(cons)

	x.refillFillRing()
}

func (x *xdpWire) refillFillRing() {
	if x.fillRing == nil || len(x.freePool) == 0 {
		return
	}

	prod := atomic.LoadUint32(x.fillRing.producer)
	cons := atomic.LoadUint32(x.fillRing.consumer)

	for len(x.freePool) > 0 {
		nextProd := prod + 1
		if nextProd >= uint32(x.fillRing.size) {
			nextProd = 0
		}
		if nextProd == cons {
			break
		}

		frameIdx := x.freePool[len(x.freePool)-1]
		x.freePool = x.freePool[:len(x.freePool)-1]
		x.fillRing.desc[prod] = unix.XDPDesc{
			Addr: uint64(frameIdx) * umemFrameSize,
		}
		prod = nextProd
	}
	x.fillRing.enqueue(prod)
}

// Close 释放 XDP 链路、eBPF 对象、socket 文件描述符及环形缓冲区等资源。
func (x *xdpWire) Close() {
	if x.xdpLink != nil {
		x.xdpLink.Close()
	}
	if x.objs != nil {
		x.objs.Close()
	}
	if x.xskFd != 0 {
		unix.Close(x.xskFd)
	}
	x.cleanupRings()
}

func (x *xdpWire) cleanupRings() {
	if x.fillRing != nil {
		unix.Munmap(x.fillRing.data)
		x.fillRing = nil
	}
	if x.compRing != nil {
		unix.Munmap(x.compRing.data)
		x.compRing = nil
	}
	if x.rxRing != nil {
		unix.Munmap(x.rxRing.data)
		x.rxRing = nil
	}
}
