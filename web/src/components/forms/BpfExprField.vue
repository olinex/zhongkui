<template>
  <v-input
    :model-value="modelValue"
    :error-messages="displayErrors"
    :focused="isFocused"
    density="compact"
  >
    <template #default="{ id, isDisabled, hasDetails }">
      <v-field
        :id="id.value"
        :label="label"
        :label-id="`${id.value}-label`"
        :active="isFocused || valueFilled"
        :dirty="valueFilled"
        :disabled="isDisabled.value"
        :focused="isFocused"
        :details="hasDetails.value"
        :error="hasError"
        class="bpf-field"
        @click="focusEditor"
      >
        <template #default="{ props: fieldProps }">
          <div v-bind="fieldProps" class="bpf-field-input">
            <div ref="hostRef" class="bpf-editor" />
          </div>
        </template>
        <template #append-inner>
          <v-menu
            v-model="helpOpen"
            :close-on-content-click="false"
            location="bottom end"
            origin="top end"
          >
            <template #activator="{ props: activatorProps }">
              <v-btn
                v-bind="activatorProps"
                icon="mdi-help-circle-outline"
                variant="text"
                size="small"
                :title="$t('bpf.title')"
                :aria-label="$t('bpf.title')"
                @click.stop
              />
            </template>
            <v-card class="zk-panel bpf-help-panel">
              <div class="zk-section-title">{{ $t("bpf.title") }}</div>
              <div v-for="group in helpGroups" :key="group.key" class="bpf-help-group">
                <span class="zk-hint">{{ $t(`bpf.group.${group.key}`) }}</span>
                <div v-for="item in group.items" :key="item.label" class="bpf-help-row">
                  <code class="bpf-help-keyword">{{ item.label }}</code>
                  <span class="zk-hint">{{ $t(item.descriptionKey) }}</span>
                </div>
              </div>
              <div class="zk-section-title">{{ $t("bpf.examplesTitle") }}</div>
              <div class="bpf-help-examples">
                <v-btn
                  v-for="example in BPF_EXAMPLES"
                  :key="example.expr"
                  variant="tonal"
                  size="small"
                  :title="$t(example.descriptionKey)"
                  @click="insertAtCursor(example.expr)"
                >
                  {{ example.expr }}
                </v-btn>
              </div>
            </v-card>
          </v-menu>
        </template>
      </v-field>
    </template>
  </v-input>
</template>

<script setup lang="ts" name="bpf-expr-field">
import {
  autocompletion,
  type Completion,
  type CompletionContext,
  type CompletionResult,
} from "@codemirror/autocomplete";
import { defaultKeymap, history, historyKeymap } from "@codemirror/commands";
import { bracketMatching, StreamLanguage, type StreamParser } from "@codemirror/language";
import { linter } from "@codemirror/lint";
import { Compartment, EditorState, type Extension } from "@codemirror/state";
import { EditorView, hoverTooltip, keymap } from "@codemirror/view";

import { useAppStore } from "@/stores/app";
import { highlightFor } from "@/utils/editorHighlight";
import i18n from "@/utils/i18n";
import {
  BPF_EXAMPLES,
  BPF_KEYWORDS,
  bpfCompletionContext,
  bpfKeywordDoc,
  bpfTokenAt,
  classifyBpfWord,
  parseBpf,
} from "@/utils/bpfExpr";

const props = withDefaults(
  defineProps<{
    /** 绑定的 BPF 表达式文本（v-model）。 */
    modelValue: string;
    /** 字段标题，由 Vuetify 的浮动标签渲染。 */
    label?: string;
    /** 表单校验错误（vee-validate `error-messages`）。 */
    errorMessages?: string | string[];
    /** 禁用编辑（只读），速查与错误展示仍可用。 */
    disabled?: boolean;
  }>(),
  {
    label: "",
    errorMessages: () => [],
    disabled: false,
  },
);

const emit = defineEmits<{ (e: "update:modelValue", value: string): void }>();

const appStore = useAppStore();

const hostRef = useTemplateRef<HTMLDivElement>("hostRef");
const view = shallowRef<EditorView>();
const helpOpen = ref(false);
const isFocused = ref(false);

/** 表单校验错误统一为字符串数组。 */
const formErrors = computed<string[]>(() => {
  const value = props.errorMessages;
  const list = Array.isArray(value) ? value : value ? [value] : [];
  return list.filter((message): message is string => Boolean(message));
});

/** 本地语法诊断随输入实时计算。 */
const localDiagnostics = computed(() => parseBpf(props.modelValue));
const valueFilled = computed(() => props.modelValue.trim() !== "");

/**
 * 展示在 Vuetify 输入框下方的消息：优先表单校验错误，
 * 其次本地语法诊断（后端 libpcap 兜底在提交时执行）。
 */
