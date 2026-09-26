<template>
  <div v-if="result !== null || loading" class="tag-event-dry-run-panel zk-editor">
    <div v-if="loading" class="dry-run-body">
      <v-progress-linear indeterminate color="primary" class="mt-1" />
    </div>
    <div v-else-if="result" class="dry-run-body">
      <!-- 规则概要：名称 / 匹配策略 / 检测时间 -->
      <div class="result-block">
        <div class="result-label zk-label-caps">{{ $t("title.dryRunSummary") }}</div>
        <div class="summary-grid zk-panel--subtle">
          <div class="summary-item">
            <span class="zk-hint">{{ $t("model.tagEventRule.field.name") }}</span>
            <span>{{ result.ruleName || $t("model.__base.value.blank") }}</span>
          </div>
          <div class="summary-item">
            <span class="zk-hint">{{ $t("model.tagEventRule.field.matchAll") }}</span>
            <span>
              {{ result.matchAll ? $t("model.__base.value.yes") : $t("model.__base.value.no") }}
            </span>
          </div>
          <div class="summary-item">
            <span class="zk-hint">{{ $t("model.tagEventCondition.field.detectedAt") }}</span>
            <span class="font-mono">
              {{ localeDatetimeFormat(DateTime.fromISO(result.detectedAt)) }}
            </span>
          </div>
        </div>
      </div>
      <!-- 检测事件：命中则逐事件展开，未命中提示到头了 -->
      <div class="result-block">
        <div class="result-label zk-label-caps">{{ $t("title.dryRunEvents") }}</div>
        <v-expansion-panels v-if="result.events?.length">
          <v-expansion-panel v-for="ev in result.events" :key="`${ev.ruleID}-${ev.detectedAt}`">
            <expansion-panel-title
              icon="mdi-check-circle-outline"
              color="success"
              :title="ev.ruleName"
              :subtitle="$t('title.dryRunEventHits', { count: ev.hits.length })"
            >
              <template #actions>
                <span class="event-time zk-hint">
                  <v-icon size="x-small" class="me-1">mdi-clock-outline</v-icon>
                  {{ localeDatetimeFormat(DateTime.fromISO(ev.detectedAt)) }}
                </span>
              </template>
            </expansion-panel-title>
            <v-expansion-panel-text>
              <v-list class="bg-transparent">
                <v-list-item v-for="hit in ev.hits" :key="hit.conditionID">
                  <template #prepend>
                    <v-icon size="small" color="primary" class="me-1">mdi-tag-outline</v-icon>
                  </template>
                  <template #title>
                    <span class="zk-editor-title">{{ hit.conditionName }}</span>
                  </template>
                  <template #subtitle>
                    <span class="zk-hint">
                      {{ hit.model }}
                      {{ hit.dimension ? ` / ${hit.dimension}` : "" }}
                      {{ hit.item ? ` / ${hit.item}` : "" }}
                    </span>
                  </template>
                  <template #append>
                    <span class="zk-metric">{{ hit.count }}/{{ hit.threshold }}</span>
                    <v-chip size="x-small" class="ms-1">
                      {{ localDurationFormat(hit.window) }}
                    </v-chip>
                  </template>
                </v-list-item>
              </v-list>
            </v-expansion-panel-text>
          </v-expansion-panel>
        </v-expansion-panels>
        <v-alert v-else type="info" :text="$t('summary.noEventDetected')" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts" name="tag-event-rule-dry-run-panel">
// TagEventRuleDryRunPanel 标签事件规则试运行结果面板
// 供创建/修改对话框内的「运行」按钮调用：读取表单当前草稿（规则名/全匹配/条件），
// 调用后端 dry-run 接口，按「规则概要 + 检测事件」两分区展示检测结果。
import { DateTime } from "luxon";

import { tagEventRuleDryRun } from "@/apis/tagEvent";
import { getResponse } from "@/apis/base";
import type { TagEventRuleConditionIn } from "@/apis/@types/admin";
import { useAppStore } from "@/stores/app";
import { localeDatetimeFormat, localDurationFormat } from "@/utils/i18n";
import type { TagEventRuleDryRunOut } from "@/pages/[language]/tag-event/@types";

const props = defineProps<{
  /** 规则名（草稿阶段可为空，后端兜底） */
  ruleName: string;
  /** 是否全条件匹配 */
  matchAll: boolean;
  /** 当前表单条件草稿（含各条件的启用状态） */
  conditions: TagEventRuleConditionIn[];
}>();

const appStore = useAppStore();
const loading = ref(false);
const result = ref<TagEventRuleDryRunOut | null>(null);

const run = async (): Promise<void> => {
  if (props.conditions.length === 0) {
    return;
  }
  result.value = null;
  loading.value = true;
  try {
    await getResponse<TagEventRuleDryRunOut>(
      tagEventRuleDryRun({
        name: props.ruleName,
        matchAll: props.matchAll,
        // dry-run 模拟检测与规则启用状态无关，固定按启用执行
        enabled: true,
        conditions: props.conditions,
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

// reset 供对话框打开时清空上一次的运行结果，避免跨规则残留
const reset = (): void => {
  result.value = null;
};

defineExpose({ run, loading, reset });
</script>

<style scoped>
/* 结果面板外壳（描边 / 圆角 / surface 背景）与三个编辑器一致，走全局 .zk-editor；
   展开面板的标题与内容内边距一律走 Vuetify 默认，此处只保留布局定位 */
.tag-event-dry-run-panel {
  margin-top: var(--zk-space-2);
}

.dry-run-body {
  padding: 0 var(--zk-space-3) var(--zk-space-3);
}

/* 结果分区：用标题分隔并明确各元素含义（与标签规则试运行面板一致） */
.result-block {
  margin-top: var(--zk-space-2);
}

.result-label {
  margin-bottom: var(--zk-space-1);
}

/* 规则概要：key / value 网格，自动换行布局（底色走 .zk-panel--subtle） */
.summary-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: var(--zk-space-1) var(--zk-space-4);
  padding: var(--zk-space-2) var(--zk-space-3);
}

.summary-item {
  display: flex;
  flex-direction: column;
}

/* 事件行时间：图标与时间水平对齐 */
.event-time {
  display: inline-flex;
  align-items: center;
}
</style>
