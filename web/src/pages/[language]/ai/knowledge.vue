<template>
  <main-layout current-nav="ai">
    <template #body>
      <search-table
        ref="searchTableRef"
        :list-request="knowledgeDocumentListGet"
        :options-request="knowledgeDocumentListOptions"
        :actions="getActions"
        :batch-actions="getBatchActions"
        :create="openCreateDialogAction"
        :toolbar-actions="toolbarActions"
        :project-fields="projectFields"
        model-name="aiDocument"
      />

      <!-- 删除确认对话框：单条删除与批量删除共用 -->
      <confirm-dialog
        ref="deleteConfirmRef"
        :title="$t('model.aiDocument.name')"
        prepend-icon="mdi-book-open-variant"
        @confirm="itemDeleteAction"
      >
        <template #body="{ proxy }">
          <confirm-alert
            v-if="batchState"
            :action="$t('action.delete')"
            :text="$t('notice.batchConfirm', { count: batchState?.items.length ?? 0 })"
          />
          <confirm-alert
            v-else
            :action="$t('action.delete')"
            :text="
              $t('notice.deleteConfirm', {
                category: $t('model.aiDocument.name'),
                key: proxy.title,
              })
            "
          />
        </template>
      </confirm-dialog>

      <!-- 重新索引确认对话框 -->
      <confirm-dialog
        ref="reindexConfirmRef"
        :title="$t('action.reindex')"
        prepend-icon="mdi-database-sync-outline"
        @confirm="itemReindexAction"
      >
        <template #body="{ proxy }">
          <confirm-alert
            :action="$t('action.reindex')"
            :text="
              $t('notice.reindexConfirm', {
                category: $t('model.aiDocument.name'),
                key: proxy.title,
              })
            "
          />
        </template>
      </confirm-dialog>

      <!-- 创建知识文档对话框 -->
      <temp-editable-dialog
        ref="createDialogRef"
        :title="$t('model.aiDocument.name')"
        :confirm-disabled="!createForm.meta.value.valid"
        force-editable
        prepend-icon="mdi-book-plus"
        @confirm="createAction"
      >
        <template #body>
          <v-form>
            <v-row dense>
              <v-col cols="12">
                <v-text-field
                  v-model="createTitle"
                  v-bind="createTitleProps"
                  :label="$t('model.aiDocument.field.title')"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-textarea
                  v-model="createContent"
                  v-bind="createContentProps"
                  :label="$t('model.aiDocument.field.content')"
                  rows="10"
                />
              </v-col>
            </v-row>
          </v-form>
        </template>
      </temp-editable-dialog>

      <!-- 编辑知识文档对话框：标题与正文均可修改（更新接口触发重新索引） -->
      <temp-editable-dialog
        ref="editDialogRef"
        :title="$t('model.aiDocument.name')"
        :confirm-disabled="!updateForm.meta.value.valid"
        prepend-icon="mdi-book-edit"
        @confirm="itemUpdateAction"
      >
        <template #body="{ editable }">
          <v-form>
            <v-row dense>
              <v-col cols="12">
                <v-text-field
                  v-model="editTitle"
                  v-bind="editTitleProps"
                  :label="$t('model.aiDocument.field.title')"
                  :disabled="!editable"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-textarea
                  v-model="editContent"
                  v-bind="editContentProps"
                  :label="$t('model.aiDocument.field.content')"
                  :disabled="!editable"
                  rows="10"
                />
              </v-col>
            </v-row>
          </v-form>
        </template>
      </temp-editable-dialog>

      <!-- 分块明细对话框：展示文档级索引摘要与逐块构建明细（读时派生，不改表） -->
      <knowledge-chunks-dialog
        v-model="chunksDialog"
        :document-id="chunksDocumentId"
        :document-title="chunksDocumentTitle"
      />

      <!-- 检索试运行对话框：输入查询与可选参数，运行完整检索链路（混合检索 → 生成回答 → 四项量化指标）并展示结果 -->
      <!-- AppDialog 未定义 after-leave 事件，此监听经根节点属性透传落到内部 VDialog，
           保留"关闭动画结束后再重置检索试运行状态"的原有行为 -->
      <app-dialog
        v-model="dryRunDialog"
        :title="$t('page.aiKnowledge.dryRun')"
        icon="mdi-flask-outline"
        size="md"
        @after-leave="resetDryRunDialog"
      >
        <v-form @submit.prevent="runDryRunAction">
          <v-row dense>
            <v-col cols="12">
              <v-text-field
                v-model="dryRunQuery"
                :label="$t('page.aiKnowledge.query')"
                hide-details="auto"
              />
            </v-col>
          </v-row>
          <!-- 高级参数：默认折叠隐藏；未展开时展示后端默认值提示，展开后表单已预填默认值 -->
          <v-row dense align="center">
            <v-col cols="auto">
              <v-btn
                variant="text"
                size="small"
                class="ms-n2"
                @click="showAdvancedParams = !showAdvancedParams"
              >
                <v-icon
                  size="small"
                  :icon="showAdvancedParams ? 'mdi-chevron-up' : 'mdi-chevron-down'"
                  class="me-1"
                />
                {{ $t("page.aiKnowledge.advancedParams") }}
              </v-btn>
            </v-col>
            <v-col v-if="!showAdvancedParams && dryRunDefaults" cols="auto">
              <span class="zk-hint">
                {{
                  $t("page.aiKnowledge.advancedParamsHint", {
                    topKLabel: $t("page.aiKnowledge.topK"),
                    candidatesLabel: $t("page.aiKnowledge.candidates"),
                    fusionLabel: $t("page.aiKnowledge.fusion"),
                    topK: dryRunDefaults.topK,
                    candidates: dryRunDefaults.candidates,
                    fusion: fusionTitleOf(dryRunDefaults.fusion),
                  })
                }}
              </span>
            </v-col>
          </v-row>
          <template v-if="showAdvancedParams">
            <div class="advanced-params zk-hairline-top">
              <v-row dense>
                <v-col cols="12" sm="4">
                  <v-text-field
                    v-model.number="dryRunTopK"
                    :label="$t('page.aiKnowledge.topK')"
                    type="number"
                    min="1"
                    hide-details="auto"
                  />
                </v-col>
                <v-col cols="12" sm="4">
                  <v-text-field
                    v-model.number="dryRunCandidates"
                    :label="$t('page.aiKnowledge.candidates')"
                    type="number"
                    min="1"
                    hide-details="auto"
                  />
                </v-col>
                <v-col cols="12" sm="4">
                  <v-select
                    v-model="dryRunFusion"
                    :label="$t('page.aiKnowledge.fusion')"
                    :items="fusionOptions"
                    item-title="title"
                    item-value="value"
                    hide-details="auto"
                  />
                </v-col>
              </v-row>
              <v-row dense>
                <v-col cols="12" sm="6">
                  <v-text-field
                    v-model.number="dryRunWeightVector"
                    :label="$t('page.aiKnowledge.weightVector')"
                    type="number"
                    step="0.1"
                    min="0"
                    max="1"
                    hide-details="auto"
                  />
                </v-col>
                <v-col cols="12" sm="6">
                  <v-text-field
                    v-model.number="dryRunWeightKeyword"
                    :label="$t('page.aiKnowledge.weightKeyword')"
                    type="number"
                    step="0.1"
                    min="0"
                    max="1"
                    hide-details="auto"
                  />
                </v-col>
              </v-row>
            </div>
          </template>
          <v-btn color="primary" :loading="dryRunRunning" type="submit" class="mt-2">
            {{ dryRunRunning ? $t("page.aiKnowledge.running") : $t("page.aiKnowledge.run") }}
          </v-btn>
          <v-btn
            v-if="dryRunRunning"
            variant="text"
            color="error"
            class="mt-2 ms-2"
            @click="stopDryRunAction"
          >
            {{ $t("page.aiKnowledge.stop") }}
          </v-btn>
        </v-form>

        <!-- 运行过程：阶段进度面板组（手风琴单开，同一时刻只展开当前查看的阶段并实时滚动其输出）；
             描边盒由外层 .zk-panel 提供，面板组自身裸放（见 AGENTS.md「展开面板族」） -->
        <div v-if="dryRunStages.length > 0" class="dry-run-process zk-panel">
          <div class="result-label zk-label-caps">{{ $t("page.aiKnowledge.process") }}</div>
          <v-expansion-panels v-model="expandedStage">
            <v-expansion-panel
              v-for="stage in dryRunStages"
              :key="stage.stage"
              :value="stage.stage"
            >
              <expansion-panel-title
                :icon="stageIconOf(stage)"
                :color="stageColorOf(stage)"
                :title="stageLabelOf(stage.stage)"
                :spin="dryRunRunning && stage.status === 'start'"
              >
                <template #actions>
                  <span
                    class="stage-meta text-caption text-on-surface-variant"
                    :title="stageMetaTextOf(stage)"
                  >
                    {{ stageMetaTextOf(stage) }}
                  </span>
                </template>
              </expansion-panel-title>
              <v-expansion-panel-text>
                <pre
                  v-if="stage.output"
                  :ref="(el) => bindStageOutputEl(stage.stage, el)"
                  class="zk-code-preview stage-output font-mono text-caption text-on-surface"
                  >{{ stage.output }}</pre>
                <p v-else class="zk-hint">{{ $t("page.aiKnowledge.stageNoOutput") }}</p>
              </v-expansion-panel-text>
            </v-expansion-panel>
          </v-expansion-panels>
          <!-- 汇总脚注：检索与总计耗时、全部调用合计 token（运行结束后由汇总结果回填） -->
          <div v-if="dryRunResult" class="metric-footnote zk-hint">
            {{ $t("page.aiKnowledge.metaRetrieveElapsed") }}
            {{ dryRunResult.meta.retrieveElapsedMs }} ms ·
            {{ $t("page.aiKnowledge.metaTotal") }}
            {{ dryRunResult.meta.totalElapsedMs }} ms ·
            {{ $t("page.aiKnowledge.metaTotalTokens") }}
            {{ dryRunResult.meta.usage.totalTokens }}
          </div>
        </div>

        <template v-if="dryRunResult">
          <!-- 评估警告与评估不可用提示：置顶展示，先于回答与指标 -->
          <v-alert v-if="dryRunWarnings.length > 0" type="warning" class="mt-4">
            <div class="text-caption text-on-surface-variant">
              <div>{{ $t("page.aiKnowledge.warnings") }}:</div>
              <div v-for="(warning, idx) in dryRunWarnings" :key="idx">
                {{ warning }}
              </div>
            </div>
          </v-alert>
          <p v-if="dryRunResult.evalError" class="text-caption text-on-surface-variant mt-3">
            {{ $t("page.aiKnowledge.evalError") }}: {{ dryRunResult.evalError }}
          </p>

          <!-- 评估成功：回答引用块 + 量化指标卡片网格 + 明细折叠面板 -->
          <div v-if="dryRunResult.evaluation" class="dry-run-result zk-panel">
            <div class="result-block">
              <div class="zk-code-preview answer-block">
                <div class="result-label zk-label-caps">{{ $t("page.aiKnowledge.answer") }}</div>
                <p class="answer-body text-body-2 text-on-surface">
                  {{ dryRunResult.evaluation.answer || $t("model.__base.value.blank") }}
                </p>
              </div>
            </div>

            <div class="result-block">
              <div class="result-label zk-label-caps">{{ $t("page.aiKnowledge.metrics") }}</div>
              <div class="metric-grid">
                <div v-for="metric in dryRunMetrics" :key="metric.key" class="metric-card zk-panel">
                  <div class="zk-hint">{{ metric.label }}</div>
                  <div class="metric-value zk-section-title">{{ percentOf(metric.value) }}</div>
                  <v-progress-linear
                    :model-value="metric.value * 100"
                    height="6"
                    rounded
                    color="primary"
                  />
                </div>
              </div>
              <div class="metric-footnote zk-hint">
                {{
                  $t("page.aiKnowledge.relevantOfTotal", {
                    relevant: dryRunResult.evaluation.relevantCount,
                    total: dryRunResult.evaluation.totalChunks,
                  })
                }}
              </div>
            </div>

            <!-- 改进建议：按封闭归类给出可执行动作（知识覆盖 / 排序调参 / 生成质量 / 上下文利用） -->
            <div class="result-block">
              <div class="result-label zk-label-caps">{{ $t("page.aiKnowledge.suggestions") }}</div>
              <div class="metric-footnote zk-hint">
                {{ $t("page.aiKnowledge.suggestionsSubtitle") }}
              </div>
              <p v-if="dryRunSuggestions.length === 0" class="zk-hint">
                {{ $t("page.aiKnowledge.suggestionsEmpty") }}
              </p>
              <v-list v-else class="bg-transparent">
                <v-list-item v-for="(suggestion, idx) in dryRunSuggestions" :key="idx">
                  <template #prepend>
                    <v-icon :icon="suggestionIconOf(suggestion.category)" size="small" />
                  </template>
                  <v-list-item-title class="text-body-2 text-on-surface">
                    {{ suggestionCategoryLabelOf(suggestion.category) }}
                  </v-list-item-title>
                  <p class="text-body-2 text-on-surface mt-1">{{ suggestion.detail }}</p>
                </v-list-item>
              </v-list>
            </div>

            <div class="result-block">
              <v-expansion-panels v-model="resultPanel">
                <v-expansion-panel value="chunks">
                  <expansion-panel-title
                    icon="mdi-file-document-multiple-outline"
                    :title="$t('page.aiKnowledge.retrievedChunks')"
                    :subtitle="$t('page.aiKnowledge.retrievedChunksSubtitle')"
                  >
                    <v-chip size="x-small">{{ dryRunResult.results.length }}</v-chip>
                  </expansion-panel-title>
                  <v-expansion-panel-text>
                    <v-list class="bg-transparent">
                      <v-list-item v-for="(result, idx) in dryRunResult.results" :key="idx">
                        <v-list-item-title class="text-body-2 text-on-surface">
                          {{ result.title }}
                        </v-list-item-title>
                        <v-list-item-subtitle class="text-caption text-on-surface-variant">
                          #{{ result.chunkIndex }} · {{ result.documentID }} ·
                          {{ result.score.toFixed(2) }}
                        </v-list-item-subtitle>
                        <p class="text-body-2 text-on-surface mt-1">{{ result.content }}</p>
                      </v-list-item>
                    </v-list>
                  </v-expansion-panel-text>
                </v-expansion-panel>

                <v-expansion-panel value="judgments">
                  <expansion-panel-title
                    icon="mdi-check-decagram-outline"
                    :title="$t('page.aiKnowledge.judgments')"
                    :subtitle="$t('page.aiKnowledge.judgmentsSubtitle')"
                  >
                    <v-chip size="x-small">
                      {{ dryRunResult.evaluation.chunkJudgments.length }}
                    </v-chip>
                  </expansion-panel-title>
                  <v-expansion-panel-text>
                    <v-list class="bg-transparent">
                      <v-list-item
                        v-for="(judgment, idx) in dryRunResult.evaluation.chunkJudgments"
                        :key="idx"
                      >
                        <template #prepend>
                          <v-icon
                            :icon="judgment.relevant ? 'mdi-check' : 'mdi-close'"
                            :color="judgment.relevant ? 'success' : 'error'"
                            size="small"
                          />
                        </template>
                        <v-list-item-title class="text-body-2 text-on-surface">
                          #{{ judgment.index }} · {{ judgment.reason }}
                        </v-list-item-title>
                        <template #append>
                          <span class="text-caption text-on-surface-variant">
                            {{ judgment.score.toFixed(2) }}
                          </span>
                        </template>
                      </v-list-item>
                    </v-list>
                  </v-expansion-panel-text>
                </v-expansion-panel>

                <v-expansion-panel value="claims">
                  <expansion-panel-title
                    icon="mdi-format-quote-open"
                    :title="$t('page.aiKnowledge.claims')"
                    :subtitle="$t('page.aiKnowledge.claimsSubtitle')"
                  >
                    <v-chip size="x-small">
                      {{ dryRunResult.evaluation.claims.length }}
                    </v-chip>
                  </expansion-panel-title>
                  <v-expansion-panel-text>
                    <v-list class="bg-transparent">
                      <v-list-item
                        v-for="(claim, idx) in dryRunResult.evaluation.claims"
                        :key="idx"
                      >
                        <template #prepend>
                          <v-icon
                            :icon="claim.supported ? 'mdi-check' : 'mdi-close'"
                            :color="claim.supported ? 'success' : 'error'"
                            size="small"
                          />
                        </template>
                        <v-list-item-title class="text-body-2 text-on-surface">
                          {{ claim.statement }}
                        </v-list-item-title>
                        <template #append>
                          <span class="text-caption text-on-surface-variant">
                            #{{ claim.chunkIndexes.join(", #") }}
                          </span>
                        </template>
                      </v-list-item>
                    </v-list>
                  </v-expansion-panel-text>
                </v-expansion-panel>

                <v-expansion-panel value="reverseQuestions">
                  <expansion-panel-title
                    icon="mdi-help-circle-outline"
                    :title="$t('page.aiKnowledge.reverseQuestions')"
                    :subtitle="$t('page.aiKnowledge.reverseQuestionsSubtitle')"
                  >
                    <v-chip size="x-small">
                      {{ dryRunResult.evaluation.reverseQuestions.length }}
                    </v-chip>
                  </expansion-panel-title>
                  <v-expansion-panel-text>
                    <v-list class="bg-transparent">
                      <v-list-item
                        v-for="(question, idx) in dryRunResult.evaluation.reverseQuestions"
                        :key="idx"
                      >
                        <v-list-item-title class="text-body-2 text-on-surface">
                          {{ question }}
                        </v-list-item-title>
                      </v-list-item>
                    </v-list>
                  </v-expansion-panel-text>
                </v-expansion-panel>
              </v-expansion-panels>
            </div>
          </div>

          <!-- 评估被跳过（如 chat 未启用）：仍展示检索分块 -->
          <div v-if="!dryRunResult.evaluation" class="mt-4">
            <div class="text-h6 text-on-surface font-weight-600">
              {{ $t("page.aiKnowledge.retrievedChunks") }}
            </div>
            <v-list class="bg-transparent">
              <v-list-item v-for="(result, idx) in dryRunResult.results" :key="idx">
                <v-list-item-title class="text-body-2 text-on-surface">
                  {{ result.title }}
                </v-list-item-title>
                <v-list-item-subtitle class="text-caption text-on-surface-variant">
                  #{{ result.chunkIndex }} · {{ result.documentID }} ·
                  {{ result.score.toFixed(2) }}
                </v-list-item-subtitle>
                <p class="text-body-2 text-on-surface mt-1">{{ result.content }}</p>
              </v-list-item>
            </v-list>
          </div>
        </template>
        <template #footer>
          <v-btn text @click="dryRunDialog = false">
            {{ $t("action.close") }}
          </v-btn>
        </template>
      </app-dialog>
    </template>
  </main-layout>
