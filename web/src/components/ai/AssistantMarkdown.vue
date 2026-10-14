<template>
  <!-- 助手消息 Markdown 渲染的唯一入口：全站仅此一处渲染助手富文本，
       因此代码高亮 / mermaid / 安全外链等规则也在此集中注册 -->
  <md-preview
    class="assistant-markdown"
    :model-value="content"
    :theme="theme"
    :no-mermaid="streaming"
  />
</template>

<script setup lang="ts" name="assistant-markdown">
import { MdPreview, config } from "md-editor-v3";
import mermaid from "mermaid";
import "md-editor-v3/lib/preview.css";

import { useAppStore } from "@/stores/app";
import { DARK_THEME } from "@/stores/local";

// md-editor-v3 的依赖与渲染规则在此集中注册（本文件是全站唯一渲染入口）：
// 1) 代码高亮沿用其内置能力，无需额外引入 highlight.js；
// 2) mermaid 使用本地实例，避免运行时从其默认 CDN 拉取依赖；
// 3) 外链统一新窗口打开并切断 opener 引用，防止被引用页面操纵本页。
config({
  editorExtensions: {
    mermaid: { instance: mermaid },
  },
  markdownItConfig: (md) => {
    md.renderer.rules.link_open = (tokens, idx, options, _env, self) => {
      tokens[idx].attrSet("target", "_blank");
      tokens[idx].attrSet("rel", "noopener noreferrer");
      return self.renderToken(tokens, idx, options);
    };
  },
});

const appStore = useAppStore();

// theme 预览主题跟随应用主题（Vuetify 主题为字符串，此处收敛为 md-editor 的字面量联合）
const theme = computed(() => (appStore.theme === DARK_THEME ? "dark" : "light"));

withDefaults(
  defineProps<{
    /** 助手消息的 Markdown 原文 */
    content: string;
    /** 是否仍在流式生成：生成期间暂不渲染 mermaid，避免对不完整图表反复重建 */
    streaming?: boolean;
  }>(),
  { streaming: false },
);
</script>

<style scoped>
/* 预览根容器（.md-editor）在深色主题下按 --md-bk-color 铺纯黑底，与抽屉 surface 的深灰不一致，
   读起来像一块突兀的黑块；置为透明，让正文直接落在对话框自身背景上。
   （内层 .md-editor-preview / .md-editor-preview-wrapper 本就没有底色，无需逐个覆盖。） */
.assistant-markdown {
  background-color: transparent;
}

/* 仅布局定位：内边距交由外层的 .zk-chat-bubble 承担，避免与 md-editor 默认内边距叠加 */
.assistant-markdown :deep(.md-editor-preview) {
  padding: 0;
}
</style>
