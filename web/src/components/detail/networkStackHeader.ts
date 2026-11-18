// 网络堆栈各层头部 JSON 的可读化元数据与格式化工具。
// 原始 headers JSON 字段名是 gopacket 技术键（如 ipv4Ttl、bitSyn），
// 本模块按协议（LayerType proto）声明字段顺序、值类型与摘要组成，
// 供 NetworkStackHeader.vue 渲染“字段名 + 可读值 + 说明”表格与一行摘要。

import i18n from "@/utils/i18n";

// ---- 专有名词常量表（协议/选项/端口名不随语言变化） ----

// IP 协议号 → 协议名（gopacket layers.IPProtocol，常见取值）
const IP_PROTO_NAMES: Record<number, string> = {
  1: "ICMP",
  2: "IGMP",
  6: "TCP",
  17: "UDP",
  41: "IPv6",
  47: "GRE",
  50: "ESP",
  51: "AH",
  58: "ICMPv6",
  89: "OSPF",
  132: "SCTP",
};

// 以太网类型（EthernetType，十进制）→ 协议名
const ETH_TYPE_NAMES: Record<number, string> = {
  0x0800: "IPv4",
  0x0806: "ARP",
  0x86dd: "IPv6",
  0x8100: "VLAN",
  0x8847: "MPLS",
  0x88a8: "QinQ",
  0x8863: "PPPoE Discovery",
  0x8864: "PPPoE Session",
  0x88cc: "LLDP",
};

// TCP 选项类型号 → 选项名（gopacket layers.TCPOptionKind，常见取值）
const TCP_OPTION_NAMES: Record<number, string> = {
  0: "EndList",
  1: "Nop",
  2: "MSS",
  3: "WindowScale",
  4: "SACK-Permitted",
  5: "SACK",
  8: "Timestamps",
};

// 常见知名端口 → 服务名
const WELL_KNOWN_PORTS: Record<number, string> = {
  21: "FTP",
  22: "SSH",
  23: "Telnet",
  25: "SMTP",
  53: "DNS",
  67: "DHCP",
  80: "HTTP",
  110: "POP3",
  143: "IMAP",
  443: "HTTPS",
  3306: "MySQL",
  5432: "PostgreSQL",
  6379: "Redis",
  8080: "HTTP-Alt",
};

// IPv4 分片标志位（gopacket layers/ip4.go：IPv4EvilBit=1<<2、IPv4DontFragment=1<<1、IPv4MoreFragments=1<<0）
const IPv4_FLAG_BITS: Array<[number, string]> = [
  [1 << 2, "model.networkStack.header.flag.evil"],
  [1 << 1, "model.networkStack.header.flag.df"],
  [1 << 0, "model.networkStack.header.flag.mf"],
];

// 802.11 类型/子类型（gopacket layers.Dot11Type）→ 名称
const DOT11_TYPE_NAMES: Record<number, string> = {
  0x00: "Association Request",
  0x04: "Association Response",
  0x08: "Reassociation Request",
  0x0c: "Reassociation Response",
  0x10: "Probe Request",
  0x14: "Probe Response",
  0x18: "Measurement Pilot",
  0x20: "Beacon",
  0x24: "ATIM",
  0x28: "Disassociation",
  0x2c: "Authentication",
  0x30: "Deauthentication",
  0x34: "Action",
  0x38: "Action No Ack",
  0x1d: "Control Wrapper",
  0x21: "Block Ack Request",
  0x25: "Block Ack",
  0x29: "PS-Poll",
  0x2d: "RTS",
  0x31: "CTS",
  0x35: "Ack",
  0x39: "CF-End",
  0x3d: "CF-End + CF-Ack",
  0x06: "Data CF-Ack",
  0x0a: "Data CF-Poll",
  0x0e: "Data CF-Ack + CF-Poll",
  0x12: "Data Null",
  0x16: "CF-Ack No Data",
  0x1a: "CF-Poll No Data",
  0x1e: "CF-Ack + CF-Poll No Data",
  0x22: "QoS Data",
  0x26: "QoS Data + CF-Ack",
  0x2a: "QoS Data + CF-Poll",
  0x2e: "QoS Data + CF-Ack + CF-Poll",
  0x32: "QoS Null",
  0x3a: "QoS CF-Poll No Data",
  0x3e: "QoS CF-Ack + CF-Poll No Data",
};

