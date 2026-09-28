import Cookies from "js-cookie";

import { SupportLanguage } from "@/types";
import { match as matchLocale } from "@formatjs/intl-localematcher";

const THEME_STORAGE_KEY: string = "theme";
export const DARK_THEME: string = "dark";
export const LIGHT_THEME: string = "light";

const NAMESPACE_KEY: string = "namespace";
export const DEFAULT_NAMESAPCE: string = "default";

const JWT_KEY: string = "jwtoken";

// 侧边栏固定展开偏好（true 时导航抽屉固定展开，不随鼠标悬停收起）
const NAV_PINNED_KEY: string = "nav-pinned";

// 首页快速开始清单配置（分组与导航入口，JSON 字符串）
const QUICK_START_KEY: string = "quick-start";

// 直达底部悬浮按钮拖动后的位置（JSON 字符串 {left, top}，单位为 px）
const SCROLL_FAB_POS_KEY: string = "scroll-fab-pos";

const LANGUAGE_KEY: string = "locale";
const AVAILABLE_LOCALES: SupportLanguage[] = [SupportLanguage.English, SupportLanguage.ZhHans];
export const DEFAULT_LOCALE: SupportLanguage = SupportLanguage.English;

export const localManager = {
  getTheme(): string {
    return localStorage.getItem(THEME_STORAGE_KEY) || DARK_THEME;
  },
  setTheme(theme: string) {
    localStorage.setItem(THEME_STORAGE_KEY, theme);
  },
  toggleTheme() {
    const theme = this.getTheme();
    if (theme === DARK_THEME) {
      localStorage.setItem(THEME_STORAGE_KEY, LIGHT_THEME);
    } else {
      localStorage.setItem(THEME_STORAGE_KEY, DARK_THEME);
    }
  },

  getNamespace(): string {
    return localStorage.getItem(NAMESPACE_KEY) || DEFAULT_NAMESAPCE;
  },
  setNamespace(namespace: string) {
    localStorage.setItem(NAMESPACE_KEY, namespace);
  },

  getNavPinned(): boolean {
    return localStorage.getItem(NAV_PINNED_KEY) === "true";
  },
  setNavPinned(pinned: boolean) {
    localStorage.setItem(NAV_PINNED_KEY, String(pinned));
  },

  getQuickStartGroups(): string | null {
    return localStorage.getItem(QUICK_START_KEY);
  },
  setQuickStartGroups(groupsJson: string) {
    localStorage.setItem(QUICK_START_KEY, groupsJson);
  },

  // 读取直达底部悬浮按钮位置；未拖动过或数据损坏时返回 null（组件回退右下角贴靠）
  getScrollFabPos(): { left: number; top: number } | null {
    const raw = localStorage.getItem(SCROLL_FAB_POS_KEY);
    if (!raw) {
      return null;
    }
    try {
      const pos = JSON.parse(raw) as { left: number; top: number };
      if (typeof pos.left === "number" && typeof pos.top === "number") {
        return pos;
      }
    } catch {
      // 损坏的本地数据忽略，回退默认位置
    }
    return null;
  },
  setScrollFabPos(pos: { left: number; top: number }): void {
    localStorage.setItem(SCROLL_FAB_POS_KEY, JSON.stringify(pos));
  },
  // 清除直达底部悬浮按钮的位置记录（视口尺寸变化时复位回右下角贴靠）
  clearScrollFabPos(): void {
    localStorage.removeItem(SCROLL_FAB_POS_KEY);
  },

  getJwt(): string | void {
    // 优先读取 localStorage 中的令牌，其次回退到 Cookie。
    // Cookie 受主机作用域限制（如 localhost 与 127.0.0.1 互不可见），
    // 而 localStorage 与页面源一致，可跨主机部署场景下可靠读取
    return localStorage.getItem(JWT_KEY) || Cookies.get(JWT_KEY);
  },

  setJwt(jwt: string): void {
    localStorage.setItem(JWT_KEY, jwt);
  },

  removeJwt(): void {
    localStorage.removeItem(JWT_KEY);
    Cookies.remove(JWT_KEY);
  },

  getLanguage(): SupportLanguage {
    const lang: string = matchLocale(
      [navigator.language, document.documentElement.lang],
      AVAILABLE_LOCALES,
      localStorage.getItem(LANGUAGE_KEY) || DEFAULT_LOCALE,
    );
    switch (lang) {
      case SupportLanguage.English:
        return SupportLanguage.English;
      case SupportLanguage.ZhHans:
        return SupportLanguage.ZhHans;
      default:
        return DEFAULT_LOCALE;
    }
  },

  setLanguage(language: SupportLanguage): void {
    localStorage.setItem(LANGUAGE_KEY, language);
    document.documentElement.lang = language;
  },
};
