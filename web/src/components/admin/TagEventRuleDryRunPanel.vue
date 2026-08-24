<template>
  <div v-if="result !== null || loading" class="tag-event-dry-run-panel">
    <div v-if="loading" class="dry-run-body">
      <v-progress-linear indeterminate color="primary" class="mt-1" />
    </div>
    <div v-else-if="result" class="dry-run-body">
      <!-- 规则概要：名称 / 匹配策略 / 检测时间 -->
      <div class="result-block">
        <div class="result-label">{{ $t("title.dryRunSummary") }}</div>
        <div class="summary-grid">
          <div class="summary-item">
            <span class="summary-key">{{ $t("model.tagEventRule.field.name") }}</span>
            <span class="summary-value">{{
              result.ruleName || $t("model.__base.value.blank")
            }}</span>
          </div>
          <div class="summary-item">
            <span class="summary-key">{{ $t("model.tagEventRule.field.matchAll") }}</span>
            <span class="summary-value">
              {{ result.matchAll ? $t("model.__base.value.yes") : $t("model.__base.value.no") }}
            </span>
          </div>
          <div class="summary-item">
            <span class="summary-key">{{ $t("model.tagEventCondition.field.detectedAt") }}</span>
            <span class="summary-value font-mono">
              {{ localeDatetimeFormat(DateTime.fromISO(result.detectedAt)) }}
            </span>
          </div>
        </div>
      </div>
      <!-- 检测事件：命中则逐事件展开，未命中提示到头了 -->
      <div class="result-block">
        <div class="result-label">{{ $t("title.dryRunEvents") }}</div>
        <v-expansion-panels v-if="result.events?.length" variant="accordion" density="compact">
          <v-expansion-panel v-for="ev in result.events" :key="`${ev.ruleID}-${ev.detectedAt}`">
            <v-expansion-panel-title>
              <v-icon icon="mdi-check-circle-outline" color="success" size="small" class="me-1" />
              <span class="event-name">{{ ev.ruleName }}</span>
              <v-spacer />
              <span class="event-time">
                <v-icon size="x-small" class="me-1">mdi-clock-outline</v-icon>
                {{ localeDatetimeFormat(DateTime.fromISO(ev.detectedAt)) }}
              </span>
            </v-expansion-panel-title>
            <v-expansion-panel-text>
              <v-list density="compact" class="bg-transparent">
                <v-list-item v-for="hit in ev.hits" :key="hit.conditionID">
                  <template #prepend>
                    <v-icon size="small" color="primary" class="me-1">mdi-tag-outline</v-icon>
                  </template>
                  <template #title>
                    <span class="hit-name">{{ hit.conditionName }}</span>
                  </template>
                  <template #subtitle>
                    <span class="hit-meta">
                      {{ hit.model }}
                      {{ hit.dimension ? ` / ${hit.dimension}` : "" }}
                      {{ hit.item ? ` / ${hit.item}` : "" }}
                    </span>
                  </template>
                  <template #append>
                    <span class="hit-count">{{ hit.count }}/{{ hit.threshold }}</span>
                    <v-chip size="x-small" variant="tonal" class="ms-1">
                      {{ localDurationFormat(hit.window) }}
                    </v-chip>
                  </template>
                </v-list-item>
              </v-list>
            </v-expansion-panel-text>
          </v-expansion-panel>
        </v-expansion-panels>
        <v-alert
          v-else
          type="info"
          density="compact"
          variant="tonal"
          :text="$t('summary.noEventDetected')"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts" name="tag-event-rule-dry-run-panel">
// TagEventRuleDryRunPanel 标签事件规则试运行结果面板
// 供创建/修改对话框内的「运行」按钮调用：读取表单当前草稿（规则名/全匹配/条件），
// 调用后端 dry-run 接口，按「规则概要 + 检测事件」两分区展示检测结果。
import { DateTime } from "luxon";

import { tagEventRuleDryRun } from "@/apis/traffic";
import { getResponse } from "@/apis/base";
import type { TagEventRuleConditionIn } from "@/apis/@types/admin";
import { useAppStore } from "@/stores/app";
import { localeDatetimeFormat, localDurationFormat } from "@/utils/i18n";
import type { TagEventRuleDryRunOut } from "@/pages/[language]/traffic/@types";

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
/* 与标签规则试运行面板一致：带边框的分区盒，保持对话框内视觉语言统一 */
.tag-event-dry-run-panel {
  margin-top: 8px;
  border: 1px solid rgb(var(--v-theme-on-surface), 0.12);
  border-radius: 4px;
  background-color: rgb(var(--v-theme-surface));
}

.dry-run-header {
  display: flex;
  align-items: center;
  min-height: 32px;
  padding: 0 4px 0 12px;
}

.dry-run-title {
  font-size: 13px;
  font-weight: 600;
  color: rgb(var(--v-theme-on-surface));
}

.dry-run-body {
  padding: 0 12px 12px;
}

/* 结果分区：用标题分隔并明确各元素含义（与标签规则试运行面板一致） */
.result-block {
  margin-top: 8px;
}

.result-label {
  margin-bottom: 4px;
  font-size: 12px;
  font-weight: 600;
  color: rgb(var(--v-theme-on-surface-variant));
}

/* 规则概要：key / value 网格，自动换行布局 */
.summary-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 4px 16px;
}

.summary-item {
  display: flex;
  flex-direction: column;
}

.summary-key {
  font-size: 12px;
  color: rgb(var(--v-theme-on-surface-variant));
}

.summary-value {
  font-size: 13px;
  color: rgb(var(--v-theme-on-surface));
}

/* 事件行：名称 / 检测时间，与标签规则规则明细行同款视觉 */
.event-name {
  font-size: 13px;
  font-weight: 600;
}

.event-time {
  display: inline-flex;
  align-items: center;
  font-size: 12px;
  color: rgb(var(--v-theme-on-surface-variant));
}

/* 命中行：条件名 / 命中分组元信息 / 计数与窗口 */
.hit-name {
  font-size: 13px;
  font-weight: 600;
}

.hit-meta {
  font-size: 12px;
  color: rgb(var(--v-theme-on-surface-variant));
}

.hit-count {
  font-size: 12px;
  font-weight: 600;
  color: rgb(var(--v-theme-primary));
}

/* ---- 检测事件展开面板：与分区盒视觉统一 ----
   去除 Vuetify 默认的悬浮阴影与深色 hover 遮罩，改为透明背景 + 淡色 hover */
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
