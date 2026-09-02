<template>
  <div v-if="result !== null || loading" class="rego-dry-run-panel">
    <div v-if="loading" class="dry-run-body">
      <v-progress-linear indeterminate color="primary" class="mt-1" />
    </div>
    <div v-else class="dry-run-body">
      <!-- 整体错误 -->
      <v-alert
        v-if="result?.error"
        type="error"
        density="compact"
        variant="tonal"
        :text="result.error"
        class="mb-2"
      />
      <!-- 标签结果：全部规则命中的标签汇总；未命中任何标签时提示 -->
      <div class="result-block">
        <div class="result-label">{{ $t("title.dryRunTags") }}</div>
        <div v-if="tagList.length" class="d-flex flex-wrap">
          <v-chip v-for="tag in tagList" :key="tag" color="primary" size="small" class="me-1 my-1">
            {{ tag }}
          </v-chip>
        </div>
        <v-alert
          v-else
          type="info"
          density="compact"
          variant="tonal"
          :text="$t('summary.noTagMatched')"
        />
      </div>
      <!-- 规则明细：每条规则的状态、耗时与命中的标签 -->
      <div v-if="ruleEntries.length" class="result-block">
        <div class="result-label">{{ $t("title.dryRunRules") }}</div>
        <v-expansion-panels variant="accordion" density="compact">
          <v-expansion-panel v-for="[name, rule] in ruleEntries" :key="name">
            <v-expansion-panel-title>
              <v-icon
                :icon="rule.error ? 'mdi-close-circle-outline' : 'mdi-check-circle-outline'"
                :color="rule.error ? 'error' : 'success'"
                size="small"
                class="me-1"
              />
              <span class="rule-name">{{ name }}</span>
              <v-spacer />
              <span class="rule-duration">
                <v-icon size="x-small" class="me-1">mdi-timer-outline</v-icon>
                {{ localDurationFormat(rule.duration) }}
              </span>
            </v-expansion-panel-title>
            <v-expansion-panel-text>
              <v-alert
                v-if="rule.error"
                type="error"
                density="compact"
                variant="tonal"
                :text="rule.error"
                class="mb-2"
              />
              <div v-if="rule.tags?.length" class="d-flex flex-wrap">
                <v-chip v-for="tag in rule.tags" :key="tag" size="small" class="me-1 my-1">
                  {{ tag }}
                </v-chip>
              </div>
            </v-expansion-panel-text>
          </v-expansion-panel>
        </v-expansion-panels>
      </div>
    </div>
  </div>

  <!-- input 文档查看对话框：与标签规则样例详情一致，按字段表格展示，
       加密/S3 字段提供明文查看 -->
  <model-detail-dialog
    v-model="inputDialog"
    :title="$t('model.tagRuleSample.field.input')"
    :fields="detailFields"
    :plaintext-loading="plaintextLoading"
    @view-plaintext="viewPlaintextAction"
  />
  <plaintext-dialog ref="plaintextDialogRef" />
</template>

<script setup lang="ts" name="rego-dry-run-panel">
import { tagRuleDryRun } from "@/apis/tagEvent";
import {
  TrafficCryptoFields,
  TrafficFieldTypes,
  TrafficS3Fields,
  trafficTypeFromModelKey,
} from "@/apis/traffic";
import { getResponse } from "@/apis/base";
import type { DetailField } from "@/components/@types/detail";
import { useAppStore } from "@/stores/app";
import { BackendDataType } from "@/types";
import { localDurationFormat } from "@/utils/i18n";
import { modelFieldTitle, modelTypeKeyToI18nKey } from "@/utils/modelField";
import type { TagRuleDryRunOut, TagRuleDryRunRuleOut } from "@/pages/[language]/tag-event/@types";

const props = defineProps<{
  model: string;
  rego: string;
  /** input document of the sample selected in the rego editor, kept in sync */
  sampleInput?: Record<string, unknown> | null;
}>();

const appStore = useAppStore();

const inputData = ref<Record<string, unknown>>({});
const loading = ref(false);
const result = ref<TagRuleDryRunOut | null>(null);

const inputDialog = ref(false);
const plaintextDialogRef = useTemplateRef("plaintextDialogRef");
const plaintextLoading = ref("");

// 与 rego 编辑器联动：编辑器内选中的样例 input 同步到此处，作为试运行输入。
watch(
  () => props.sampleInput,
  (val) => {
    inputData.value = val ?? {};
  },
  { immediate: true },
);

const tagList = computed<string[]>(() => result.value?.tags ?? []);
const ruleEntries = computed<[string, TagRuleDryRunRuleOut][]>(() =>
  Object.entries(result.value?.rules ?? {}),
);

