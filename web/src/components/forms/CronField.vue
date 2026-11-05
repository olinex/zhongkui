<template>
  <v-text-field
    :model-value="modelValue"
    :label="label"
    :placeholder="placeholderText"
    :density="density"
    :clearable="clearable"
    :disabled="disabled"
    :readonly="readonly"
    :rules="rules"
    :error-messages="errorMessages"
    min-width="160"
    prepend-inner-icon="mdi-calendar-clock"
    @update:model-value="update"
  >
    <template #append-inner>
      <v-menu
        v-model="panelOpened"
        :close-on-content-click="false"
        :disabled="disabled || readonly"
        location="bottom end"
        origin="top end"
        min-width="0"
      >
        <template #activator="{ props: activatorProps }">
          <v-btn
            v-bind="activatorProps"
            icon="mdi-tune-variant"
            variant="text"
            :title="$t('cron.builder')"
            :aria-label="$t('cron.builder')"
            :disabled="disabled || readonly"
          />
        </template>
        <v-card class="zk-panel cron-panel">
          <div class="cron-panel-body">
            <span class="zk-section-title">{{ $t("cron.title") }}</span>

            <!-- descriptor 模式只接受宏描述符，不提供五段频率引导 -->
            <span v-if="mode === 'descriptor'" class="zk-hint">{{
              $t("cron.descriptorHint")
            }}</span>
            <!-- 引导模式：按频率逐步收起无关字段，只暴露当前频率需要设置的项 -->
            <template v-if="mode !== 'descriptor'">
              <v-select
                :model-value="builder.frequency"
                :items="frequencyItems"
                item-title="title"
                item-value="value"
                :label="$t('cron.frequency')"
                hide-details
                @update:model-value="setFrequency"
              />
              <div v-if="needsHour" class="cron-row">
                <v-select
                  :model-value="builder.hour"
                  :items="hourItems"
                  item-title="title"
                  item-value="value"
                  :label="$t('cron.field.hour')"
                  hide-details
                  @update:model-value="setHour"
                />
                <v-select
                  :model-value="builder.minute"
                  :items="minuteItems"
                  item-title="title"
                  item-value="value"
                  :label="$t('cron.field.minute')"
                  hide-details
                  @update:model-value="setMinute"
                />
              </div>
              <v-select
                v-else-if="builder.frequency === 'hourly'"
                :model-value="builder.minute"
                :items="minuteItems"
                item-title="title"
                item-value="value"
                :label="$t('cron.field.minute')"
                hide-details
                @update:model-value="setMinute"
              />
              <template v-if="builder.frequency === 'weekly'">
                <v-select
                  :model-value="builder.daysOfWeek"
                  :items="weekdayItems"
                  item-title="title"
                  item-value="value"
                  :label="$t('cron.field.dayOfWeek')"
                  multiple
                  chips
                  closable-chips
                  hide-details
                  @update:model-value="setDaysOfWeek"
                />
                <span v-if="builder.daysOfWeek.length === 0" class="zk-hint">
                  {{ $t("cron.weekdayRequired") }}
                </span>
              </template>
              <div v-if="builder.frequency === 'yearly'" class="cron-row">
                <v-select
                  :model-value="builder.month"
                  :items="monthItems"
                  item-title="title"
                  item-value="value"
                  :label="$t('cron.field.month')"
                  hide-details
                  @update:model-value="setMonth"
                />
                <v-select
                  :model-value="builder.dayOfMonth"
                  :items="dayOfMonthItems"
                  item-title="title"
                  item-value="value"
                  :label="$t('cron.field.dayOfMonth')"
                  hide-details
                  @update:model-value="setDayOfMonth"
                />
              </div>
              <v-select
                v-else-if="builder.frequency === 'monthly'"
                :model-value="builder.dayOfMonth"
                :items="dayOfMonthItems"
                item-title="title"
                item-value="value"
                :label="$t('cron.field.dayOfMonth')"
                hide-details
                @update:model-value="setDayOfMonth"
              />
            </template>

            <span v-if="isCustom && mode !== 'descriptor'" class="zk-hint">
              {{ $t("cron.customHint") }}
            </span>

            <v-divider />

            <!-- 当前表达式的可读描述、校验结果与下次执行时间 -->
            <div class="cron-summary">
              <span v-if="errorText" class="zk-error-text">{{ errorText }}</span>
              <span v-else-if="description">{{ description }}</span>
              <span class="zk-hint">{{ $t("cron.nextRuns") }}</span>
              <span v-if="nextRunText">{{ nextRunText }}</span>
              <span v-else class="zk-hint">{{ $t("cron.noNextRuns") }}</span>
            </div>

            <v-divider />

            <!-- 宏描述符快捷项在所有非 cron 模式可用（cron 模式不接受描述符） -->
            <div v-if="mode !== 'cron'" class="cron-section">
              <span class="zk-label-caps">{{ $t("cron.descriptorTitle") }}</span>
              <div class="d-flex flex-wrap ga-1">
                <v-chip
                  v-for="descriptor in DESCRIPTORS"
                  :key="descriptor"
                  @click="update(descriptor)"
                >
                  {{ descriptor }}
                </v-chip>
              </div>
            </div>
            <span v-if="mode !== 'descriptor'" class="zk-hint">{{ $t("cron.guide") }}</span>
          </div>
        </v-card>
      </v-menu>
    </template>
  </v-text-field>
