import Cookies from 'js-cookie';

import { SupportLanguage } from '@/types';
import { match as matchLocale } from '@formatjs/intl-localematcher';

const THEME_STORAGE_KEY: string = "theme";
export const DARK_THEME: string = "dark";
export const LIGHT_THEME: string = "light";

const NAMESPACE_KEY: string = "namespace";
export const DEFAULT_NAMESAPCE: string = "default";

const JWT_COOKIE_KEY: string = "jwtoken";

const LANGUAGE_KEY: string = "locale";
const AVAILABLE_LOCALES: SupportLanguage[] = [
  SupportLanguage.English,
  SupportLanguage.ZhHans,
];
export const DEFAULT_LOCALE: SupportLanguage = SupportLanguage.English;

export const localManager = {
  getTheme(): string {
    return localStorage.getItem(THEME_STORAGE_KEY) || DARK_THEME;
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

  getJwt(): string | void {
    return Cookies.get(JWT_COOKIE_KEY);
  },

  removeJwt(): void {
    Cookies.remove(JWT_COOKIE_KEY);
  },

  getLanguage(): SupportLanguage {
    let lang: string = matchLocale(
      [navigator.language, document.documentElement.lang],
      AVAILABLE_LOCALES,
      localStorage.getItem(LANGUAGE_KEY) || DEFAULT_LOCALE
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
