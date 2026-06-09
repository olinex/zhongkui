# XDP Native + AF_XDP 抓包方案设计文档

## 1. 概述

本项目实现了基于 Linux XDP（eXpress Data Path）Native 模式的高性能网络数据包捕获方案。XDP 程序在网卡驱动层运行，配合 AF_XDP 零拷贝技术将匹配的数据包直接传递到用户态，性能远高于传统的 AF_PACKET 或 Libpcap 方案。

整个方案的核心竞争力在于：**所有过滤逻辑的编译和加载均在 Go 进程内完成**，无需调用 `clang` 等外部编译器，支持运行时动态热更新过滤规则。

---

## 2. 架构总览

```
                        内核空间 (Kernel)                       用户空间 (Userspace)
               ┌─────────────────────────────┐          ┌───────────────────────────┐
网卡驱动       │  eBPF XDP 程序               │          │  xdpWire (Go)             │
   │           │  ┌──────────────────────┐    │ AF_XDP   │  ┌─────────────────┐      │
   │   XDP hook│  │ 1. 加载 ctx->data    │    │ 共享内存  │  │  UMEM (16MB)    │      │
   └──────────>│  │ 2. 预计算包长度(R9)  │    │ ───────> │  │  4096帧×4096B   │      │
               │  │ 3. 以太网头校验(14B) │    │          │  └─────────────────┘      │
               │  │ 4. cBPF 过滤(JIT)    │    │          │                           │
               │  │    ├─ 不匹配 → PASS  │    │          │  三环管理:                │
               │  │    └─ 匹配 → redirect│    │          │  ┌─ Fill Ring (U→K) ─┐   │
               │  │       ↓              │    │          │  ├─ Comp Ring (K→U) ─┤   │
               │  │  bpf_redirect_map    │    │          │  └─ RX Ring   (K→U) ─┘   │
               │  │  (xsk_map, queue 0)  ├────┼────────> │                           │
               │  └──────────────────────┘    │          │  ZeroCopyReadPacketData() │
               └─────────────────────────────┘          └───────────────────────────┘
```

### 2.1 数据包路径

1. 网卡收到数据包 → 触发 XDP hook（驱动层，早于内核网络栈）
2. eBPF 程序进行过滤检查：
   - 取 `xdp_md->data` / `xdp_md->data_end`
   - 校验最小帧长（14 字节以太网头）
   - 执行 cBPF 过滤逻辑（如 `tcp port 80`）
3. 匹配的数据包通过 `bpf_redirect_map` 写入 AF_XDP socket 的 UMEM 区域
4. 不匹配的数据包返回 `XDP_PASS`，走正常内核协议栈
5. 用户态 Go 程序从 RX Ring 读取数据包描述符，从 UMEM 拷贝数据

### 2.2 平台降级策略

```
NewPlatformWire(config)
  ├── newXdpWire(config)      // 优先 XDP (内核 5.4+, 网卡驱动支持)
  │   └── 失败 → 降级
  ├── newAfpacketWire(config)  // AF_PACKET 备选
  │   └── 失败 → 返回错误
  └── 上层 NewWire 再降级到 NewLibpcapWire  // 最终兜底
```

---

## 3. 文件清单

| 文件 | 职责 |
|------|------|
| `base.go` | `Wire` 接口定义、`RawPacket`、平台无关的 `NewWire` 入口 |
| `linux.go` | Linux 平台 `NewPlatformWire` 工厂，XDP→AF_PACKET 降级 |
| `linux_xdp.go` | XDP Native + AF_XDP 完整实现（UMEM、三环管理、包读取） |
| `linux_ebpf_asm.go` | cBPF→eBPF JIT 翻译器 + XDP 骨架程序组装器 |
| `linux_bpf_compiler.go` | `CompileBPFProgram` 对外入口，委托 JIT |
| `linux_afpacket.go` | AF_PACKET 备选实现 |
| `libpcap.go` | Libpcap 兜底实现 |

---

## 4. Wire 接口

```go
type Wire interface {
    Name() string                       // 方案名称："xdp" / "afpacket" / "libpcap"
    LinkType() layers.LinkType          // 链路层类型：Ethernet
    SetBPFFilter(expr string) error     // 动态设置/更新 BPF 过滤规则
    ZeroCopyReadPacketData() *RawPacket // 读取一个数据包（非阻塞）
    Close()                             // 清理资源
}
```