// 802.11 帧控制标志位（gopacket layers.Dot11Flags）→ i18n 键
const DOT11_FLAG_BITS: Array<[number, string]> = [
  [1 << 0, "model.networkStack.header.flag.toDs"],
  [1 << 1, "model.networkStack.header.flag.fromDs"],
  [1 << 2, "model.networkStack.header.flag.moreFrag"],
  [1 << 3, "model.networkStack.header.flag.retry"],
  [1 << 4, "model.networkStack.header.flag.powerManagement"],
  [1 << 5, "model.networkStack.header.flag.moreData"],
  [1 << 6, "model.networkStack.header.flag.protected"],
  [1 << 7, "model.networkStack.header.flag.order"],
];

// 802.11 QoS 确认策略（gopacket layers.Dot11AckPolicy）→ i18n 键
const DOT11_ACK_POLICY_KEYS: Record<number, string> = {
  0: "model.networkStack.header.dot11AckPolicy.normal",
  1: "model.networkStack.header.dot11AckPolicy.none",
  2: "model.networkStack.header.dot11AckPolicy.noExplicit",
  3: "model.networkStack.header.dot11AckPolicy.block",
};

// Linux SLL / SLL2 数据包类型（gopacket LinuxSLLPacketType / LinuxSLL2PacketType）→ i18n 键
const SLL_PACKET_TYPE_KEYS: Record<number, string> = {
  0: "model.networkStack.header.sllPacketType.host",
  1: "model.networkStack.header.sllPacketType.broadcast",
  2: "model.networkStack.header.sllPacketType.multicast",
  3: "model.networkStack.header.sllPacketType.otherhost",
  4: "model.networkStack.header.sllPacketType.outgoing",
  5: "model.networkStack.header.sllPacketType.loopback",
  6: "model.networkStack.header.sllPacketType.fastroute",
};

// ARP 硬件类型（gopacket layers.ARPHardwareType）→ 名称
const ARP_HARDWARE_TYPE_NAMES: Record<number, string> = {
  1: "Ethernet",
  770: "Frame Relay Access Device",
  772: "Loopback",
  778: "GRE over IP",
  803: "IEEE 802.11 + radiotap",
};

// PPP 协议类型（gopacket layers.PPPType 及常见控制协议）→ 名称
const PPP_TYPE_NAMES: Record<number, string> = {
  0x0021: "IPv4",
  0x0057: "IPv6",
  0x0281: "MPLS Unicast",
  0x0283: "MPLS Multicast",
  0xc021: "LCP",
  0x8021: "IPCP",
  0xc023: "PAP",
  0xc223: "CHAP",
};

// PPPoE 消息码（gopacket layers.PPPoECode）→ 名称
const PPPOE_CODE_NAMES: Record<number, string> = {
  0x00: "Session",
  0x09: "PADI",
  0x07: "PADO",
  0x19: "PADR",
  0x65: "PADS",
  0xa7: "PADT",
};

// 字段值类型决定详情表格中“值”列的格式化方式
export type HeaderValueKind =
  | "mac"
  | "ipv4"
  | "ipv6"
  | "ipv4Flags"
  | "ipProto"
  | "ethType"
  | "port"
  | "bool"
  | "tcpOptions"
  | "ipv4Options"
  | "hex"
  | "bytes"
  | "json"
  | "dot11Type"
  | "dot11Flags"
  | "dot11AckPolicy"
  | "sllPacketType"
  | "arpHardwareType"
  | "pppType"
  | "pppoeCode"
  | "raw";

// HeaderFieldMeta 描述单个头部字段的元数据：技术键名与值类型。
export interface HeaderFieldMeta {
  // headers JSON 中的技术键名
  key: string;
  kind: HeaderValueKind;
  // i18n 键段覆盖：多个协议共用同名字段（如 length/type/code）时指向专用文案，
  // 缺省时直接使用 key，即 model.networkStack.header.field.<key> / desc.<key>
  i18nKey?: string;
}

