// stores/assistant.ts
// 全局 AI 助手抽屉 store：仅承载抽屉开关状态。会话与消息状态由抽屉组件自身持有，
// 抽屉关闭后组件不卸载、会话不丢失，故此处不持久化任何会话数据。

import { defineStore } from "pinia";

// useAssistantStore 全局助手抽屉开关：顶栏图标按钮与全局抽屉（App.vue）共用。
export const useAssistantStore = defineStore("assistant", {
  state: () => ({
    // opened 抽屉是否展开
    opened: false,
  }),
  actions: {
    open() {
      this.opened = true;
    },
    close() {
      this.opened = false;
    },
    toggle() {
      this.opened = !this.opened;
    },
  },
});
