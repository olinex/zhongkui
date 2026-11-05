<template>
  <!-- 全屏时 Teleport 到 body，脱离对话框 .v-overlay__content 的 transform 祖先，
       否则 position: fixed 的相对包含块仍是对话框容器，无法真正铺满视口 -->
  <teleport to="body" :disabled="!fullscreen">
    <div
      class="markdown-editor-wrap"
      :class="{ 'is-fullscreen': fullscreen, 'bg-surface': fullscreen }"
    >
      <!-- 标题行：字段标题 + 校验状态 + 操作区。全屏按钮固定最右（与 RegoEditor 一致）：
           全屏为查看类动作，只读 / 禁用态仍可进入 -->
      <div class="zk-editor-header">
        <span v-if="label" class="zk-editor-title">{{ label }}</span>
        <v-spacer />
        <v-icon v-if="valid" size="x-small" color="success" :title="$t('action.validatePassed')">
          mdi-check-circle
        </v-icon>
        <v-icon v-if="!!shownError" size="x-small" color="error" :title="shownError">
          mdi-alert-circle
        </v-icon>
        <v-btn
          :icon="fullscreen ? 'mdi-fullscreen-exit' : 'mdi-fullscreen'"
          :variant="fullscreen ? 'tonal' : 'text'"
          :title="$t(fullscreen ? 'action.exitFullscreen' : 'action.fullscreen')"
          :aria-label="$t(fullscreen ? 'action.exitFullscreen' : 'action.fullscreen')"
          @click="toggleFullscreen"
        />
      </div>
      <!-- 明文与预览为两个独立开关：仅明文 / 仅预览 / 两者同时开启（左右分屏），至少保留一个 -->
      <div class="zk-editor markdown-editor-box" :class="{ 'is-error': !!shownError }">
        <div class="markdown-editor-body" :class="{ 'is-split': isSplit }">
          <!-- 左半：CodeMirror 原文编辑区（明文与分屏时可见；切到仅预览时仅隐藏不销毁） -->
          <div
            v-show="showSource"
            ref="hostRef"
            class="markdown-editor-host zk-markdown-editor"
          />
          <!-- 右半：预览区（预览与分屏时挂载；仅明文视图不渲染，避免对不可见内容做整篇渲染） -->
          <div v-if="showPreview" class="markdown-editor-preview">
            <assistant-markdown v-if="hasContent" :content="modelValue" />
            <span v-else class="zk-hint">{{ placeholderText }}</span>
          </div>
        </div>
        <!-- 底部工具行：左侧语法校验提示、右侧明文 / 预览两个独立开关，与 JsonEditor 的操作行同构 -->
        <div class="markdown-editor-toolbar">
          <v-icon
            v-if="firstIssue"
            size="x-small"
            :icon="firstIssue.severity === 'error' ? 'mdi-alert-circle' : 'mdi-alert'"
            :color="firstIssue.severity === 'error' ? 'error' : 'warning'"
          />
          <span
            v-if="firstIssue"
            class="markdown-editor-lint text-body-small"
            :class="firstIssue.severity === 'error' ? 'text-error' : 'text-warning'"
            :title="lintSummary"
          >
            {{ lintSummary }}
          </span>
          <v-spacer />
          <v-btn-toggle
            v-model="visiblePanes"
            multiple
            mandatory
            density="compact"
            size="small"
            variant="plain"
            color="primary"
          >
            <v-btn value="source" :text="$t('action.markdownSource')" />
            <v-btn value="preview" :text="$t('action.markdownPreview')" />
          </v-btn-toggle>
        </div>
      </div>
      <div v-if="shownError" class="zk-error-text">{{ shownError }}</div>
      <div v-if="hint" class="zk-hint">{{ hint }}</div>
    </div>
  </teleport>
</template>

<script setup lang="ts" name="markdown-editor">
import { defaultKeymap, history, historyKeymap } from "@codemirror/commands";
import { markdown } from "@codemirror/lang-markdown";
import { bracketMatching } from "@codemirror/language";
import { linter } from "@codemirror/lint";
import { Compartment, EditorState, Text, type Extension } from "@codemirror/state";
import {
  EditorView,
  highlightActiveLine,
  keymap,
  lineNumbers,
  placeholder,
} from "@codemirror/view";

