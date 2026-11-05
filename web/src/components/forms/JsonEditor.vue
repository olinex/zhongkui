<template>
  <div class="json-editor-wrap">
    <div class="zk-editor-header">
      <span class="zk-editor-title">{{ label }}</span>
      <v-spacer />
      <v-icon v-if="valid" size="x-small" color="success" :title="$t('action.validatePassed')">
        mdi-check-circle
      </v-icon>
      <v-icon v-if="shownError" size="x-small" color="error" :title="shownError">
        mdi-alert-circle
      </v-icon>
    </div>
    <!-- 编辑区与操作行同处一个描边外壳内：操作按钮行位于框内底部右侧，
         与助手输入区（assistant-composer 的 toolbar 行）的组织方式一致 -->
    <div class="zk-editor json-editor-box" :class="{ 'is-error': !!shownError }">
      <div ref="hostRef" class="json-editor-host" />
      <div class="json-editor-toolbar">
        <v-btn
          class="beautify-btn"
          variant="text"
          size="small"
          icon="mdi-auto-fix"
          :disabled="disabled || !canBeautify"
          :title="$t('action.beautifyJson')"
          :aria-label="$t('action.beautifyJson')"
          @click="beautify"
        />
      </div>
    </div>
    <div v-if="shownError" class="zk-error-text">{{ shownError }}</div>
    <div v-if="hint" class="zk-hint">{{ hint }}</div>
  </div>
</template>

<script setup lang="ts" name="json-editor">
import { defaultKeymap, history, historyKeymap } from "@codemirror/commands";
import { json, jsonParseLinter } from "@codemirror/lang-json";
import { bracketMatching } from "@codemirror/language";
import { linter } from "@codemirror/lint";
import { Compartment, EditorState, type Extension } from "@codemirror/state";
import { EditorView, keymap, lineNumbers, placeholder } from "@codemirror/view";

import { useAppStore } from "@/stores/app";
import { highlightFor } from "@/utils/editorHighlight";

const props = withDefaults(
  defineProps<{
    modelValue: string;
    /** 字段标题，展示于编辑器头部 */
    label?: string;
    /** 辅助说明，渲染在编辑器下方 */
    hint?: string;
    /** 校验通过，渲染成功标记 */
    valid?: boolean;
    /** 外部（表单）校验错误信息，优先于组件自身的 JSON 语法错误 */
    errorMessage?: string;
    /** 禁用编辑（只读） */
    disabled?: boolean;
    /** 空文档提示文案 */
    placeholderText?: string;
  }>(),
  {
    label: "",
    hint: "",
    valid: false,
    errorMessage: "",
    disabled: false,
    placeholderText: "",
  },
);

const emit = defineEmits<{ (e: "update:modelValue", value: string): void }>();

const appStore = useAppStore();

const hostRef = useTemplateRef<HTMLDivElement>("hostRef");
const view = shallowRef<EditorView>();

// 语法高亮随主题切换，配色统一走 utils/editorHighlight 的共享方案。
const highlightCompartment = new Compartment();

// 组件自身的 JSON 语法错误：空文档不视为错误，解析失败时给出错误信息。
const parseError = computed(() => {
  const text = props.modelValue.trim();
  if (!text) {
    return "";
  }
  try {
    JSON.parse(text);
    return "";
  } catch (e) {
    return String((e as Error).message);
  }
});

// 外部（表单）错误优先展示，否则回落到组件自身的 JSON 语法错误。
const shownError = computed(() => props.errorMessage || parseError.value);

// 仅当内容非空且能成功解析为 JSON 时，才允许美化。
const canBeautify = computed(() => props.modelValue.trim() !== "" && parseError.value === "");

// 美化：以 2 空格缩进重排当前 JSON 文本。
const beautify = (): void => {
  const parsed = JSON.parse(props.modelValue);
  emit("update:modelValue", JSON.stringify(parsed, null, 2));
};

// 动态切换只读编辑状态。
const editableCompartment = new Compartment();

const extensions: Extension[] = [
  lineNumbers(),
  json(),
  highlightCompartment.of(highlightFor(appStore.theme)),
  linter(jsonParseLinter()),
  bracketMatching(),
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

// 同步外部内容变更（如表单复位 / 详情回填 / 美化）。
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
.json-editor-wrap {
  display: flex;
  flex-direction: column;
  gap: var(--zk-space-1);
}

/* 描边外壳承载编辑区与操作行两层（与 .zk-editor 的描边/圆角叠加使用） */
.json-editor-box {
  display: flex;
  flex-direction: column;
}

/* 编辑区固定高度：长 JSON 在 CodeMirror 自身的滚动容器内纵向浏览，与 MarkdownEditor 正文同高 */
.json-editor-host {
  flex: 0 0 auto;
  height: var(--zk-editor-body-height);
}

/* 操作行：按钮贴右，与助手输入区的 toolbar 行同构 */
.json-editor-toolbar {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  padding: 0 var(--zk-space-1) var(--zk-space-1);
}
</style>