// 按 LayerType（proto）声明各层头部字段的展示顺序与值类型；
// Loopback（54）无真实链路层信息（eth 字段全空），不逐字段展示
export const HEADER_FIELDS: Record<number, HeaderFieldMeta[]> = {
  // 以太网
  17: [
    { key: "ethDstMac", kind: "mac" },
    { key: "ethSrcMac", kind: "mac" },
    { key: "ethType", kind: "ethType" },
    { key: "ethLength", kind: "raw" },
  ],
  // 802.1Q / QinQ VLAN 标签
  15: [
    { key: "priority", kind: "raw" },
    { key: "dropEligible", kind: "bool" },
    { key: "vlanIdentifier", kind: "raw" },
    { key: "type", kind: "ethType", i18nKey: "ethType" },
  ],
  // IEEE 802.2 LLC
  22: [
    { key: "dsap", kind: "hex" },
    { key: "ig", kind: "bool" },
    { key: "ssap", kind: "hex" },
    { key: "cr", kind: "bool" },
    { key: "control", kind: "hex" },
  ],
  // LLC/SNAP 扩展
  23: [
    { key: "organizationalCode", kind: "raw" },
    { key: "type", kind: "ethType", i18nKey: "ethType" },
  ],
  // PPP
  25: [
    { key: "pppType", kind: "pppType" },
    { key: "pppHasPPTPHeader", kind: "bool" },
  ],
  // PPPoE
  26: [
    { key: "version", kind: "raw", i18nKey: "pppoeVersion" },
    { key: "type", kind: "raw", i18nKey: "pppoeType" },
    { key: "code", kind: "pppoeCode", i18nKey: "pppoeCode" },
    { key: "sessionID", kind: "raw" },
    { key: "length", kind: "raw", i18nKey: "pppoeLength" },
  ],
  // RadioTap 无线抓包元数据
  64: [
    { key: "version", kind: "raw", i18nKey: "radioTapVersion" },
    { key: "length", kind: "raw", i18nKey: "radioTapLength" },
  ],
  // IEEE 802.11
  65: [
    { key: "dot11Type", kind: "dot11Type" },
    { key: "dot11Proto", kind: "raw" },
    { key: "dot11Flags", kind: "dot11Flags" },
    { key: "dot11DurationID", kind: "raw" },
    { key: "dot11Address1", kind: "mac" },
    { key: "dot11Address2", kind: "mac" },
    { key: "dot11Address3", kind: "mac" },
    { key: "dot11Address4", kind: "mac" },
    { key: "dot11SequenceNumber", kind: "raw" },
    { key: "dot11FragmentNumber", kind: "raw" },
    { key: "dot11HasQOS", kind: "bool" },
    { key: "dot11QOSTID", kind: "raw" },
    { key: "dot11QOSEOSP", kind: "bool" },
    { key: "dot11QOSAckPolicy", kind: "dot11AckPolicy" },
    { key: "dot11QOSTXOP", kind: "raw" },
  ],
  // Linux SLL
  113: [
    { key: "sllPacketType", kind: "sllPacketType" },
    { key: "sllAddrLen", kind: "raw" },
    { key: "sllAddr", kind: "mac" },
    { key: "sllAddrType", kind: "raw" },
    { key: "sllProtocol", kind: "ethType", i18nKey: "ethType" },
  ],
  // Prism 无线抓包元数据
  115: [
    { key: "code", kind: "raw", i18nKey: "prismCode" },
    { key: "length", kind: "raw", i18nKey: "prismLength" },
    { key: "deviceName", kind: "raw" },
  ],
  // Linux SLL2
  276: [
    { key: "sll2ProtocolType", kind: "ethType", i18nKey: "ethType" },
    { key: "sll2InterfaceIndex", kind: "raw" },
    { key: "sll2ArpHardwareType", kind: "arpHardwareType" },
    { key: "sll2PacketType", kind: "sllPacketType" },
    { key: "sll2AddrLength", kind: "raw" },
    { key: "sll2Addr", kind: "mac" },
  ],
  // IPv4
  20: [
    { key: "ipv4Version", kind: "raw" },
    { key: "ipv4Ihl", kind: "raw" },
    { key: "ipv4Tos", kind: "raw" },
    { key: "ipv4Length", kind: "raw" },
    { key: "ipv4ID", kind: "raw" },
    { key: "ipv4Flags", kind: "ipv4Flags" },
    { key: "ipv4FragOffset", kind: "raw" },
    { key: "ipv4Ttl", kind: "raw" },
    { key: "ipv4IpProtocol", kind: "ipProto" },
    { key: "ipv4Checksum", kind: "hex" },
    { key: "ipv4Src", kind: "ipv4" },
    { key: "ipv4Dst", kind: "ipv4" },
    { key: "ipv4Options", kind: "ipv4Options" },
    { key: "ipv4Padding", kind: "bytes" },
  ],
  // IPv6
  21: [
    { key: "ipv6Version", kind: "raw" },
    { key: "ipv6TrafficClass", kind: "raw" },
    { key: "ipv6FlowLabel", kind: "raw" },
    { key: "ipv6Length", kind: "raw" },
    { key: "ipv6NextHeader", kind: "ipProto" },
    { key: "ipv6HopLimit", kind: "raw" },
    { key: "ipv6Src", kind: "ipv6" },
    { key: "ipv6Dst", kind: "ipv6" },
    { key: "ipv6HopByHop", kind: "json" },
    { key: "ipv6LastHeader", kind: "ipProto" },
  ],
  // TCP
  44: [
    { key: "srcPort", kind: "port" },
    { key: "dstPort", kind: "port" },
    { key: "seq", kind: "raw" },
    { key: "ack", kind: "raw" },
    { key: "dataOffset", kind: "raw" },
    { key: "bitFin", kind: "bool" },
    { key: "bitSyn", kind: "bool" },
    { key: "bitRst", kind: "bool" },
    { key: "bitPsh", kind: "bool" },
    { key: "bitAck", kind: "bool" },
    { key: "bitUrg", kind: "bool" },
    { key: "bitEce", kind: "bool" },
    { key: "bitCwr", kind: "bool" },
    { key: "bitNs", kind: "bool" },
    { key: "window", kind: "raw" },
    { key: "checksum", kind: "hex" },
    { key: "urgent", kind: "raw" },
    { key: "options", kind: "tcpOptions" },
  ],
  // UDP
  45: [
    { key: "srcPort", kind: "port" },
    { key: "dstPort", kind: "port" },
    { key: "length", kind: "raw" },
    { key: "checksum", kind: "hex" },
  ],
};

