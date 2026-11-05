<template>
  <!-- 助手消息 Markdown 渲染的唯一入口：全站仅此一处渲染助手富文本，
       因此代码高亮 / mermaid / 安全外链等规则也在此集中注册 -->
  <!-- key 随主题与流式态翻转：md-editor-v3 只在预览组件初始化时按 noMermaid 注册 mermaid
       的 markdown-it 插件、也只在渲染时读取 mermaid 主题配置；运行中改 prop 不会补注册 /
       重绘。不重挂载的话，流式期间挂载的消息在生成结束后会一直是 mermaid 源码块，切换
       明暗时现有图表也会停留在旧配色 -->
  <div ref="rootRef" class="zk-chat-markdown-root">
    <!-- 操作区锚点：sticky 且零高度，只负责把按钮组钉在滚动容器顶部，不占据正文高度 -->
    <div class="zk-markdown-actions-anchor">
      <!-- 操作区：悬停显现，避免在每条消息旁常驻占位；全屏为查看类动作，流式生成期间同样可用 -->
      <div class="zk-markdown-actions">
        <v-btn
          icon="mdi-fullscreen"
          size="small"
          variant="text"
          :title="$t('action.fullscreenView')"
          :aria-label="$t('action.fullscreenView')"
          @click="openFullscreen"
        />
        <v-btn
          icon="mdi-file-pdf-box"
          size="small"
          variant="text"
          :disabled="exporting"
          :title="$t('action.exportPdf')"
          :aria-label="$t('action.exportPdf')"
          @click="exportPdf"
        />
        <v-btn
          icon="mdi-image-outline"
          size="small"
          variant="text"
          :disabled="exporting"
          :title="$t('action.exportImage')"
          :aria-label="$t('action.exportImage')"
          @click="exportImage"
        />
        <v-btn
          icon="mdi-language-html5"
          size="small"
          variant="text"
          :disabled="exporting"
          :title="$t('action.exportHtml')"
          :aria-label="$t('action.exportHtml')"
          @click="exportHtml"
        />
      </div>
    </div>
    <md-preview
      :key="previewKey"
      class="zk-chat-markdown"
      :model-value="rendered"
      :theme="theme"
      :no-mermaid="streaming"
    />
  </div>

  <!-- 全屏查看：Teleport 到 body，脱离对话框 / 抽屉的 transform 祖先，position: fixed 才能铺满视口 -->
  <teleport to="body">
    <div v-if="fullscreen" class="zk-markdown-fullscreen bg-surface">
      <div class="zk-markdown-fullscreen-header">
        <span class="zk-section-title text-truncate">
          {{ documentTitle || $t("markdown.fullscreenTitle") }}
        </span>
        <v-spacer />
        <v-btn
          icon="mdi-file-pdf-box"
          size="small"
          variant="text"
          :disabled="exporting"
          :title="$t('action.exportPdf')"
          :aria-label="$t('action.exportPdf')"
          @click="exportPdf"
        />
        <v-btn
          icon="mdi-image-outline"
          size="small"
          variant="text"
          :disabled="exporting"
          :title="$t('action.exportImage')"
          :aria-label="$t('action.exportImage')"
          @click="exportImage"
        />
        <v-btn
          icon="mdi-language-html5"
          size="small"
          variant="text"
          :disabled="exporting"
          :title="$t('action.exportHtml')"
          :aria-label="$t('action.exportHtml')"
          @click="exportHtml"
        />
        <v-btn
          icon="mdi-close"
          size="small"
          variant="text"
          :title="$t('action.close')"
          :aria-label="$t('action.close')"
          @click="closeFullscreen"
        />
      </div>
      <div ref="fullscreenRef" class="zk-markdown-fullscreen-body">
        <div class="zk-markdown-fullscreen-doc">
          <md-preview
            :key="previewKey"
            class="zk-chat-markdown"
            :model-value="rendered"
            :theme="theme"
            :no-mermaid="streaming"
          />
        </div>
      </div>
    </div>
  </teleport>
</template>

<script setup lang="ts" name="assistant-markdown">
import { toBlob } from "html-to-image";
import { MdPreview, config } from "md-editor-v3";
import mermaid from "mermaid";
import "md-editor-v3/lib/preview.css";

