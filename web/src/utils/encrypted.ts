// 加密/明文相关工具：统一判定“空密文”，避免把空对象占位当作真实密文请求明文接口。
// 空配置在列表/接口层可能以三种形态出现：null/undefined、空串 ""、空对象 JSON "{}"，
// 以及经 base64 编码后的 "{}"（即 "e30="）。is-empty-ciphertext 覆盖全部形态。
// Utilities for encrypted/plaintext handling: uniformly judge "empty ciphertext" so that an
// empty-object placeholder is not mistaken for real ciphertext when calling the plaintext endpoint.
// An empty config may surface as null/undefined, "" , the empty-object JSON "{}", or its base64
// encoding "{}" (i.e. "e30="). is-empty-ciphertext covers all these forms.

// base64 编码的空对象 "{}"；作为“无明文”占位在接口层返回
// Base64 encoding of the empty object "{}"; returned as the "no plaintext" placeholder.
const EMPTY_OBJECT_CIPHERTEXT = "e30=";

// isEmptyCiphertext 判定密文是否为“空/占位”（无实际可查看的明文）。
// isEmptyCiphertext tells whether a ciphertext is empty/placeholder (no viewable plaintext).
export function isEmptyCiphertext(value: unknown): boolean {
  if (value === null || value === undefined) {
    return true;
  }
  if (typeof value !== "string") {
    return false;
  }
  const trimmed = value.trim();
  return trimmed === "" || trimmed === "{}" || trimmed === EMPTY_OBJECT_CIPHERTEXT;
}