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

// modelTypeKeyToI18nKey 目标模型类型键 → i18n 枚举模型段（camelCase）。
// 纯推导：取最后一段并将首字母转小写（…/network.HttpTransport → httpTransport；
// 对已是 httpTransport 的键幂等）。与后端 tagengine.ModelKey 保持同一规则，前后端单源一致，
// 新增模型时无需维护人工映射表。
export const modelTypeKeyToI18nKey = (typeKey: string): string => {
  const last = typeKey.split(".").pop() ?? typeKey;
  return last.charAt(0).toLowerCase() + last.slice(1);
};