// detailFields 试运行 input 字段：遍历 input 文档按模型类型翻译字段名；
// 加密/S3 字段经模型类型键逆映射识别后提供明文查看，其余字段按流量模型类型
// （TrafficFieldTypes）由 AdaptiveCell 以列表页方式渲染
const detailFields = computed<DetailField[]>(() => {
  const modelKey = modelTypeKeyToI18nKey(props.model);
  const trafficType = trafficTypeFromModelKey(props.model);
  const cryptoKeys = new Set(trafficType ? (TrafficCryptoFields[trafficType] ?? []) : []);
  const s3Keys = new Set(trafficType ? (TrafficS3Fields[trafficType] ?? []) : []);
  // 字段渲染类型：按流量类型查 TrafficFieldTypes，未知类型/字段回退 STRING
  const fieldTypes = trafficType ? TrafficFieldTypes[trafficType] : {};
  return Object.entries(inputData.value).map(([key, value]) => {
    const label = modelFieldTitle(modelKey, key);
    const fieldType = fieldTypes[key] ?? BackendDataType.STRING;
    if (cryptoKeys.has(key) && typeof value === "string" && value !== "") {
      return {
        key,
        label,
        display: "",
        kind: "crypto" as const,
        length: 0,
        value,
        type: fieldType,
      };
    }
    if (s3Keys.has(key) && typeof value === "string" && value !== "") {
      return {
        key,
        label,
        display: value,
        kind: "s3" as const,
        length: Number(inputData.value[`${key}Length`] ?? 0),
        value,
        type: fieldType,
      };
    }
    return { key, label, display: "", kind: "plain" as const, length: 0, value, type: fieldType };
  });
});

// viewPlaintextAction 明文查看统一入口：model 参数用流量类型名（与后端权限点
// plaintext:{crypto|s3}:{model}:{field} 对应），loading 反馈给详情行按钮
const viewPlaintextAction = async (field: DetailField): Promise<void> => {
  const trafficType = trafficTypeFromModelKey(props.model);
  if (!trafficType) {
    return;
  }
  plaintextLoading.value = field.key;
  try {
    await plaintextDialogRef.value?.open(field, trafficType, field.label);
  } finally {
    plaintextLoading.value = "";
  }
};

const run = async (): Promise<void> => {
  result.value = null;
  loading.value = true;
  try {
    await getResponse<TagRuleDryRunOut>(
      tagRuleDryRun({
        // 草稿阶段 name 可为空（后端以占位名兜底）
        name: undefined,
        model: props.model,
        rego: props.rego,
        input: inputData.value,
      }),
      async (rsp) => {
        result.value = rsp.data;
      },
      async (rsp) => {
        appStore.notify({
          text: rsp.summary,
          timeout: 3000,
          color: "error",
        });
      },
    );
  } finally {
    loading.value = false;
  }
};

// 供父级调用：运行按钮（与校验按钮同行）与 input 查看按钮（样例选择框 suffix）
// 都位于本组件之外，通过 expose 暴露方法给 dialog 内的按钮绑定。
const openInput = (): void => {
  inputDialog.value = true;
};

defineExpose({ run, loading, openInput });
</script>

<style scoped>
/* 与 rego 编辑器各区块（样例选择/输入字段/编辑器）统一为带边框的分区盒，
   去除原 v-card tonal 的悬浮卡片感，保持对话框内视觉语言一致 */
.rego-dry-run-panel {
  margin-top: 8px;
  border: 1px solid rgb(0 0 0 / 24%);
  border-radius: 4px;
  background-color: rgb(var(--v-theme-surface));
}

.dry-run-body {
  padding: 12px;
}

/* 结果分区：标签汇总 / 规则明细，用标题分隔并明确各元素含义 */
.result-block {
  margin-top: 8px;
}

.result-label {
  margin-bottom: 4px;
  font-size: 12px;
  font-weight: 600;
  color: rgb(var(--v-theme-on-surface-variant));
}

.rule-name {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 13px;
}

.rule-duration {
  display: inline-flex;
  align-items: center;
  font-size: 12px;
  color: rgb(var(--v-theme-on-surface-variant));
}

/* ---- 试运行结果规则展开面板：与分区盒视觉统一 ----
   去除 Vuetify 默认的悬浮阴影、currentColor 深色 hover 遮罩与大号标题，
   改为透明背景 + 淡色 hover（与字段列表行 hover 同款）、紧凑标题行 */
.dry-run-body :deep(.v-expansion-panels) {
  background: transparent;
}

.dry-run-body :deep(.v-expansion-panel) {
  background: transparent;
  border-radius: 4px;
}

/* stylelint-disable-next-line selector-class-pattern -- Vuetify 内部类名 */
.dry-run-body :deep(.v-expansion-panel__shadow) {
  display: none;
}

.dry-run-body :deep(.v-expansion-panel-title) {
  min-height: 36px;
  padding: 6px 12px;
  font-size: 13px;
}

/* stylelint-disable-next-line selector-class-pattern -- Vuetify 内部类名 */
.dry-run-body :deep(.v-expansion-panel-title__overlay) {
  background-color: rgb(var(--v-theme-on-surface), 0.06);
  opacity: 0;
}

/* stylelint-disable-next-line selector-class-pattern -- Vuetify 内部类名 */
.dry-run-body :deep(.v-expansion-panel-title:hover > .v-expansion-panel-title__overlay) {
  opacity: 1;
}

/* stylelint-disable-next-line selector-class-pattern -- Vuetify 内部类名 */
.dry-run-body :deep(.v-expansion-panel-text__wrapper) {
  padding: 4px 12px 12px;
}
</style>
