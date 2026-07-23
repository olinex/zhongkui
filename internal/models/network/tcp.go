package network

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"net"

	"github.com/gopacket/gopacket/layers"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/hash"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

var (
	_ driver.Valuer = (*TCPOptions)(nil)
)

// TCPOption 表示TCP协议选项
// TCP选项用于在TCP连接建立和通信过程中传递额外信息
// 每个TCP选项包含类型、长度和数据三个部分
type TCPOption struct {
	Type   layers.TCPOptionKind `json:"type"`   // 选项类型，如MSS、窗口缩放因子、时间戳等
	Length uint8                `json:"length"` // 选项总长度（包括类型和长度字段）
	Data   []byte               `json:"data"`   // 选项数据内容
}

// TCPOptions 表示TCP选项列表
// 用于存储多个TCP选项，支持数据库序列化和反序列化
type TCPOptions []TCPOption

// NewTCPOptions 从gopacket TCP选项创建TCPOptions
// 将底层gopacket库的TCP选项转换为应用层的TCPOptions结构
// 参数options: gopacket库解析的原始TCP选项列表
// 返回值: 转换后的TCPOptions结构
func NewTCPOptions(options []layers.TCPOption) TCPOptions {
	tcpOptions := make(TCPOptions, 0, len(options))
	for _, option := range options {
		tcpOptions = append(tcpOptions, TCPOption{
			Type:   option.OptionType,
			Length: option.OptionLength,
			Data:   option.OptionData,
		})
	}
	return tcpOptions
}

// Value 实现数据库序列化接口
// 将TCPOptions序列化为JSON格式，用于存储到数据库
// 返回值driver.Value: 序列化后的数据库值
// 返回值error: 序列化过程中的错误
func (o TCPOptions) Value() (driver.Value, error) {
	if o == nil {
		return nil, nil
	}
	return json.Marshal(o)
}

// Scan 实现数据库反序列化接口
// 从数据库读取数据并反序列化为TCPOptions结构
// 参数value: 从数据库读取的原始值，可以是[]byte或string类型
// 返回值error: 反序列化过程中的错误
func (o *TCPOptions) Scan(value any) error {
	if value == nil {
		*o = nil
		return nil
	}
	var bytes []byte
	switch raw := value.(type) {
	case []byte:
		if len(raw) == 0 {
			*o = nil
			return nil
		}
		bytes = raw
	case string:
		if raw == "" {
			*o = nil
			return nil
		}
		bytes = []byte(raw)
	default:
		return errors.Newf("Tcp_options scan failed, except []byte, got %T", value)
	}
	return json.Unmarshal(bytes, o)
}

// TCPHeaders 表示TCP协议头部信息
// 包含TCP协议头部的所有字段，用于存储和操作TCP数据包头部信息
// 支持数据库存储和JSON序列化，使用GORM标签定义数据库字段属性
type TCPHeaders struct {
	TCPSrcPort    layers.TCPPort `gorm:"type:UInt16 CODEC(ZSTD(3))" json:"src_port"`
	TCPDstPort    layers.TCPPort `gorm:"type:UInt16 CODEC(ZSTD(3))" json:"dst_port"`
	TCPSeq        uint32         `gorm:"type:UInt32 CODEC(ZSTD(3))" json:"seq"`
	TCPAck        uint32         `gorm:"type:UInt32 CODEC(ZSTD(3))" json:"ack"`
	TCPDataOffset uint8          `gorm:"type:UInt8 CODEC(ZSTD(3))" json:"data_offset"`
	TCPBitFIN     bool           `gorm:"type:Bool" json:"bit_fin"`
	TCPBitSYN     bool           `gorm:"type:Bool" json:"bit_syn"`
	TCPBitRST     bool           `gorm:"type:Bool" json:"bit_rst"`
	TCPBitPSH     bool           `gorm:"type:Bool" json:"bit_psh"`
	TCPBitACK     bool           `gorm:"type:Bool" json:"bit_ack"`
	TCPBitURG     bool           `gorm:"type:Bool" json:"bit_urg"`
	TCPBitECE     bool           `gorm:"type:Bool" json:"bit_ece"`
	TCPBitCWR     bool           `gorm:"type:Bool" json:"bit_cwr"`
	TCPBitNS      bool           `gorm:"type:Bool" json:"bit_ns"`
	TCPWindow     uint16         `gorm:"type:UInt16 CODEC(ZSTD(3))" json:"window"`
	TCPChecksum   uint16         `gorm:"type:UInt16 CODEC(ZSTD(3))" json:"checksum"`
	TCPUrgent     uint16         `gorm:"column:tcp_urgent;type:UInt16 CODEC(ZSTD(3))" json:"urgent"`
	TCPOptions    TCPOptions     `gorm:"type:String CODEC(ZSTD(3))" json:"options"`
}