import type { RetrievalResult } from "@/apis/@types/ai";
import { useAppStore } from "@/stores/app";
import { DARK_THEME } from "@/stores/local";
import { knowledgeDocumentHref } from "@/utils/ai";
import i18n from "@/utils/i18n";
import {
  decorateMermaidSvg,
  mermaidCanvasBackground,
  mermaidConfigFor,
} from "@/utils/mermaidTheme";

// CITE_FRAGMENT 引用标注链接的片段标识：正文中的 [n] 被改写为带此前缀的站内文档链接，
// 渲染规则据此识别引用链接并挂上引用样式，无需再向渲染器传逐消息的引用映射。
const CITE_FRAGMENT = "#zk-cite-";

// md-editor-v3 的依赖与渲染规则在此集中注册（本文件是全站唯一渲染入口）：
// 1) 代码高亮沿用其内置能力，无需额外引入 highlight.js；
// 2) mermaid 使用本地实例，避免运行时从其默认 CDN 拉取依赖；主题由 mermaidConfig 统一
//    改写为 Trae 图示风格（见 utils/mermaidTheme.ts），明暗跟随应用主题；
// 3) 外链统一新窗口打开并切断 opener 引用，防止被引用页面操纵本页；
// 4) 引用标注沿用同一链接通道：去掉片段标识后即来源文档详情页，新窗口打开。
config({
  editorExtensions: {
    mermaid: { instance: mermaid },
  },
  mermaidConfig: mermaidConfigFor,
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

// previewKey 合并主题与流式态：md-editor 的 mermaid 只在渲染时读取主题配置，切换明暗须
// 重挂载预览才会按新配色重绘；流式结束后同样要重挂载，mermaid 插件才会补注册。
const previewKey = computed(() => `${theme.value}-${props.streaming ? "streaming" : "settled"}`);

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

// SVG_NS SVG 命名空间：向克隆出的图表注入样式时按 SVG 规则创建节点。
const SVG_NS = "http://www.w3.org/2000/svg";
// PNG_SCALE 导出图片的像素倍率：2 倍在高分屏与放大查看时仍清晰。
const PNG_SCALE = 2;
// EXPORT_STYLE 是栅格化时需要随 SVG 一起带走的样式：SVG 序列化后不再继承页面 CSS，
// 这里内联与 main.scss 的 mermaid 规则等价的最小集合（圆角/描边/字重/线头）。
const EXPORT_STYLE = [
  ".node rect,.node polygon{rx:8px;ry:8px;stroke-width:1px;}",
  ".node text,.nodeLabel,.nodeLabel p{font-weight:500;}",
  ".flowchart-link{stroke-linecap:round;stroke-linejoin:round;}",
  ".edgeLabel,.edgeLabel p{font-size:12px;font-weight:400;}",
].join("");

// renderSvgToPngBlob 把单张 mermaid SVG 栅格化为 PNG：按 viewBox 设定尺寸，先铺画布底色再
// 绘制，避免透明底在聊天/文档里不可读。流程图标签已由 mermaid 配置为 SVG <text>
// （htmlLabels=false），否则经 <img> 栅格化时 foreignObject 内容会整块丢失。
const renderSvgToPngBlob = async (svg: SVGSVGElement, background: string): Promise<Blob> => {
  const box = svg.viewBox.baseVal;
  const width = Math.max(1, Math.round(box.width || svg.getBoundingClientRect().width));
  const height = Math.max(1, Math.round(box.height || svg.getBoundingClientRect().height));
  const clone = svg.cloneNode(true) as SVGSVGElement;
  clone.removeAttribute("style");
  clone.setAttribute("xmlns", SVG_NS);
  clone.setAttribute("width", String(width));
  clone.setAttribute("height", String(height));
  const exportStyle = document.createElementNS(SVG_NS, "style");
  exportStyle.textContent = EXPORT_STYLE;
  clone.insertBefore(exportStyle, clone.firstChild);

  const markup = new XMLSerializer().serializeToString(clone);
  const url = URL.createObjectURL(new Blob([markup], { type: "image/svg+xml;charset=utf-8" }));
  try {
    const image = new Image();
    await new Promise<void>((resolve, reject) => {
      image.onload = () => resolve();
      image.onerror = () => reject(new Error("mermaid svg load failed"));
      image.src = url;
    });
    const canvas = document.createElement("canvas");
    canvas.width = width * PNG_SCALE;
    canvas.height = height * PNG_SCALE;
    const context = canvas.getContext("2d");
    if (!context) {
      throw new Error("canvas 2d context unavailable");
    }
    context.fillStyle = background;
    context.fillRect(0, 0, canvas.width, canvas.height);
    context.drawImage(image, 0, 0, canvas.width, canvas.height);
    return await new Promise<Blob>((resolve, reject) => {
      canvas.toBlob(
        (blob) => (blob ? resolve(blob) : reject(new Error("canvas toBlob failed"))),
        "image/png",
      );
    });
  } finally {
    URL.revokeObjectURL(url);
  }
};

// copyTextToClipboard 是文本兜底：优先异步 Clipboard API；非安全域（http）或权限被拒时
// navigator.clipboard 不存在或写入抛错，退回隐藏 textarea + execCommand（与 md-editor 原实现
// 的兜底路径一致，HTTP 部署同样可用）。
const copyTextToClipboard = async (text: string): Promise<void> => {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return;
    } catch {
      // 落入 execCommand 兜底
    }
  }
  const textarea = document.createElement("textarea");
  textarea.value = text;
  textarea.setAttribute("readonly", "");
  textarea.style.cssText = "position:fixed;top:-9999px;left:-9999px;opacity:0;";
  document.body.appendChild(textarea);
  textarea.select();
  const copied = document.execCommand("copy");
  document.body.removeChild(textarea);
  if (!copied) {
    throw new Error("copy text failed");
  }
};

