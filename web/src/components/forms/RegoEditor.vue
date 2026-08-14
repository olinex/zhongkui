<template>
  <div ref="hostRef" class="rego-editor" :style="{ minHeight }" />
</template>

<script setup lang="ts">
import {
  autocompletion,
  completeFromList,
  type Completion,
  type CompletionContext,
  type CompletionResult,
  type CompletionSource,
} from "@codemirror/autocomplete";
import { defaultKeymap, history, historyKeymap } from "@codemirror/commands";
import { bracketMatching, defaultHighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { linter, lintGutter, setDiagnostics, type Diagnostic } from "@codemirror/lint";
import { EditorState, Compartment, type Extension } from "@codemirror/state";
import { EditorView, highlightActiveLine, keymap, lineNumbers } from "@codemirror/view";
import { oneDarkHighlightStyle } from "@codemirror/theme-one-dark";
import { rego } from "codemirror-lang-rego";

import { useAppStore } from "@/stores/app";
import { DARK_THEME } from "@/stores/local";
import { INPUT_FIELD_SUGGESTIONS, type RegoValidationError } from "@/utils/rego";

const props = withDefaults(
  defineProps<{
    modelValue: string;
    errors?: RegoValidationError[];
    /** extra `input.<field>` completions, e.g. fields referenced by existing rules */
    extraCompletions?: string[];
    /** disables editing (read-only), the component stays interactive */
    disabled?: boolean;
    minHeight?: string;
  }>(),
  {
    errors: () => [],
    extraCompletions: () => [],
    disabled: false,
    minHeight: "240px",
  },
);

const emit = defineEmits<{ (e: "update:modelValue", value: string): void }>();

const appStore = useAppStore();

const hostRef = useTemplateRef<HTMLDivElement>("hostRef");
const view = shallowRef<EditorView>();

// 语法高亮随主题切换：light 用 CM 默认浅色配色，dark 用 One Dark 配色，
// 避免深色字压在深色背景上对比度不足（如错误标记内的关键字/字符串）
const highlightCompartment = new Compartment();
const highlightFor = (theme: string): Extension =>
  syntaxHighlighting(theme === DARK_THEME ? oneDarkHighlightStyle : defaultHighlightStyle);

/** Inserts the full snippet and places the cursor inside its quotes. */
function applySnippet(
  editorView: EditorView,
  _completion: Completion,
  from: number,
  to: number,
  snippet: string,
): void {
  const cursor = Math.max(0, snippet.indexOf('"')) + 1;
  editorView.dispatch({
    changes: { from, to, insert: snippet },
    selection: { anchor: from + cursor },
  });
}

/**
 * Custom completion source: completes `input.<field>` from the suggestion
 * dictionary plus `extraCompletions`, then falls back to the Rego language
 * sources (keywords, OPA built-ins) registered by `codemirror-lang-rego`.
 */
async function regoCompletion(context: CompletionContext): Promise<CompletionResult | null> {
  const line = context.state.doc.lineAt(context.pos);
  const textBefore = line.text.slice(0, context.pos - line.from);
  const dotMatch = /input\.([a-zA-Z_][a-zA-Z0-9_]*)$/.exec(textBefore);
  if (dotMatch) {
    const items: Completion[] = [];
    for (const field of INPUT_FIELD_SUGGESTIONS) {
      items.push({
        label: field.field,
        detail: field.description,
        type: "variable",
        apply: (v, c, from, to) => applySnippet(v, c, from, to, field.snippet),
      });
    }
    for (const field of props.extraCompletions) {
      items.push({
        label: field,
        detail: "existing rule field",
        type: "variable",
        apply: (v, c, from, to) => applySnippet(v, c, from, to, `input.${field} == ""`),
      });
    }
    const result = await completeFromList(items)(context);
    if (result) {
      return result;
    }
  }
  const sources = context.state.languageDataAt<CompletionSource>("autocomplete", context.pos);
  for (const source of sources) {
    const result = await source(context);
    if (result) {
      return result;
    }
  }
  return null;
}

// Toggles read-only editing dynamically (e.g. edit dialog locks fields until
// the user switches to edit mode).
const editableCompartment = new Compartment();

const extensions: Extension[] = [
  rego({ autocomplete: true, hover: true }),
  lineNumbers(),
  highlightActiveLine(),
  highlightCompartment.of(highlightFor(appStore.theme)),
  bracketMatching(),
  autocompletion({ override: [regoCompletion] }),
  // Stub linter so external diagnostics pushed via `setDiagnostics` render.
  linter(() => []),
  lintGutter(),
  history(),
  keymap.of([...defaultKeymap, ...historyKeymap]),
  editableCompartment.of(EditorState.readOnly.of(props.disabled)),
  EditorView.lineWrapping,
  EditorView.theme({
    "&": { fontSize: "13px", height: "100%" },
    ".cm-scroller": {
      fontFamily: "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace",
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

// Sync external content changes (e.g. skeleton generation, snippet chips).
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

// Toggle read-only state when the `disabled` prop changes.
watch(
  () => props.disabled,
  (val) => {
    view.value?.dispatch({
      effects: editableCompartment.reconfigure(EditorState.readOnly.of(val)),
    });
  },
);

// Switch syntax highlighting when the app theme changes.
watch(
  () => appStore.theme,
  (theme) => {
    view.value?.dispatch({
      effects: highlightCompartment.reconfigure(highlightFor(theme)),
    });
  },
);

// Push validation diagnostics (line/column) into the CodeMirror lint state.
watch(
  () => props.errors,
  (errs) => {
    const editorView = view.value;
    if (!editorView) {
      return;
    }
    const doc = editorView.state.doc;
    const diagnostics: Diagnostic[] = (errs ?? []).map((err) => {
      const lineNo = Math.max(1, Math.min(err.line, doc.lines));
      const line = doc.line(lineNo);
      const from = line.from + Math.max(0, Math.min(err.column - 1, line.length));
      return { from, to: line.to, severity: "error", message: err.message };
    });
    editorView.dispatch(setDiagnostics(editorView.state, diagnostics));
  },
  { immediate: true },
);

defineExpose({ focus: () => view.value?.focus() });
</script>

<style scoped>
.rego-editor {
  width: 100%;

  /* visible：允许 lint tooltip 向上溢出显示（如第一行出错时 tooltip 展开在编辑器
     上方），内容滚动仍由 .cm-scroller 管理，不会产生页面溢出 */
  overflow: visible;
  border: 1px solid rgb(0 0 0 / 24%);
  border-radius: 4px;
  background-color: rgb(var(--v-theme-surface));
}

/* CodeMirror tooltip（lint 错误消息 / 补全菜单）跟随 Vuetify 主题。
   否则 dark 下 CM 默认浅底 #f5f5f5 + 继承暗色编辑器的亮色文字，
   亮字亮底对比度不足（如 lint 错误消息几乎不可见）。
   z-index 提升到最上层，避免被对话框内其他元素遮挡 */
:deep(.cm-tooltip) {
  z-index: 5000;
  background-color: rgb(var(--v-theme-surface)) !important;
  border: 1px solid rgb(var(--v-theme-on-surface), 0.12) !important;
  color: rgb(var(--v-theme-on-surface));
}

/* stylelint-disable-next-line selector-class-pattern -- CodeMirror 内部类名 */
:deep(.cm-tooltip .cm-diagnosticText) {
  color: rgb(var(--v-theme-on-surface)) !important;
}
</style>