</template>

<script setup lang="ts" name="cron-field">
import {
  DAILY_DESCRIPTOR,
  buildCronSpec,
  createBuilderState,
  describeCronSpec,
  nextCronTriggers,
  parseCronBuilder,
  parseCronSpecForMode,
  type CronBuilderState,
  type CronFrequency,
  type CronMode,
} from "@/utils/cron";
import i18n, { localeDatetimeFormat } from "@/utils/i18n";

// 类型
import type { Density } from "../@types/base";

// Rule 底层字段的校验规则形态：合法时返回 `true`，否则返回需展示的错误信息。
type Rule = (value: unknown) => boolean | string;

// NEXT_RUN_COUNT 面板中预览的下次执行次数。
const NEXT_RUN_COUNT = 3;

// DESCRIPTORS 可一键套用的常用描述符（后端 ParseStandard 支持同一组名称）。
const DESCRIPTORS = ["@hourly", "@daily", "@weekly", "@monthly", "@yearly"];

// FREQUENCIES 引导模式支持的频率，顺序由高频到低频。
const FREQUENCIES: CronFrequency[] = ["minute", "hourly", "daily", "weekly", "monthly", "yearly"];

// WEEKDAY_KEYS 星期取值（cron 以周日为 0）到名称键的映射。
const WEEKDAY_KEYS = ["sun", "mon", "tue", "wed", "thu", "fri", "sat"] as const;

// pad2 补零到两位，用于时刻与日期下拉的展示。
const pad2 = (value: number): string => String(value).padStart(2, "0");

// numberItems 生成「数值 + 展示标题」的下拉项。
const numberItems = (
  min: number,
  max: number,
  padded = false,
): Array<{ title: string; value: number }> =>
  Array.from({ length: max - min + 1 }, (_, index) => {
    const value = min + index;
    return { title: padded ? pad2(value) : String(value), value };
  });

const minuteItems = numberItems(0, 59, true);
const hourItems = numberItems(0, 23, true);
const dayOfMonthItems = numberItems(1, 31);
const monthItems = numberItems(1, 12);
const weekdayItems = computed(() =>
  WEEKDAY_KEYS.map((key, value) => ({ title: i18n.global.t(`cron.weekday.${key}`), value })),
);
const frequencyItems = computed(() =>
  FREQUENCIES.map((value) => ({ title: i18n.global.t(`cron.frequencyOption.${value}`), value })),
);

// props 组件入参：绑定值、展示配置与底层字段校验规则
const props = defineProps({
  modelValue: {
    type: String,
    required: true,
  },
  // 输入框标签。
  label: {
    type: String,
    default: "",
  },
  // 输入框占位提示。
  placeholder: {
    type: String,
    default: "",
  },
  // 输入框密度。
  density: {
    type: String as PropType<Density>,
    default: "compact",
  },
  // 取值模式：descriptor 仅宏描述符、cron 仅 5 段表达式、both 两者兼容。
  mode: {
    type: String as PropType<CronMode>,
    default: "both",
  },
  // 是否提供清空按钮。
  clearable: {
    type: Boolean,
    default: true,
  },
  // 是否禁用输入。
  disabled: {
    type: Boolean,
    default: false,
  },
  // 是否只读。
  readonly: {
    type: Boolean,
    default: false,
  },
  // 错误消息文本。
  errorMessages: {
    type: String,
    default: "",
  },
  // 校验规则列表。
  rules: {
    type: Array as PropType<Rule[]>,
    default: () => [],
  },
});

// emits 值变更事件，供父组件以 v-model 绑定。
const emits = defineEmits(["update:modelValue"]);

const panelOpened = ref(false);