</template>

<route lang="yaml">
name: AI Knowledge List
meta:
  title: title.aiKnowledge
  requiresAuth: true
  requiresPerm: true
</route>

<script setup lang="ts" name="ai-knowledge-list">
import * as yup from "yup";
import { useForm } from "vee-validate";

import {
  knowledgeDocumentCreate,
  knowledgeDocumentDelete,
  knowledgeDocumentDetailGet,
  knowledgeDocumentListGet,
  knowledgeDocumentListOptions,
  knowledgeDocumentReindex,
  knowledgeDocumentUpdate,
  knowledgeRetrievalDryRunStream,
  knowledgeRetrievalOptions,
} from "@/apis/ai";
import type {
  DiagnosisCategory,
  DiagnosisSuggestion,
  DryRunCallUsage,
  DryRunStageEvent,
  EvalStage,
  EvalStageStatus,
  KnowledgeDocumentDetail,
  KnowledgeDocumentReindexOut,
  RetrievalDryRunIn,
  RetrievalDryRunOut,
  RetrievalOptionsOut,
} from "@/apis/@types/ai";
import { getResponse, getSimpleResponse } from "@/apis/base";
import type { BatchResultOut } from "@/apis/@types/admin";
import type { BatchActions, ItemActions } from "@/components/@types/table";
import type { ActionButton } from "@/components/base/ActionButtonGroup.vue";
import { useAppStore } from "@/stores/app";
import { EnumClass, enumLabelOf, ensureEnum } from "@/utils/enums";
import { vuetifyCfg } from "@/utils/form";
import i18n from "@/utils/i18n";