// SrcPort 返回源端口号
// 返回值: 源端口号，layers.TCPPort类型
func (t *TCPHeaders) SrcPort() layers.TCPPort {
	return t.TCPSrcPort
}

// DstPort 返回目的端口号
// 返回值: 目的端口号，layers.TCPPort类型
func (t *TCPHeaders) DstPort() layers.TCPPort {
	return t.TCPDstPort
}

// SeqNum 返回序列号
// 序列号用于标识TCP数据段在数据流中的位置
// 返回值: 32位无符号整数序列号
func (t *TCPHeaders) SeqNum() uint32 {
	return t.TCPSeq
}

// AckNum 返回确认号
// 确认号表示期望接收的下一个字节的序列号
// 返回值: 32位无符号整数确认号
func (t *TCPHeaders) AckNum() uint32 {
	return t.TCPAck
}

// IsSyn 判断是否为SYN包
// SYN包用于建立TCP连接，同步序列号
// 返回值: true表示是SYN包（SYN=1, ACK=0, FIN=0）
func (t *TCPHeaders) IsSyn() bool {
	return t.TCPBitSYN && !t.TCPBitACK && !t.TCPBitFIN
}

// IsSynAck 判断是否为SYN-ACK包
// SYN-ACK包用于响应SYN包，确认连接建立
// 返回值: true表示是SYN-ACK包（SYN=1, ACK=1）
func (t *TCPHeaders) IsSynAck() bool {
	return t.TCPBitSYN && t.TCPBitACK
}

// IsAck 判断是否为ACK包
// ACK包用于确认数据接收，不包含SYN或FIN标志
// 返回值: true表示是ACK包（SYN=0, ACK=1, FIN=0）
func (t *TCPHeaders) IsAck() bool {
	return !t.TCPBitSYN && t.TCPBitACK && !t.TCPBitFIN
}

// IsFin 判断是否为FIN包
// FIN包用于关闭TCP连接，表示发送方完成数据发送
// 返回值: true表示是FIN包（FIN=1）
func (t *TCPHeaders) IsFin() bool {
	return t.TCPBitFIN
}

// IsRst 判断是否为RST包
// RST包用于立即终止TCP连接，通常表示异常情况
// 返回值: true表示是RST包（RST=1）
func (t *TCPHeaders) IsRst() bool {
	return t.TCPBitRST
}

// NewTCPHeaders 从gopacket TCP层创建TCPHeaders
// 将底层gopacket库解析的TCP头部信息转换为应用层的TCPHeaders结构
// 参数tcp: gopacket库解析的TCP层对象，包含原始TCP头部信息
// 返回值: 新创建的TCPHeaders指针，包含所有TCP头部字段
func NewTCPHeaders(tcp *layers.TCP) *TCPHeaders {
	return &TCPHeaders{
		TCPSrcPort:    tcp.SrcPort,                // 源端口
		TCPDstPort:    tcp.DstPort,                // 目的端口
		TCPSeq:        tcp.Seq,                    // 序列号
		TCPAck:        tcp.Ack,                    // 确认号
		TCPDataOffset: tcp.DataOffset,             // 数据偏移量
		TCPBitFIN:     tcp.FIN,                    // FIN标志位
		TCPBitSYN:     tcp.SYN,                    // SYN标志位
		TCPBitRST:     tcp.RST,                    // RST标志位
		TCPBitPSH:     tcp.PSH,                    // PSH标志位
		TCPBitACK:     tcp.ACK,                    // ACK标志位
		TCPBitURG:     tcp.URG,                    // URG标志位
		TCPBitECE:     tcp.ECE,                    // ECE标志位
		TCPBitCWR:     tcp.CWR,                    // CWR标志位
		TCPBitNS:      tcp.NS,                     // NS标志位
		TCPWindow:     tcp.Window,                 // 窗口大小
		TCPChecksum:   tcp.Checksum,               // 校验和
		TCPUrgent:     tcp.Urgent,                 // 紧急指针
		TCPOptions:    NewTCPOptions(tcp.Options), // TCP选项列表
	}
}

