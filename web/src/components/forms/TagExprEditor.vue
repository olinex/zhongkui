<template>
  <code-mirror-field
    :label="$t('model.tagEventCondition.field.expression')"
    :value="modelValue"
    :error-messages="errorMessage"
    :disabled="disabled"
    :min-height="minHeight"
    @focus="focusEditor"
  >
    <div ref="hostRef" class="tag-expr-editor-host" />
  </code-mirror-field>
</template>

<script setup lang="ts" name="tag-expr-editor">
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
import { parseBooleanExpr } from "@/utils/booleanExpr";
import { findSlashPos, slashKeyBinding, withSlashApply } from "@/utils/slashTrigger";
import i18n from "@/utils/i18n";

const props = withDefaults(
  defineProps<{
    /** 绑定的表达式文本（v-model）。 */
    modelValue: string;
    /** 自动补全可用的标签（标签池）。 */
    tags?: string[];
    /** 校验通过，渲染成功标记。 */
    valid?: boolean;
    /** 校验错误消息，渲染在编辑器下方。 */
    errorMessage?: string;
    /** 禁用编辑（只读）。 */
    disabled?: boolean;
    /** 编辑器最小高度。 */
    minHeight?: string;
  }>(),
  {
    tags: () => [],
    valid: false,
    errorMessage: "",
    disabled: false,
    minHeight: "260px",
  },
);

// 表达式文本变更事件。
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

// 表达式词法流：空白 → 关键词（AND/OR/NOT）→ 括号 → 标签标识符（Unicode）→ 其余视为非法
const exprStreamParser: StreamParser<unknown> = {
  name: "tagExpr",
  token(stream) {
    if (stream.eatSpace()) {
      return null;
    }
    if (stream.match(/\b(AND|OR|NOT)\b/)) {
      return "keyword";
    }
    if (stream.peek() === "(" || stream.peek() === ")") {
      stream.next();
      return "bracket";
    }
    if (stream.match(/[\p{L}][\p{L}\p{N}_-]*/u)) {
      return "variableName";
    }
    stream.next();
    return "invalid";
  },
};
const exprLanguage = StreamLanguage.define(exprStreamParser);

// ---- 实时检查：复用前后端一致的 parseBooleanExpr，出错位置标红下划线 + 悬停提示
const exprLinter = linter((editorView) => {
  const text = editorView.state.doc.toString();
  // 空表达式交给表单必填校验，避免首次打开就出现“不能为空”的红色错误
  if (!text.trim()) {
    return [];
  }
  const p = parseBooleanExpr(text);
  if (p.ok) {
    return [];
  }
  const pos = Math.min(p.offset ?? 0, editorView.state.doc.length);
  const line = editorView.state.doc.lineAt(pos);
  const col = Math.max(0, pos - line.from);
  return [
    {
      from: line.from + col,
      to: line.to,
      severity: "error" as const,
      message: p.error ?? "invalid expression",
    },
  ];
});

// ---- autocomplete：关键词 + 标签池（位置感知的智能提示）----
// 输入词元时按前缀过滤关键词与标签；光标位于期待操作数的位置
// （表达式开头、AND/OR/NOT/左括号之后）时，自动列出可用标签与 NOT。

/** 光标前的文本是否期待一个新的操作数（标签 / NOT / 括号）。 */
function expectsOperand(textBefore: string): boolean {
  const t = textBefore.trimEnd();
  if (!t) {
    return true;
  }
  return /(?:^|[\s(]+)(AND|OR|NOT)\s*$/i.test(t) || /\(\s*$/.test(t);
}

/** 去重后的可用标签补全项；prefix 非空时仅保留前缀匹配（大小写不敏感，与 CM 内置一致）的标签。 */
function tagCompletions(prefix: string): Completion[] {
  const seen = new Set<string>();
  const options: Completion[] = [];
  const matches = (tag: string): boolean => tag.toLowerCase().startsWith(prefix.toLowerCase());
  for (const tag of props.tags) {
    if (seen.has(tag)) {
      continue;
    }
    seen.add(tag);
    if (!prefix || matches(tag)) {
      options.push({ label: tag, type: "variable", detail: "tag" });
    }
  }
  return options;
}

async function exprCompletion(context: CompletionContext): Promise<CompletionResult | null> {
  const line = context.state.doc.lineAt(context.pos);
  const textBefore = line.text.slice(0, context.pos - line.from);

  // 1. "/" 标签触发器：弹出可用标签下拉并随输入实时过滤，规则与 rego 编辑器
  //    一致（三条斜杠输入规则见 slashTrigger.ts）。from 指向触发斜杠本身，使
  //    孤立 "/"（空查询）也满足 from < to 立即弹层；filter:false 关闭 CM 内置
  //    过滤（由 tagCompletions 按斜杠后查询过滤），不设 validFor 每次输入重查
  //    实现「输入即筛选」。标签名可含 Unicode/连字符，查询与过滤按标签标识符字符集判定。
  const slashFrom = findSlashPos(context.state, context.pos);
  if (slashFrom !== null) {
    const query = textBefore.slice(slashFrom - line.from + 1);
    if (/^[\p{L}\p{N}_-]*$/u.test(query)) {
      const options = withSlashApply(tagCompletions(query));
      if (options.length > 0) {
        return { from: slashFrom, options, filter: false };
      }
    }
    // 触发斜杠存在但查询含非法字符（粘贴等场景）：不弹下拉，也不落到关键词补全
    return null;
  }
  // "//" 视为字面量斜杠：不弹任何下拉
  if (/\/\/$/.test(textBefore)) {
    return null;
  }

  const m = /[\p{L}\p{N}_-]*$/u.exec(textBefore);
  const from = m ? context.pos - m[0].length : context.pos;
  const prefix = m?.[0] ?? "";

  if (!prefix && !context.explicit && !expectsOperand(textBefore)) {
    return null;
  }

  const options: Completion[] = [];
  if (prefix) {
    for (const kw of ["AND", "OR", "NOT"]) {
      if (kw.startsWith(prefix.toUpperCase())) {
        options.push({ label: kw, type: "keyword", detail: "operator" });
      }
    }
  }
  options.push(...tagCompletions(prefix));
  if (!prefix) {
    options.push({ label: "NOT", type: "keyword", detail: "operator" });
    options.push({ label: "(", type: "bracket" });
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
  exprLanguage,
  highlightCompartment.of(highlightFor(appStore.theme)),
  highlightActiveLine(),
  bracketMatching(),
  autocompletion({ override: [exprCompletion] }),
  exprLinter,
  // 空文档时显示 "/" 用法提示（点击输入后自动隐藏），与 rego 编辑器一致
  placeholder(i18n.global.t("action.tagExprFieldHint")),
  history(),
  // "/" 输入规则见 slashTrigger.ts（与 rego 编辑器一致）
  keymap.of([slashKeyBinding]),
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

// 同步外部内容变更（如追加标签芯片 / 生成表达式）。
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

// 当 disabled prop 变化时切换只读状态。
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
.tag-expr-editor-host {
  flex: 1 1 auto;
  min-height: 0;
  height: 100%;
}
</style>
