// Utilities
import { defineStore } from "pinia";
import i18n from "@/utils/i18n";
import { localManager } from "./local";
import type { SupportLanguage } from "@/types";
import { setLocale } from "@vee-validate/i18n";

export enum MessageLevel {
  Info = "info",
  Warn = "warn",
  Error = "error",
}

export type Message = {
  text: string;
  timeout: number;
  color: string;
};

export const useAppStore = defineStore("app", {
  state: () => ({
    theme: localManager.getTheme(),
    namespace: localManager.getNamespace(),
    language: localManager.getLanguage(),
    messages: [] as Message[],
  }),
  actions: {
    asNamespace(namespace: string) {
      if (this.namespace === namespace) {
        return;
      }
      this.namespace = namespace;
      localManager.setNamespace(namespace);
    },
    asLanguage(language?: SupportLanguage) {
      if (!language) {
        language = localManager.getLanguage();
      }
      if (this.language === language) {
        return;
      }
      setLocale(language);
      this.language = language;
      localManager.setLanguage(language);
      i18n.global.locale.value = language;
    },
    toggleTheme() {
      localManager.toggleTheme();
      this.theme = localManager.getTheme();
    },
    notify(msg: Message) {
      this.messages.push(msg);
    },
  },
});
