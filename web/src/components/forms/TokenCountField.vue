<template>
  <v-number-input
    :model-value="numberValue"
    :label="label"
    :error-messages="errorMessages"
    :min="min"
    :max="max"
    :disabled="disabled"
    :readonly="readonly"
    :precision="0"
    @update:model-value="onValueChange"
  >
    <!-- 单位选择器内嵌于输入框内：与数值同处一个边框/标签/错误信息，呈现「128 K」的一体感 -->
    <template #append-inner>
      <v-menu location="bottom end" :disabled="disabled || readonly">
        <template #activator="{ props: activatorProps }">
          <v-btn
            v-bind="activatorProps"
            class="token-unit-trigger"
            variant="text"
            size="small"
            :disabled="disabled || readonly"
            :ripple="false"
          >
            <span>{{ unitSuffix }}</span>
            <v-icon icon="mdi-menu-down" size="small" />
          </v-btn>
        </template>
        <v-list density="compact">
          <v-list-item
            v-for="opt in unitOptions"
            :key="opt.factor"
            :title="opt.title"
            :active="opt.factor === unitFactor"
            @click="selectUnit(opt.factor)"
          />
        </v-list>
      </v-menu>
    </template>
  </v-number-input>
</template>

<script setup lang="ts" name="token-count-field">
import i18n from "@/utils/i18n";
import { joinTokenCount, labelOfFactor, splitTokenCount } from "@/utils/token";

// modelValue 为 token 总量（整数）；组件内部拆成「数值 + 单位」展示。
// label / errorMessages 承接 vee-validate 经 v-bind 透传的字段配置；onBlur/onChange/onInput
// 作为 fallthrough 落到唯一的根节点 v-number-input，从而触发校验。
const props = defineProps({
  modelValue: {
    type: Number,
    required: true,
  },
  label: {
    type: String,
    default: "",
  },
  errorMessages: {
    type: String,
    default: "",
  },
  min: {
    type: Number,
    default: 0,
  },
  max: {
    type: Number,
    default: undefined,
  },
  disabled: {
    type: Boolean,
    default: false,
  },
  readonly: {
    type: Boolean,
    default: false,
  },
});

// emits 值变更事件，供父组件以 v-model 绑定（发出的是「数值 × 单位因子」的 token 总量）。
const emits = defineEmits(["update:modelValue"]);

// 初始按「能整除的最大单位」拆分，保证回显无损。
const seed = splitTokenCount(props.modelValue);
const unitFactor = ref(seed.factor);
const displayValue = ref(seed.value);
// numberValue 传给 v-number-input 的数值（上层总量为 0 即「未填写」，此处呈现为空）：
// v-number-input 会自行按 min 判定越界并置为 error 态（不产生错误文字），未填写时显示红框
// 属于误报；故 0 转为空值，取值范围仍由 schema 校验负责。
const numberValue = computed(() => displayValue.value || null);
// lastEmitted 记录本组件发出的最近一次总量，用于识别「自己回写」并跳过，避免输入过程中被覆盖。
const lastEmitted = ref(props.modelValue);

watch(
  () => props.modelValue,
  (next) => {
    if (next === lastEmitted.value) {
      return;
    }
    const { value, factor } = splitTokenCount(next);
    unitFactor.value = factor;
    displayValue.value = value;
    lastEmitted.value = next;
  },
);

// emitTotal 记录并发出 token 总量。
const emitTotal = (total: number) => {
  lastEmitted.value = total;
  emits("update:modelValue", total);
};

// onValueChange 数值部分变更（清空时 v-number-input 发出 null，归零交由上层校验拦截）。
const onValueChange = (value: number | null) => {
  const num = value ?? 0;
  displayValue.value = num;
  emitTotal(joinTokenCount(num, unitFactor.value));
};

// selectUnit 切换单位：保留当前数值，按新单位换算总量。
const selectUnit = (factor: number) => {
  unitFactor.value = factor;
  emitTotal(joinTokenCount(displayValue.value, factor));
};

// unitSuffix 触发区展示的单位后缀（无单位档为空串，仅显菜单箭头）。
const unitSuffix = computed(() => labelOfFactor(unitFactor.value));

// unitOptions 单位下拉项（由大到小），标题经 i18n 翻译。
const unitOptions = computed(() => [
  { factor: 1_000_000, title: i18n.global.t("unit.tokenM") },
  { factor: 1_000, title: i18n.global.t("unit.tokenK") },
  { factor: 1, title: i18n.global.t("unit.tokenNone") },
]);
</script>

<style scoped>
/* 仅布局定位：固定触发区最小宽度，使「无单位 / K / M」切换时宽度不抖动 */
.token-unit-trigger {
  min-width: var(--zk-token-unit-trigger-min-width);
}
</style>
