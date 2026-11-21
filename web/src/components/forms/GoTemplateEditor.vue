<template>
  <code-mirror-field
    :label="label"
    :value="modelValue"
    :error-messages="errorMessage"
    :disabled="disabled"
    :min-height="minHeight"
    @focus="focusEditor"
  >
    <div ref="hostRef" class="go-template-editor-host" />
  </code-mirror-field>
</template>

<script setup lang="ts" name="go-template-editor">
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
import {
  EditorView,
  highlightActiveLine,
  keymap,
  lineNumbers,
  placeholder,
} from "@codemirror/view";

import { useAppStore } from "@/stores/app";
import { highlightFor } from "@/utils/editorHighlight";

const props = withDefaults(
  defineProps<{
    modelValue: string;
    /** 可用模板变量名清单（不含点前缀），来自后端逐事件事件字段 */
    fields?: string[];
    /** 字段标题，展示于编辑器头部 */
    label?: string;
    /** 校验通过，渲染成功标记 */
    valid?: boolean;
    /** 校验错误信息，渲染在编辑器下方 */
    errorMessage?: string;
    /** 禁用编辑（只读） */
    disabled?: boolean;
    /** 编辑器最小高度 */
    minHeight?: string;
    /** 空文档提示文案 */
    placeholderText?: string;
  }>(),
  {
    fields: () => [],
    label: "",
    valid: false,
    errorMessage: "",
    disabled: false,
    minHeight: "120px",
    placeholderText: "",
  },
);

const emit = defineEmits<{ (e: "update:modelValue", value: string): void }>();

const appStore = useAppStore();

const hostRef = useTemplateRef<HTMLDivElement>("hostRef");
const view = shallowRef<EditorView>();

/** 点击字段任意区域时把焦点交给 CodeMirror。 */
function focusEditor(): void {
  view.value?.focus();
}

// 语法高亮随主题切换，配色统一走 utils/editorHighlight 的共享方案。
const highlightCompartment = new Compartment();

// ---- Go text/template 语法词法 ----
const GO_KEYWORDS = new Set([
  "if",
  "else",
  "end",
  "range",
  "with",
  "define",
  "template",
  "block",
  "var",
]);
const GO_FUNCS = new Set([
  "and",
  "or",
  "not",
  "eq",
  "ne",
  "lt",
  "le",
  "gt",
  "ge",
  "len",
  "index",
  "slice",
  "printf",
  "println",
  "print",
  "call",
]);

interface GoTemplateState {
  // 当前是否处于 {{ ... }} 动作块内
  inAction: boolean;
}

function goTemplateStartState(): GoTemplateState {
  return { inAction: false };
}

const goTemplateStreamParser: StreamParser<GoTemplateState> = {
  name: "goTemplate",
  startState: goTemplateStartState,
  token(stream, state) {
    if (stream.eatSpace()) {
      return null;
    }
    // {{ 或 {{- 开始动作
    if (stream.match(/{{-?/)) {
      state.inAction = true;
      return "meta";
    }
    // }} 或 -}} 结束动作
    if (stream.match(/-?}}/)) {
      state.inAction = false;
      return "meta";
    }
    if (!state.inAction) {
      // 动作块外的纯文本：原样输出
      stream.next();
      return null;
    }
    const c = stream.peek();
    if (c === "|") {
      stream.next();
      return "operator";
    }
    if (c === "(" || c === ")") {
      stream.next();
      return "bracket";
    }
    if (c === "=" || c === ":") {
      stream.next();
      return "operator";
    }
    if (c === '"') {
      // 字符串字面量：自开引号起逐个吞字符，忽略转义，直到闭引号或行尾
      stream.next();
      while (!stream.eol()) {
        if (stream.peek() === "\\") {
          stream.next();
          if (!stream.eol()) {
            stream.next();
          }
          continue;
        }
        if (stream.peek() === '"') {
          stream.next();
          break;
        }
        stream.next();
      }
      return "string";
    }
    if (stream.match(/[+\-]?[0-9]+(?:\.[0-9]+)?/)) {
      return "number";
    }
    if (c === "$") {
      // 变量 $var / $ 根变量
      stream.next();
      stream.match(/[\p{L}\p{N}_]*/u);
      return "variableName";
    }
    if (c === ".") {
      // 当前值 "."；否则字段引用 .Field / .Field.Sub
      if (stream.match(/\.\s*$/)) {
        return "operator";
      }
      stream.next();
      stream.match(/[\p{L}\p{N}_]*(?:\.[\p{L}\p{N}_]*)*/u);
      return "propertyName";
    }
    const w = stream.match(/[\p{L}\p{N}_]+/u) ? stream.current() : "";
    if (w) {
      if (GO_KEYWORDS.has(w)) {
        return "keyword";
      }
      if (GO_FUNCS.has(w)) {
        return "function";
      }
      return "variableName";
    }
    stream.next();
    return null;
  },
};