import { useAppStore } from "@/stores/app";
import { highlightFor } from "@/utils/editorHighlight";
import i18n from "@/utils/i18n";

const props = withDefaults(
  defineProps<{
    modelValue: string;
    /** 字段标题，展示于编辑器头部 */
    label?: string;
    /** 辅助说明，渲染在编辑器下方 */
    hint?: string;
    /** 校验通过，渲染成功标记 */
    valid?: boolean;
    /** 校验错误信息，渲染在编辑器下方 */
    errorMessage?: string;
    /** 禁用编辑（只读），预览仍可用 */
    disabled?: boolean;
    /** 只读（内容由外部生成，如导入原件解析结果），预览仍可用 */
    readonly?: boolean;
    /** 空文档提示文案 */
    placeholderText?: string;
  }>(),
  {
    label: "",
    hint: "",
    valid: false,
    errorMessage: "",
    disabled: false,
    readonly: false,
    placeholderText: "",
  },
);

const emit = defineEmits<{ (e: "update:modelValue", value: string): void }>();

const appStore = useAppStore();

// visiblePanes 当前可见分区：source=Markdown 明文 / preview=渲染预览。两个开关相互独立，
// mandatory 保证至少保留一个；默认仅开启明文（按需再开启预览 / 两者同时开启即左右分屏）
const visiblePanes = ref<Array<"source" | "preview">>(["source"]);
const showSource = computed(() => visiblePanes.value.includes("source"));
const showPreview = computed(() => visiblePanes.value.includes("preview"));
const isSplit = computed(() => showSource.value && showPreview.value);

// hasContent 空正文不渲染预览（空预览会让外壳看起来像渲染失败）
const hasContent = computed(() => props.modelValue.trim() !== "");

// shownError 外部（表单）校验错误；与 JsonEditor 一致，只展示外部错误，组件自身不产生语法错误
const shownError = computed(() => props.errorMessage);

// ---- Markdown 语法校验（前端轻量规则）----
// 只覆盖「写错就渲染不出预期结构」的高置信问题，避免对自由书写的正文产生误报：
//   error   代码围栏未闭合、链接缺少右括号
//   warning 标题 # 后缺少空格、列表标记后缺少空格、链接地址为空
interface MarkdownIssue {
  from: number;
  to: number;
  /** 1 起算的行号，用于工具行文案 */
  line: number;
  severity: "error" | "warning";
  message: string;
}