const t = (key: string): string => i18n.global.t(key);

const none = (): string => t("model.networkStack.header.value.none");

// formatHeaderValue 按字段值类型将原始值格式化为可读文本（含 i18n 翻译）
export const formatHeaderValue = (kind: HeaderValueKind, value: unknown): string => {
  switch (kind) {
    case "mac":
      return typeof value === "string" && value !== "" ? value : none();
    case "ipv4":
    case "ipv6":
      return typeof value === "string" && value !== "" ? value : none();
    case "ipv4Flags":
      return formatIpv4Flags(Number(value ?? 0));
    case "ipProto":
      return formatIpProto(Number(value ?? 0));
    case "ethType":
      return formatEthType(Number(value ?? 0));
    case "port":
      return formatPort(Number(value ?? 0));
    case "bool":
      return value
        ? t("model.networkStack.header.value.yes")
        : t("model.networkStack.header.value.no");
    case "tcpOptions":
      return formatTcpOptions(value);
    case "ipv4Options":
      return formatIpv4Options(value);
    case "hex":
      return formatHex(Number(value ?? 0));
    case "bytes":
      return formatBytes(value);
    case "json":
      return value != null ? JSON.stringify(value) : none();
    case "dot11Type":
      return formatDot11Type(Number(value ?? 0));
    case "dot11Flags":
      return formatDot11Flags(Number(value ?? 0));
    case "dot11AckPolicy":
      return formatDot11AckPolicy(Number(value ?? 0));
    case "sllPacketType":
      return formatSllPacketType(Number(value ?? 0));
    case "arpHardwareType":
      return formatNamedValue(Number(value ?? 0), ARP_HARDWARE_TYPE_NAMES);
    case "pppType":
      return formatNamedValue(Number(value ?? 0), PPP_TYPE_NAMES, 4);
    case "pppoeCode":
      return formatNamedValue(Number(value ?? 0), PPPOE_CODE_NAMES, 2);
    default:
      return value == null || value === "" ? none() : String(value);
  }
};

