<template>
  <div class="rego-editor-wrap">
    <div v-if="samples.length > 0" class="sample-selector">
      <div class="panel-header">
        <span class="panel-title">{{ $t("action.selectSample") }}</span>
      </div>
      <div class="sample-selector-body">
        <v-select
          v-model="selectedSampleName"
          :items="samples.map((s) => ({ title: s.name, value: s.name }))"
          density="compact"
          variant="outlined"
          hide-details
          clearable
        >
          <template #append-inner>
            <v-btn
              variant="text"
              size="small"
              icon="mdi-eye-outline"
              :disabled="!selectedSampleName"
              :title="$t('action.viewInput')"
              @click.stop="emit('viewInput')"
            />
          </template>
        </v-select>
      </div>
    </div>
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
import {
  EditorView,
  highlightActiveLine,
  hoverTooltip,
  keymap,
  lineNumbers,
} from "@codemirror/view";
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
    /** available samples for the current model, drives the sample selector */
    samples?: { name: string; input: Record<string, unknown> }[];
    /** disables editing (read-only), the component stays interactive */
    disabled?: boolean;
    minHeight?: string;
  }>(),
  {
    errors: () => [],
    extraCompletions: () => [],
    fields: () => [],
    samples: () => [],
    disabled: false,
    minHeight: "240px",
  },
);

const emit = defineEmits<{
  (e: "update:modelValue", value: string): void;
  (e: "update:sampleInput", value: Record<string, unknown> | null): void;
  (e: "viewInput"): void;
}>();

const appStore = useAppStore();

const hostRef = useTemplateRef<HTMLDivElement>("hostRef");
const view = shallowRef<EditorView>();

// ---- sample selector state ----
// 试运行样例与编辑器联动：用户预先选择一个样例后，编辑器对 `input.<field>`
// 的补全与悬停提示会展示该字段在样例中的实际值，便于快速判断规则合法性。
const selectedSampleName = ref("");
const selectedSampleInput = shallowRef<Record<string, unknown> | null>(null);

/** Formats a sample value for display in completion/hover hints. */
function formatSampleValue(value: unknown): string {
  if (value === null) {
    return "null";
  }
  if (value === undefined) {
    return "undefined";
  }
  if (typeof value === "string") {
    return JSON.stringify(value);
  }
  if (typeof value === "object") {
    return JSON.stringify(value);
  }
  return String(value);
}

/** Looks up the sample value of an `input.<field>` reference, if any. */
function sampleValueFor(field: string): unknown {
  return selectedSampleInput.value?.[field];
}

/** Finds the `input.<field>` token covering the given document position. */
function inputFieldAt(
  editorView: EditorView,
  pos: number,
): { field: string; from: number; to: number } | null {
  const line = editorView.state.doc.lineAt(pos);
  const rel = pos - line.from;
  const re = /input\.([a-zA-Z_][a-zA-Z0-9_]*)/g;
  let m: RegExpExecArray | null;
  while ((m = re.exec(line.text)) !== null) {
    const start = m.index;
    const end = m.index + m[0].length;
    if (rel >= start && rel <= end) {
      return { field: m[1], from: line.from + start, to: line.from + end };
    }
  }
  return null;
}

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
 * fields get a marker in the label and a hint in the detail text. When a
 * sample is selected, the field's actual value is appended to the detail so
 * the user can judge rule correctness while typing.
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
    const sampleValue = sampleValueFor(field.field);
    if (sampleValue !== undefined) {
      parts.push(`= ${formatSampleValue(sampleValue)}`);
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
    const parts = ["existing rule field"];
    const sampleValue = sampleValueFor(field);
    if (sampleValue !== undefined) {
      parts.push(`= ${formatSampleValue(sampleValue)}`);
    }
    items.push({
      label: field,
      detail: parts.join(" · "),
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

// Hover tooltip: when a sample is selected, hovering an `input.<field>`
// reference shows the field's actual value from the sample, so the user can
// judge rule correctness without running a dry-run.
const sampleHover = hoverTooltip((editorView, pos) => {
  const hit = inputFieldAt(editorView, pos);
  if (!hit) {
    return null;
  }
  const value = sampleValueFor(hit.field);
  if (value === undefined) {
    return null;
  }
  return {
    pos: hit.from,
    end: hit.to,
    above: true,
    create: () => {
      const dom = document.createElement("div");
      dom.className = "cm-sample-hover";
      const label = document.createElement("span");
      label.className = "cm-sample-hover-label";
      label.textContent = hit.field;
      const val = document.createElement("span");
      val.className = "cm-sample-hover-value";
      val.textContent = formatSampleValue(value);
      dom.append(label, val);
      return { dom };
    },
  };
});

const extensions: Extension[] = [
  rego({ autocomplete: true, hover: true }),
  lineNumbers(),
  highlightActiveLine(),
  highlightCompartment.of(highlightFor(appStore.theme)),
  bracketMatching(),
  autocompletion({ override: [regoCompletion] }),
  sampleHover,
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

// Reset the sample selection when the sample list changes (e.g. model switch).
watch(
  () => props.samples,
  (list) => {
    selectedSampleName.value = list[0] ? list[0].name : "";
  },
  { immediate: true },
);

// Emit the selected sample's input so the parent can share it with the
// dry-run panel, keeping the editor and the dry-run feature in sync.
watch(selectedSampleName, (name) => {
  const found = props.samples.find((s) => s.name === name);
  selectedSampleInput.value = found ? found.input : null;
  emit("update:sampleInput", selectedSampleInput.value);
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

/* Sample selector sits above the editor as a bordered section matching the
   input-fields panel and the editor box, so the user can pre-select a sample
   while writing the rule; the selected sample drives input value hints. */
.sample-selector {
  border: 1px solid rgb(0 0 0 / 24%);
  border-radius: 4px;
  background-color: rgb(var(--v-theme-surface));
}

.sample-selector-body {
  padding: 0 12px 12px;
}

/* Hover tooltip content for `input.<field>` sample values. */
:deep(.cm-sample-hover) {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 2px 0;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12px;
}

:deep(.cm-sample-hover-label) {
  color: rgb(var(--v-theme-on-surface-variant));
}

:deep(.cm-sample-hover-value) {
  font-weight: 600;
  color: rgb(var(--v-theme-primary));
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
