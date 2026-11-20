package network

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"net"
	"time"

	"github.com/gopacket/gopacket/layers"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/hash"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

var (
	_ driver.Valuer = (*TCPOptions)(nil)
)

// computeStreamID 计算TCP流标识符
// 对已排序的IP字节序列和端口号计算64-bit maphash 哈希，生成方向无关的唯一流标识符。
// 同一TCP连接的双向数据包会产生相同的流ID。
// 参数leftIP/rightIP: 已排序后的IP地址字节序列（IPv4为4字节，IPv6为16字节）
// 参数leftPort/rightPort: 已排序后的端口号
// 返回值: 16字符的十六进制64-bit哈希字符串
func computeStreamID(leftIP, rightIP []byte, leftPort, rightPort uint16) string {
	if leftPort > rightPort {
		leftPort, rightPort = rightPort, leftPort
	}
	if bytes.Compare(leftIP, rightIP) > 0 {
		leftIP, rightIP = rightIP, leftIP
	}

	h := hash.Acquire()
	defer h.Release()

	h.WriteUint16(leftPort)
	h.WriteUint16(rightPort)
	h.WriteBytes(leftIP)
	h.WriteBytes(rightIP)

	return h.SumHex()
}

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
	// 源端口
	TCPSrcPort layers.TCPPort `gorm:"type:UInt16 CODEC(ZSTD(3))" json:"srcPort"`
	// 目的端口
	TCPDstPort layers.TCPPort `gorm:"type:UInt16 CODEC(ZSTD(3))" json:"dstPort"`
	// 序列号
	TCPSeq uint32 `gorm:"type:UInt32 CODEC(ZSTD(3))" json:"seq"`
	// 确认号
	TCPAck uint32 `gorm:"type:UInt32 CODEC(ZSTD(3))" json:"ack"`
	// 数据偏移量（TCP 头部长度，单位 4 字节）
	TCPDataOffset uint8 `gorm:"type:UInt8 CODEC(ZSTD(3))" json:"dataOffset"`
	// FIN 标志位（结束连接）
	TCPBitFIN bool `gorm:"type:Bool" json:"bitFin"`
	// SYN 标志位（同步序列号）
	TCPBitSYN bool `gorm:"type:Bool" json:"bitSyn"`
	// RST 标志位（重置连接）
	TCPBitRST bool `gorm:"type:Bool" json:"bitRst"`
	// PSH 标志位（推送数据）
	TCPBitPSH bool `gorm:"type:Bool" json:"bitPsh"`
	// ACK 标志位（确认号有效）
	TCPBitACK bool `gorm:"type:Bool" json:"bitAck"`
	// URG 标志位（紧急指针有效）
	TCPBitURG bool `gorm:"type:Bool" json:"bitUrg"`
	// ECE 标志位（ECN 回显）
	TCPBitECE bool `gorm:"type:Bool" json:"bitEce"`
	// CWR 标志位（拥塞窗口减小）
	TCPBitCWR bool `gorm:"type:Bool" json:"bitCwr"`
	// NS 标志位（ECN nonce 隐蔽保护）
	TCPBitNS bool `gorm:"type:Bool" json:"bitNs"`
	// 窗口大小
	TCPWindow uint16 `gorm:"type:UInt16 CODEC(ZSTD(3))" json:"window"`
	// 校验和
	TCPChecksum uint16 `gorm:"type:UInt16 CODEC(ZSTD(3))" json:"checksum"`
	// 紧急指针
	TCPUrgent uint16 `gorm:"column:tcp_urgent;type:UInt16 CODEC(ZSTD(3))" json:"urgent"`
	// TCP 选项列表
	TCPOptions TCPOptions `gorm:"type:String CODEC(ZSTD(3))" json:"options"`
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

// PacketTCPFingerprint 表示单条 TCP 包记录上的被动指纹识别结果（p0f 风格）。
// 描述该数据包发送端（源地址）的 OS / MTU 等特征，由 pcap 导入阶段计算并落库。
type PacketTCPFingerprint struct {
	// 识别出的操作系统
	OS string `gorm:"column:fp_os;type:String CODEC(ZSTD(3))" json:"fpOs"`
	// 操作系统识别置信度
	OSQuality float64 `gorm:"column:fp_os_quality;type:Float64 CODEC(ZSTD(3))" json:"fpOsQuality"`
	// p0f 指纹签名
	Sig string `gorm:"column:fp_sig;type:String CODEC(ZSTD(3))" json:"fpSig"`
	// 最大传输单元
	MTU uint16 `gorm:"column:fp_mtu;type:UInt16 CODEC(ZSTD(3))" json:"fpMtu"`
}