// buildHeaderSummary 为已知协议的层生成一行可读摘要；
// 未知协议返回 null，由调用方回退到原始 JSON 展示
export const buildHeaderSummary = (
  proto: number,
  parsed: Record<string, unknown>,
): { primary: string; secondary: string } | null => {
  const str = (v: unknown): string => (typeof v === "string" && v !== "" ? v : "?");
  switch (proto) {
    case 54:
      return {
        primary: "Loopback",
        secondary: t("model.networkStack.header.loopbackNote"),
      };
    case 17:
      return {
        primary: `${str(parsed.ethSrcMac)} → ${str(parsed.ethDstMac)}`,
        secondary: `type=${formatEthType(Number(parsed.ethType ?? 0))}`,
      };
    case 20: {
      const parts = [
        formatIpProto(Number(parsed.ipv4IpProtocol ?? 0)),
        `TTL ${Number(parsed.ipv4Ttl ?? 0)}`,
        formatIpv4FlagsShort(Number(parsed.ipv4Flags ?? 0)),
        `len=${Number(parsed.ipv4Length ?? 0)}`,
      ];
      return {
        primary: `${str(parsed.ipv4Src)} → ${str(parsed.ipv4Dst)}`,
        secondary: parts.filter((p) => p !== "").join(" · "),
      };
    }
    case 21: {
      const parts = [
        formatIpProto(Number(parsed.ipv6NextHeader ?? 0)),
        `HL ${Number(parsed.ipv6HopLimit ?? 0)}`,
        `len=${Number(parsed.ipv6Length ?? 0)}`,
      ];
      return {
        primary: `${str(parsed.ipv6Src)} → ${str(parsed.ipv6Dst)}`,
        secondary: parts.filter((p) => p !== "").join(" · "),
      };
    }
    case 44: {
      const flags: string[] = [];
      for (const key of [
        "bitSyn",
        "bitAck",
        "bitFin",
        "bitRst",
        "bitPsh",
        "bitUrg",
        "bitEce",
        "bitCwr",
        "bitNs",
      ]) {
        if (parsed[key]) {
          flags.push(key.slice(3).toUpperCase());
        }
      }
      const parts = [
        flags.join(" "),
        `seq=${Number(parsed.seq ?? 0)}`,
        `win=${Number(parsed.window ?? 0)}`,
      ];
      return {
        primary: `${String(parsed.srcPort ?? "?")} → ${String(parsed.dstPort ?? "?")}`,
        secondary: parts.filter((p) => p !== "").join(" · "),
      };
    }
    case 45:
      return {
        primary: `${String(parsed.srcPort ?? "?")} → ${String(parsed.dstPort ?? "?")}`,
        secondary: `len=${Number(parsed.length ?? 0)}`,
      };
    case 15:
      return {
        primary: `VLAN ${Number(parsed.vlanIdentifier ?? 0)}`,
        secondary: [
          `PCP ${Number(parsed.priority ?? 0)}`,
          `DEI ${parsed.dropEligible ? t("model.networkStack.header.value.yes") : t("model.networkStack.header.value.no")}`,
          `type=${formatEthType(Number(parsed.type ?? 0))}`,
        ].join(" · "),
      };
    case 22:
      return {
        primary: `DSAP ${formatHex(Number(parsed.dsap ?? 0))} → SSAP ${formatHex(Number(parsed.ssap ?? 0))}`,
        secondary: `control=${formatHex(Number(parsed.control ?? 0))}`,
      };
    case 23:
      return {
        primary: `OUI ${str(parsed.organizationalCode)}`,
        secondary: `type=${formatEthType(Number(parsed.type ?? 0))}`,
      };
    case 25:
      return {
        primary: `PPP ${formatNamedValue(Number(parsed.pppType ?? 0), PPP_TYPE_NAMES, 4)}`,
        secondary: `HDLC ${parsed.pppHasPPTPHeader ? t("model.networkStack.header.value.yes") : t("model.networkStack.header.value.no")}`,
      };
    case 26:
      return {
        primary: formatNamedValue(Number(parsed.code ?? 0), PPPOE_CODE_NAMES, 2),
        secondary: `session=${Number(parsed.sessionID ?? 0)} · len=${Number(parsed.length ?? 0)}`,
      };
    case 64:
      return {
        primary: "RadioTap",
        secondary: `v${Number(parsed.version ?? 0)} · len=${Number(parsed.length ?? 0)}`,
      };
    case 65:
      return {
        primary: `${str(parsed.dot11Address2)} → ${str(parsed.dot11Address1)}`,
        secondary: [
          formatDot11Type(Number(parsed.dot11Type ?? 0)),
          `seq=${Number(parsed.dot11SequenceNumber ?? 0)}`,
          formatDot11FlagsShort(Number(parsed.dot11Flags ?? 0)),
        ]
          .filter((p) => p !== "")
          .join(" · "),
      };
    case 113:
      return {
        primary: str(parsed.sllAddr),
        secondary: [
          formatSllPacketType(Number(parsed.sllPacketType ?? 0)),
          `type=${formatEthType(Number(parsed.sllProtocol ?? 0))}`,
        ].join(" · "),
      };
    case 115:
      return {
        primary: `Prism ${str(parsed.deviceName)}`,
        secondary: `code=${Number(parsed.code ?? 0)} · len=${Number(parsed.length ?? 0)}`,
      };
    case 276:
      return {
        primary: str(parsed.sll2Addr),
        secondary: [
          formatSllPacketType(Number(parsed.sll2PacketType ?? 0)),
          `ifindex=${Number(parsed.sll2InterfaceIndex ?? 0)}`,
          `type=${formatEthType(Number(parsed.sll2ProtocolType ?? 0))}`,
        ].join(" · "),
      };
    default:
      return null;
  }
};

