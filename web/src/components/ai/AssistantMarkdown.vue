<template>
  <!-- 助手消息 Markdown 渲染的唯一入口：全站仅此一处渲染助手富文本，
       因此代码高亮 / mermaid / 安全外链等规则也在此集中注册 -->
  <md-preview
    class="zk-chat-markdown"
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
        tokens[idx].attrJoin("class", "zk-chat-citation-ref");
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

// annotateCitations 把正文中的 [n] 改写为指向第 n 条引用的站内链接：编号越界（常见于模型复用
// 历史轮次正文里的旧 [n]）时直接移除该标注，避免正文出现当前消息没有对应来源的假引用；历史消息
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
        if (!citation) {
          return "";
        }
        if (!citation.namespaceName) {
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
