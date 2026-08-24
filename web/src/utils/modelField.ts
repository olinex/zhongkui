import { HTTP_TRANSPORT_MODEL_KEY } from "@/utils/rego";
import i18n from "@/utils/i18n";

// modelFieldTitle 模型字段标题：优先取 model.<modelKey>.field.<key> 翻译；
// 未命中时去 reqHdr/rspHdr 前缀、驼峰转单词并按单词大写兜底
export const modelFieldTitle = (modelKey: string, key: string): string => {
  const typeTpl = `model.${modelKey}.field.${key}`;
  const t = i18n.global.t(typeTpl);
  if (t !== typeTpl) {
    return t;
  }
  return key
    .replace(/^(reqHdr|rspHdr)/, "")
    .replace(/([a-z0-9])([A-Z])/g, "$1 $2")
    .replace(/\b\w/g, (c) => c.toUpperCase());
};

// 已知模型类型键 → i18n 模型段（如 …/network.HttpTransport → httpTransport）
const KNOWN_MODEL_KEYS: Record<string, string> = {
  [HTTP_TRANSPORT_MODEL_KEY]: "httpTransport",
};

// modelTypeKeyToI18nKey 目标模型类型键 → i18n 模型段：已知模型显式映射；
// 未知模型取类型名末段转小驼峰兜底，保证后续新增模型自动切换展示格式。
export const modelTypeKeyToI18nKey = (typeKey: string): string => {
  const known = KNOWN_MODEL_KEYS[typeKey];
  if (known) {
    return known;
  }
  const last = typeKey.split(".").pop() ?? typeKey;
  return last.charAt(0).toLowerCase() + last.slice(1);
};

// modelTypeKeyLabel 目标模型类型键 → 可读模型名称：优先取 model.<modelKey>.name 翻译
// （如 …/network.HttpTransport → "HTTP 传输"）；未命中时回退返回原始类型键。
export const modelTypeKeyLabel = (typeKey: string): string => {
  const nameTpl = `model.${modelTypeKeyToI18nKey(typeKey)}.name`;
  const t = i18n.global.t(nameTpl);
  if (t !== nameTpl) {
    return t;
  }
  return typeKey;
};