// ---- 表单校验 schema（正文为多行文本域，内容中的空白具有语义，不做 trim）----
const createDocSchema = yup.object({
  title: yup
    .string()
    .trim()
    .nonNullable()
    .required()
    .min(1)
    .max(255)
    .label("model.aiDocument.field.title"),
  content: yup.string().nonNullable().required().min(1).label("model.aiDocument.field.content"),
});

const updateDocSchema = yup.object({
  title: yup
    .string()
    .trim()
    .nonNullable()
    .required()
    .min(1)
    .max(255)
    .label("model.aiDocument.field.title"),
  content: yup.string().nonNullable().required().min(1).label("model.aiDocument.field.content"),
});

const appStore = useAppStore();
const searchTableRef = useTemplateRef("searchTableRef");
const createDialogRef = useTemplateRef("createDialogRef");
const editDialogRef = useTemplateRef("editDialogRef");
const deleteConfirmRef = useTemplateRef("deleteConfirmRef");
const reindexConfirmRef = useTemplateRef("reindexConfirmRef");

// ---- forms ----
const createForm = useForm({ validationSchema: createDocSchema });
const updateForm = useForm({ validationSchema: updateDocSchema });

const [createTitle, createTitleProps] = createForm.defineField("title", vuetifyCfg);
const [createContent, createContentProps] = createForm.defineField("content", vuetifyCfg);

