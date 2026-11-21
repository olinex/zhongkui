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

// LINK_SLOT_GROUP_ORDER 槽位语义分组的展示顺序（默认语义摘要按此顺序拼接）
const LINK_SLOT_GROUP_ORDER = ["ethernet", "dot11", "linuxSll", "linuxSll2", "ppp"] as const;

// LINK_SLOT_GROUP_LINK_TYPE_KEYS 槽位语义分组 → 链路类型 i18n 键段；
// 分组名称直接复用链路类型翻译，避免维护第二套文案
const LINK_SLOT_GROUP_LINK_TYPE_KEYS: Record<string, string> = {
  ethernet: "ethernet",
  dot11: "ieee80211",
  linuxSll: "linuxSll",
  linuxSll2: "linuxSll2",
  ppp: "ppp",
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

// linkTypeOptions 链路类型下拉选项：按数值升序输出，title 为 i18n 键供下拉翻译
export const linkTypeOptions = (): Array<{ title: string; value: number }> => {
  return Object.entries(LINK_TYPE_KEYS)
    .map(([value, key]) => ({ value: Number(value), key }))
    .sort((a, b) => a.value - b.value)
    .map((item) => ({
      value: item.value,
      title: `model.__base.value.linkType.${item.key}`,
    }));
};

// linkSlotGroupOf 链路类型 → 槽位语义分组；未收录的链路类型返回 undefined
const linkSlotGroupOf = (linkType: unknown): string | undefined => {
  return LINK_SLOT_GROUPS[Number(linkType)];
};

// linkSlotSemantic 分组下指定槽位的语义；该分组未定义该槽位时返回 undefined
const linkSlotSemantic = (group: string, slot: number): string | undefined => {
  const key = `model.__base.value.linkSlot.${group}.${slot}`;
  const semantic = i18n.global.t(key);
  return semantic === key ? undefined : semantic;
};

// linkSlotSemanticOf 链路类型下指定槽位的语义；未收录链路类型或槽位时返回 undefined
export const linkSlotSemanticOf = (linkType: unknown, slot: number): string | undefined => {
  const group = linkSlotGroupOf(linkType);
  return group ? linkSlotSemantic(group, slot) : undefined;
};

// linkSlotDescription 槽位在各链路类型下的语义摘要（筛选下拉与列设置的补充说明）；
// 无任何已收录语义时返回空串
export const linkSlotDescription = (slot: number): string => {
  return LINK_SLOT_GROUP_ORDER.map((group) => {
    const semantic = linkSlotSemantic(group, slot);
    if (!semantic) {
      return "";
    }
    const groupKey = LINK_SLOT_GROUP_LINK_TYPE_KEYS[group];
    const groupName = i18n.global.t(`model.__base.value.linkType.${groupKey}`);
    return `${groupName}·${semantic}`;
  })
    .filter((item) => item !== "")
    .join(" / ");
};

// linkSlotName 按 linkType 解释槽位语义：未收录链路类型或槽位时回退通用名称
export const linkSlotName = (linkType: unknown, slot: number): string => {
  const generic = i18n.global.t("model.__base.value.linkSlot.generic", { slot });
  const semantic = linkSlotSemanticOf(linkType, slot);
  return semantic ? `${generic} · ${semantic}` : generic;
};