### 4.1 RawPacket 的重试语义

```go
func (r *RawPacket) HasNext() bool {
    // EAGAIN → 重试（无数据，等待）
    // net.Error.Temporary() → 重试
    // EBADF / EOF / Closed Pipe → 终止
}
```

上层调用方通过 `HasNext()` 决定是否继续轮询，`EAGAIN` 错误会自动重试。

---

## 5. linux_xdp.go — XDP Native + AF_XDP 核心实现

### 5.1 关键常量

| 常量 | 值 | 说明 |
|------|-----|------|
| `umemFrameSize` | 4096 | 每帧 4KB，匹配常见 PMD 页大小 |
| `umemFrameCount` | 4096 | 共 4096 帧，UMEM 总大小 16MB |
| `rxRingSize` | 2048 | RX Ring 最多 2048 个待消费描述符 |
| `fillRingSize` | 2048 | Fill Ring 最多 2048 个可用帧描述符 |
| `compRingSize` | 2048 | Completion Ring 最多 2048 个已完成描述符 |

### 5.2 初始化流程

```
newXdpWire(config)
  │
  ├── 1. net.InterfaceByName(device) → 获取 ifindex
  │
  ├── 2. setupUmem()
  │      ├── make([]byte, 16MB) — 用户态大块内存
  │      ├── 构建 XDPUmemReg 描述符（Addr, Len, Size, Headroom）
  │      └── 初始化 freePool（所有帧索引）
  │
  ├── 3. setupXdpSocket()
  │      ├── unix.Socket(AF_XDP, SOCK_RAW, 0)
  │      ├── setsockopt(XDP_UMEM_REG)     — 注册 UMEM
  │      ├── setsockopt(XDP_RX_RING)      — 设置 RX Ring 大小
  │      ├── setsockopt(XDP_UMEM_FILL_RING)  — 设置 Fill Ring 大小
  │      ├── setsockopt(XDP_UMEM_COMPLETION_RING) — 设置 Comp Ring 大小
  │      ├── getsockopt(XDP_MMAP_OFFSETS) — 获取 mmap 偏移量
  │      ├── 按偏移量 mmap 三个 Ring 区域
  │      ├── fillInitialFrames() — 将所有帧填入 Fill Ring
  │      ├── setsockopt(XDP_OPTIONS) — 设置选项（非 zerocopy 模式）
  │      └── bind SockaddrXDP{Ifindex, QueueID=0}
  │
  └── 4. setupXdpProgram()
         ├── bpfSpec.LoadAndAssign() → 加载 eBPF 程序 + map
         ├── xsk_map.Put(queueID=0, xskFd) — 关联 AF_XDP socket
         └── link.AttachXDP(XDPDriverMode) — 挂载到网卡
```

### 5.3 三环管理

AF_XDP 使用三个共享内存 Ring 来协调内核与用户态的数据交换：

```
 Fill Ring (用户→内核)
 ┌──────────────────────────────────────────────────┐
 │ Producer: 用户写完自增    Consumer: 内核读完自增    │
 │ 用户将空闲帧地址写入 Fill Ring，内核取走用于填充数据  │
 └──────────────────────────────────────────────────┘
                          ↓
 Completion Ring (内核→用户)
 ┌──────────────────────────────────────────────────┐
 │ Producer: 内核写完自增    Consumer: 用户读完自增    │
 │ 内核通知用户哪些帧的数据已被上层消费完成，帧可回收     │
 └──────────────────────────────────────────────────┘
                          ↓
 RX Ring (内核→用户)
 ┌──────────────────────────────────────────────────┐
 │ Producer: 内核写完自增    Consumer: 用户读完自增    │
 │ 内核将捕获到的数据包描述符写入 RX Ring             │
 └──────────────────────────────────────────────────┘
```

**关键实现细节：**

- 所有 Ring 通过 `mmap` 映射为 Go byte slice，producer/consumer 指针指向 mmap 内固定偏移
- producer 写入使用 `atomic.StoreUint32`，consumer 读取使用 `atomic.LoadUint32`
- 只有用户态单端 + 内核单端，无需跨核同步锁

### 5.4 包数据读取

