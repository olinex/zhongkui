package traffic

import (
	"time"

	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"gorm.io/gorm"
	"zhongkuisec.com/zhongkui/internal/manager/middlewares"
	"zhongkuisec.com/zhongkui/internal/manager/response"
	"zhongkuisec.com/zhongkui/internal/manager/views"
	"zhongkuisec.com/zhongkui/internal/models/network"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
	"zhongkuisec.com/zhongkui/internal/pkg/utils"
)

// TcpOptionOut 视图层 TCP 选项镜像结构（对应 network.TCPOption）。
// 独立定义以遵守视图层 In/Out 结构体不得引用模型层结构体的强约束，
// JSON 输出与模型保持一致的字段名与类型。
type TcpOptionOut struct {
	// Type 选项类型，如 MSS、窗口缩放因子、时间戳等
	Type layers.TCPOptionKind `json:"type"`
	// Length 选项总长度（包括类型和长度字段）
	Length uint8 `json:"length"`
	// Data 选项数据内容
	Data []byte `json:"data"`
}

// LayerInfoOut 视图层网络层信息镜像结构（对应 network.LayerInfo）。
// 独立定义以遵守视图层 In/Out 结构体不得引用模型层结构体的强约束。
type LayerInfoOut struct {
	// Level 协议层级（如链路层、网络层等）
	Level uint8 `json:"level"`
	// Proto 协议类型（如 Ethernet、IPv4、TCP 等）
	Proto gopacket.LayerType `json:"proto"`
	// Headers 协议头部信息，具体类型由 Proto 决定
	Headers any `json:"headers"`
}

// Ipv4TcpListGetListItemOut IPv4 TCP 包列表输出项
// 独立扁平结构体，逐一镜像 network.PacketIPv4TCP 的全部顶层 json 字段
// （payload 已转存 S3，单条返回字节量可控），BaseTotalOut 承接 COUNT(*) OVER() 的总数
type Ipv4TcpListGetListItemOut struct {
	// ID 行全局唯一标识，供行详情接口按 namespace + id 定位
	ID serializer.HexUUID `json:"id"`
	utils.BaseTotalOut
	// ---- BaseMeta ----
	// Namespace 命名空间
	Namespace string `json:"namespace"`
	// Capture 抓包器
	Capture string `json:"capture"`
	// Version 抓包器版本
	Version string `json:"version"`
	// SessionID 会话标识
	SessionID serializer.HexUUID `json:"sessionID"`
	// ---- TimestampMeta ----
	// Timestamp 采集时间
	Timestamp time.Time `json:"timestamp"`
	// CreatedAt 入库时间
	CreatedAt time.Time `json:"createdAt"`
	// ---- CaptureMeta ----
	// Truncated 是否截断
	Truncated bool `json:"truncated"`
	// CaptureLength 捕获长度
	CaptureLength uint32 `json:"captureLength"`
	// ---- PacketMeta ----
	// LayerStack 网络层堆栈信息
	LayerStack []LayerInfoOut `json:"layerStack"`
	// ---- EthernetHeaders ----
	// EthDstMAC 目的 MAC 地址
	EthDstMAC serializer.MacAddr `json:"ethDstMac"`
	// EthSrcMAC 源 MAC 地址
	EthSrcMAC serializer.MacAddr `json:"ethSrcMac"`
	// EthType 以太网类型
	EthType layers.EthernetType `json:"ethType"`
	// EthLength 帧长度
	EthLength uint16 `json:"ethLength"`
	// ---- IPv4Headers ----
	// IPv4Version 协议版本号
	IPv4Version uint8 `json:"ipv4Version"`
	// IPv4IHL 头部长度
	IPv4IHL uint8 `json:"ipv4Ihl"`
	// IPv4TOS 服务类型
	IPv4TOS uint8 `json:"ipv4Tos"`
	// IPv4Length 总长度
	IPv4Length uint16 `json:"ipv4Length"`
	// IPv4ID 标识符
	IPv4ID uint16 `json:"ipv4ID"`
	// IPv4Flags 标志位
	IPv4Flags layers.IPv4Flag `json:"ipv4Flags"`
	// IPv4FragOffset 分片偏移
	IPv4FragOffset uint16 `json:"ipv4FragOffset"`
	// IPv4TTL 生存时间
	IPv4TTL uint8 `json:"ipv4Ttl"`
	// IPv4IPProtocol 上层协议类型
	IPv4IPProtocol layers.IPProtocol `json:"ipv4IpProtocol"`
	// IPv4Checksum 头部校验和
	IPv4Checksum uint16 `json:"ipv4Checksum"`
	// IPv4Src 源 IP 地址
	IPv4Src serializer.IPv4Addr `json:"ipv4Src"`
	// IPv4Dst 目的 IP 地址
	IPv4Dst serializer.IPv4Addr `json:"ipv4Dst"`
	// IPv4Options IPv4 选项列表
	IPv4Options []layers.IPv4Option `json:"ipv4Options"`
	// IPv4Padding 填充字节
	IPv4Padding string `json:"ipv4Padding"`
	// ---- TCPHeaders ----
	// TCPSrcPort 源端口
	TCPSrcPort layers.TCPPort `json:"srcPort"`
	// TCPDstPort 目的端口
	TCPDstPort layers.TCPPort `json:"dstPort"`
	// TCPSeq 序列号
	TCPSeq uint32 `json:"seq"`
	// TCPAck 确认号
	TCPAck uint32 `json:"ack"`
	// TCPDataOffset 数据偏移
	TCPDataOffset uint8 `json:"dataOffset"`
	// TCPBitFIN FIN 标志位
	TCPBitFIN bool `json:"bitFin"`
	// TCPBitSYN SYN 标志位
	TCPBitSYN bool `json:"bitSyn"`
	// TCPBitRST RST 标志位
	TCPBitRST bool `json:"bitRst"`
	// TCPBitPSH PSH 标志位
	TCPBitPSH bool `json:"bitPsh"`
	// TCPBitACK ACK 标志位
	TCPBitACK bool `json:"bitAck"`
	// TCPBitURG URG 标志位
	TCPBitURG bool `json:"bitUrg"`
	// TCPBitECE ECE 标志位
	TCPBitECE bool `json:"bitEce"`
	// TCPBitCWR CWR 标志位
	TCPBitCWR bool `json:"bitCwr"`
	// TCPBitNS NS 标志位
	TCPBitNS bool `json:"bitNs"`
	// TCPWindow 窗口大小
	TCPWindow uint16 `json:"window"`
	// TCPChecksum 校验和
	TCPChecksum uint16 `json:"checksum"`
	// TCPUrgent 紧急指针
	TCPUrgent uint16 `json:"urgent"`
	// TCPOptions TCP 选项列表
	TCPOptions []TcpOptionOut `json:"options"`
	// ---- PacketTCPFingerprint ----
	// FpOs 客户端操作系统
	FpOs string `json:"fpOs"`
	// FpOsQuality 操作系统识别置信度
	FpOsQuality float64 `json:"fpOsQuality"`
	// FpSig TCP 签名
	FpSig string `json:"fpSig"`
	// FpMtu MTU 值
	FpMtu uint16 `json:"fpMtu"`
	// ---- 流与载荷 ----
	// TcpStreamID TCP 流标识
	TcpStreamID string `json:"tcpStreamID" form:"kind=hex"`
	// Payload 数据包载荷（base64 密文，明文经明文权限接口查看）
	Payload *serializer.Crypto `json:"payload" form:"kind=crypto"`
}