const displayErrors = computed<string[]>(() =>
  formErrors.value.length > 0
    ? formErrors.value
    : localDiagnostics.value.map((diagnostic) => diagnostic.message),
);
const hasError = computed(() => displayErrors.value.length > 0);

const helpGroups = computed(() => [
  { key: "logical", items: BPF_KEYWORDS.filter((item) => item.kind === "logical") },
  { key: "protocol", items: BPF_KEYWORDS.filter((item) => item.kind === "protocol") },
  { key: "direction", items: BPF_KEYWORDS.filter((item) => item.kind === "direction") },
  { key: "qualifier", items: BPF_KEYWORDS.filter((item) => item.kind === "qualifier") },
]);

/** 点击字段任意区域时把焦点交给 CodeMirror。 */
function focusEditor(): void {
  view.value?.focus();
}

/** 语法高亮随主题切换，配色统一走 utils/editorHighlight 的共享方案。 */
const highlightCompartment = new Compartment();
const editableCompartment = new Compartment();

// 表达式词法流：空白 → 逻辑词 / 协议 / 限定词 / 普通值 → 括号 → 其余视为非法。
const bpfStreamParser: StreamParser<unknown> = {
  name: "bpfExpr",
  token(stream) {
    if (stream.eatSpace()) {
      return null;
    }
    const ch = stream.peek();
    if (ch === "(" || ch === ")" || ch === "[" || ch === "]") {
      stream.next();
      return "bracket";
    }
    if (stream.match("&&") || stream.match("||") || stream.match("!")) {
      return "operator";
    }
    if (stream.match(/^(and|or|not)\b/i)) {
      return "operator";
    }
    if (stream.match(/^[&|=<>^]+/)) {
      return "operator";
    }
    if (stream.match(/^[A-Za-z0-9_./:\\-]+/)) {
      const word = stream.current().toLowerCase();
      if (/^\d+$/.test(word)) {
        return "number";
      }
      const kind = classifyBpfWord(word);
      if (kind === "protocol") {
        return "keyword";
      }
      if (kind === "direction" || kind === "qualifier") {
        return "typeName";
      }
      return "variableName";
    }
    stream.next();
    return "invalid";
  },
};
const bpfLanguage = StreamLanguage.define(bpfStreamParser);

// 本地即时语法校验：与表单规则相互独立，提供行列级定位。
const bpfLinter = linter(
  (editorView) =>
    parseBpf(editorView.state.doc.toString()).map((diagnostic) => ({
      from: diagnostic.from,
      to: Math.max(diagnostic.to, diagnostic.from + 1),
      severity: diagnostic.severity,
      message: diagnostic.message,
    })),
  { delay: 150 },
);

/** 补全：由文法工具给出位置与候选，这里只做 CodeMirror 适配。 */
function bpfCompletion(context: CompletionContext): CompletionResult | null {
  const result = bpfCompletionContext(context.state.doc.toString(), context.pos);
  if (result.items.length === 0) {
    return null;
  }
  // 操作符位置（已完成一个 primitive）默认不自动弹出，避免打断输入；
  // 显式触发（Ctrl+Space）或已有前缀时仍展示。
  if (result.kind === "operator" && !result.prefix && !context.explicit) {
    return null;
  }
  const options: Completion[] = result.items.map((item) => ({
    label: item.label,
    detail: item.detail,
    type: item.type,
    apply: (editorView, _completion, from, to) => {
      editorView.dispatch({
        changes: { from, to, insert: item.insertText },
        selection: { anchor: from + item.insertText.length },
      });
    },
  }));
  return { from: result.from, options };
}

// 悬停说明：命中文法关键字时展示含义与示例。
const keywordHover = hoverTooltip((editorView, pos) => {
  const token = bpfTokenAt(editorView.state.doc.toString(), pos);
  if (
    !token ||
    (token.type !== "word" && token.type !== "and" && token.type !== "or" && token.type !== "not")
  ) {
    return null;
  }
  const meta = bpfKeywordDoc(token.text);
  if (!meta) {
    return null;
  }
  return {
    pos: token.from,
    end: token.to,
    above: true,
    create: () => {
      const dom = document.createElement("div");
      dom.className = "cm-bpf-hover";
      const title = document.createElement("div");
      title.className = "cm-bpf-hover-title";
      title.textContent = meta.label;
      const description = document.createElement("div");
      description.className = "cm-bpf-hover-desc";
      description.textContent = i18n.global.t(meta.descriptionKey);
      dom.append(title, description);
      if (meta.example) {
        const example = document.createElement("pre");
        example.className = "cm-bpf-hover-example";
        example.textContent = meta.example;
        dom.append(example);
      }
      return { dom };
    },
  };
});

/** 在光标处插入速查示例；非空表达式时补一个空格分隔。 */
function insertAtCursor(text: string): void {
  const editor = view.value;
  if (!editor) {
    return;
  }
  const current = editor.state.doc.toString();
  const insert = current.trim() && !/\s$/.test(current) ? ` ${text}` : text;
  const head = editor.state.selection.main.head;
  editor.dispatch({
    changes: { from: head, insert },
    selection: { anchor: head + insert.length },
  });
  editor.focus();
  helpOpen.value = false;
}