// CODE_TIP_RESTORE_MS 复制反馈停留时长：与 md-editor 自身一致，1.5s 后还原按钮文案。
const CODE_TIP_RESTORE_MS = 1500;

// onCodeCopyClick 在捕获阶段接住 md-editor 的代码复制按钮：其内置实现在安全域下直接调用
// navigator.clipboard.writeText，写入被拒（iframe 权限策略 / 文档失焦等）即失败，且不像非安全域
// 那样回退 execCommand；这里统一改走 copyTextToClipboard（失败自动回退），并在按钮上就地给出
// 成功 / 失败反馈，不额外弹通知。事件委托挂在渲染根容器上，后续新渲染 / 重挂载的代码块同样生效。
const onCodeCopyClick = (event: MouseEvent): void => {
  const target =
    event.target instanceof Element ? event.target.closest(".md-editor-copy-button") : null;
  if (!target) {
    return;
  }
  const container = target.closest(".md-editor-code");
  const code =
    (container?.querySelector("input:checked + pre code") as HTMLElement | null) ??
    (container?.querySelector("pre code") as HTMLElement | null);
  if (!code) {
    return;
  }
  // 阻断 md-editor 自身的 handler，同时避免点击复制按钮时把 <details> 折叠块收起
  event.preventDefault();
  event.stopPropagation();

  // 初始文案由 data-tips 承载：图标形态改 dataset.tips，文本形态改 textContent（与 md-editor 一致）
  const original = target.getAttribute("data-tips") ?? target.textContent ?? "";
  const showTip = (text: string): void => {
    if (target.hasAttribute("data-is-icon")) {
      (target as HTMLElement).dataset.tips = text;
      return;
    }
    target.textContent = text;
  };
  void copyTextToClipboard(code.textContent ?? "")
    .then(() => showTip(i18n.global.t("notice.copied")))
    .catch(() => showTip(i18n.global.t("notice.copyFailed")))
    .finally(() => {
      window.setTimeout(() => showTip(original), CODE_TIP_RESTORE_MS);
    });
};

