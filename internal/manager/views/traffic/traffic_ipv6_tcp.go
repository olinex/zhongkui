package traffic

import (
	"time"

	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"
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

// Ipv6TcpListGetListItemOut IPv6 TCP 包列表输出项
// 独立扁平结构体，逐一镜像 network.PacketIPv6TCP 的全部顶层 json 字段
// （payload 已转存 S3，单条返回字节量可控），BaseTotalOut 承接 COUNT(*) OVER() 的总数
type Ipv6TcpListGetListItemOut struct {
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
	// ---- IPv6Base ----
	// IPv6Version 协议版本号
	IPv6Version uint8 `json:"ipv6Version"`
	// IPv6TrafficClass 流量类别
	IPv6TrafficClass uint8 `json:"ipv6TrafficClass"`
	// IPv6FlowLabel 流标签
	IPv6FlowLabel uint32 `json:"ipv6FlowLabel"`
	// IPv6Length 有效载荷长度
	IPv6Length uint16 `json:"ipv6Length"`
	// IPv6NextHeader 下一个头部类型
	IPv6NextHeader layers.IPProtocol `json:"ipv6NextHeader"`
	// IPv6HopLimit 跳数限制
	IPv6HopLimit uint8 `json:"ipv6HopLimit"`
	// IPv6Src 源 IPv6 地址
	IPv6Src serializer.IPv6Addr `json:"ipv6Src"`
	// IPv6Dst 目的 IPv6 地址
	IPv6Dst serializer.IPv6Addr `json:"ipv6Dst"`
	// ---- IPv6Headers ----
	// IPv6LastHeader 最后一个头部类型
	IPv6LastHeader layers.IPProtocol `json:"ipv6LastHeader"`
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
	TcpStreamID string `json:"tcpStreamID"`
	// Payload 数据包载荷（base64 密文，明文经明文权限接口查看）
	Payload *serializer.Crypto `json:"payload" form:"kind:crypto"`
}

// Ipv6TcpListGet 处理 IPv6 TCP 包列表查询请求
// 通过 Ipv6TcpListGetSearchIn 应用过滤、排序与分页，查询 raw 库（ClickHouse）中的包数据，
// 以局部扫描容器承载 gorm 模型与 total 列，再映射为独立 Out 结构（含全部字段）
func Ipv6TcpListGet(c *gin.Context) {
	// 初始化搜索输入参数，设置默认值
	var in views.Ipv6TcpListGetSearchIn
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
	db := middlewares.GetRawDatabase(c).Model(&network.PacketIPv6TCP{})

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
	var rows []network.PacketIPv6TCP
	if dataQuery, err := utils.BuildSearchDataQuery(db, &in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else if err := dataQuery.Find(&rows).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 字段投影：按请求 fields 投影，未指定时回退全部白名单字段（封装于 utils.ProjectSearchItems）
	projected := utils.ProjectSearchItems(rows, &in, func(o *network.PacketIPv6TCP) any {
		return newIpv6TcpListGetListItemOut(o)
	})
	response.SimpleOK.AbortWithGinW(c, utils.NewSearchProjectedOut(uint(total), projected))
}

// newIpv6TcpListGetListItemOut 将 network.PacketIPv6TCP 映射为独立 Out 结构。
// 模型层专属类型（LayerStack/TCPOptions）在此转换为视图层等价类型。
func newIpv6TcpListGetListItemOut(p *network.PacketIPv6TCP) *Ipv6TcpListGetListItemOut {
	return &Ipv6TcpListGetListItemOut{
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
		// IPv6Base
		IPv6Version:      p.IPv6Version,
		IPv6TrafficClass: p.IPv6TrafficClass,
		IPv6FlowLabel:    p.IPv6FlowLabel,
		IPv6Length:       p.IPv6Length,
		IPv6NextHeader:   p.IPv6NextHeader,
		IPv6HopLimit:     p.IPv6HopLimit,
		IPv6Src:          p.IPv6Src,
		IPv6Dst:          p.IPv6Dst,
		// IPv6Headers
		IPv6LastHeader: p.IPv6LastHeader,
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

// Ipv6TcpDetailGetIn IPv6 TCP 包详情查询入参
// 按命名空间 + 行唯一 ID 精确查询单条 IPv6 TCP 包记录
type Ipv6TcpDetailGetIn struct {
	// Namespace 命名空间
	Namespace string `json:"namespace" binding:"required"`
	// ID 行全局唯一标识
	ID serializer.HexUUID `json:"id" binding:"required"`
}

// Ipv6TcpDetailGet 处理 IPv6 TCP 包详情查询请求
// 以 namespace + id 精确定位单条记录；列表投影后行详情/收藏样例/堆栈下钻等
// 消费方经此接口补齐全量字段
func Ipv6TcpDetailGet(c *gin.Context) {
	var in Ipv6TcpDetailGetIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	db := middlewares.GetRawDatabase(c).Model(&network.PacketIPv6TCP{})
	var row network.PacketIPv6TCP
	if err := db.Where("namespace = ? AND id = ?", in.Namespace, in.ID).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinW(c, newIpv6TcpListGetListItemOut(&row))
}