```go
func (x *xdpWire) ZeroCopyReadPacketData() *RawPacket {
    // 1. 从 RX Ring dequeue 获取下一个描述符索引
    idx, ok := x.rxRing.dequeue()    // 空 → 处理 Completion 后返回 EAGAIN

    // 2. 读取描述符: {Addr, Len, Options}
    desc := x.rxRing.desc[idx]

    // 3. 按 snaplen 截断
    // 4. 从 UMEM 拷贝数据（注意：真正零拷贝需要上层直接使用 UMEM 切片，
    //    这里做了一次 copy 是为了简化与现有 capture 管线的兼容）
    data := make([]byte, pktLen)
    copy(data, x.umem[desc.Addr:])

    // 5. release RX Ring consumer
    x.rxRing.release(idx + 1)
}
```

### 5.5 帧回收流程

```
processCompletions()
  └── 遍历 Completion Ring
        ├── 提取帧索引 (desc.Addr / umemFrameSize)
        ├── 归还到 freePool
        └── 更新 consumer
            │
            └── refillFillRing()
                  └── 将 freePool 中的帧重新写入 Fill Ring
```

### 5.6 裸 Syscall 处理

由于 `golang.org/x/sys/unix` 没有为 `XDP_UMEM_REG` 等提供封装的 `SetsockoptBytes` / `GetsockoptBytes` 方法，这里直接使用 `unix.Syscall6` 调用原始系统调用：

```go
func setsockoptRaw(fd, level, opt int, val unsafe.Pointer, vallen uint32) error {
    _, _, e := unix.Syscall6(unix.SYS_SETSOCKOPT, ...)
}
func getsockoptRaw(fd, level, opt int, val unsafe.Pointer, vallen *uint32) error {
    _, _, e := unix.Syscall6(unix.SYS_GETSOCKOPT, ...)
}
```

---

## 6. linux_ebpf_asm.go — 纯 Go eBPF JIT 编译器

### 6.1 设计动机

传统方案（已废弃）：

```
pcap filter → cBPF → cbpfc.ToC() → clang exec → eBPF .o → LoadCollectionSpec
```

**问题**：
1. 依赖 `clang` 外部命令，运行环境必须安装 LLVM 工具链
2. `exec.Command` 启动外部进程开销大，不适合频繁热更新
3. 源码级 C 模板（`xdp_filter_kern.c`）维护和调试复杂

新方案：

```
pcap filter → cBPF → Go asm 指令转换 → CollectionSpec → LoadAndAssign
```

**优势**：
1. 零外部依赖，纯 Go 实现
2. 内存内编译，无进程启动开销，适合热加载
3. 指令级精确控制，无需处理 C 编译器的优化行为

### 6.2 cBPF 指令编码回顾

cBPF 每条指令固定 8 字节：

```
opcode:16  jt:8  jf:8  k:32
```

opcode 位域布局：

```
  7  6  5  4  3  2  1  0
┌────┬────┬────┬────┬────┬────┬────┬────┐
│        class        │              │
├─────────────────────┼──────────────┤
│ 0=LD 1=LDX 2=ST    │ 低 3 位      │
│ 3=STX 4=ALU 5=JMP  │ class 标识    │
│ 6=RET 7=MISC        │              │
└─────────────────────┴──────────────┘

LD/LDX: [mode:3][size:2][0][class:3]    mode=IMM/ABS/IND/MEM/LEN/MSH
ALU:    [op:4][src:1][0][class:3]       src=K/X
JMP:    [op:4][src:1][0][class:3]       src=K/X
RET:    [mode:3][0][0][class:3]         class=6
```

### 6.3 cBPF → eBPF 寄存器映射

| 虚拟概念 | cBPF 寄存器 | eBPF 寄存器 | 说明 |
|----------|------------|-------------|------|
| 累加器 | A | R0 | ALU 操作目标、比较值、返回值 |
| 索引寄存器 | X | R8 | LD_IND 偏移、STX 源 |
| 包长度 | BPF_LEN | R9 | 骨架中预计算 `data_end - data` |
| 暂存区 | M[0..15] | 栈 R10-(i+1)*4 | 16 个 32-bit 内存槽 |
| 上下文 | — | R1 | XDP 程序参数 `xdp_md*` |
| 帧指针 | — | R10 | 只读栈指针 |

### 6.4 指令翻译对照表

#### 6.4.1 LD（加载类）

