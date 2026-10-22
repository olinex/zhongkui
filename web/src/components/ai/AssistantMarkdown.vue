<template>
  <!-- 助手消息 Markdown 渲染的唯一入口：全站仅此一处渲染助手富文本，
       因此代码高亮 / mermaid / 安全外链等规则也在此集中注册 -->
  <md-preview
    class="assistant-markdown"
    :model-value="rendered"
    :theme="theme"
    :no-mermaid="streaming"
  />
</template>

<script setup lang="ts" name="assistant-markdown">
import { MdPreview, config } from "md-editor-v3";
import mermaid from "mermaid";
import "md-editor-v3/lib/preview.css";

import type { RetrievalResult } from "@/apis/@types/ai";
import { useAppStore } from "@/stores/app";
import { DARK_THEME } from "@/stores/local";
import { knowledgeDocumentHref } from "@/utils/ai";
import i18n from "@/utils/i18n";

// CITE_FRAGMENT 引用标注链接的片段标识：正文中的 [n] 被改写为带此前缀的站内文档链接，
// 渲染规则据此识别引用链接并挂上引用样式，无需再向渲染器传逐消息的引用映射。
const CITE_FRAGMENT = "#zk-cite-";

// md-editor-v3 的依赖与渲染规则在此集中注册（本文件是全站唯一渲染入口）：
// 1) 代码高亮沿用其内置能力，无需额外引入 highlight.js；
// 2) mermaid 使用本地实例，避免运行时从其默认 CDN 拉取依赖；
// 3) 外链统一新窗口打开并切断 opener 引用，防止被引用页面操纵本页；
// 4) 引用标注沿用同一链接通道：去掉片段标识后即来源文档详情页，新窗口打开。
config({
  editorExtensions: {
    mermaid: { instance: mermaid },
  },
  markdownItConfig: (md) => {
    md.renderer.rules.link_open = (tokens, idx, options, _env, self) => {
      tokens[idx].attrSet("target", "_blank");
      tokens[idx].attrSet("rel", "noopener noreferrer");
      if ((tokens[idx].attrGet("href") ?? "").includes(CITE_FRAGMENT)) {
        tokens[idx].attrJoin("class", "assistant-citation-ref");
        tokens[idx].attrSet("title", i18n.global.t("page.aiAssistant.citationOpen"));
      }
      return self.renderToken(tokens, idx, options);
    };
  },
});

const appStore = useAppStore();

// theme 预览主题跟随应用主题（Vuetify 主题为字符串，此处收敛为 md-editor 的字面量联合）
const theme = computed(() => (appStore.theme === DARK_THEME ? "dark" : "light"));

const props = withDefaults(
  defineProps<{
    /** 助手消息的 Markdown 原文 */
    content: string;
    /** 是否仍在流式生成：生成期间暂不渲染 mermaid，避免对不完整图表反复重建 */
    streaming?: boolean;
    /** 本条消息的引用来源：用于把正文中的 [n] 标注链接到对应来源文档 */
    citations?: RetrievalResult[];
  }>(),
  { streaming: false, citations: () => [] },
);

// CODE_SEGMENT 切出围栏代码块与行内代码：其中的下标（如 arr[1]）不得被当作来源标注改写
const CODE_SEGMENT = /(```[\s\S]*?```|~~~[\s\S]*?~~~|`[^`\n]*`)/g;
// CITATION_MARKER 匹配正文中的来源标注 [n]；紧跟 ( 的是已有链接，不再改写
const CITATION_MARKER = /\[(\d+)\](?!\()/g;

// annotateCitations 把正文中的 [n] 改写为指向第 n 条引用的站内链接；编号越界或历史消息
// 缺失命名空间（无法溯源）时保持原文，避免产生死链
const annotateCitations = (text: string, citations: RetrievalResult[]): string => {
  if (!text || citations.length === 0) {
    return text;
  }
  return text
    .split(CODE_SEGMENT)
    .map((segment, index) => {
      if (index % 2 === 1) {
        return segment;
      }
      return segment.replace(CITATION_MARKER, (match, raw) => {
        const number = Number(raw);
        const citation = citations[number - 1];
        if (!citation || !citation.namespaceName) {
          return match;
        }
        const href = knowledgeDocumentHref(citation.documentID, citation.namespaceName);
        return `[${number}](${href}${CITE_FRAGMENT}${number})`;
      });
    })
    .join("");
};

const rendered = computed(() => annotateCitations(props.content, props.citations));
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

/* 正文内来源标注：上标小号、主题主色、无下划线，与正文区分但不抢视线；
   标注后紧跟的来源列表给出同一编号，用户可据此对照 */
.assistant-markdown :deep(.assistant-citation-ref) {
  margin-inline-start: 1px;
  color: rgb(var(--v-theme-primary));
  font-size: 0.75em;
  font-variant-numeric: tabular-nums;
  line-height: 0;
  vertical-align: super;
  text-decoration: none;
}

.assistant-markdown :deep(.assistant-citation-ref:hover) {
  text-decoration: underline;
}
</style>