// FENCE_LINE 行首围栏标记（≤3 空格缩进 + 至少三个反引号或波浪线）
const FENCE_LINE = /^ {0,3}(`{3,}|~{3,})/;
const HEADING_NO_SPACE = /^#{1,6}[^#\s]/;
const UNORDERED_NO_SPACE = /^[-+][^\s-]/;
const ORDERED_NO_SPACE = /^\d+\.[^\s\d]/;
const EMPTY_LINK_TARGET = /\]\(\s*\)/g;
const LINK_OPEN = /\]\(/g;

// lintMarkdown 逐行扫描正文并产出诊断区间；围栏内的行跳过正文规则（代码块中不应出现告警）
const lintMarkdown = (text: string): MarkdownIssue[] => {
  const issues: MarkdownIssue[] = [];
  const doc = Text.of(text.split("\n"));
  // 当前打开的围栏：记录起止位置与标记（闭合需同字符、且长度不短于开启标记）
  let fence: { char: string; length: number; from: number; to: number; line: number } | null = null;

  for (let number = 1; number <= doc.lines; number += 1) {
    const line = doc.line(number);
    const fenceMatch = FENCE_LINE.exec(line.text);
    if (fence) {
      if (fenceMatch && fenceMatch[1][0] === fence.char && fenceMatch[1].length >= fence.length) {
        fence = null;
      }
      continue;
    }
    if (fenceMatch) {
      const offset = fenceMatch[0].length - fenceMatch[1].length;
      fence = {
        char: fenceMatch[1][0],
        length: fenceMatch[1].length,
        from: line.from + offset,
        to: line.from + fenceMatch[0].length,
        line: number,
      };
      continue;
    }

    // 正文规则基于去缩进后的行文本，区间再折算回全文偏移
    const trimmed = line.text.trimStart();
    const indent = line.text.length - trimmed.length;
    const base = line.from + indent;
    const push = (
      start: number,
      length: number,
      severity: MarkdownIssue["severity"],
      message: string,
    ) => {
      issues.push({
        from: base + start,
        to: base + start + length,
        line: number,
        severity,
        message,
      });
    };

    if (HEADING_NO_SPACE.test(trimmed)) {
      const hashes = /^#{1,6}/.exec(trimmed)?.[0].length ?? 0;
      push(0, hashes, "warning", i18n.global.t("markdown.lint.headingMissingSpace"));
    }
    if (UNORDERED_NO_SPACE.test(trimmed)) {
      push(0, 1, "warning", i18n.global.t("markdown.lint.listMissingSpace"));
    }
    if (ORDERED_NO_SPACE.test(trimmed)) {
      const marker = /^\d+\./.exec(trimmed)?.[0].length ?? 0;
      push(0, marker, "warning", i18n.global.t("markdown.lint.listMissingSpace"));
    }
    // 链接地址为空：]( ) 之间没有任何目标
    for (const match of line.text.matchAll(EMPTY_LINK_TARGET)) {
      issues.push({
        from: line.from + (match.index ?? 0),
        to: line.from + (match.index ?? 0) + match[0].length,
        line: number,
        severity: "warning",
        message: i18n.global.t("markdown.lint.linkEmptyTarget"),
      });
    }
    // 链接缺少右括号：]( 之后本行再无 ) 可闭合
    for (const match of line.text.matchAll(LINK_OPEN)) {
      const start = match.index ?? 0;
      if (line.text.indexOf(")", start + match[0].length) === -1) {
        issues.push({
          from: line.from + start,
          to: line.from + start + match[0].length,
          line: number,
          severity: "error",
          message: i18n.global.t("markdown.lint.linkMissingParen"),
        });
      }
    }
  }

  if (fence) {
    issues.push({
      from: fence.from,
      to: fence.to,
      line: fence.line,
      severity: "error",
      message: i18n.global.t("markdown.lint.fenceUnclosed"),
    });
  }
  return issues;
};

// issues 由正文派生：既驱动编辑器内的诊断标记，也驱动底部工具行左侧的首处问题提示
const issues = computed(() => lintMarkdown(props.modelValue));
const firstIssue = computed(() => issues.value[0]);
const lintSummary = computed(() => {
  const issue = firstIssue.value;
  if (!issue) {
    return "";
  }
  return issues.value.length > 1
    ? i18n.global.t("markdown.lint.summaryMore", {
        line: issue.line,
        message: issue.message,
        count: issues.value.length,
      })
    : i18n.global.t("markdown.lint.summary", { line: issue.line, message: issue.message });
});

const hostRef = useTemplateRef<HTMLDivElement>("hostRef");
const view = shallowRef<EditorView>();

// fullscreen 全屏编辑：正文固定高度下长文档在区内滚动，全屏时铺满视口
const fullscreen = ref(false);

const toggleFullscreen = (): void => {
  fullscreen.value = !fullscreen.value;
};

// 全屏或可见分区切换后重新测量：分屏与单栏的可用宽度不同，进入/退出全屏也会改变宽度，
// 两者都需要让 CodeMirror 重新分行
watch([fullscreen, visiblePanes], async () => {
  await nextTick();
  view.value?.requestMeasure();
});

// 语法高亮随主题切换，配色统一走 utils/editorHighlight 的共享方案。
const highlightCompartment = new Compartment();
// 只读态由 disabled / readonly 合并驱动（二者对编辑能力的语义相同，区别仅在调用方意图）。
const editableCompartment = new Compartment();

const readOnly = computed(() => props.disabled || props.readonly);

// 诊断区间由 issues 直接复用，避免每次键入重复扫描；延迟合并连续输入后再刷新标记。
const markdownLinter = linter(() => issues.value, { delay: 400 });

const extensions: Extension[] = [
  lineNumbers(),
  markdown(),
  highlightCompartment.of(highlightFor(appStore.theme)),
  highlightActiveLine(),
  bracketMatching(),
  markdownLinter,
  ...(props.placeholderText ? [placeholder(props.placeholderText)] : []),
  history(),
  keymap.of([...defaultKeymap, ...historyKeymap]),
  editableCompartment.of(EditorState.readOnly.of(readOnly.value)),
  EditorView.lineWrapping,
  EditorView.theme({
    "&": { fontSize: "var(--zk-editor-font-size)", height: "100%" },
    ".cm-scroller": {
      fontFamily: "var(--zk-font-mono)",
      lineHeight: "1.6",
    },
    "&.cm-focused": { outline: "none" },
  }),
  EditorView.updateListener.of((update) => {
    if (update.docChanged) {
      emit("update:modelValue", update.state.doc.toString());
    }
  }),
];

onMounted(() => {
  if (!hostRef.value) {
    return;
  }
  view.value = new EditorView({
    state: EditorState.create({ doc: props.modelValue, extensions }),
    parent: hostRef.value,
  });
});

onBeforeUnmount(() => {
  view.value?.destroy();
  view.value = undefined;
});

// 同步外部内容变更（如表单复位 / 详情回填 / 导入 Markdown）。
watch(
  () => props.modelValue,
  (val) => {
    const editorView = view.value;
    if (!editorView || editorView.state.doc.toString() === val) {
      return;
    }
    editorView.dispatch({
      changes: { from: 0, to: editorView.state.doc.length, insert: val },
    });
  },
);

// 只读状态变化时切换编辑能力。
watch(readOnly, (val) => {
  view.value?.dispatch({
    effects: editableCompartment.reconfigure(EditorState.readOnly.of(val)),
  });
});

// 应用主题变化时切换语法高亮。
watch(
  () => appStore.theme,
  (theme) => {
    view.value?.dispatch({
      effects: highlightCompartment.reconfigure(highlightFor(theme)),
    });
  },
);

defineExpose({ focus: () => view.value?.focus() });
</script>

<style scoped>
.markdown-editor-wrap {
  display: flex;
  flex-direction: column;
  gap: var(--zk-space-1);
}

.markdown-editor-box {
  display: flex;
  flex-direction: column;
}

/* 编辑与预览按当前查看方式排布：明文 / 预览为单栏，split 为左右各半同高 */
.markdown-editor-body {
  display: flex;
  min-width: 0;
}

/* 左半编辑区：固定高度，正文超出时由 CodeMirror 自身的滚动容器纵向滚动 */
.markdown-editor-host {
  flex: 1 1 0;
  min-width: 0;
  height: var(--zk-editor-body-height);
}

/* 右半预览区：长文档在预览内部纵向滚动；内边距由外壳内容区自身提供 */
.markdown-editor-preview {
  flex: 1 1 0;
  min-width: 0;
  height: var(--zk-editor-body-height);
  overflow-y: auto;
  padding: var(--zk-space-2);
}

/* 分屏时补一条 hairline 竖直分隔，区分原文与渲染结果 */
.markdown-editor-body.is-split .markdown-editor-preview {
  border-left: var(--zk-hairline-width) solid
    rgb(var(--v-theme-on-surface), var(--zk-hairline-opacity));
}

/* 底部工具行：承载语法校验提示（首处问题的行号与原因），与 JsonEditor 的操作行同构 */
.markdown-editor-toolbar {
  display: flex;
  align-items: center;
  gap: var(--zk-space-1);
  padding: 0 var(--zk-space-1) var(--zk-space-1);
}

/* 校验提示只占一行：超长文案省略，完整内容由 title 悬浮提示提供 */
.markdown-editor-lint {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 全屏编辑：铺满视口（fixed），标题行保留，正文区占满剩余高度 */
.markdown-editor-wrap.is-fullscreen {
  position: fixed;
  inset: 0;
  z-index: var(--zk-z-fullscreen);
  padding: var(--zk-space-4);
}

/* 外壳与正文区在全屏下改为按剩余高度拉伸，不再使用固定高度 */
.markdown-editor-wrap.is-fullscreen .markdown-editor-box {
  flex: 1 1 0;
  min-height: 0;
}

.markdown-editor-wrap.is-fullscreen .markdown-editor-body {
  flex: 1 1 0;
  min-height: 0;
}

.markdown-editor-wrap.is-fullscreen .markdown-editor-body .markdown-editor-host,
.markdown-editor-wrap.is-fullscreen .markdown-editor-body .markdown-editor-preview {
  flex: 1 1 0;
  height: auto;
  min-height: 0;
}
</style>