| cBPF | eBPF asm | 说明 |
|------|----------|------|
| `LD #K` | `Mov R0, imm` | 加载立即数到 A |
| `LD [K]` | `LoadAbs(offset, size)` | 从包数据加载，eBPF XDP 原生支持 |
| `LD [X+K]` | `LoadInd(R0, R8, offset, size)` | 从包数据 X+K 偏移加载 |
| `LD M[K]` | `LoadMem(R0, R10, offset, size)` | 从栈读取暂存区 |
| `LD len` | `Mov R0, R9` | 包长度（骨架预计算） |

#### 6.4.2 LDX（加载到 X 寄存器）

| cBPF | eBPF asm | 说明 |
|------|----------|------|
| `LDX #K` | `Mov R8, imm` | 加载立即数到 X |
| `LDX M[K]` | `LoadMem(R8, R10, offset, size)` | 从栈读取暂存区 |
| `LDX 4*([K]&0xf)` | `LoadAbs(offset, byte) → LSh 2 → Mov R8, R0` | MSH 模式，3 指令展开 |

#### 6.4.3 存储类

| cBPF | eBPF asm |
|------|----------|
| `ST M[K]` | `StoreImm(R10, offset, value, size)` |
| `STX M[K]` | `StoreMem(R10, offset, R8, size)` |

#### 6.4.4 ALU 运算

cBPF 的 12 种 ALU 操作（Add/Sub/Mul/Div/Or/And/LSh/RSh/Neg/Mod/Xor，各含 K/X 源）一对一映射到 `cilium/ebpf/asm` 对应的 `Op.Imm` / `Op.Reg` 调用。

#### 6.4.5 JMP 跳转

cBPF 条件跳转的 jt/jf 偏移量编码 `(i + offset + 1)`，需要计算目标指令索引后映射为 eBPF 的 label 引用。每个条件跳转展开为两条 eBPF 指令：

```go
// cBPF: JEQ #80, jt=2, jf=5
// → eBPF:
JEq.Imm(R0, 80, "cbpf_5")   // 条件成立跳转到 jt 目标
Ja.Label("cbpf_8")           // 不成立跳到 jf 目标
```

无条件跳转 `JA` 直接映射为 `Ja.Label`。

#### 6.4.6 RET 返回

| cBPF | 语义 | eBPF |
|------|------|------|
| `RET #0` | 返回 A=0（不匹配） | `JA filter_done` |
| `RET #K (K≠0)` | 返回 A=K（匹配） | `Mov R0, K; JA filter_done` |

`filter_done` 标签后续由 `JEq R0, 0, "pass"` 判断。

#### 6.4.7 MISC 杂项

| cBPF | eBPF |
|------|------|
| `TAX` (A→X) | `Mov R8, R0` |
| `TXA` (X→A) | `Mov R0, R8` |

### 6.5 两遍编译流程

```
convertCbpfToEbpf(cbpf)
  │
  ├── 第一遍 (instructionSize)
  │     └── 计算每条 cBPF 指令展开后的 eBPF 指令数
  │         - LD/LDX M[K]: 1
  │         - LDX MSH: 3
  │         - JMP 条件: 2
  │         - RET K≠0: 2
  │         - 其他: 1
  │
  └── 第二遍 (emitInstruction)
        ├── 为每条 cBPF 指令生成 eBPF 指令
        ├── 每段指令首条设置 Symbol("cbpf_N") 标签
        └── JMP 跳转目标通过 labelFor() 解析为标签引用
```

`labelFor()` 函数将越界的 cBPF 跳转目标（如 `RET` 跳转到程序末尾）统一映射到 `filter_done` 标签。

### 6.6 XDP 骨架程序

`BuildXdpCollectionSpec` 构造完整的 XDP + AF_XDP eBPF `CollectionSpec`，包含一个 XSKMap 和一个 XDP 程序：

```asm
// === 前导：上下文加载 ===
R2 = *(u32 *)(R1 + 0)    // ctx->data
R3 = *(u32 *)(R1 + 4)    // ctx->data_end
R6 = R2                   // 保存 data
R7 = R3                   // 保存 data_end
R9 = R3 - R2              // 包长度（供 cBPF BPF_LEN 使用）

// === 最小帧校验 ===
if R9 < 14 goto pass

// === [inline cBPF 过滤器] ===
// R0 = 0 表示不匹配
// R0 ≠ 0 表示匹配

// === filter_done: 分发 ===
if R0 == 0 goto pass

// === 匹配：重定向到 AF_XDP ===
R1 = &xsk_map
R2 = 0  (queue_id)
R3 = 0  (flags)
call bpf_redirect_map  // FnRedirectMap = 51
return

// === pass: XDP_PASS ===
pass:
R0 = 2  (XDP_PASS)
return
```

