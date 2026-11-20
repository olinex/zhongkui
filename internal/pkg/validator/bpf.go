package validator

import (
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcap"
)

// BPF 过滤器编译校验使用的快照长度，覆盖常见抓包配置。
const bpfSnapLen = 65535

// bpfLinkTypes 是提交校验时依次尝试的链路类型。
// 采集端可能运行在以太网、Linux cooked、loopback 或 802.11 设备上；
// 只按以太网编译会把其它链路类型的合法表达式（如 `wlan type mgt`）误判为非法。
// 任一链路类型编译成功即视为语法合法；实际设备不匹配会在 reporter 运行时暴露。
var bpfLinkTypes = []layers.LinkType{
	layers.LinkTypeEthernet,
	layers.LinkTypeLinuxSLL,
	layers.LinkTypeLinuxSLL2,
	layers.LinkTypeNull,
	layers.LinkTypeLoop,
	layers.LinkTypeIEEE802_11,
	layers.LinkTypeIEEE80211Radio,
}

// bpfLinkTypeIndependentErrors 是重试其它链路类型也不会改变的语义错误片段：
// 命中后立即判为非法，避免对含主机名的表达式重复触发名字解析（DNS/NSS）。
var bpfLinkTypeIndependentErrors = []string{
	"unknown host",
	"invalid IPv4 address",
	"illegal port number",
	"mask length must",
	"unknown port in range",
	"VLAN tag",
	"'gateway' not supported",
}

// IsValidBPF 判断 BPF 过滤表达式能否被 libpcap 编译。
// 依次尝试以太网 / Linux cooked / loopback / 802.11 等链路类型，任一成功即合法；
// 含主机名、网络名或服务名的表达式会在编译期触发一次名字解析。
func IsValidBPF(value string) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}
	for _, linkType := range bpfLinkTypes {
		instructions, err := pcap.CompileBPFFilter(linkType, bpfSnapLen, value)
		if err == nil && len(instructions) > 0 {
			return true
		}
		if isBPFLinkTypeIndependentError(err) {
			return false
		}
	}
	return false
}

// ValidateBPF 是 bpf 标签的验证函数：字段值须为可被 libpcap 编译的 BPF 过滤表达式。
func ValidateBPF(fl validator.FieldLevel) bool {
	v, ok := stringFieldValue(fl)
	if !ok {
		return false
	}
	return IsValidBPF(v)
}

// isBPFLinkTypeIndependentError 判断编译错误是否与链路类型无关。
// 这类错误重试其它链路类型也不会成功，提前返回可避免重复的名字解析。
func isBPFLinkTypeIndependentError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	for _, fragment := range bpfLinkTypeIndependentErrors {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}
