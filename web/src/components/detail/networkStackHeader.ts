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
  | "raw";

export interface HeaderFieldMeta {
  // headers JSON 中的技术键名，同时决定 i18n 键：
  // 字段名 = model.networkStack.header.field.<key>，说明 = model.networkStack.header.desc.<key>
  key: string;
  kind: HeaderValueKind;
}

// 按 LayerType（proto）声明各层头部字段的展示顺序与值类型；
// Loopback（54）无真实链路层信息（eth 字段全空），不逐字段展示
export const HEADER_FIELDS: Record<number, HeaderFieldMeta[]> = {
  // Ethernet
  17: [
    { key: "ethDstMac", kind: "mac" },
    { key: "ethSrcMac", kind: "mac" },
    { key: "ethType", kind: "ethType" },
    { key: "ethLength", kind: "raw" },
  ],
  // IPv4
  20: [
    { key: "ipv4Version", kind: "raw" },
    { key: "ipv4Ihl", kind: "raw" },
    { key: "ipv4Tos", kind: "raw" },
    { key: "ipv4Length", kind: "raw" },
    { key: "ipv4Id", kind: "raw" },
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
      return value ? t("model.networkStack.header.value.yes") : t("model.networkStack.header.value.no");
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
  return [...value]
    .map((ch) => ch.charCodeAt(0).toString(16).padStart(2, "0"))
    .join(" ");
};