const [editTitle, editTitleProps] = updateForm.defineField("title", vuetifyCfg);
const [editContent, editContentProps] = updateForm.defineField("content", vuetifyCfg);

// ---- 创建对话框 ----
const openCreateDialogAction = (): void => {
  createForm.resetField("title", { value: "" });
  createForm.resetField("content", { value: "" });
  createDialogRef.value?.open({});
};

const createAction = createForm.handleSubmit(async (values) => {
  await getSimpleResponse(
    knowledgeDocumentCreate({
      // 标题为单行文本，提交前去除首尾空白，避免误输入空白创建出看似相同实则不同的记录
      title: (values.title as string).trim(),
      content: values.content,
    }),
    async (rsp) => {
      await searchTableRef.value?.refresh();
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "success",
      });
    },
    async (rsp) => {
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "error",
      });
    },
  );
});

// ---- 编辑对话框 ----
let editDocumentId = 0;

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const detailGetAction = async (item: Record<string, any>): Promise<void> => {
  await getResponse<KnowledgeDocumentDetail>(
    knowledgeDocumentDetailGet(item.id),
    async (rsp) => {
      editDocumentId = rsp.data.id;
      updateForm.resetField("title", { value: rsp.data.title });
      updateForm.resetField("content", { value: rsp.data.content });
      editDialogRef.value?.open(rsp.data);
    },
    async (rsp) => {
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "error",
      });
    },
  );
};