// PacketIPv4TCP 表示IPv4 TCP数据包
// 组合了PacketIPv4和TCPHeaders，表示完整的IPv4 TCP数据包信息
// 包含以太网层、IPv4层和TCP层的所有头部信息
type PacketIPv4TCP struct {
	PacketIPv4                    // 嵌入IPv4数据包基础结构
	TCPHeaders                    // 嵌入TCP头部信息
	Payload    *serializer.Crypto `gorm:"type:String CODEC(ZSTD(3));serializer:crypto" json:"payload" secure:"mode=\"aes-gcm\",offset=38278,length=32"`
}

// StreamID 返回TCP流ID
// 通过 hash/maphash 对源IP、目的IP、源端口、目的端口计算64-bit哈希，生成唯一的流标识符
// 流ID用于识别属于同一TCP连接的所有数据包
// 返回值: 16字符的十六进制64-bit哈希字符串
func (p *PacketIPv4TCP) StreamID() string {
	h := hash.Acquire()
	defer h.Release()

	leftPort, rightPort := uint16(p.TCPSrcPort), uint16(p.TCPDstPort)
	leftIP := net.IP(p.IPv4Src).To4()
	rightIP := net.IP(p.IPv4Dst).To4()

	if leftPort > rightPort {
		leftPort, rightPort = rightPort, leftPort
	}
	if bytes.Compare(leftIP, rightIP) > 0 {
		leftIP, rightIP = rightIP, leftIP
	}

	h.WriteUint16(leftPort)
	h.WriteUint16(rightPort)
	h.WriteBytes(leftIP)
	h.WriteBytes(rightIP)

	return h.SumHex()
}

// Meta 返回基础元数据
// 提供对数据包基础元数据的访问接口
// 返回值: 指向BaseMeta结构的指针
func (p *PacketIPv4TCP) Meta() *BaseMeta {
	return &p.BaseMeta
}

// FrameInfo 返回帧信息
// 提供对数据包时间戳和协议层堆栈信息的访问接口
// 返回值: 包含时间戳和层堆栈的FrameInfo结构
func (p *PacketIPv4TCP) FrameInfo() *FrameInfo {
	return &FrameInfo{
		Timestamp:  p.Timestamp,  // 数据包捕获时间戳
		LayerStack: p.LayerStack, // 协议层堆栈信息
	}
}

// GetPayload 返回TCP数据包的载荷数据
// 从加密存储的 Payload 字段中获取解密后的原始载荷字节
func (p *PacketIPv4TCP) GetPayload() []byte {
	if p.Payload == nil {
		return nil
	}
	return p.Payload.Value()
}

// FusedIPv4TCP 表示融合的IPv4 TCP数据
// 组合了FusedIPv4和TCPHeaders，用于存储融合处理后的IPv4 TCP数据
// 融合数据通常包含聚合统计信息，用于分析和监控
type FusedIPv4TCP struct {
	FusedIPv4                     // 嵌入融合IPv4数据基础结构
	TCPHeaders                    // 嵌入TCP头部信息
	Payload    *serializer.Crypto `gorm:"type:String CODEC(ZSTD(3));serializer:crypto" json:"payload" secure:"mode=\"aes-gcm\",offset=49915,length=32"`
}

