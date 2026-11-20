<template>
  <div class="cm-field-shell" @focusin="isFocused = true" @focusout="onFocusOut">
    <v-input
      :error-messages="errorList"
      :focused="isFocused"
      :disabled="disabled"
      :hide-details="hideDetails"
      density="compact"
    >
      <template #default="{ id, isDisabled, hasDetails }">
        <v-field
          :id="id.value"
          :label="label"
          :label-id="`${id.value}-label`"
          active
          :dirty="valueFilled"
          :disabled="isDisabled.value"
          :focused="isFocused"
          :details="hasDetails.value"
          :error="hasError"
          class="cm-field"
          @click="emit('focus')"
        >
          <template #default="{ props: fieldProps }">
            <div
              v-bind="fieldProps"
              class="cm-field-input"
              :style="minHeight ? { minHeight } : undefined"
            >
              <slot />
            </div>
          </template>
          <template v-if="$slots['append-inner']" #append-inner>
            <slot name="append-inner" />
          </template>
        </v-field>
      </template>
    </v-input>
  </div>
</template>

<script setup lang="ts" name="code-mirror-field">
/**
 * CodeMirror 编辑器共享的 Vuetify 字段外壳：
 * 描边、浮动标签、聚焦色与错误消息全部由 v-input / v-field 提供，
 * 子组件只负责在默认插槽内放置编辑器宿主。
 *
 * 多行编辑器的标签始终浮在描边缺口，避免与编辑器内容 / 占位符重叠；
 * 聚焦态仅用于描边高亮。
 */
const props = withDefaults(
  defineProps<{
    /** 浮动标签文案。 */
    label?: string;
    /** 当前值，用于判断标签是否浮动与字段是否 dirty。 */
    value: string;
    /** 表单 / 编辑器错误消息，渲染在 Vuetify details 区。 */
    errorMessages?: string | string[];
    /** 禁用编辑（只读）。 */
    disabled?: boolean;
    /** 编辑区最小高度（CSS 长度），不传则由内部内容决定。 */
    minHeight?: string;
    /** 是否隐藏 details 区；默认 auto，仅在出现消息时展示。 */
    hideDetails?: boolean | "auto";
  }>(),
  {
    label: "",
    errorMessages: () => [],
    disabled: false,
    minHeight: "",
    hideDetails: "auto",
  },
);

const emit = defineEmits<{ (e: "focus"): void }>();

const isFocused = ref(false);

/** 错误消息统一为字符串数组。 */
const errorList = computed<string[]>(() => {
  const value = props.errorMessages;
  const list = Array.isArray(value) ? value : value ? [value] : [];
  return list.filter((message): message is string => Boolean(message));
});
const valueFilled = computed(() => props.value.trim() !== "");
const hasError = computed(() => errorList.value.length > 0);

/** 焦点离开整个字段后才取消聚焦态，避免在编辑器内部移动焦点时标签回落。 */
function onFocusOut(event: FocusEvent): void {
  const root = event.currentTarget as HTMLElement | null;
  const next = event.relatedTarget as Node | null;
  if (root && next && root.contains(next)) {
    return;
  }
  isFocused.value = false;
}
</script>

<style scoped>
.cm-field-shell {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.cm-field-shell :deep(.v-input) {
  flex: 1 1 auto;
  min-width: 0;
  min-height: 0;
}

/* stylelint-disable-next-line selector-class-pattern -- Vuetify 内部类名 */
.cm-field-shell :deep(.v-input__control) {
  flex: 1 1 auto;
  min-width: 0;
  min-height: 0;
}

.cm-field-shell :deep(.v-field) {
  flex: 1 1 auto;
  min-width: 0;
  min-height: 0;
}

/* stylelint-disable-next-line selector-class-pattern -- Vuetify 内部类名 */
.cm-field-shell :deep(.v-field__field) {
  flex: 1 1 auto;
  min-width: 0;
  min-height: 0;
}

/* 多行编辑区：纵向排布，不再使用单行输入的垂直居中/flex-wrap 行为 */
.cm-field-shell :deep(.cm-field-input) {
  display: flex;
  flex-flow: column nowrap;
  align-items: stretch;
  flex: 1 1 auto;
  min-width: 0;
  min-height: 0;
}
</style>