const itemUpdateAction = () => {
  updateForm.handleSubmit(async (values) => {
    await getSimpleResponse(
      knowledgeDocumentUpdate(editDocumentId, {
        id: editDocumentId,
        title: (values.title as string).trim(),
        content: values.content,
      }),
      async (rsp) => {
        await searchTableRef.value?.refresh();
        appStore.notify({
          text: rsp.summary,
          timeout: 3000,
          color: "success",
        });
      },
      async (rsp) => {
        appStore.notify({
          text: rsp.summary,
          timeout: 3000,
          color: "error",
        });
      },
    );
  })();
};

// ---- 列表操作 ----
// projectFields 字段投影：行操作依赖字段恒定投影，避免用户隐藏对应列后行操作失效
const projectFields = (visible: string[]): string[] => ["id", "title", ...visible];

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const getActions = (item: Record<string, any>): ItemActions => {
  return {
    edit: () => detailGetAction(item),
    chunks: () => {
      chunksDocumentId.value = item.id as number;
      chunksDocumentTitle.value = item.title as string;
      chunksDialog.value = true;
    },
    reindex: () => {
      batchState.value = null;
      reindexConfirmRef.value?.open(item);
    },
    delete: () => {
      batchState.value = null;
      deleteConfirmRef.value?.open(item);
    },
  };
};

// ---- 分块明细 ----
// 明细由后端读时派生（不改表），打开对话框时按当前文档 id 现取现算
const chunksDialog = ref(false);
const chunksDocumentId = ref(0);
const chunksDocumentTitle = ref("");

// ---- 重新索引 ----
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const itemReindexAction = async (proxy: Record<string, any>): Promise<void> => {
  await getResponse<KnowledgeDocumentReindexOut>(
    knowledgeDocumentReindex(proxy.id),
    async (rsp) => {
      await searchTableRef.value?.refresh();
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "success",
      });
    },
    async (rsp) => {
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "error",
      });
    },
  );
};

// ---- 批量删除 ----
// applyBatchResult 统一处理 BatchResultOut 响应：存在失败条目时以 error 提示失败数，否则沿用成功摘要
const applyBatchResult = (summary: string, result: BatchResultOut): void => {
  if (result.failed.length > 0) {
    appStore.notify({
      text: i18n.global.t("notice.batchResult", {
        succeeded: result.succeeded,
        failed: result.failed.length,
      }),
      timeout: 3000,
      color: "error",
    });
    return;
  }
  appStore.notify({
    text: summary,
    timeout: 3000,
    color: "success",
  });
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type BatchState = { items: Record<string, any>[]; action: "delete" };
const batchState = ref<BatchState | null>(null);

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const getBatchActions = (items: Record<string, any>[]): BatchActions => {
  return {
    delete: () => {
      batchState.value = { items, action: "delete" };
      deleteConfirmRef.value?.open({});
    },
  };
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const batchDeleteAction = async (items: Record<string, any>[]): Promise<void> => {
  const ids = items.map((i) => i.id as number);
  await getResponse<BatchResultOut>(
    knowledgeDocumentDelete(ids),
    async (rsp) => {
      applyBatchResult(rsp.summary, rsp.data);
      searchTableRef.value?.clearSelection?.();
      await searchTableRef.value?.refresh();
    },
    async (rsp) => {
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "error",
      });
    },
  );
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const itemDeleteAction = async (proxy: Record<string, any>): Promise<void> => {
  const state = batchState.value;
  if (state) {
    batchState.value = null;
    await batchDeleteAction(state.items);
    return;
  }
  await getResponse<BatchResultOut>(
    knowledgeDocumentDelete([proxy.id]),
    async (rsp) => {
      applyBatchResult(rsp.summary, rsp.data);
      await searchTableRef.value?.refresh();
    },
    async (rsp) => {
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "error",
      });
    },
  );
};