// onMermaidCopyClick 在捕获阶段接住 md-editor 的 mermaid 复制按钮：其默认行为是复制图表源码，
// 这里改为复制 PNG 图片；图片剪贴板不可用（非安全域 / 无权限 / 浏览器不支持）或栅格化失败时，
// 退回复制源码（同样阻断默认行为，由本处理器统一给出提示）。
// 事件委托挂在渲染根容器上，后续新渲染 / 重挂载的图表同样生效。
const onMermaidCopyClick = (event: MouseEvent): void => {
  const target =
    event.target instanceof Element ? event.target.closest(".md-editor-mermaid-copy") : null;
  if (!target) {
    return;
  }
  const container = target.closest(".md-editor-mermaid");
  const svg = container?.querySelector("svg");
  const content = (container as HTMLElement | null)?.dataset.content ?? "";
  event.preventDefault();
  event.stopPropagation();

  void (async () => {
    const canCopyImage =
      window.isSecureContext &&
      typeof ClipboardItem !== "undefined" &&
      typeof navigator.clipboard?.write === "function";
    if (canCopyImage && svg instanceof SVGSVGElement) {
      try {
        // ClipboardItem 支持 Promise<Blob>：栅格化是异步的，但 write 必须仍在用户激活
        // 窗口内调用，故把 Promise 直接交给 ClipboardItem 而不是先 await 再 write。
        const blobPromise = renderSvgToPngBlob(
          svg,
          mermaidCanvasBackground(theme.value === "dark"),
        );
        await navigator.clipboard.write([new ClipboardItem({ "image/png": blobPromise })]);
        appStore.notify({ text: i18n.global.t("notice.copied"), timeout: 2000, color: "success" });
        return;
      } catch {
        // 图片写入失败：退回复制源码
      }
    }
    try {
      await copyTextToClipboard(content);
      appStore.notify({
        text: i18n.global.t("notice.copyImageFallback"),
        timeout: 4000,
        color: "info",
      });
    } catch {
      appStore.notify({ text: i18n.global.t("notice.copyFailed"), timeout: 3000, color: "error" });
    }
  })();
};

// mermaid 由 md-editor 异步渲染（先输出占位再替换为 SVG），渲染时机无法预知，
// 故在容器上挂 MutationObserver，按帧合并后再做后处理（强调节点上色与出边取色，
// 见 decorateMermaidSvg；连线走向保持 mermaid 原生渲染）。
const rootRef = ref<HTMLElement | null>(null);
const fullscreenRef = ref<HTMLElement | null>(null);
let frame = 0;

// surfaceObservers 记录已绑定的预览容器及其观察器：正文与全屏副本是两棵独立渲染树，
// 图表配色与复制按钮的事件委托都要各绑一份。
const surfaceObservers = new WeakMap<HTMLElement, MutationObserver>();

const colorizeMermaid = (): void => {
  cancelAnimationFrame(frame);
  frame = requestAnimationFrame(() => {
    const dark = theme.value === "dark";
    if (rootRef.value) {
      decorateMermaidSvg(rootRef.value, dark);
    }
    if (fullscreenRef.value) {
      decorateMermaidSvg(fullscreenRef.value, dark);
    }
  });
};

const bindSurface = (surface: HTMLElement | null): void => {
  if (!surface || surfaceObservers.has(surface)) {
    return;
  }
  surface.addEventListener("click", onMermaidCopyClick, true);
  surface.addEventListener("click", onCodeCopyClick, true);
  const observer = new MutationObserver(colorizeMermaid);
  observer.observe(surface, { childList: true, subtree: true });
  surfaceObservers.set(surface, observer);
};

const unbindSurface = (surface: HTMLElement | null): void => {
  const observer = surface ? surfaceObservers.get(surface) : undefined;
  if (!surface || !observer) {
    return;
  }
  observer.disconnect();
  surfaceObservers.delete(surface);
  surface.removeEventListener("click", onMermaidCopyClick, true);
  surface.removeEventListener("click", onCodeCopyClick, true);
};

onMounted(() => {
  bindSurface(rootRef.value);
  colorizeMermaid();
});

// ---- 全屏查看 ----
const fullscreen = ref(false);
// bodyOverflow 全屏前的 body overflow 值：全屏期间锁定页面滚动，退出时原样还原
let bodyOverflow = "";

const openFullscreen = (): void => {
  fullscreen.value = true;
};

const closeFullscreen = (): void => {
  fullscreen.value = false;
};

// onFullscreenKeydown 用 ESC 退出全屏，与对话框的关闭习惯一致
const onFullscreenKeydown = (event: KeyboardEvent): void => {
  if (event.key === "Escape") {
    closeFullscreen();
  }
};