// PacketIPv4TCP 表示IPv4 TCP数据包
// 组合了PacketIPv4和TCPHeaders，表示完整的IPv4 TCP数据包信息
// 包含以太网层、IPv4层和TCP层的所有头部信息
type PacketIPv4TCP struct {
	PacketIPv4           // 嵌入IPv4数据包基础结构（含 RowMeta 行唯一标识）
	TCPHeaders           // 嵌入TCP头部信息
	PacketTCPFingerprint // 嵌入TCP被动指纹信息
	// TCP 流标识符（双向一致）
	TCPStreamID string `gorm:"column:tcp_stream_id;type:FixedString(16) CODEC(ZSTD(3))" json:"tcpStreamID"`
	// 加密存储的 TCP 载荷
	Payload *serializer.Crypto `gorm:"type:String CODEC(ZSTD(3));serializer:crypto" json:"payload" crypto:"mode=\"aes-gcm\",offset=38278,length=32"`
}

// CaptureTimestamp 返回数据包的捕获时间戳
// 时间戳用于记录数据包的捕获时间，用于后续的分析和处理
func (p *PacketIPv4TCP) CaptureTimestamp() time.Time {
	return p.Timestamp
}

// IngestedAt 返回数据包的入库时间戳
// 入库时间标识数据包写入数据库的时间，用于分布式编排器推进消费水位
func (p *PacketIPv4TCP) IngestedAt() time.Time {
	return p.CreatedAt
}

// TcpFingerprint 返回数据包携带的被动 TCP 指纹。
// 指纹由 pcap 导入阶段对 SYN / SYN-ACK 握手包识别并落库，
// 组包阶段通过该方法透传给流重组器；无指纹时返回 nil。
func (p *PacketIPv4TCP) TcpFingerprint() *PacketTCPFingerprint {
	if p.OS == "" && p.Sig == "" {
		return nil
	}
	return &p.PacketTCPFingerprint
}