// ---- 检索试运行 ----
// 单个阶段的前端运行态：status 为后端上报的阶段状态，output 为该阶段模型原始输出增量累计
interface DryRunStageState {
  stage: EvalStage;
  status: EvalStageStatus;
  elapsedMs: number;
  usage?: DryRunCallUsage;
  error?: string;
  output: string;
}

const dryRunDialog = ref(false);
const dryRunQuery = ref("");
const dryRunTopK = ref<number | null>(null);
const dryRunCandidates = ref<number | null>(null);
const dryRunFusion = ref<string | null>(null);
const dryRunWeightVector = ref<number | null>(null);
const dryRunWeightKeyword = ref<number | null>(null);
const dryRunRunning = ref(false);
const dryRunResult = ref<RetrievalDryRunOut | null>(null);
// 运行过程区：阶段进度列表与当前展开的阶段（v-expansion-panels 的 v-model）。
// 面板为手风琴式，同一时刻只展开一项，故为单值而非列表；运行中自动跟随最新上报的阶段，
// 空串表示全部收起
const dryRunStages = ref<DryRunStageState[]>([]);
const expandedStage = ref<string>("");
// 各阶段输出容器的 DOM 引用：面板内容仅在展开时渲染、收起后即卸载（Vuetify useLazy 行为），
// 同名模板 ref 在 v-for 中还会退化成数组，无法稳定定位，故按阶段登记到 Map，
// 由模板中的函数 ref 在元素挂载 / 卸载时写入与清除
const stageOutputEls = new Map<EvalStage, HTMLPreElement>();
const bindStageOutputEl = (stage: EvalStage, el: unknown): void => {
  if (el) {
    stageOutputEls.set(stage, el as HTMLPreElement);
  } else {
    stageOutputEls.delete(stage);
  }
};
// 高级参数是否展开（默认折叠隐藏，仅展示查询问题）
const showAdvancedParams = ref(false);
// 后端 [llm].rerank 配置的默认参数，用于折叠提示与展开表单预填
const dryRunDefaults = ref<RetrievalOptionsOut | null>(null);
// 结果明细折叠面板的展开状态（手风琴式，同一时刻仅一项；空串表示全部收起）。
// 与运行过程面板一致默认全部收起，由用户按需展开，避免长明细一上来就撑满对话框
const resultPanel = ref<string>("");
// 当前流式请求的中止函数（为空表示当前没有进行中的流）
const dryRunAbort = ref<(() => void) | null>(null);

// 融合策略选项（rrf / weighted）
const fusionOptions = [
  { title: "RRF", value: "rrf" },
  { title: "Weighted", value: "weighted" },
];

// fusionTitleOf 将融合策略值映射为下拉选项的展示名（如 rrf → RRF），未命中时原样返回
const fusionTitleOf = (value: string): string =>
  fusionOptions.find((option) => option.value === value)?.title ?? value;

// 四项量化指标（0-1 原始值，展示时经 percentOf 转为百分比）
const dryRunMetrics = computed(() => {
  const metrics = dryRunResult.value?.evaluation?.metrics;
  if (!metrics) {
    return [];
  }
  return [
    {
      key: "contextPrecision",
      label: i18n.global.t("page.aiKnowledge.contextPrecision"),
      value: metrics.contextPrecision,
    },
    {
      key: "faithfulness",
      label: i18n.global.t("page.aiKnowledge.faithfulness"),
      value: metrics.faithfulness,
    },
    {
      key: "answerRelevance",
      label: i18n.global.t("page.aiKnowledge.answerRelevance"),
      value: metrics.answerRelevance,
    },
    {
      key: "contextUtilization",
      label: i18n.global.t("page.aiKnowledge.contextUtilization"),
      value: metrics.contextUtilization,
    },
  ];
});

// percentOf 将 0-1 指标值转换为百分比文本（保留一位小数）
const percentOf = (value: number): string => `${(value * 100).toFixed(1)}%`;

// dryRunWarnings 评估警告列表（评估缺失时为空数组，避免模板中的可选链）
const dryRunWarnings = computed<string[]>(() => dryRunResult.value?.evaluation?.warnings ?? []);

// dryRunSuggestions 改进建议列表（评估缺失时为空数组，避免模板中的可选链）
const dryRunSuggestions = computed<DiagnosisSuggestion[]>(
  () => dryRunResult.value?.evaluation?.suggestions ?? [],
);

// 改进建议归类 → 图标：以图标区分「补文档」与「调参数」两类动作（归类取值由后端下发，未命中降级兜底）
const suggestionIcons: Record<string, string> = {
  coverage: "mdi-book-plus-outline",
  ranking: "mdi-sort-variant",
  generation: "mdi-text-box-edit-outline",
  context: "mdi-link-variant",
  other: "mdi-lightbulb-outline",
};

// suggestionIconOf 建议归类图标，未知归类降级为兜底图标
const suggestionIconOf = (category: DiagnosisCategory): string =>
  suggestionIcons[category] ?? suggestionIcons.other;

// suggestionCategoryLabelOf 建议归类展示名（经枚举注册表 diagnosisCategory 按当前语言渲染 label）
const suggestionCategoryLabelOf = (category: DiagnosisCategory): string =>
  enumLabelOf(EnumClass.diagnosisCategory, category);

// stageLabelOf 阶段展示名（经枚举注册表 evalStage 按当前语言渲染 label）
const stageLabelOf = (stage: EvalStage): string => enumLabelOf(EnumClass.evalStage, stage);