// PacketIPv6TCP 表示IPv6 TCP数据包
// 组合了PacketIPv6和TCPHeaders，表示完整的IPv6 TCP数据包信息
// 包含以太网层、IPv6层和TCP层的所有头部信息
type PacketIPv6TCP struct {
	PacketIPv6                    // 嵌入IPv6数据包基础结构
	TCPHeaders                    // 嵌入TCP头部信息
	Payload    *serializer.Crypto `gorm:"type:String CODEC(ZSTD(3));serializer:crypto" json:"payload" secure:"mode=\"aes-gcm\",offset=50359,length=32"`
}

// StreamID 返回TCP流ID
// 通过计算源IPv6地址、目的IPv6地址、源端口、目的端口的MD5哈希值生成唯一的流标识符
// 流ID用于识别属于同一TCP连接的所有数据包，支持IPv6地址
// 返回值: 16字符的十六进制64-bit哈希字符串
func (p *PacketIPv6TCP) StreamID() string {
	h := hash.Acquire()
	defer h.Release()

	leftPort, rightPort := uint16(p.TCPSrcPort), uint16(p.TCPDstPort)
	leftIP := net.IP(p.IPv6Src).To16()
	rightIP := net.IP(p.IPv6Dst).To16()

	if leftPort > rightPort {
		leftPort, rightPort = rightPort, leftPort
	}
	if bytes.Compare(leftIP, rightIP) > 0 {
		leftIP, rightIP = rightIP, leftIP
	}

	h.WriteUint16(leftPort)
	h.WriteUint16(rightPort)
	h.WriteBytes(leftIP)
	h.WriteBytes(rightIP)

	return h.SumHex()
}

// Meta 返回基础元数据
// 提供对数据包基础元数据的访问接口
// 返回值: 指向BaseMeta结构的指针
func (p *PacketIPv6TCP) Meta() *BaseMeta {
	return &p.BaseMeta
}

// FrameInfo 返回帧信息
// 提供对数据包时间戳和协议层堆栈信息的访问接口
// 返回值: 包含时间戳和层堆栈的FrameInfo结构
func (p *PacketIPv6TCP) FrameInfo() *FrameInfo {
	return &FrameInfo{
		Timestamp:  p.Timestamp,  // 数据包捕获时间戳
		LayerStack: p.LayerStack, // 协议层堆栈信息
	}
}

// GetPayload 返回TCP数据包的载荷数据
// 从加密存储的 Payload 字段中获取解密后的原始载荷字节
func (p *PacketIPv6TCP) GetPayload() []byte {
	if p.Payload == nil {
		return nil
	}
	return p.Payload.Value()
}

// FusedIPv6TCP 表示融合的IPv6 TCP数据
// 组合了FusedIPv6和TCPHeaders，用于存储融合处理后的IPv6 TCP数据
// 融合数据通常包含聚合统计信息，用于分析和监控
type FusedIPv6TCP struct {
	FusedIPv6                     // 嵌入融合IPv6数据基础结构
	TCPHeaders                    // 嵌入TCP头部信息
	Payload    *serializer.Crypto `gorm:"type:String CODEC(ZSTD(3));serializer:crypto" json:"payload" secure:"mode=\"aes-gcm\",offset=16648,length=32"`
}

type FusedIPv4UDP struct {
	FusedIPv4                     // 嵌入融合IPv4数据基础结构
	UDPHeaders                    // 嵌入UDP头部信息
	Payload    *serializer.Crypto `gorm:"type:String CODEC(ZSTD(3));serializer:crypto" json:"payload" secure:"mode=\"aes-gcm\",offset=51725,length=32"`
}

type FusedIPv6UDP struct {
	FusedIPv6                     // 嵌入融合IPv6数据基础结构
	UDPHeaders                    // 嵌入UDP头部信息
	Payload    *serializer.Crypto `gorm:"type:String CODEC(ZSTD(3));serializer:crypto" json:"payload" secure:"mode=\"aes-gcm\",offset=36791,length=32"`
}