const formatIpv4Flags = (flags: number): string => {
  if (!flags) {
    return none();
  }
  const names: string[] = [];
  for (const [bit, key] of IPv4_FLAG_BITS) {
    if ((flags & bit) === bit) {
      names.push(t(key));
    }
  }
  return names.join(" | ");
};

// formatIpv4FlagsShort 摘要用：无标志位时返回空串，避免在摘要里出现“无”
const formatIpv4FlagsShort = (flags: number): string => {
  return flags ? formatIpv4Flags(flags) : "";
};

// formatDot11Type 输出 802.11 类型/子类型：0x20 (Beacon)
const formatDot11Type = (v: number): string => {
  const name = DOT11_TYPE_NAMES[v];
  const hex = `0x${v.toString(16).toUpperCase().padStart(2, "0")}`;
  return name ? `${hex} (${name})` : hex;
};

// formatDot11Flags 输出置位的 802.11 帧控制标志；无标志位时返回“无”
const formatDot11Flags = (flags: number): string => {
  if (!flags) {
    return none();
  }
  const names: string[] = [];
  for (const [bit, key] of DOT11_FLAG_BITS) {
    if ((flags & bit) === bit) {
      names.push(t(key));
    }
  }
  return names.join(" | ");
};

// formatDot11FlagsShort 摘要用：无标志位时返回空串，避免在摘要里出现“无”
const formatDot11FlagsShort = (flags: number): string => {
  return flags ? formatDot11Flags(flags) : "";
};

// formatDot11AckPolicy 输出 802.11 QoS 确认策略
const formatDot11AckPolicy = (v: number): string => {
  const key = DOT11_ACK_POLICY_KEYS[v];
  return key ? t(key) : String(v);
};

// formatSllPacketType 输出 Linux SLL / SLL2 数据包类型
const formatSllPacketType = (v: number): string => {
  const key = SLL_PACKET_TYPE_KEYS[v];
  return key ? t(key) : String(v);
};

// formatNamedValue 输出“数值 (名称)”；hexWidth 大于 0 时按十六进制展示数值
const formatNamedValue = (v: number, names: Record<number, string>, hexWidth = 0): string => {
  const label = hexWidth ? `0x${v.toString(16).toUpperCase().padStart(hexWidth, "0")}` : String(v);
  const name = names[v];
  return name ? `${label} (${name})` : label;
};

const formatIpProto = (v: number): string => {
  const name = IP_PROTO_NAMES[v];
  return name ? `${name} (${v})` : String(v);
};

const formatEthType = (v: number): string => {
  const name = ETH_TYPE_NAMES[v];
  const hex = `0x${v.toString(16).toUpperCase().padStart(4, "0")}`;
  return name ? `${hex} (${name})` : hex;
};

const formatPort = (v: number): string => {
  const name = WELL_KNOWN_PORTS[v];
  return name ? `${v} (${name})` : String(v);
};

const formatTcpOptions = (value: unknown): string => {
  if (!Array.isArray(value) || value.length === 0) {
    return none();
  }
  return value
    .map((o) => {
      const item = (o ?? {}) as { type?: number; length?: number };
      const name = TCP_OPTION_NAMES[Number(item.type ?? 0)];
      const len = Number(item.length ?? 0);
      return `${name ?? `type=${Number(item.type ?? 0)}`}${len ? `(len=${len})` : ""}`;
    })
    .join(" · ");
};

const formatIpv4Options = (value: unknown): string => {
  if (!Array.isArray(value) || value.length === 0) {
    return none();
  }
  return value
    .map((o) => {
      const item = (o ?? {}) as { optionType?: number; optionLength?: number };
      const type = Number(item.optionType ?? 0);
      const len = Number(item.optionLength ?? 0);
      return `type=${type}${len ? `(len=${len})` : ""}`;
    })
    .join(" · ");
};

const formatHex = (v: number): string => {
  return `0x${v.toString(16).toUpperCase()}`;
};

const formatBytes = (value: unknown): string => {
  if (typeof value !== "string" || value === "") {
    return none();
  }
  return [...value].map((ch) => ch.charCodeAt(0).toString(16).padStart(2, "0")).join(" ");
};