// stageIconOf 阶段状态图标：运行中旋转、完成打勾、失败告警；流已停止仍在进行的阶段展示为已中止
const stageIconOf = (stage: DryRunStageState): string => {
  if (stage.status === "done") {
    return "mdi-check-circle-outline";
  }
  if (stage.status === "failed") {
    return "mdi-alert-circle-outline";
  }
  return dryRunRunning.value ? "mdi-loading" : "mdi-stop-circle-outline";
};

// stageColorOf 阶段状态语义色
const stageColorOf = (stage: DryRunStageState): string => {
  if (stage.status === "done") {
    return "success";
  }
  if (stage.status === "failed") {
    return "warning";
  }
  return dryRunRunning.value ? "primary" : "warning";
};

// 当前展开阶段的模型原始输出（阶段未登记或全部收起时为空串）
const expandedStageOutput = computed<string>(
  () => dryRunStages.value.find((item) => item.stage === expandedStage.value)?.output ?? "",
);

// 输出更新后把展开中的输出容器滚动到底部，始终展示最新增量
watch(expandedStageOutput, async () => {
  await nextTick();
  const target = expandedStage.value ? stageOutputEls.get(expandedStage.value) : undefined;
  if (target) {
    target.scrollTop = target.scrollHeight;
  }
});

// applyStageEvent 应用一次阶段状态事件：首次出现时建行，后续仅更新状态字段
const applyStageEvent = (ev: DryRunStageEvent): void => {
  const target = dryRunStages.value.find((item) => item.stage === ev.stage);
  if (target) {
    target.status = ev.status ?? target.status;
    target.elapsedMs = ev.elapsedMs ?? target.elapsedMs;
    target.usage = ev.usage ?? target.usage;
    target.error = ev.error ?? target.error;
  } else {
    dryRunStages.value.push({
      stage: ev.stage,
      status: ev.status ?? "start",
      elapsedMs: ev.elapsedMs ?? 0,
      usage: ev.usage,
      error: ev.error,
      output: "",
    });
  }
  // 自动跟随最新阶段：展开到刚上报的阶段，运行中无需手动切换即可看到当前阶段输出
  expandedStage.value = ev.stage;
};

// appendStageDelta 累计某阶段的模型原始输出增量
const appendStageDelta = (stage: EvalStage, delta: string): void => {
  const target = dryRunStages.value.find((item) => item.stage === stage);
  if (target) {
    target.output += delta;
  }
};

// modelOfStage 该阶段实际调用的模型名：仅后端汇总的调用元信息携带（流式阶段事件不含），
// 故运行结束后才有值，运行中为空串
const modelOfStage = (stage: EvalStage): string =>
  dryRunResult.value?.meta.calls.find((call) => call.stage === stage)?.model ?? "";

// stageMetaTextOf 阶段行右侧摘要：模型、耗时、token 用量与失败原因（无内容时为空串）。
// 这里是调用信息的唯一落点——原「调用元信息」列表除模型名与合计 Token 外的字段均与此重复，
// 已整块合并进来，不再单独成块（见 AGENTS.md「展开面板族」）
const stageMetaTextOf = (stage: DryRunStageState): string => {
  const parts: string[] = [];
  const model = modelOfStage(stage.stage);
  if (model) {
    parts.push(`${i18n.global.t("page.aiKnowledge.metaModel")} ${model}`);
  }
  if (stage.elapsedMs > 0) {
    parts.push(`${stage.elapsedMs} ms`);
  }
  if (stage.usage) {
    parts.push(
      `${i18n.global.t("page.aiKnowledge.metaPromptTokens")} ${stage.usage.promptTokens}`,
      `${i18n.global.t("page.aiKnowledge.metaCompletionTokens")} ${stage.usage.completionTokens}`,
      `${i18n.global.t("page.aiKnowledge.metaTotalTokens")} ${stage.usage.totalTokens}`,
    );
  }
  if (stage.error) {
    parts.push(`${i18n.global.t("page.aiKnowledge.stageFailed")}: ${stage.error}`);
  }
  return parts.join(" · ");
};

// loadDryRunDefaults 拉取后端 [llm].rerank 配置的默认参数并预填表单：
// 仅填充尚未设置（null/空）的字段，避免覆盖用户修改
const loadDryRunDefaults = async (): Promise<void> => {
  await getResponse<RetrievalOptionsOut>(
    knowledgeRetrievalOptions(),
    async (rsp) => {
      dryRunDefaults.value = rsp.data;
      if (dryRunTopK.value === null) {
        dryRunTopK.value = rsp.data.topK;
      }
      if (dryRunCandidates.value === null) {
        dryRunCandidates.value = rsp.data.candidates;
      }
      if (!dryRunFusion.value) {
        dryRunFusion.value = rsp.data.fusion;
      }
      if (dryRunWeightVector.value === null) {
        dryRunWeightVector.value = rsp.data.weights.vector;
      }
      if (dryRunWeightKeyword.value === null) {
        dryRunWeightKeyword.value = rsp.data.weights.keyword;
      }
    },
    async (rsp) => {
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "error",
      });
    },
  );
};

// stopDryRunAction 中止当前流式请求（客户端断开后后端会停止后续大模型调用），
// 并把运行态复位
const stopDryRunAction = (): void => {
  dryRunAbort.value?.();
  dryRunAbort.value = null;
  dryRunRunning.value = false;
};

// resetDryRunDialog 清空检索试运行的全部状态：关闭对话框后查询、参数、过程与结果
// 均不残留，下次打开为全新输入
const resetDryRunDialog = (): void => {
  stopDryRunAction();
  dryRunQuery.value = "";
  dryRunResult.value = null;
  dryRunStages.value = [];
  expandedStage.value = "";
  stageOutputEls.clear();
  dryRunDefaults.value = null;
  showAdvancedParams.value = false;
  dryRunTopK.value = null;
  dryRunCandidates.value = null;
  dryRunFusion.value = null;
  dryRunWeightVector.value = null;
  dryRunWeightKeyword.value = null;
  resultPanel.value = "";
};

