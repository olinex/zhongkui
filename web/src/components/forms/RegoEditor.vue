<template>
  <div class="rego-editor-wrap">
    <div v-if="fields.length > 0" class="input-fields-panel">
      <div class="panel-header">
        <span class="panel-title">{{ $t("title.inputFields") }}</span>
        <v-btn
          variant="text"
          size="x-small"
          density="comfortable"
          :icon="panelCollapsed ? 'mdi-chevron-down' : 'mdi-chevron-up'"
          @click="panelCollapsed = !panelCollapsed"
        />
      </div>
      <div v-show="!panelCollapsed" class="panel-body">
        <v-text-field
          v-model="fieldFilter"
          density="compact"
          variant="outlined"
          :placeholder="$t('action.searchFields')"
          hide-details
          clearable
          class="field-search"
        />
        <div class="field-list">
          <button
            v-for="field in filteredFields"
            :key="field.field"
            type="button"
            class="field-item"
            @click="insertField(field)"
          >
            <span class="field-name">{{ field.field }}</span>
            <v-chip size="x-small" density="comfortable" variant="tonal" class="field-type">
              {{ inputTypeLabel(field.type) }}
            </v-chip>
            <v-icon v-if="field.sensitive" size="x-small">mdi-lock</v-icon>
            <span v-if="field.description" class="field-desc">{{ field.description }}</span>
          </button>
        </div>
      </div>
    </div>
    <div ref="hostRef" class="rego-editor" :style="{ minHeight }" />
  </div>
</template>

<script setup lang="ts" name="rego-editor">
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
import i18n from "@/utils/i18n";
import { classifyRegoCompletionContext, fieldSnippet, isRegoExpressionKeyword } from "@/utils/rego";
import type { InputFieldMeta, RegoValidationError } from "@/utils/rego";

const props = withDefaults(
  defineProps<{
    modelValue: string;
    errors?: RegoValidationError[];
    /** extra `input.<field>` completions, e.g. fields referenced by existing rules */
    extraCompletions?: string[];
    /** input document field metadata from backend, drives `input.` completion & fields panel */
    fields?: InputFieldMeta[];
    /** disables editing (read-only), the component stays interactive */
    disabled?: boolean;
    minHeight?: string;
  }>(),
  {
    errors: () => [],
    extraCompletions: () => [],
    fields: () => [],
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

// Collapsible "input fields" panel state, expanded by default.
const panelCollapsed = ref(false);
const fieldFilter = ref("");

/** Fields filtered by the panel search box against name and description. */
const filteredFields = computed<InputFieldMeta[]>(() => {
  const keyword = fieldFilter.value.trim().toLowerCase();
  if (!keyword) {
    return props.fields;
  }
  return props.fields.filter(
    (f) =>
      f.field.toLowerCase().includes(keyword) ||
      (f.description ?? "").toLowerCase().includes(keyword),
  );
});

/** Inserts a field snippet at the current cursor position, then refocuses. */
function insertField(field: InputFieldMeta): void {
  const editorView = view.value;
  if (!editorView) {
    return;
  }
  const head = editorView.state.selection.main.head;
  editorView.dispatch({
    changes: { from: head, to: head, insert: fieldSnippet(field) },
  });
  editorView.focus();
}

/** Localizes a semantic input type key, falling back to the raw type text. */
function inputTypeLabel(type: string): string {
  const key = `inputType.${type}`;
  return i18n.global.te(key) ? i18n.global.t(key) : type;
}

/**
 * Builds `input.<field>` completions from the `fields` metadata merged with
 * `extraCompletions` (deduplicated by field name, metadata wins). Sensitive
 * fields get a marker in the label and a hint in the detail text.
 */
function inputFieldCompletions(): Completion[] {
  const items: Completion[] = [];
  const seen = new Set<string>();
  for (const field of props.fields) {
    seen.add(field.field);
    const parts = [inputTypeLabel(field.type)];
    if (field.description) {
      parts.push(field.description);
    }
    if (field.sensitive) {
      parts.push("sensitive");
    }
    items.push({
      label: field.sensitive ? `${field.field} (sensitive)` : field.field,
      detail: parts.join(" · "),
      type: "variable",
      apply: (v, c, from, to) => applySnippet(v, c, from, to, fieldSnippet(field)),
    });
  }
  for (const field of props.extraCompletions) {
    if (seen.has(field)) {
      continue;
    }
    seen.add(field);
    items.push({
      label: field,
      detail: "existing rule field",
      type: "variable",
      apply: (v, c, from, to) => applySnippet(v, c, from, to, `input.${field} == ""`),
    });
  }
  return items;
}

/**
 * Custom completion source: completes `input.<field>` from field metadata and
 * `extraCompletions`, then falls back to the Rego language sources (keywords,
 * OPA built-ins) registered by `codemirror-lang-rego`. The fallback is
 * position-aware: no completion is offered where the next word would be a
 * syntax error (e.g. right after `input.field == "..."`), and term positions
 * (e.g. after `==`) drop statement-starting keywords like `if`/`else`.
 */
async function regoCompletion(context: CompletionContext): Promise<CompletionResult | null> {
  const line = context.state.doc.lineAt(context.pos);
  const textBefore = line.text.slice(0, context.pos - line.from);
  const dotMatch = /input\.([a-zA-Z_][a-zA-Z0-9_]*)$/.exec(textBefore);
  if (dotMatch) {
    const result = await completeFromList(inputFieldCompletions())(context);
    if (result) {
      return result;
    }
  }
  const kind = classifyRegoCompletionContext(textBefore);
  if (kind === "block") {
    return null;
  }
  const sources = context.state.languageDataAt<CompletionSource>("autocomplete", context.pos);
  for (const source of sources) {
    const result = await source(context);
    if (result) {
      if (kind === "term") {
        return {
          from: result.from,
          validFor: result.validFor,
          options: result.options.filter((c) => !isRegoExpressionKeyword(c.label)),
        };
      }
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
      const hint = err.hint ? `\n${err.hint}` : "";
      return {
        from,
        to: line.to,
        severity: err.type === "warning" ? "warning" : "error",
        message: err.message + hint,
      };
    });
    editorView.dispatch(setDiagnostics(editorView.state, diagnostics));
  },
  { immediate: true },
);

defineExpose({ focus: () => view.value?.focus() });
</script>

<style scoped>
.rego-editor-wrap {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

/* Input fields panel reuses the editor border/radius/surface so both blocks
   read as one unit and follow the app theme (light/dark). */
.input-fields-panel {
  border: 1px solid rgb(0 0 0 / 24%);
  border-radius: 4px;
  background-color: rgb(var(--v-theme-surface));
}

.panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 32px;
  padding: 0 4px 0 12px;
}

.panel-title {
  font-size: 13px;
  font-weight: 600;
  color: rgb(var(--v-theme-on-surface));
}

.panel-body {
  padding: 0 12px 12px;
}

.field-search {
  margin-bottom: 8px;
}

.field-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  max-height: 180px;
  overflow-y: auto;
}

.field-item {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 4px 8px;
  border: none;
  border-radius: 4px;
  background: transparent;
  color: rgb(var(--v-theme-on-surface));
  text-align: left;
  cursor: pointer;
}

.field-item:hover {
  background: rgb(var(--v-theme-on-surface), 0.06);
}

.field-name {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 13px;
  white-space: nowrap;
}

.field-desc {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  font-size: 12px;
  color: rgb(var(--v-theme-on-surface-variant));
  text-overflow: ellipsis;
  white-space: nowrap;
}

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