const goTemplateLanguage = StreamLanguage.define(goTemplateStreamParser);

// ---- live lint：轻量前端提示（{{ / }} 配对、未闭合动作）；权威语法校验由后端接口完成 ----
const goTemplateLinter = linter((editorView) => {
  const text = editorView.state.doc.toString();
  const errors: { from: number; to: number; severity: "error"; message: string }[] = [];
  let depth = 0;
  const re = /\{\{-?|-?\}\}/g;
  let matched: RegExpExecArray | null;
  while ((matched = re.exec(text)) !== null) {
    const isOpen = matched[0].startsWith("{{");
    if (isOpen) {
      depth += 1;
    } else {
      depth -= 1;
      if (depth < 0) {
        errors.push({
          from: matched.index,
          to: matched.index + matched[0].length,
          severity: "error",
          message: "多余的 }}",
        });
        depth = 0;
      }
    }
  }
  if (depth > 0) {
    const last = text.lastIndexOf("{{");
    errors.push({
      from: last,
      to: last + 2,
      severity: "error",
      message: "未闭合的 {{ 动作",
    });
  }
  return errors;
});

// ---- autocomplete：Go 模板关键字/函数 + 当前事件的可选变量 ----
// 光标位于 {{ ... }} 动作内 或显式触发（Ctrl+Space）时提供补全；
// 前缀以 "." 开头时仅补全变量字段，否则补全关键字/函数（空前缀时同时列出变量）。
function insideAction(fullBefore: string): boolean {
  let depth = 0;
  const re = /\{\{-?|-?\}\}/g;
  let matched: RegExpExecArray | null;
  while ((matched = re.exec(fullBefore)) !== null) {
    if (matched[0].startsWith("{{")) {
      depth += 1;
    } else {
      depth -= 1;
    }
  }
  return depth > 0;
}

async function goTemplateCompletion(context: CompletionContext): Promise<CompletionResult | null> {
  const fullBefore = context.state.doc.toString().slice(0, context.pos);
  if (!insideAction(fullBefore) && !context.explicit) {
    return null;
  }
  const line = context.state.doc.lineAt(context.pos);
  const before = line.text.slice(0, context.pos - line.from);
  const m = /[.\w-]*$/u.exec(before);
  const from = m ? context.pos - m[0].length : context.pos;
  const prefix = m?.[0] ?? "";
  const options: Completion[] = [];
  const isField = prefix.startsWith(".");
  if (!isField) {
    for (const kw of GO_KEYWORDS) {
      if (!prefix || kw.startsWith(prefix.toLowerCase())) {
        options.push({ label: kw, type: "keyword", detail: "template" });
      }
    }
    for (const fn of GO_FUNCS) {
      if (!prefix || fn.startsWith(prefix.toLowerCase())) {
        options.push({ label: fn, type: "function", detail: "builtin" });
      }
    }
  }
  const fpref = isField ? prefix.slice(1) : prefix;
  for (const field of props.fields) {
    if (isField || prefix === "") {
      if (!fpref || field.toLowerCase().startsWith(fpref.toLowerCase())) {
        options.push({ label: `.${field}`, type: "variable", detail: "field" });
      }
    }
  }
  if (options.length === 0) {
    return null;
  }
  return { from, options };
}

// 动态切换只读编辑状态。
const editableCompartment = new Compartment();

const extensions: Extension[] = [
  lineNumbers(),
  goTemplateLanguage,
  highlightCompartment.of(highlightFor(appStore.theme)),
  highlightActiveLine(),
  bracketMatching(),
  autocompletion({ override: [goTemplateCompletion] }),
  goTemplateLinter,
  ...(props.placeholderText ? [placeholder(props.placeholderText)] : []),
  history(),
  keymap.of([...defaultKeymap, ...historyKeymap]),
  editableCompartment.of(EditorState.readOnly.of(props.disabled)),
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

// 同步外部内容变更。
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

// 当 disabled 属性变化时切换只读状态。
watch(
  () => props.disabled,
  (val) => {
    view.value?.dispatch({
      effects: editableCompartment.reconfigure(EditorState.readOnly.of(val)),
    });
  },
);

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
.go-template-editor-host {
  flex: 1 1 auto;
  min-height: 0;
  height: 100%;
}
</style>