// 引导模式状态：表达式可由引导模式表达时随输入同步回填，用户改动任一选项即写回表达式。
const builder = reactive<CronBuilderState>(createBuilderState());

const syncBuilder = (value: unknown): void => {
  const text = String(value ?? "").trim();
  if (text === "") {
    // 空值（含对话框复位的重置）回到默认计划，避免沿用上一次编辑留下的选项。
    Object.assign(builder, createBuilderState());
    return;
  }
  const state = parseCronBuilder(text);
  if (state === null) {
    return;
  }
  Object.assign(builder, state);
};

watch(() => props.modelValue, syncBuilder, { immediate: true });

// update 将字段值透传给父组件，空值统一归一为空串（清空按钮会发出 null）。
const update = (value: unknown): void => {
  emits("update:modelValue", value === null || value === undefined ? "" : String(value));
};

const applyBuilder = (): void => {
  const spec = props.mode === "descriptor" ? DAILY_DESCRIPTOR : buildCronSpec(builder);
  if (spec !== "") {
    update(spec);
  }
};

const setFrequency = (value: unknown): void => {
  if (typeof value === "string") {
    builder.frequency = value as CronFrequency;
    applyBuilder();
  }
};

const setMinute = (value: unknown): void => {
  if (typeof value === "number") {
    builder.minute = value;
    applyBuilder();
  }
};

const setHour = (value: unknown): void => {
  if (typeof value === "number") {
    builder.hour = value;
    applyBuilder();
  }
};

const setMonth = (value: unknown): void => {
  if (typeof value === "number") {
    builder.month = value;
    applyBuilder();
  }
};

const setDayOfMonth = (value: unknown): void => {
  if (typeof value === "number") {
    builder.dayOfMonth = value;
    applyBuilder();
  }
};

const setDaysOfWeek = (value: unknown): void => {
  if (!Array.isArray(value)) {
    return;
  }
  builder.daysOfWeek = value.map((item) => Number(item)).sort((left, right) => left - right);
  applyBuilder();
};

// placeholderText descriptor 模式固定展示默认宏描述符，避免沿用调用方传入的 cron 示例。
const placeholderText = computed(() =>
  props.mode === "descriptor" ? DAILY_DESCRIPTOR : props.placeholder,
);

// needsHour 需要设置「时」的频率（每分钟与每小时只涉及分钟）。
const needsHour = computed(() => builder.frequency !== "minute" && builder.frequency !== "hourly");

// 当前输入值与引导模式值分开计算：输入为空时预览引导模式将要写回的表达式。
const currentValue = computed(() => String(props.modelValue ?? "").trim());
const builderValue = computed(() =>
  props.mode === "descriptor" ? DAILY_DESCRIPTOR : buildCronSpec(builder),
);
const previewValue = computed(() => currentValue.value || builderValue.value);
const description = computed(() => describeCronSpec(previewValue.value));
const nextRunText = computed(() =>
  nextCronTriggers(previewValue.value, NEXT_RUN_COUNT)
    .map((trigger) => localeDatetimeFormat(trigger))
    .join(" · "),
);

// errorText 仅针对已填写的内容给出，避免空值时把「必填」重复展示在面板里。
const errorText = computed(() => {
  if (currentValue.value === "") {
    return "";
  }
  return parseCronSpecForMode(currentValue.value, props.mode).error ?? "";
});

// isCustom 当前表达式不属于引导模式可表达的形状时，提示改动选项会覆盖它。
const isCustom = computed(
  () => currentValue.value !== "" && parseCronBuilder(currentValue.value) === null,
);

// 空表达式打开引导面板时写入默认计划（每天 09:00），使面板预览与字段取值一致。
watch(panelOpened, (opened) => {
  if (opened && currentValue.value === "") {
    applyBuilder();
  }
});
</script>

<style scoped>
/* 仅布局：宽度与内部分组，表面、描边与配色来自 .zk-panel 与主题令牌。
   面板宽度沿用下拉候选族的宽度令牌，使引导面板与同类浮层保持同一尺度。 */
.cron-panel {
  inline-size: var(--zk-select-max-width);
}

.cron-panel-body {
  display: flex;
  flex-direction: column;
  gap: var(--zk-space-2);
  padding: var(--zk-space-4);
}

.cron-row {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--zk-space-2);
}

.cron-summary {
  display: flex;
  flex-direction: column;
  gap: var(--zk-space-1);
}

.cron-section {
  display: flex;
  flex-direction: column;
  gap: var(--zk-space-2);
}
</style>