const openDryRunDialog = async (): Promise<void> => {
  // 打开时重置状态并拉取后端默认值预填，供折叠提示与展开表单展示
  resetDryRunDialog();
  dryRunDialog.value = true;
  // 阶段名与建议归类经统一枚举注册表渲染 label，按当前语言预取（缓存到达后模板自动刷新）
  void ensureEnum(EnumClass.evalStage);
  void ensureEnum(EnumClass.diagnosisCategory);
  await loadDryRunDefaults();
};

const runDryRunAction = (): void => {
  // 查询为单行文本，提交前去除首尾空白；为空或已在运行则直接返回
  const query = dryRunQuery.value.trim();
  if (!query || dryRunRunning.value) {
    return;
  }
  const payload: RetrievalDryRunIn = { query };
  if (dryRunTopK.value !== null && dryRunTopK.value > 0) {
    payload.topK = dryRunTopK.value;
  }
  if (dryRunCandidates.value !== null && dryRunCandidates.value > 0) {
    payload.candidates = dryRunCandidates.value;
  }
  if (dryRunFusion.value) {
    payload.fusion = dryRunFusion.value;
  }
  // 权重两值任一非 0 才携带；Number 归一化空串/缺省为 0
  const vectorWeight = Number(dryRunWeightVector.value ?? 0);
  const keywordWeight = Number(dryRunWeightKeyword.value ?? 0);
  if (vectorWeight !== 0 || keywordWeight !== 0) {
    payload.weights = { vector: vectorWeight, keyword: keywordWeight };
  }
  // 新一轮运行清空上一轮的过程与结果
  dryRunResult.value = null;
  dryRunStages.value = [];
  expandedStage.value = "";
  stageOutputEls.clear();
  resultPanel.value = "";
  dryRunRunning.value = true;
  dryRunAbort.value = knowledgeRetrievalDryRunStream(payload, {
    onStage: applyStageEvent,
    onDelta: appendStageDelta,
    onDone: (out) => {
      dryRunResult.value = out;
      dryRunRunning.value = false;
      dryRunAbort.value = null;
      // 运行中自动展开最新阶段便于跟读输出，结束后收起：过程与结果都已定格，
      // 折叠成清单让用户按需回看（报错时不收起——此时结果区不存在，过程是唯一内容）
      expandedStage.value = "";
    },
    onError: (msg) => {
      dryRunRunning.value = false;
      dryRunAbort.value = null;
      appStore.notify({
        text: msg,
        timeout: 3000,
        color: "error",
      });
    },
  });
};

// 工具栏操作按钮：检索试运行入口（alwaysShow 常驻，收起工具栏时也保持展示）

const toolbarActions = computed<ActionButton[]>(() => [
  {
    icon: "mdi-flask-outline",
    title: i18n.global.t("page.aiKnowledge.dryRun"),
    // 检索试运行对知识库管理是核心能力，收起工具栏时也保持展示
    alwaysShow: true,
    onClick: openDryRunDialog,
  },
]);
</script>

<style scoped>
/* 高级参数区：与上方表单的分隔线走 .zk-hairline-top。v-row dense 自带 -4px 负外边距，
   必须由容器内边距吸收，否则表单会向上压进折叠按钮的盒子造成重叠 */
.advanced-params {
  margin-top: var(--zk-space-1);
  padding: var(--zk-space-2) 0 var(--zk-space-1);
}

/* 检索试运行结果区：分区盒外观（描边 + 圆角）走 .zk-panel */
.dry-run-result {
  margin-top: var(--zk-space-3);
  padding: var(--zk-space-3);
}

/* 检索试运行过程区：与结果区同一分区盒视觉语言（外观走 .zk-panel） */
.dry-run-process {
  margin-top: var(--zk-space-3);
  padding: var(--zk-space-3);
}

/* 阶段行右侧元信息：面板标题行高固定，超长内容（如失败原因）以省略号收束，完整文本经 title 提示；
   min-width: 0 让该项可被压缩，否则 nowrap 文本会撑破标题行 */
.stage-meta {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 阶段模型原始输出：等宽走 .font-mono，底色 / 换行 / 内边距走 .zk-code-preview，
   此处仅保留 margin 归零（pre 自带 1em）与本地滚动所需的限高与行距 */
.stage-output {
  margin: 0;
  max-height: calc(var(--zk-editor-min-height) * 1.25);
  overflow: auto;
  line-height: 1.5;
}

.result-block {
  margin-top: 8px;
}

.result-block:first-child {
  margin-top: 0;
}

.result-label {
  margin-bottom: var(--zk-space-1);
}

/* 生成回答：引用块底色 / 换行 / 内边距走 .zk-code-preview，无剩余本地声明 */

.answer-body {
  margin: 0;
  line-height: 1.6;
  white-space: pre-wrap;
  overflow-wrap: break-word;
}

/* 量化指标：自适应卡片网格 */
.metric-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: var(--zk-space-2);
}

.metric-card {
  padding: var(--zk-space-2) var(--zk-space-3);
}

.metric-value {
  margin: var(--zk-space-1) 0 var(--zk-space-1);
  line-height: 1.4;
}

.metric-footnote {
  margin-top: var(--zk-space-1);
}
</style>
