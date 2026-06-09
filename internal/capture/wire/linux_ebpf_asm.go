//go:build linux

package wire

import (
	"fmt"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/gopacket/gopacket/pcap"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

const (
	cBPFClassLD   = 0x00
	cBPFClassLDX  = 0x01
	cBPFClassST   = 0x02
	cBPFClassSTX  = 0x03
	cBPFClassALU  = 0x04
	cBPFClassJMP  = 0x05
	cBPFClassRET  = 0x06
	cBPFClassMISC = 0x07
)

const (
	cBPFSizeW = 0x00
	cBPFSizeH = 0x08
	cBPFSizeB = 0x10
)

const (
	cBPFModeIMM = 0x00
	cBPFModeABS = 0x20
	cBPFModeIND = 0x40
	cBPFModeMEM = 0x60
	cBPFModeLEN = 0x80
	cBPFModeMSH = 0xa0
)

const (
	cBPFSrcK = 0x00
	cBPFSrcX = 0x08
)

const (
	cBPFALUAdd = 0x00
	cBPFALUSub = 0x10
	cBPFALUMul = 0x20
	cBPFALUDiv = 0x30
	cBPFALUOr  = 0x40
	cBPFALUAnd = 0x50
	cBPFALULSh = 0x60
	cBPFALURSh = 0x70
	cBPFALUNeg = 0x80
	cBPFALUMod = 0x90
	cBPFALUXor = 0xa0
)

const (
	cBPFJMPJA  = 0x00
	cBPFJMPJEQ = 0x10
	cBPFJMPJGT = 0x20
	cBPFJMPJGE = 0x30
	cBPFJMPSET = 0x40
)

const (
	cBPFMISCTAX = 0x00
	cBPFMISCTXA = 0x80
)

const (
	// eBPF register mapping
	ebpfA = asm.R0 // cBPF accumulator
	ebpfX = asm.R8 // cBPF index register
	ebpfL = asm.R9 // precomputed packet length for BPF_LEN
)

func cbpfClass(code uint16) uint16 { return code & 0x07 }
func cbpfSize(code uint16) uint16  { return code & 0x18 }
func cbpfMode(code uint16) uint16  { return code & 0xe0 }
func cbpfSrc(code uint16) uint16   { return code & 0x08 }
func cbpfALUOp(code uint16) uint16 { return code & 0xf0 }
func cbpfJMPOp(code uint16) uint16 { return code & 0xf0 }

func cbpfSizeToAsm(sz uint16) asm.Size {
	switch sz {
	case cBPFSizeB:
		return asm.Byte
	case cBPFSizeH:
		return asm.Half
	case cBPFSizeW:
		return asm.Word
	}
	return asm.Word
}

type cbpfToEbpf struct {
	cbpf []pcap.BPFInstruction

	// Maps cBPF instruction index to eBPF instruction offset
	offsetMap       []int
	labels          []string
	filterDoneLabel string
}

func convertCbpfToEbpf(cbpf []pcap.BPFInstruction) (asm.Instructions, error) {
	if len(cbpf) == 0 {
		return nil, errors.New("empty cBPF program")
	}

	c := &cbpfToEbpf{
		cbpf:            cbpf,
		offsetMap:       make([]int, len(cbpf)),
		labels:          make([]string, len(cbpf)),
		filterDoneLabel: "filter_done",
	}

	for i := range cbpf {
		c.labels[i] = fmt.Sprintf("cbpf_%d", i)
	}

	// First pass: calculate offsets
	offset := 0
	for i := range c.cbpf {
		c.offsetMap[i] = offset
		offset += c.instructionSize(i)
	}

	// Second pass: emit instructions
	var insns asm.Instructions
	for i := range c.cbpf {
		label := c.labels[i]
		block, err := c.emitInstruction(i)
		if err != nil {
			return nil, err
		}
		if len(block) > 0 {
			block[0] = block[0].WithSymbol(label)
		}
		insns = append(insns, block...)
	}

	return insns, nil
}

func (c *cbpfToEbpf) instructionSize(i int) int {
	ins := c.cbpf[i]
	cls := cbpfClass(ins.Code)

	switch cls {
	case cBPFClassLD:
		mode := cbpfMode(ins.Code)
		switch mode {
		case cBPFModeLEN:
			return 1 // Mov R0, R9
		case cBPFModeIMM:
			return 1 // Mov R0, imm
		case cBPFModeABS:
			return 1 // LoadAbs
		case cBPFModeIND:
			return 1 // LoadInd
		case cBPFModeMEM:
			return 1 // LoadMem
		default:
			return 1
		}
	case cBPFClassLDX:
		mode := cbpfMode(ins.Code)
		switch mode {
		case cBPFModeIMM:
			return 1 // Mov R8, imm
		case cBPFModeMEM:
			return 1 // LoadMem R8
		case cBPFModeMSH:
			return 3 // LoadAbs + LSh + Mov
		default:
			return 1
		}
	case cBPFClassST:
		return 1 // StoreImm
	case cBPFClassSTX:
		return 1 // StoreMem
	case cBPFClassALU:
		return 1 // ALU op
	case cBPFClassJMP:
		return 2 // conditional jump + unconditional jump
	case cBPFClassRET:
		return 1 // JA to filter_done
	case cBPFClassMISC:
		return 1 // TAX or TXA
	}
	return 1
}

func (c *cbpfToEbpf) emitInstruction(i int) (asm.Instructions, error) {
	ins := c.cbpf[i]
	cls := cbpfClass(ins.Code)

	switch cls {
	case cBPFClassLD:
		return c.emitLD(i, ins)
	case cBPFClassLDX:
		return c.emitLDX(i, ins)
	case cBPFClassST:
		return c.emitST(i, ins)
	case cBPFClassSTX:
		return c.emitSTX(i, ins)
	case cBPFClassALU:
		return c.emitALU(i, ins)
	case cBPFClassJMP:
		return c.emitJMP(i, ins)
	case cBPFClassRET:
		return c.emitRET(i, ins)
	case cBPFClassMISC:
		return c.emitMISC(i, ins)
	}
	return nil, errors.New("unknown cBPF instruction class")
}

func (c *cbpfToEbpf) emitLD(i int, ins pcap.BPFInstruction) (asm.Instructions, error) {
	mode := cbpfMode(ins.Code)
	sz := cbpfSize(ins.Code)

	switch mode {
	case cBPFModeLEN:
		return asm.Instructions{
			asm.Mov.Reg(ebpfA, ebpfL),
		}, nil

	case cBPFModeIMM:
		return asm.Instructions{
			asm.Mov.Imm(ebpfA, int32(ins.K)),
		}, nil

	case cBPFModeABS:
		return asm.Instructions{
			asm.LoadAbs(int32(ins.K), cbpfSizeToAsm(sz)),
		}, nil

	case cBPFModeIND:
		return asm.Instructions{
			asm.LoadInd(ebpfA, ebpfX, int32(ins.K), cbpfSizeToAsm(sz)),
		}, nil

	case cBPFModeMEM:
		offset := int16(-4 * (int(ins.K) + 1))
		return asm.Instructions{
			asm.LoadMem(ebpfA, asm.R10, offset, cbpfSizeToAsm(sz)),
		}, nil
	}

	return nil, errors.New("unsupported LD mode")
}

func (c *cbpfToEbpf) emitLDX(i int, ins pcap.BPFInstruction) (asm.Instructions, error) {
	mode := cbpfMode(ins.Code)
	sz := cbpfSize(ins.Code)

	switch mode {
	case cBPFModeIMM:
		return asm.Instructions{
			asm.Mov.Imm(ebpfX, int32(ins.K)),
		}, nil

	case cBPFModeMEM:
		offset := int16(-4 * (int(ins.K) + 1))
		return asm.Instructions{
			asm.LoadMem(ebpfX, asm.R10, offset, cbpfSizeToAsm(sz)),
		}, nil

	case cBPFModeMSH:
		return asm.Instructions{
			asm.LoadAbs(int32(ins.K), asm.Byte),
			asm.LSh.Imm(ebpfA, 2),
			asm.Mov.Reg(ebpfX, ebpfA),
		}, nil
	}

	return nil, errors.New("unsupported LDX mode")
}

func (c *cbpfToEbpf) emitST(i int, ins pcap.BPFInstruction) (asm.Instructions, error) {
	offset := int16(-4 * (int(ins.K) + 1))
	sz := cbpfSizeToAsm(cbpfSize(ins.Code))
	return asm.Instructions{
		asm.StoreMem(asm.R10, offset, ebpfA, sz),
	}, nil
}

func (c *cbpfToEbpf) emitSTX(i int, ins pcap.BPFInstruction) (asm.Instructions, error) {
	offset := int16(-4 * (int(ins.K) + 1))
	sz := cbpfSizeToAsm(cbpfSize(ins.Code))
	return asm.Instructions{
		asm.StoreMem(asm.R10, offset, ebpfX, sz),
	}, nil
}

func (c *cbpfToEbpf) emitALU(i int, ins pcap.BPFInstruction) (asm.Instructions, error) {
	op := cbpfALUOp(ins.Code)
	src := cbpfSrc(ins.Code)

	switch {
	case op == cBPFALUAdd && src == cBPFSrcK:
		return asm.Instructions{asm.Add.Imm(ebpfA, int32(ins.K))}, nil
	case op == cBPFALUAdd && src == cBPFSrcX:
		return asm.Instructions{asm.Add.Reg(ebpfA, ebpfX)}, nil
	case op == cBPFALUSub && src == cBPFSrcK:
		return asm.Instructions{asm.Sub.Imm(ebpfA, int32(ins.K))}, nil
	case op == cBPFALUSub && src == cBPFSrcX:
		return asm.Instructions{asm.Sub.Reg(ebpfA, ebpfX)}, nil
	case op == cBPFALUMul && src == cBPFSrcK:
		return asm.Instructions{asm.Mul.Imm(ebpfA, int32(ins.K))}, nil
	case op == cBPFALUMul && src == cBPFSrcX:
		return asm.Instructions{asm.Mul.Reg(ebpfA, ebpfX)}, nil
	case op == cBPFALUDiv && src == cBPFSrcK:
		return asm.Instructions{asm.Div.Imm(ebpfA, int32(ins.K))}, nil
	case op == cBPFALUDiv && src == cBPFSrcX:
		return asm.Instructions{asm.Div.Reg(ebpfA, ebpfX)}, nil
	case op == cBPFALUOr && src == cBPFSrcK:
		return asm.Instructions{asm.Or.Imm(ebpfA, int32(ins.K))}, nil
	case op == cBPFALUOr && src == cBPFSrcX:
		return asm.Instructions{asm.Or.Reg(ebpfA, ebpfX)}, nil
	case op == cBPFALUAnd && src == cBPFSrcK:
		return asm.Instructions{asm.And.Imm(ebpfA, int32(ins.K))}, nil
	case op == cBPFALUAnd && src == cBPFSrcX:
		return asm.Instructions{asm.And.Reg(ebpfA, ebpfX)}, nil
	case op == cBPFALULSh && src == cBPFSrcK:
		return asm.Instructions{asm.LSh.Imm(ebpfA, int32(ins.K))}, nil
	case op == cBPFALULSh && src == cBPFSrcX:
		return asm.Instructions{asm.LSh.Reg(ebpfA, ebpfX)}, nil
	case op == cBPFALURSh && src == cBPFSrcK:
		return asm.Instructions{asm.RSh.Imm(ebpfA, int32(ins.K))}, nil
	case op == cBPFALURSh && src == cBPFSrcX:
		return asm.Instructions{asm.RSh.Reg(ebpfA, ebpfX)}, nil
	case op == cBPFALUNeg:
		return asm.Instructions{asm.Neg.Imm(ebpfA, 0)}, nil
	case op == cBPFALUMod && src == cBPFSrcK:
		return asm.Instructions{asm.Mod.Imm(ebpfA, int32(ins.K))}, nil
	case op == cBPFALUMod && src == cBPFSrcX:
		return asm.Instructions{asm.Mod.Reg(ebpfA, ebpfX)}, nil
	case op == cBPFALUXor && src == cBPFSrcK:
		return asm.Instructions{asm.Xor.Imm(ebpfA, int32(ins.K))}, nil
	case op == cBPFALUXor && src == cBPFSrcX:
		return asm.Instructions{asm.Xor.Reg(ebpfA, ebpfX)}, nil
	}

	return nil, errors.New("unsupported ALU operation")
}

func (c *cbpfToEbpf) emitJMP(i int, ins pcap.BPFInstruction) (asm.Instructions, error) {
	op := cbpfJMPOp(ins.Code)

	if op == cBPFJMPJA {
		target := i + int(ins.K) + 1
		if target < 0 || target > len(c.cbpf) {
			target = len(c.cbpf) // jump to filter_done
		}
		targetLabel := c.labelFor(target)
		return asm.Instructions{
			asm.Ja.Label(targetLabel),
		}, nil
	}

	src := cbpfSrc(ins.Code)
	jtTarget := i + int(ins.Jt) + 1
	jfTarget := i + int(ins.Jf) + 1

	jtLabel := c.labelFor(jtTarget)
	jfLabel := c.labelFor(jfTarget)

	var cond asm.Instruction
	switch {
	case op == cBPFJMPJEQ && src == cBPFSrcK:
		cond = asm.JEq.Imm(ebpfA, int32(ins.K), jtLabel)
	case op == cBPFJMPJEQ && src == cBPFSrcX:
		cond = asm.JEq.Reg(ebpfA, ebpfX, jtLabel)
	case op == cBPFJMPJGT && src == cBPFSrcK:
		cond = asm.JGT.Imm(ebpfA, int32(ins.K), jtLabel)
	case op == cBPFJMPJGT && src == cBPFSrcX:
		cond = asm.JGT.Reg(ebpfA, ebpfX, jtLabel)
	case op == cBPFJMPJGE && src == cBPFSrcK:
		cond = asm.JGE.Imm(ebpfA, int32(ins.K), jtLabel)
	case op == cBPFJMPJGE && src == cBPFSrcX:
		cond = asm.JGE.Reg(ebpfA, ebpfX, jtLabel)
	case op == cBPFJMPSET && src == cBPFSrcK:
		cond = asm.JSet.Imm(ebpfA, int32(ins.K), jtLabel)
	case op == cBPFJMPSET && src == cBPFSrcX:
		cond = asm.JSet.Reg(ebpfA, ebpfX, jtLabel)
	default:
		return nil, errors.New("unsupported JMP operation")
	}

	return asm.Instructions{
		cond,
		asm.Ja.Label(jfLabel),
	}, nil
}

func (c *cbpfToEbpf) emitRET(i int, ins pcap.BPFInstruction) (asm.Instructions, error) {
	src := cbpfSrc(ins.Code)
	if src == cBPFSrcK && ins.K != 0 {
		return asm.Instructions{
			asm.Mov.Imm(ebpfA, int32(ins.K)),
			asm.Ja.Label(c.filterDoneLabel),
		}, nil
	}
	return asm.Instructions{
		asm.Ja.Label(c.filterDoneLabel),
	}, nil
}

func (c *cbpfToEbpf) emitMISC(i int, ins pcap.BPFInstruction) (asm.Instructions, error) {
	miscOp := ins.Code & 0xf8

	switch miscOp {
	case cBPFMISCTAX:
		return asm.Instructions{
			asm.Mov.Reg(ebpfX, ebpfA),
		}, nil
	case cBPFMISCTXA:
		return asm.Instructions{
			asm.Mov.Reg(ebpfA, ebpfX),
		}, nil
	}

	return nil, errors.New("unsupported MISC operation")
}

func (c *cbpfToEbpf) labelFor(cbpfIndex int) string {
	if cbpfIndex >= len(c.cbpf) {
		return c.filterDoneLabel
	}
	if cbpfIndex < 0 {
		return c.filterDoneLabel
	}
	return c.labels[cbpfIndex]
}

// CompileBPFProgramJIT 使用纯 Go eBPF 汇编器将过滤表达式编译为 *ebpf.CollectionSpec
// 完全不需要 clang 外部调用，支持热编译热加载
func CompileBPFProgramJIT(filterExpr string) (*ebpf.CollectionSpec, error) {
	pcapHandle, err := pcap.OpenLive("any", 65536, true, pcap.BlockForever)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer pcapHandle.Close()

	cbpfProg, err := pcapHandle.CompileBPFFilter(filterExpr)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return BuildXdpCollectionSpec(cbpfProg)
}

// BuildXdpCollectionSpec 基于 cBPF 指令构建完整的 XDP + AF_XDP eBPF CollectionSpec
func BuildXdpCollectionSpec(cbpfInsns []pcap.BPFInstruction) (*ebpf.CollectionSpec, error) {
	ebpfFilter, err := convertCbpfToEbpf(cbpfInsns)
	if err != nil {
		return nil, err
	}

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
		// Load data and data_end from xdp_md context
		asm.LoadMem(asm.R2, asm.R1, 0, asm.Word), // r2 = ctx->data
		asm.LoadMem(asm.R3, asm.R1, 4, asm.Word), // r3 = ctx->data_end

		// Save pointers
		asm.Mov.Reg(asm.R6, asm.R2), // r6 = data
		asm.Mov.Reg(asm.R7, asm.R3), // r7 = data_end

		// Precompute packet length for cBPF BPF_LEN support
		asm.Mov.Reg(ebpfL, asm.R7), // r9 = data_end
		asm.Sub.Reg(ebpfL, asm.R6), // r9 = data_end - data = pktlen

		// Check minimum Ethernet header size (14 bytes)
		asm.JLT.Imm(ebpfL, 14, "pass"),
	}

	// Inline translated cBPF filter
	prog = append(prog, ebpfFilter...)

	// Post-filter: R0=0 means no match (PASS), R0!=0 means match (redirect)
	prog = append(prog,
		asm.JEq.Imm(asm.R0, 0, "pass").WithSymbol("filter_done"),

		// Redirect to AF_XDP via xsk_map
		asm.LoadMapPtr(asm.R1, 0).WithReference("xsk_map"),
		asm.Mov.Imm(asm.R2, 0),
		asm.Mov.Imm(asm.R3, 0),
		asm.FnRedirectMap.Call(),
		asm.Return(),

		// pass label: return XDP_PASS
		asm.Mov.Imm(asm.R0, 2).WithSymbol("pass"),
		asm.Return(),
	)

	spec.Programs["xdp_filter_prog"] = &ebpf.ProgramSpec{
		Type:         ebpf.XDP,
		Instructions: prog,
		License:      "GPL",
	}

	return spec, nil
}
