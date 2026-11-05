<template>
  <div class="d-flex flex-column ga-2">
    <div class="d-flex align-center ga-2">
      <span class="zk-editor-title">{{ $t("model.agentSkill.field.inputFields") }}</span>
      <v-spacer />
      <v-btn
        variant="tonal"
        size="small"
        prepend-icon="mdi-plus"
        :disabled="disabled"
        :text="$t('page.aiSkill.addInputField')"
        @click="addField"
      />
    </div>
    <span class="zk-hint">{{ $t("page.aiSkill.inputFieldsHint") }}</span>
    <div v-if="!modelValue.length" class="zk-hint">
      {{ $t("model.agentSkill.field.noInputFields") }}
    </div>
    <div
      v-for="(field, index) in modelValue"
      :key="index"
      class="zk-panel--subtle pa-2 d-flex flex-column ga-2"
    >
      <div class="d-flex align-center ga-2 flex-wrap">
        <v-text-field
          :model-value="field.name"
          density="compact"
          hide-details
          :label="$t('page.aiSkill.inputFieldName')"
          :disabled="disabled || isNowTimeField(field)"
          :title="isNowTimeField(field) ? $t('page.aiSkill.nowTimeLocked') : undefined"
          style="min-width: 160px"
          @update:model-value="(value: string) => updateName(index, value)"
        />
        <v-select
          :model-value="field.type"
          :items="inputFieldTypes"
          density="compact"
          hide-details
          :label="$t('page.aiSkill.inputFieldType')"
          :disabled="disabled || isNowTimeField(field)"
          :title="isNowTimeField(field) ? $t('page.aiSkill.nowTimeLocked') : undefined"
          style="min-width: 130px"
          @update:model-value="
            (value: string | null) => updateField(index, { type: value ?? 'string' })
          "
        />
        <v-checkbox
          :model-value="isNowTimeField(field) || field.required"
          hide-details
          density="compact"
          :label="$t('page.aiSkill.inputFieldRequired')"
          :disabled="disabled || isNowTimeField(field)"
          :title="isNowTimeField(field) ? $t('page.aiSkill.nowTimeLocked') : undefined"
          @update:model-value="(value: boolean | null) => updateField(index, { required: !!value })"
        />
        <v-spacer />
        <v-btn
          variant="text"
          size="small"
          icon="mdi-delete-outline"
          :disabled="disabled || isNowTimeField(field)"
          :title="isNowTimeField(field) ? $t('page.aiSkill.nowTimeLocked') : $t('action.delete')"
          @click="removeField(index)"
        />
      </div>
      <v-text-field
        :model-value="field.description"
        density="compact"
        hide-details
        :label="$t('page.aiSkill.inputFieldDescription')"
        :disabled="disabled"
        @update:model-value="(value: string) => updateField(index, { description: value })"
      />
    </div>
    <div v-if="nowTimeError" class="zk-error-text">{{ nowTimeError }}</div>
  </div>
</template>

<script setup lang="ts" name="skill-input-fields-editor">
import type { AgentSkillInputFieldOut } from "@/apis/@types/ai";
import i18n from "@/utils/i18n";

// inputFieldTypes 是技能输入字段的类型取值空间，与后端 SkillInputFieldType 常量一致。
const inputFieldTypes: string[] = ["date", "number", "string", "enum", "boolean"];

// NOW_TIME_NAME nowTime 是周期技能的运行时注入字段：名字与类型固定，由周期开关自动增删。
const NOW_TIME_NAME = "nowTime";

// isNowTimeField 判定运行时注入字段；比较去掉首尾空白，与提交校验口径一致。
const isNowTimeField = (field: AgentSkillInputFieldOut): boolean =>
  field.name.trim() === NOW_TIME_NAME;

const props = withDefaults(
  defineProps<{
    modelValue: AgentSkillInputFieldOut[];
    /** 当前技能是否声明为可周期执行；为 true 时必须包含 nowTime/date 字段 */
    periodic: boolean;
    /** 禁用编辑 */
    disabled?: boolean;
  }>(),
  {
    disabled: false,
  },
);

const emit = defineEmits<{
  (e: "update:modelValue", value: AgentSkillInputFieldOut[]): void;
}>();

// updateField 以不可变方式替换单个字段，保证父组件持有的数组触发更新。
const updateField = (index: number, patch: Partial<AgentSkillInputFieldOut>): void => {
  emit(
    "update:modelValue",
    props.modelValue.map((field, i) => (i === index ? { ...field, ...patch } : field)),
  );
};

// updateName 字段改名为 nowTime 时自动把类型纠正为 date，避免周期技能提交非法组合。
const updateName = (index: number, name: string): void => {
  updateField(index, name.trim() === NOW_TIME_NAME ? { name, type: "date" } : { name });
};

// addField 追加一个空输入字段，类型默认 string。
const addField = (): void => {
  emit("update:modelValue", [
    ...props.modelValue,
    { name: "", type: "string", required: false, description: "" },
  ]);
};

// removeField 删除指定输入字段；nowTime 由系统按周期运行时注入，随周期开关自动增删，禁止手动移除。
const removeField = (index: number): void => {
  const field = props.modelValue[index];
  if (!field || isNowTimeField(field)) {
    return;
  }
  emit(
    "update:modelValue",
    props.modelValue.filter((_, i) => i !== index),
  );
};

// syncNowTimeField 周期开关联动：开启时把 name=nowTime、type=date、required=true 的字段补齐到
// 列表首位（已存在则不重复添加，类型或必填被改动时纠正回来）；关闭时移除该字段。
// disabled（内置只读技能）下不改写外部数据，避免只读态产生待提交的变更。
watch([() => props.periodic, () => props.modelValue], ([periodic, fields]) => {
  if (props.disabled) {
    return;
  }
  if (!periodic) {
    if (fields.some(isNowTimeField)) {
      emit(
        "update:modelValue",
        fields.filter((field) => !isNowTimeField(field)),
      );
    }
    return;
  }
  const nowTime = fields.find(isNowTimeField);
  if (!nowTime) {
    emit("update:modelValue", [
      { name: NOW_TIME_NAME, type: "date", required: true, description: "" },
      ...fields,
    ]);
    return;
  }
  if (nowTime.type !== "date" || !nowTime.required) {
    emit(
      "update:modelValue",
      fields.map((field) =>
        isNowTimeField(field) ? { ...field, type: "date", required: true } : field,
      ),
    );
  }
});

// nowTimeError 周期技能缺少 nowTime/date 字段时在编辑器内即时提示，提交前仍由页面与后端兜底。
const nowTimeError = computed(() =>
  props.periodic &&
  !props.modelValue.some((field) => isNowTimeField(field) && field.type === "date")
    ? i18n.global.t("page.aiSkill.nowTimeRequired")
    : "",
);
</script>