// 全屏副本在 v-if 挂载后才存在：打开时补绑事件与观察器；关闭时 watch 先于 DOM 更新触发，
// 此时 fullscreenRef 仍指向待移除的节点，可正常解绑，避免观察器泄漏。
watch(fullscreen, (open) => {
  if (open) {
    bodyOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    window.addEventListener("keydown", onFullscreenKeydown);
  } else {
    unbindSurface(fullscreenRef.value);
    window.removeEventListener("keydown", onFullscreenKeydown);
    document.body.style.overflow = bodyOverflow;
  }
  void nextTick(() => {
    if (open) {
      bindSurface(fullscreenRef.value);
      colorizeMermaid();
    }
  });
});

onBeforeUnmount(() => {
  unbindSurface(rootRef.value);
  unbindSurface(fullscreenRef.value);
  cancelAnimationFrame(frame);
  if (fullscreen.value) {
    window.removeEventListener("keydown", onFullscreenKeydown);
    document.body.style.overflow = bodyOverflow;
  }
});

// ---- 导出 ----
const exporting = ref(false);

// documentTitle 取正文首个标题：作为全屏标题与导出文件名，无标题时由调用方兜底
const documentTitle = computed(() => {
  const heading = /^#{1,6}\s+(.+)$/m.exec(props.content)?.[1] ?? "";
  return heading
    .replace(/[`*_~#>]/g, "")
    .replace(/\s+/g, " ")
    .trim();
});

// timestampName 无标题时的文件名兜底：本地时间戳保证多次导出不覆盖
const timestampName = (): string => {
  const now = new Date();
  const pad = (value: number): string => String(value).padStart(2, "0");
  return `markdown-${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}-${pad(
    now.getHours(),
  )}${pad(now.getMinutes())}${pad(now.getSeconds())}`;
};

// fileNameBase 导出文件名（不含扩展名）：标题中的路径分隔符与空白替换为连字符
const fileNameBase = (): string => {
  const title = documentTitle.value
    .replace(/[\\/:*?"<>|\s]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 60);
  return title || timestampName();
};

// previewElement 取当前可见预览的 md-editor 根节点：导出目标是正文本身，
// 不含本组件的悬停操作区，避免把按钮截进图片 / HTML。
const previewElement = (): HTMLElement | null => {
  const surface = fullscreen.value ? fullscreenRef.value : rootRef.value;
  return surface?.querySelector<HTMLElement>(".md-editor") ?? null;
};

const notify = (text: string, color: "success" | "info" | "error" = "success"): void => {
  appStore.notify({ text, timeout: color === "error" ? 4000 : 2500, color });
};

// saveBlobAsFile 触发浏览器保存：对象 URL 生成临时链接触发下载（与知识库原文下载同一做法）
const saveBlobAsFile = (blob: Blob, filename: string): void => {
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  anchor.click();
  window.setTimeout(() => URL.revokeObjectURL(url), 0);
};

// EXPORT_INTERACTION_SELECTORS 导出前从克隆中移除的交互装饰：复制 / 缩放 / 折叠按钮不进入导出物
const EXPORT_INTERACTION_SELECTORS = [
  ".md-editor-copy-button",
  ".md-editor-mermaid-action",
  ".md-editor-mermaid-zoom",
  ".md-editor-collapse-tips",
];

// cloneStaticPreview 克隆预览为静态副本：去掉交互按钮，并解除折叠代码块的屏幕限高
// （导出物应展示完整代码，而不是被滚动容器截断的部分）。
const cloneStaticPreview = (target: HTMLElement): HTMLElement => {
  const clone = target.cloneNode(true) as HTMLElement;
  EXPORT_INTERACTION_SELECTORS.forEach((selector) => {
    clone.querySelectorAll(selector).forEach((node) => node.remove());
  });
  clone.querySelectorAll<HTMLElement>("pre").forEach((pre) => {
    pre.style.maxHeight = "none";
    pre.style.overflow = "visible";
  });
  return clone;
};

// EXPORT_FONT_STACK 导出文件脱离应用字体资源，显式给出系统中文字体栈，避免回落为衬线默认字体
const EXPORT_FONT_STACK = [
  "-apple-system",
  "BlinkMacSystemFont",
  '"Segoe UI"',
  '"PingFang SC"',
  '"Hiragino Sans GB"',
  '"Microsoft YaHei"',
  '"Noto Sans SC"',
  "Roboto",
  "sans-serif",
].join(", ");

// EXPORT_BASE_CSS 导出文档的版式：屏幕阅读时保留纸张式留白，打印时取消留白并避免区块跨页截断。
// 颜色仍取应用主题变量（导出文件内联了应用样式表与 .v-theme--* 变量块）。
const EXPORT_BASE_CSS = `
html, body { margin: 0; padding: 0; }
body { font-family: ${EXPORT_FONT_STACK}; }
.zk-md-export { display: block; background-color: rgb(var(--v-theme-background)); }
.zk-md-export-page {
  box-sizing: border-box;
  max-width: var(--zk-dialog-width-lg);
  margin-inline: auto;
  padding: var(--zk-space-6) var(--zk-space-4);
  background-color: rgb(var(--v-theme-surface));
}
.zk-md-export .md-editor { height: auto; border: 0; box-shadow: none; }
.zk-md-export .md-editor-preview { padding: 0; }
.zk-md-export .md-editor-code[open] > pre { max-height: none; overflow: visible; }
.zk-md-print-bar {
  position: sticky;
  top: 0;
  z-index: 1;
  display: flex;
  align-items: center;
  gap: var(--zk-space-2);
  padding: var(--zk-space-2) var(--zk-space-4);
  border-bottom: var(--zk-hairline-width) solid
    rgb(var(--v-theme-on-surface), var(--zk-hairline-opacity));
  background-color: rgb(var(--v-theme-surface));
}
.zk-md-print-bar button {
  padding: var(--zk-space-1) var(--zk-space-3);
  border: var(--zk-hairline-width) solid rgb(var(--v-theme-on-surface), var(--zk-hairline-opacity));
  border-radius: var(--zk-radius-sm);
  background-color: transparent;
  color: rgb(var(--v-theme-on-surface));
  font: inherit;
  cursor: pointer;
}
.zk-md-print-bar button.is-primary {
  border-color: rgb(var(--v-theme-primary));
  color: rgb(var(--v-theme-primary));
}
@page { size: A4; margin: 14mm 12mm; }
@media print {
  .zk-md-print-bar { display: none; }
  .zk-md-export { background-color: rgb(var(--v-theme-surface)); }
  .zk-md-export-page { max-width: none; padding: 0; background-color: transparent; }
  .zk-md-export h1, .zk-md-export h2, .zk-md-export h3, .zk-md-export h4 { break-after: avoid; }
  .zk-md-export table, .zk-md-export pre, .zk-md-export blockquote,
  .zk-md-export .md-editor-mermaid, .zk-md-export img { break-inside: avoid; }
  .zk-md-export tr { break-inside: avoid; }
  .zk-md-export a { text-decoration: underline; }
}
`;

// escapeHtml 转义单行文本中的 HTML 特殊字符（标题等由正文派生的字符串）
const escapeHtml = (value: string): string =>
  value.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");

// collectExportStyles 汇总同源样式表规则：应用样式与 md-editor 预览样式内联进导出文件，
// 脱离应用后仍按原样渲染；跨域样式表读取 cssRules 会抛错，跳过即可。
// 字体文件与 @import 不内联：前者在离线文件中无对应资源，后者已由打包器展开。
const collectExportStyles = (): string => {
  const parts: string[] = [];
  Array.from(document.styleSheets).forEach((sheet) => {
    let rules: CSSRuleList;
    try {
      rules = sheet.cssRules;
    } catch {
      return;
    }
    Array.from(rules).forEach((rule) => {
      if (rule instanceof CSSFontFaceRule || rule instanceof CSSImportRule) {
        return;
      }
      parts.push(rule.cssText);
    });
  });
  return parts.join("\n");
};

// buildExportDocument 生成自包含 HTML：内联样式 + 预览静态副本；
// forPrint 时固定浅色主题（深色底在纸面上费墨且对比度差），并附带屏幕端打印条。
const buildExportDocument = (target: HTMLElement, title: string, forPrint: boolean): string => {
  const clone = cloneStaticPreview(target);
  if (forPrint) {
    clone.setAttribute("data-theme", "light");
  }
  const themeClass = forPrint
    ? "v-theme--light"
    : `v-theme--${theme.value === "dark" ? "dark" : "light"}`;
  const printBar = forPrint
    ? `<div class="zk-md-print-bar"><button type="button" class="is-primary" onclick="window.print()">${escapeHtml(
        i18n.global.t("action.print"),
      )}</button><button type="button" onclick="window.close()">${escapeHtml(
        i18n.global.t("action.close"),
      )}</button></div>`
    : "";
  return `<!doctype html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>${escapeHtml(title)}</title>
<style>
${collectExportStyles()}
${EXPORT_BASE_CSS}
</style>
</head>
<body>
<div class="v-application ${themeClass} zk-md-export">
${printBar}
<div class="zk-md-export-page">${clone.outerHTML}</div>
</div>
</body>
</html>`;
};

// PRINT_READY_DELAY_MS 等文档写入后完成排版再调起打印，避免打印到尚未完成排版的页面
const PRINT_READY_DELAY_MS = 300;
// PRINT_FRAME_KEEP_MS 打印浮层的兜底存活时长：afterprint 未触发时（部分浏览器）
// 到点后强制移除，避免隐藏 iframe 常驻
const PRINT_FRAME_KEEP_MS = 60000;

// printInFrame 在当前页内用隐藏 iframe 调起打印：弹窗被拦截时的兜底路径，
// 打印内容与弹窗视图同源同排版，且不依赖新窗口权限。
const printInFrame = (html: string): boolean => {
  const frame = document.createElement("iframe");
  frame.setAttribute("aria-hidden", "true");
  frame.style.cssText =
    "position:fixed;left:0;bottom:0;width:0;height:0;border:0;visibility:hidden;";
  document.body.appendChild(frame);
  const frameWindow = frame.contentWindow;
  const frameDocument = frame.contentDocument;
  if (!frameWindow || !frameDocument) {
    frame.remove();
    return false;
  }
  frameDocument.open();
  frameDocument.write(html);
  frameDocument.close();
  const cleanup = (): void => {
    frameWindow.removeEventListener("afterprint", cleanup);
    frame.remove();
  };
  frameWindow.addEventListener("afterprint", cleanup);
  window.setTimeout(() => {
    frameWindow.focus();
    frameWindow.print();
  }, PRINT_READY_DELAY_MS);
  window.setTimeout(cleanup, PRINT_FRAME_KEEP_MS);
  return true;
};

// exportPdf 用浏览器打印导出 PDF：优先新窗口承载干净排版（用户选择「另存为 PDF」）；
// 弹窗被拦截时退回隐藏 iframe，保证导出路径仍可用。文字保持可选中，分页由浏览器排版。
const exportPdf = (): void => {
  const target = previewElement();
  if (!target) {
    return;
  }
  const html = buildExportDocument(target, fileNameBase(), true);
  const win = window.open("", "_blank");
  if (win) {
    win.document.open();
    win.document.write(html);
    win.document.close();
    // 写入后确认文档真的落地：部分环境会返回一个可写但被拦截的空白窗口，此时转走浮层打印
    if (win.document.querySelector(".zk-md-export")) {
      window.setTimeout(() => {
        win.focus();
        win.print();
      }, PRINT_READY_DELAY_MS);
      notify(i18n.global.t("notice.printReady"), "info");
      return;
    }
    win.close();
  }
  if (printInFrame(html)) {
    notify(i18n.global.t("notice.printReady"), "info");
    return;
  }
  notify(i18n.global.t("notice.printBlocked"), "error");
};

// exportHtml 导出自包含 HTML 文件：样式与渲染结果一并内联，可直接在浏览器离线打开
const exportHtml = (): void => {
  const target = previewElement();
  if (!target) {
    return;
  }
  try {
    const base = fileNameBase();
    const html = buildExportDocument(target, base, false);
    saveBlobAsFile(new Blob([html], { type: "text/html;charset=utf-8" }), `${base}.html`);
    notify(i18n.global.t("notice.exportHtmlSuccess"));
  } catch {
    notify(i18n.global.t("notice.exportFailed"), "error");
  }
};

// EXPORT_IMAGE_SCALE 图片像素倍率；超高文档降为 1 倍，避免触发浏览器的 canvas 尺寸上限
const EXPORT_IMAGE_SCALE = 2;
const EXPORT_IMAGE_SCALE_HEIGHT = 6000;
// EXPORT_IMAGE_PADDING 画布内边距引用间距令牌（导出容器在页面内即用 CSS 变量）：
// 正文四周留白，避免文字紧贴图片边缘
const EXPORT_IMAGE_PADDING = "var(--zk-space-6)";

// imagePaddingPx 取间距令牌的实际像素值：离屏容器的宽度需按像素计算，CSS 变量无法参与算术
const imagePaddingPx = (): number => {
  const value = parseFloat(
    window.getComputedStyle(document.documentElement).getPropertyValue("--zk-space-6"),
  );
  return Number.isFinite(value) ? value : 0;
};

// exportImage 导出 PNG：先把静态副本放进离屏容器（带底色与内边距），再交给 html-to-image
// 栅格化；离屏容器在导出结束后立即移除，不污染页面。
const exportImage = async (): Promise<void> => {
  const target = previewElement();
  if (!target || exporting.value) {
    return;
  }
  exporting.value = true;
  const background = mermaidCanvasBackground(theme.value === "dark");
  // offscreen 只负责把捕获容器移出视口；捕获目标是其子节点 holder——html-to-image 会连同
  // 节点的 computed position/left 一起序列化，若把偏移写在 holder 上，内容会被整体平移到
  // 画布之外，只能截到一块纯底色。
  const offscreen = document.createElement("div");
  const holder = document.createElement("div");
  try {
    const clone = cloneStaticPreview(target);
    offscreen.style.cssText = "position: fixed; left: -100000px; top: 0;";
    holder.className = `v-application v-theme--${theme.value === "dark" ? "dark" : "light"}`;
    holder.style.cssText = [
      `width: ${target.clientWidth + imagePaddingPx() * 2}px`,
      `padding: ${EXPORT_IMAGE_PADDING}`,
      "box-sizing: border-box",
      `background-color: ${background}`,
    ].join("; ");
    holder.appendChild(clone);
    offscreen.appendChild(holder);
    document.body.appendChild(offscreen);
    const blob = await toBlob(holder, {
      pixelRatio: holder.scrollHeight > EXPORT_IMAGE_SCALE_HEIGHT ? 1 : EXPORT_IMAGE_SCALE,
      backgroundColor: background,
      // 站点引用了跨域字体样式表（Google Fonts），读取其 cssRules 会抛 SecurityError；
      // 栅格化改用系统字体栈即可，故跳过字体内联。
      skipFonts: true,
    });
    if (!blob) {
      throw new Error("markdown image export failed");
    }
    saveBlobAsFile(blob, `${fileNameBase()}.png`);
    notify(i18n.global.t("notice.exportImageSuccess"));
  } catch {
    notify(i18n.global.t("notice.exportFailed"), "error");
  } finally {
    offscreen.remove();
    exporting.value = false;
  }
};
</script>

<style scoped>
/* 预览外壳只负责定位；操作区的表面 / 悬停态与全屏外壳的留白见 styles/main.scss */
.zk-chat-markdown-root {
  position: relative;
}

/* 操作区锚点：sticky 零高度，让按钮组随正文滚动时始终停在滚动容器顶部，
   又不因占据高度而把正文往下推（按钮的实际留白由 .zk-markdown-actions 的 margin 提供） */
.zk-markdown-actions-anchor {
  position: sticky;
  top: 0;
  z-index: 1;
  display: flex;
  align-items: flex-start;
  justify-content: flex-end;
  height: 0;
}

.zk-markdown-actions {
  display: flex;
  align-items: center;
  margin: var(--zk-space-1);
}

/* 全屏查看：铺满视口，标题行固定、正文区独立滚动 */
.zk-markdown-fullscreen {
  position: fixed;
  inset: 0;
  z-index: var(--zk-z-fullscreen);
  display: flex;
  flex-direction: column;
}

.zk-markdown-fullscreen-header {
  display: flex;
  align-items: center;
  gap: var(--zk-space-1);
}

/* 标题占据剩余宽度并允许收缩，长标题省略而不挤压右侧操作按钮 */
.zk-markdown-fullscreen-header .zk-section-title {
  flex: 1 1 auto;
  min-width: 0;
}

.zk-markdown-fullscreen-body {
  flex: 1 1 0;
  min-height: 0;
  overflow-y: auto;
}

.zk-markdown-fullscreen-doc {
  max-width: var(--zk-dialog-width-lg);
  margin-inline: auto;
}
</style>