// 单行输入：吞掉 Enter，并把含换行的粘贴内容压成一行，
// 使编辑器行为与原生 input 一致，避免出现第二行。
const singleLineKeymap = keymap.of([
  { key: "Enter", run: () => true },
  { key: "Shift-Enter", run: () => true },
  { key: "Mod-Enter", run: () => true },
]);

const singleLinePaste = EditorView.domEventHandlers({
  paste(event, editorView) {
    const text = event.clipboardData?.getData("text/plain") ?? "";
    if (!/[\r\n]/.test(text)) {
      return false;
    }
    event.preventDefault();
    const cleaned = text.replace(/[\r\n]+/g, " ");
    const { from, to } = editorView.state.selection.main;
    editorView.dispatch({
      changes: { from, to, insert: cleaned },
      selection: { anchor: from + cleaned.length },
    });
    return true;
  },
});

const extensions: Extension[] = [
  bpfLanguage,
  highlightCompartment.of(highlightFor(appStore.theme)),
  bracketMatching(),
  autocompletion({ override: [bpfCompletion] }),
  bpfLinter,
  keywordHover,
  singleLinePaste,
  history(),
  singleLineKeymap,
  keymap.of([...defaultKeymap, ...historyKeymap]),
  editableCompartment.of(EditorState.readOnly.of(props.disabled)),
  // 单行代码编辑区：外边距 / 描边 / 高度全部交给 Vuetify VField，
  // 这里只保留等宽字体、行高与横向滚动，避免与输入框外观分裂。
  EditorView.theme({
    "&": { background: "transparent" },
    ".cm-scroller": {
      fontFamily: "var(--zk-font-mono)",
      // 与 Vuetify 输入框行盒一致（1.5rem = 24px），使字段总高精确为 40px。
      lineHeight: "1.5",
      overflowX: "auto",
      overflowY: "hidden",
      scrollbarWidth: "none",
    },
    ".cm-scroller::-webkit-scrollbar": { height: "0" },
    ".cm-content": { padding: "0" },
    "&.cm-focused": { outline: "none" },
  }),
  EditorView.updateListener.of((update) => {
    if (update.focusChanged) {
      isFocused.value = update.view.hasFocus;
    }
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

// 同步外部内容变化（表单复位、详情回填）。
watch(
  () => props.modelValue,
  (value) => {
    const editor = view.value;
    if (!editor || editor.state.doc.toString() === value) {
      return;
    }
    editor.dispatch({
      changes: { from: 0, to: editor.state.doc.length, insert: value },
    });
  },
);

// `disabled` 变化时切换只读状态。
watch(
  () => props.disabled,
  (value) => {
    view.value?.dispatch({
      effects: editableCompartment.reconfigure(EditorState.readOnly.of(value)),
    });
  },
);

// 主题变化时切换语法高亮。
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
.bpf-field-input {
  width: 100%;
}

.bpf-editor {
  flex: 1 1 auto;
  min-width: 0;
  display: flex;
  align-items: center;
}

/* 编辑器只占字段内容区，描边 / 圆角 / 标签由 Vuetify VField 提供 */
.bpf-editor :deep(.cm-editor) {
  width: 100%;
  min-width: 0;
  background: transparent;
}

.bpf-help-panel {
  display: flex;
  flex-direction: column;
  gap: var(--zk-space-2);
  width: 380px;
  max-width: 90vw;
  max-height: 420px;
  overflow-y: auto;
  padding: var(--zk-space-3);
}

.bpf-help-group {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.bpf-help-row {
  display: flex;
  align-items: baseline;
  gap: var(--zk-space-2);
}

.bpf-help-keyword {
  min-width: 72px;
  font-family: var(--zk-font-mono);
  font-size: var(--zk-font-size-caption);
  color: rgb(var(--v-theme-primary));
  background: none;
}

.bpf-help-examples {
  display: flex;
  flex-wrap: wrap;
  gap: var(--zk-space-1);
}

/* hover tooltip 由 CodeMirror 动态创建，需经 :deep 命中 */
:deep(.cm-bpf-hover) {
  display: flex;
  flex-direction: column;
  gap: 2px;
  max-width: 320px;
}

:deep(.cm-bpf-hover-title) {
  font-family: var(--zk-font-mono);
  font-weight: var(--zk-font-weight-semibold);
}

:deep(.cm-bpf-hover-desc) {
  font-size: var(--zk-font-size-caption);
  color: rgb(var(--v-theme-on-surface-variant));
}

:deep(.cm-bpf-hover-example) {
  margin: 0;
  font-family: var(--zk-font-mono);
  font-size: var(--zk-font-size-caption);
}
</style>