// StreamID 返回TCP流ID
// 通过 hash/maphash 对源IP、目的IP、源端口、目的端口计算64-bit哈希，生成唯一的流标识符
// 流ID用于识别属于同一TCP连接的所有数据包
// 返回值: 16字符的十六进制64-bit哈希字符串
func (p *PacketIPv4TCP) StreamID() string {
	leftPort, rightPort := uint16(p.TCPSrcPort), uint16(p.TCPDstPort)
	leftIP, rightIP := net.IP(p.IPv4Src).To4(), net.IP(p.IPv4Dst).To4()
	return computeStreamID(leftIP, rightIP, leftPort, rightPort)
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

// NetworkFrame 返回数据包的网络层帧信息
// 将数据包的网络层堆栈展平为NetworkStack记录列表，用于记录数据包的捕获上下文。
// 每条记录的 Seq、TraceID、Direction 与载荷字节偏移（PayloadStart/EndOffset）
// 由TCP流侧在数据包进入缓冲区时补充设置。
func (p *PacketIPv4TCP) NetworkFrame() NetworkFrame {
	return NewNetworkFrame(p.LayerStack, p.BaseMeta, p.Timestamp)
}

// GetPayload 返回TCP数据包的载荷数据
// 从加密存储的 Payload 字段中获取解密后的原始载荷字节
func (p *PacketIPv4TCP) GetPayload() []byte {
	if p.Payload == nil {
		return nil
	}
	return p.Payload.Value()
}

// PacketIPv6TCP 表示IPv6 TCP数据包
// 组合了PacketIPv6和TCPHeaders，表示完整的IPv6 TCP数据包信息
// 包含以太网层、IPv6层和TCP层的所有头部信息
type PacketIPv6TCP struct {
	// PacketIPv6 嵌入IPv6数据包基础结构（含经 PacketEthernet 继承的 RowMeta 行唯一标识）
	PacketIPv6
	TCPHeaders           // 嵌入TCP头部信息
	PacketTCPFingerprint // 嵌入TCP被动指纹信息
	// TCP 流标识符（双向一致）
	TCPStreamID string `gorm:"column:tcp_stream_id;type:FixedString(16) CODEC(ZSTD(3))" json:"tcpStreamID"`
	// 加密存储的 TCP 载荷
	Payload *serializer.Crypto `gorm:"type:String CODEC(ZSTD(3));serializer:crypto" json:"payload" crypto:"mode=\"aes-gcm\",offset=50359,length=32"`
}

// CaptureTimestamp 返回数据包的捕获时间戳
// 时间戳用于记录数据包的捕获时间，用于后续的分析和处理
func (p *PacketIPv6TCP) CaptureTimestamp() time.Time {
	return p.Timestamp
}

// IngestedAt 返回数据包的入库时间戳
// 入库时间标识数据包写入数据库的时间，用于分布式编排器推进消费水位
func (p *PacketIPv6TCP) IngestedAt() time.Time {
	return p.CreatedAt
}

// TcpFingerprint 返回数据包携带的被动 TCP 指纹。
// 指纹由 pcap 导入阶段对 SYN / SYN-ACK 握手包识别并落库，
// 组包阶段通过该方法透传给流重组器；无指纹时返回 nil。
func (p *PacketIPv6TCP) TcpFingerprint() *PacketTCPFingerprint {
	if p.OS == "" && p.Sig == "" {
		return nil
	}
	return &p.PacketTCPFingerprint
}

// StreamID 返回TCP流ID
// 通过计算源IPv6地址、目的IPv6地址、源端口、目的端口的哈希值生成唯一的流标识符
// 流ID用于识别属于同一TCP连接的所有数据包，支持IPv6地址
// 返回值: 16字符的十六进制64-bit哈希字符串
func (p *PacketIPv6TCP) StreamID() string {
	leftPort, rightPort := uint16(p.TCPSrcPort), uint16(p.TCPDstPort)
	leftIP, rightIP := net.IP(p.IPv6Src).To16(), net.IP(p.IPv6Dst).To16()
	return computeStreamID(leftIP, rightIP, leftPort, rightPort)
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

// NetworkFrame 返回数据包的网络层帧信息
// 将数据包的网络层堆栈展平为NetworkStack记录列表，用于记录数据包的捕获上下文。
// 每条记录的 Seq、TraceID、Direction 与载荷字节偏移（PayloadStart/EndOffset）
// 由TCP流侧在数据包进入缓冲区时补充设置。
func (p *PacketIPv6TCP) NetworkFrame() NetworkFrame {
	return NewNetworkFrame(p.LayerStack, p.BaseMeta, p.Timestamp)
}

// GetPayload 返回TCP数据包的载荷数据
// 从加密存储的 Payload 字段中获取解密后的原始载荷字节
func (p *PacketIPv6TCP) GetPayload() []byte {
	if p.Payload == nil {
		return nil
	}
	return p.Payload.Value()
}

// AppTailer 表示应用层的尾部信息
// 包含客户端和服务器端的已消耗字节数、数据偏移量、时间戳等信息
type AppTailer struct {
	// 客户端已消耗字节数
	ClientConsumed uint64 `gorm:"type:UInt64 CODEC(ZSTD(3))" json:"clientConsumed"`
	// 客户端起始载荷偏移
	ClientStartPayloadOffset uint64 `gorm:"type:UInt64 CODEC(ZSTD(3))" json:"clientStartPayloadOffset"`
	// 客户端结束载荷偏移
	ClientEndPayloadOffset uint64 `gorm:"type:UInt64 CODEC(ZSTD(3))" json:"clientEndPayloadOffset"`
	// 客户端起始时间戳
	ClientStartTimestamp time.Time `gorm:"type:DateTime64(6, 'UTC') CODEC(ZSTD(3))" json:"clientStartTimestamp"`
	// 客户端结束时间戳
	ClientEndTimestamp time.Time `gorm:"type:DateTime64(6, 'UTC') CODEC(ZSTD(3))" json:"clientEndTimestamp"`
	// 服务端已消耗字节数
	ServerConsumed uint64 `gorm:"type:UInt64 CODEC(ZSTD(3))" json:"serverConsumed"`
	// 服务端起始载荷偏移
	ServerStartPayloadOffset uint64 `gorm:"type:UInt64 CODEC(ZSTD(3))" json:"serverStartPayloadOffset"`
	// 服务端结束载荷偏移
	ServerEndPayloadOffset uint64 `gorm:"type:UInt64 CODEC(ZSTD(3))" json:"serverEndPayloadOffset"`
	// 服务端起始时间戳
	ServerStartTimestamp time.Time `gorm:"type:DateTime64(6, 'UTC') CODEC(ZSTD(3))" json:"serverStartTimestamp"`
	// 服务端结束时间戳
	ServerEndTimestamp time.Time `gorm:"type:DateTime64(6, 'UTC') CODEC(ZSTD(3))" json:"serverEndTimestamp"`
}
