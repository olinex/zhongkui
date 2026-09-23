<template>
  <div v-if="result !== null || loading" class="rego-dry-run-panel zk-editor zk-flat-panels">
    <div v-if="loading" class="dry-run-body">
      <v-progress-linear indeterminate color="primary" class="mt-1" />
    </div>
    <div v-else class="dry-run-body">
      <!-- 整体错误 -->
      <v-alert
        v-if="result?.error"
        type="error"
        :text="result.error"
        class="mb-2"
      />
      <!-- 标签结果：全部规则命中的标签汇总；未命中任何标签时提示 -->
      <div class="result-block">
        <div class="result-label zk-label-caps">{{ $t("title.dryRunTags") }}</div>
        <div v-if="tagList.length" class="d-flex flex-wrap">
          <v-chip v-for="tag in tagList" :key="tag" color="primary" class="me-1 my-1">
            {{ tag }}
          </v-chip>
        </div>
        <v-alert
          v-else
          type="info"
          :text="$t('summary.noTagMatched')"
        />
      </div>
      <!-- 规则明细：每条规则的状态、耗时与命中的标签 -->
      <div v-if="ruleEntries.length" class="result-block">
        <div class="result-label zk-label-caps">{{ $t("title.dryRunRules") }}</div>
        <v-expansion-panels>
          <v-expansion-panel v-for="[name, rule] in ruleEntries" :key="name">
            <v-expansion-panel-title>
              <v-icon
                :icon="rule.error ? 'mdi-close-circle-outline' : 'mdi-check-circle-outline'"
                :color="rule.error ? 'error' : 'success'"
                size="small"
                class="me-1"
              />
              <span class="font-mono">{{ name }}</span>
              <v-spacer />
              <span class="rule-duration zk-hint">
                <v-icon size="x-small" class="me-1">mdi-timer-outline</v-icon>
                {{ localDurationFormat(rule.duration) }}
              </span>
            </v-expansion-panel-title>
            <v-expansion-panel-text>
              <v-alert
                v-if="rule.error"
                type="error"
                :text="rule.error"
                class="mb-2"
              />
              <div v-if="rule.tags?.length" class="d-flex flex-wrap">
                <v-chip v-for="tag in rule.tags" :key="tag" class="me-1 my-1">
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
    class="rego-sample-input-viewer"
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
/* 结果面板外壳（描边 / 圆角 / surface 背景）与三个编辑器一致，走全局 .zk-editor，
   展开面板的扁平外观走全局 .zk-flat-panels，此处只保留布局定位 */
.rego-dry-run-panel {
  margin-top: var(--zk-space-2);
}

.dry-run-body {
  padding: var(--zk-space-3);
}

/* 结果分区：标签汇总 / 规则明细，用标题分隔并明确各元素含义 */
.result-block {
  margin-top: var(--zk-space-2);
}

.result-label {
  margin-bottom: var(--zk-space-1);
}

.rule-duration {
  display: inline-flex;
  align-items: center;
}

/* 规则明细展开面板：标题行与内容区的布局（外观走 .zk-flat-panels） */
.dry-run-body :deep(.v-expansion-panel-title) {
  min-height: var(--zk-control-height-compact);
  padding: var(--zk-space-1) var(--zk-space-3);
}

/* stylelint-disable-next-line selector-class-pattern -- Vuetify 内部类名 */
.dry-run-body :deep(.v-expansion-panel-text__wrapper) {
  padding: var(--zk-space-1) var(--zk-space-3) var(--zk-space-3);
}

/* input 查看对话框：全屏 rego 编辑器的自定义 z-index 会盖住 Vuetify 默认 overlay 层级，
   该对话框（teleport 到 body）需单独拉高到全屏编辑器之上才能正常展示 */
:global(.rego-sample-input-viewer) {
  z-index: 2800 !important;
}
</style>
