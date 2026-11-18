// 链路类型（gopacket layers.LinkType）与通用链路槽位的展示辅助。
// 槽位含义由 linkType 决定（见 internal/models/network/*.go 的 LinkSlotValues），
// 列表与详情共用同一套映射，避免在页面内散落协议分支。
import i18n from "@/utils/i18n";

// LINK_TYPE_KEYS 链路类型数值 → i18n 键段（model.__base.value.linkType.<key>）
const LINK_TYPE_KEYS: Record<number, string> = {
  0: "null",
  1: "ethernet",
  9: "ppp",
  51: "pppEthernet",
  101: "raw",
  105: "ieee80211",
  108: "loop",
  113: "linuxSll",
  119: "prism",
  127: "ieee80211Radio",
  228: "ipv4",
  229: "ipv6",
  276: "linuxSll2",
};

// LINK_SLOT_GROUPS 链路类型数值 → 槽位语义分组（相同链路模型共用槽位约定）
const LINK_SLOT_GROUPS: Record<number, string> = {
  1: "ethernet",
  9: "ppp",
  51: "ppp",
  105: "dot11",
  119: "dot11",
  127: "dot11",
  113: "linuxSll",
  276: "linuxSll2",
};

// linkSlotIndex 解析槽位字段名（link1～link16）为槽号；非槽位字段返回 0
export const linkSlotIndex = (key: string): number => {
  const matched = /^link(1[0-6]|[1-9])$/.exec(key);
  return matched ? Number(matched[1]) : 0;
};

// linkTypeName 链路类型的可读名称：已知类型输出「数值 (名称)」，未知回退数值
export const linkTypeName = (value: unknown): string => {
  const numeric = Number(value);
  const key = LINK_TYPE_KEYS[numeric];
  if (!key) {
    return String(value ?? "");
  }
  return `${numeric} (${i18n.global.t(`model.__base.value.linkType.${key}`)})`;
};

// linkSlotName 按 linkType 解释槽位语义：未收录链路类型或槽位时回退通用名称
export const linkSlotName = (linkType: unknown, slot: number): string => {
  const generic = i18n.global.t("model.__base.value.linkSlot.generic", { slot });
  const group = LINK_SLOT_GROUPS[Number(linkType)];
  if (!group) {
    return generic;
  }
  const key = `model.__base.value.linkSlot.${group}.${slot}`;
  const semantic = i18n.global.t(key);
  return semantic === key ? generic : `${generic} · ${semantic}`;
};