// Ipv4TcpListGet 处理 IPv4 TCP 包列表查询请求
// 通过 Ipv4TcpListGetSearchIn 应用过滤、排序与分页，查询 raw 库（ClickHouse）中的包数据，
// 以局部扫描容器承载 gorm 模型与 total 列，再映射为独立 Out 结构（含全部字段）
func Ipv4TcpListGet(c *gin.Context) {
	// 初始化搜索输入参数，设置默认值
	var in views.Ipv4TcpListGetSearchIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 绑定并验证 JSON 请求体
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}

	// 获取 raw 库（ClickHouse）连接并指定查询模型
	db := middlewares.GetRawDatabase(c).Model(&network.PacketIPv4TCP{})

	// 先以轻量 count 统计总数：ClickHouse 只扫过滤列与 granule 标记，
	// 避免 COUNT(*) OVER() 窗口函数在 LIMIT 前物化全部命中行的所有列（payload/layer_stack 等宽列）。
	var total int64
	if countQuery, err := utils.BuildSearchCondQuery(db, &in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else if err := countQuery.Count(&total).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 再查询当前页数据：ORDER BY timestamp 命中 MergeTree 排序键，可倒序读 + LIMIT 早停，
	// 仅物化当前页行；扫描容器与模型同型，避免 GORM 按目的地枚举额外列（如 total）。
	var rows []network.PacketIPv4TCP
	if dataQuery, err := utils.BuildSearchDataQuery(db, &in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else if err := dataQuery.Find(&rows).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 字段投影：按请求 fields 投影，未指定时回退全部白名单字段（封装于 utils.ProjectSearchItems）
	projected := utils.ProjectSearchItems(rows, &in, func(o *network.PacketIPv4TCP) any {
		return newIpv4TcpListGetListItemOut(o)
	})
	response.SimpleOK.AbortWithGinW(c, utils.NewSearchProjectedOut(uint(total), projected))
}

// newIpv4TcpListGetListItemOut 将 network.PacketIPv4TCP 映射为独立 Out 结构。
// 模型层专属类型（LayerStack/TCPOptions/IPv4Options）在此转换为视图层等价类型。
func newIpv4TcpListGetListItemOut(p *network.PacketIPv4TCP) *Ipv4TcpListGetListItemOut {
	return &Ipv4TcpListGetListItemOut{
		// RowMeta
		ID: p.ID,
		// BaseMeta
		Namespace: p.Namespace,
		Capture:   p.Capture,
		Version:   p.Version,
		SessionID: p.SessionID,
		// TimestampMeta
		Timestamp: p.Timestamp,
		CreatedAt: p.CreatedAt,
		// CaptureMeta
		Truncated:     p.Truncated,
		CaptureLength: p.CaptureLength,
		// PacketMeta
		LayerStack: toLayerInfoOuts(p.LayerStack),
		// EthernetHeaders
		EthDstMAC: p.EthDstMAC,
		EthSrcMAC: p.EthSrcMAC,
		EthType:   p.EthType,
		EthLength: p.EthLength,
		// IPv4Headers
		IPv4Version:    p.IPv4Version,
		IPv4IHL:        p.IPv4IHL,
		IPv4TOS:        p.IPv4TOS,
		IPv4Length:     p.IPv4Length,
		IPv4ID:         p.IPv4ID,
		IPv4Flags:      p.IPv4Flags,
		IPv4FragOffset: p.IPv4FragOffset,
		IPv4TTL:        p.IPv4TTL,
		IPv4IPProtocol: p.IPv4IPProtocol,
		IPv4Checksum:   p.IPv4Checksum,
		IPv4Src:        p.IPv4Src,
		IPv4Dst:        p.IPv4Dst,
		IPv4Options:    []layers.IPv4Option(p.IPv4Options),
		IPv4Padding:    p.IPv4Padding,
		// TCPHeaders
		TCPSrcPort:    p.TCPSrcPort,
		TCPDstPort:    p.TCPDstPort,
		TCPSeq:        p.TCPSeq,
		TCPAck:        p.TCPAck,
		TCPDataOffset: p.TCPDataOffset,
		TCPBitFIN:     p.TCPBitFIN,
		TCPBitSYN:     p.TCPBitSYN,
		TCPBitRST:     p.TCPBitRST,
		TCPBitPSH:     p.TCPBitPSH,
		TCPBitACK:     p.TCPBitACK,
		TCPBitURG:     p.TCPBitURG,
		TCPBitECE:     p.TCPBitECE,
		TCPBitCWR:     p.TCPBitCWR,
		TCPBitNS:      p.TCPBitNS,
		TCPWindow:     p.TCPWindow,
		TCPChecksum:   p.TCPChecksum,
		TCPUrgent:     p.TCPUrgent,
		TCPOptions:    toTcpOptionOuts(p.TCPOptions),
		// PacketTCPFingerprint
		FpOs:        p.OS,
		FpOsQuality: p.OSQuality,
		FpSig:       p.Sig,
		FpMtu:       p.MTU,
		// 流与载荷
		TcpStreamID: p.TCPStreamID,
		Payload:     p.Payload,
	}
}

// toTcpOptionOuts 将模型层 TCP 选项列表转换为视图层镜像列表。
func toTcpOptionOuts(options network.TCPOptions) []TcpOptionOut {
	if options == nil {
		return nil
	}
	out := make([]TcpOptionOut, 0, len(options))
	for _, o := range options {
		out = append(out, TcpOptionOut{Type: o.Type, Length: o.Length, Data: o.Data})
	}
	return out
}

// toLayerInfoOuts 将模型层网络层堆栈转换为视图层镜像列表。
func toLayerInfoOuts(stack network.LayerStack) []LayerInfoOut {
	if stack == nil {
		return nil
	}
	out := make([]LayerInfoOut, 0, len(stack))
	for _, li := range stack {
		if li == nil {
			continue
		}
		out = append(out, LayerInfoOut{Level: uint8(li.Level), Proto: li.Proto, Headers: li.Headers})
	}
	return out
}

// Ipv4TcpDetailGetIn IPv4 TCP 包详情查询入参
// 按命名空间 + 行唯一 ID 精确查询单条 IPv4 TCP 包记录
type Ipv4TcpDetailGetIn struct {
	// Namespace 命名空间
	Namespace string `json:"namespace" binding:"required"`
	// ID 行全局唯一标识
	ID serializer.HexUUID `json:"id" binding:"required"`
}

// Ipv4TcpDetailGet 处理 IPv4 TCP 包详情查询请求
// 以 namespace + id 精确定位单条记录；列表投影后行详情/收藏样例/堆栈下钻等
// 消费方经此接口补齐全量字段
func Ipv4TcpDetailGet(c *gin.Context) {
	var in Ipv4TcpDetailGetIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	db := middlewares.GetRawDatabase(c).Model(&network.PacketIPv4TCP{})
	var row network.PacketIPv4TCP
	if err := db.Where("namespace = ? AND id = ?", in.Namespace, in.ID).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinW(c, newIpv4TcpListGetListItemOut(&row))
}
