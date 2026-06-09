import { useAppStore } from "@/stores/app";

import i18n from "./i18n";

/**
 * 使用 Clipboard API 复制文本到剪贴板（现代浏览器推荐）
 * @param text 要复制的文本
 * @returns Promise<boolean> 复制成功返回 true，失败返回 false
 */
export async function copyToClipboardModern(text: string): Promise<boolean> {
  try {
    // 现代浏览器使用 Clipboard API（需用户交互触发，如点击事件）
    if (navigator.clipboard === undefined) {
      return false;
    }
    await navigator.clipboard.writeText(text);
    return true;
  } catch (error) {
    console.error("复制失败（Clipboard API）:", error);
    return false;
  }
}

/**
 * 使用 textarea 临时元素复制文本到剪贴板（兼容旧浏览器）
 * @param text 要复制的文本
 * @returns boolean 复制成功返回 true，失败返回 false
 */
export function copyToClipboardLegacy(text: string): boolean {
  // 创建临时 textarea 元素
  const textarea = document.createElement("textarea");
  textarea.value = text;

  // 隐藏元素（避免影响页面布局）
  textarea.style.position = "fixed";
  textarea.style.opacity = "0";
  textarea.style.pointerEvents = "none";

  document.body.appendChild(textarea);
  textarea.select();

  try {
    // 执行复制命令（需用户交互触发）
    const success = document.execCommand("copy");
    return success;
  } catch (error) {
    console.error("复制失败（传统方法）:", error);
    return false;
  } finally {
    // 清理临时元素
    document.body.removeChild(textarea);
  }
}

export async function copyToClipboard(text: string): Promise<boolean> {
  let success = false;
  if (await copyToClipboardModern(text)) {
    success = true;
  } else if (copyToClipboardLegacy(text)) {
    success = true;
  }
  const appStore = useAppStore();
  if (success) {
    appStore.notify({
      text: i18n.global.t("notice.copied"),
      timeout: 3000,
      color: "success",
    });
  } else {
    appStore.notify({
      text: i18n.global.t("notice.copyFailed"),
      timeout: 3000,
      color: "error",
    });
  }
  return success;
}