### 6.7 CollectionSpec 声明

```go
spec := &ebpf.CollectionSpec{
    Maps: map[string]*ebpf.MapSpec{
        "xsk_map": {
            Type:       ebpf.XSKMap,   // BPF_MAP_TYPE_XSKMAP
            KeySize:    4,             // int (queue_id)
            ValueSize:  4,             // int (xsk_fd)
            MaxEntries: 64,
        },
    },
    Programs: map[string]*ebpf.ProgramSpec{
        "xdp_filter_prog": {
            Type:         ebpf.XDP,
            Instructions: prog,
            License:      "GPL",
        },
    },
}
```

---

## 7. 动态 BPF 过滤热更新

### 7.1 编译 → 加载流程

```
SetBPFFilter("tcp port 80")
  │
  ├── CompileBPFProgram(expr)
  │     ├── pcap.OpenLive("any")      // 临时 pcap handle（仅用于编译）
  │     ├── pcap.CompileBPFFilter()   // pcap 表达式 → cBPF 指令
  │     └── CompileBPFProgramJIT()    // 委托纯 Go JIT
  │           ├── convertCbpfToEbpf() // cBPF → eBPF asm
  │           └── BuildXdpCollectionSpec() // 组装完整 CollectionSpec
  │
  ├── 卸载旧程序
  │     ├── xdpLink.Close()           // detach XDP
  │     └── objs.Close()              // 释放 eBPF collection
  │
  ├── x.bpfSpec = newSpec
  │
  └── setupXdpProgram()
        ├── spec.LoadAndAssign()      // 加载到内核
        ├── xsk_map.Put(0, xskFd)     // 关联 AF_XDP socket
        └── link.AttachXDP()          // 挂载到网卡
```

### 7.2 热更新特性

- **零停机**：AF_XDP socket、UMEM、Ring 在过滤规则更新期间保持存活
- **原子切换**：先 detach 旧程序再 attach 新程序，不留孤立 hook
- **错误安全**：编译失败时旧过滤规则不受影响（新 spec 在卸载旧程序前完成编译）

---

## 8. 边界情况与限制

### 8.1 设备限制
- XDP 不支持 `any` 设备（`newXdpWire` 直接返回错误）
- 需要网卡驱动原生支持 XDP（Native/Driver 模式），不支持则降级到 AF_PACKET

### 8.2 UMEM 大小
- 16MB UMEM（4096 帧 × 4096 字节），需 2MB 大页支持
- 单帧 4KB：足够容纳标准以太网 MTU 1500 + 头部开销
- Jumbo Frame（>4KB）会被截断

### 8.3 并发安全
- `ZeroCopyReadPacketData` 设计为非线程安全，单 goroutine 调用
- Ring 操作使用 atomic 指令，与内核端并发安全

### 8.4 数据拷贝
- 当前实现从 UMEM 拷贝数据到新切片（`make + copy`）
- 原因：兼容现有 capture 管线（`RawPacket.Data` 需要独立生命周期）
- 优化方向：如果上层保证在下一帧覆盖前消费完毕，可直接引用 UMEM 切片

### 8.5 macOS 交叉编译
- `go vet` 在 macOS 上报 `go:embed` 错误（bpf 模板文件已废弃但 LSP 缓存未清）
- `GOOS=linux go build` 因 gopacket/pcap CGo 依赖 libpcap 无法交叉编译
- 实际编译在 Linux 机器上进行即可

---

## 9. 依赖库

| 库 | 用途 |
|----|------|
| `github.com/cilium/ebpf` | eBPF 程序加载、map 操作、XDP 挂载 |
| `github.com/cilium/ebpf/asm` | 纯 Go eBPF 指令汇编器（JIT 核心） |
| `github.com/cilium/ebpf/link` | XDP 程序 attach/detach |
| `golang.org/x/sys/unix` | AF_XDP socket、UMEM、Ring mmap、syscall |
| `github.com/gopacket/gopacket/pcap` | pcap 过滤表达式 → cBPF 编译 |
| `github.com/gopacket/gopacket/layers` | LinkType 常量 |
