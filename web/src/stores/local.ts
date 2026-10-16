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

// AI 运行续播游标（按会话持久化最后一次已消费的 SSE 事件序号）的存储键前缀
const AGENT_RUN_SEQ_KEY_PREFIX: string = "agent-run-seq-";

// AI 助手「思考」开关的用户偏好（跨会话记忆，重开抽屉保持上次选择）
const AGENT_THINKING_KEY: string = "agent-thinking";

// 上次选中的 AI 助手会话 ID（跨刷新恢复当前会话，供刷新后打开抽屉即自动续播）
const AGENT_ACTIVE_SESSION_KEY: string = "agent-active-session";

// AI 助手待发提问队列（按会话持久化，值为 JSON 字符串数组）的存储键前缀：
// 生成中提交的追问先入队，随会话本地记忆，刷新 / 切换会话后仍保留待发顺序
const AGENT_QUEUE_KEY_PREFIX: string = "agent-queue-";

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

  // 读取某会话的最后已消费事件序号（无记录返回 0），供运行续播的 after 参数使用
  getAgentRunSeq(sessionID: number): number {
    const raw = localStorage.getItem(`${AGENT_RUN_SEQ_KEY_PREFIX}${sessionID}`);
    const value = raw === null ? NaN : Number(raw);
    return Number.isFinite(value) && value > 0 ? Math.floor(value) : 0;
  },
  // 写入某会话的最后已消费事件序号（非正整数一律归零）
  setAgentRunSeq(sessionID: number, seq: number): void {
    const value = Number.isFinite(seq) && seq > 0 ? Math.floor(seq) : 0;
    localStorage.setItem(`${AGENT_RUN_SEQ_KEY_PREFIX}${sessionID}`, String(value));
  },

  // 读取 AI 助手「思考」开关偏好；未设置过时默认关闭（false）
  getAgentThinking(): boolean {
    return localStorage.getItem(AGENT_THINKING_KEY) === "true";
  },
  // 写入 AI 助手「思考」开关偏好
  setAgentThinking(on: boolean): void {
    localStorage.setItem(AGENT_THINKING_KEY, String(on));
  },

  // 读取上次选中的 AI 助手会话 ID（无记录返回 null），供刷新后恢复当前会话并自动续播
  getAgentActiveSession(): number | null {
    const raw = localStorage.getItem(AGENT_ACTIVE_SESSION_KEY);
    const value = raw === null ? NaN : Number(raw);
    return Number.isFinite(value) && value > 0 ? Math.floor(value) : null;
  },
  // 写入当前选中的 AI 助手会话 ID（传入 null 或非正整数时清除记录，表示处于待创建态）
  setAgentActiveSession(sessionID: number | null): void {
    if (sessionID !== null && Number.isFinite(sessionID) && sessionID > 0) {
      localStorage.setItem(AGENT_ACTIVE_SESSION_KEY, String(Math.floor(sessionID)));
      return;
    }
    localStorage.removeItem(AGENT_ACTIVE_SESSION_KEY);
  },

  // 读取某会话的待发提问队列（本地持久化的正文数组）；无记录或数据损坏时返回空数组
  getAgentQueue(sessionID: number): string[] {
    const raw = localStorage.getItem(`${AGENT_QUEUE_KEY_PREFIX}${sessionID}`);
    if (!raw) {
      return [];
    }
    try {
      const list = JSON.parse(raw) as unknown;
      if (Array.isArray(list)) {
        return list.filter(
          (text): text is string => typeof text === "string" && text.trim() !== "",
        );
      }
    } catch {
      // 损坏的本地数据忽略，按空队列处理
    }
    return [];
  },
  // 写入某会话的待发提问队列（空队列直接清除记录，避免残留空数组）
  setAgentQueue(sessionID: number, texts: string[]): void {
    if (texts.length === 0) {
      localStorage.removeItem(`${AGENT_QUEUE_KEY_PREFIX}${sessionID}`);
      return;
    }
    localStorage.setItem(`${AGENT_QUEUE_KEY_PREFIX}${sessionID}`, JSON.stringify(texts));
  },
  // 清除某会话的待发提问队列（会话删除时调用，避免留下孤儿记录）
  clearAgentQueue(sessionID: number): void {
    localStorage.removeItem(`${AGENT_QUEUE_KEY_PREFIX}${sessionID}`);
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
