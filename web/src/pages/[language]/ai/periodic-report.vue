<template>
  <main-layout current-nav="ai">
    <template #body>
      <!-- 通用列表页：表头/筛选/排序/分页/字段投影均由后端 OPTIONS 与搜索契约驱动 -->
      <search-table
        ref="searchTableRef"
        :list-request="periodicReportPlanListGet"
        :options-request="periodicReportPlanListOptions"
        :actions="getActions"
        :create="openCreateDialogAction"
        :project-fields="projectFields"
        :default-order-by="defaultOrderBy"
        model-name="periodicReportPlan"
      >
        <!-- 启停列：开关即时写入（乐观更新，失败回退） -->
        <template #cell-enabled="{ item }">
          <enabled-switch-cell
            :model-value="item.enabled"
            :request="(enabled: boolean) => periodicReportPlanToggle(item.id, enabled)"
            @update:model-value="(v: boolean) => (item.enabled = v)"
          />
        </template>

        <!-- 失效时间：空表示长期有效 -->
        <template #cell-expiresAt="{ item }">
          <span v-if="item.expiresAt">
            {{ localeDatetimeFormat(DateTime.fromISO(item.expiresAt)) }}
          </span>
          <span v-else class="zk-hint">{{ $t("page.periodicReportPlan.longTerm") }}</span>
        </template>

        <!-- 下次执行：空表示待分发器计算 -->
        <template #cell-nextRunAt="{ item }">
          <span v-if="item.nextRunAt">
            {{ localeDatetimeFormat(DateTime.fromISO(item.nextRunAt)) }}
          </span>
          <span v-else class="zk-hint">{{ $t("page.periodicReportPlan.pendingSchedule") }}</span>
        </template>

        <!-- 输入参数由后端 OPTIONS 元数据标注为 json 列，交由自适应单元格统一渲染 -->
      </search-table>

      <!-- 创建报告计划 -->
      <temp-editable-dialog
        ref="createDialogRef"
        :title="$t('page.periodicReportPlan.create')"
        :confirm-disabled="!createForm.meta.value.valid"
        force-editable
        prepend-icon="mdi-file-chart-outline"
        @confirm="createAction"
      >
        <template #body>
          <v-form>
            <!-- 技能资格提醒：说明可用技能范围与固定字段偏置规则 -->
            <v-row density="compact">
              <v-col cols="12">
                <v-alert type="info">
                  {{ $t("page.periodicReportPlan.scopeHint") }}
                </v-alert>
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <v-text-field
                  v-model="createName"
                  v-bind="createNameProps"
                  :label="$t('model.periodicReportPlan.field.name')"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <v-select
                  v-model="createSkillID"
                  v-bind="createSkillIDProps"
                  :items="skillOptions"
                  item-title="name"
                  item-value="id"
                  :label="$t('model.periodicReportPlan.field.skill')"
                  @update:model-value="onCreateSkillChange"
                >
                  <template #item="{ props: itemProps, item }">
                    <v-list-item
                      v-bind="itemProps"
                      :title="item.name"
                      :subtitle="item.description"
                    />
                  </template>
                </v-select>
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="6">
                <cron-field
                  v-model="createCronSpec"
                  v-bind="createCronSpecProps"
                  :label="$t('model.periodicReportPlan.field.cronSpec')"
                  :placeholder="$t('page.periodicReportPlan.cronPlaceholder')"
                />
              </v-col>
              <v-col cols="6">
                <v-text-field
                  v-model="createRetentionDays"
                  v-bind="createRetentionDaysProps"
                  type="number"
                  :label="$t('model.periodicReportPlan.field.retentionDays')"
                  :hint="$t('page.periodicReportPlan.retentionHint')"
                  persistent-hint
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <date-field
                  :model-value="createExpiresAtDate"
                  v-bind="createExpiresAtProps"
                  :label="$t('model.periodicReportPlan.field.expiresAt')"
                  :hint="$t('page.periodicReportPlan.expiresHint')"
                  persistent-hint
                  @update:model-value="(value: unknown) => (createExpiresAtDate = isoDateOf(value))"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <v-textarea
                  v-model="createDescription"
                  v-bind="createDescriptionProps"
                  :label="$t('model.periodicReportPlan.field.description')"
                  rows="3"
                />
              </v-col>
            </v-row>
            <!-- nowTime 由系统在运行时注入，计划表单不渲染也不提交 -->
            <v-row v-if="createHasNowTime" density="compact">
              <v-col cols="12">
                <v-alert type="info" variant="tonal" density="compact">
                  {{ $t("page.periodicReportPlan.nowTimeInjected") }}
                </v-alert>
              </v-col>
            </v-row>
            <!-- 输入字段：标题与固定字段编辑器标题同基线 -->
            <v-row v-if="createInputFields.length" density="compact">
              <v-col cols="12">
                <div class="zk-editor-header">
                  <span class="zk-editor-title">
                    {{ $t("page.periodicReportPlan.inputFieldsTitle") }}
                  </span>
                </div>
              </v-col>
            </v-row>
            <!-- 技能输入参数：按技能 inputFields 动态渲染，提交时保留原始 JSON 类型 -->
            <v-row
              v-for="field in createInputFields"
              :key="`create-${field.name}`"
              density="compact"
            >
              <v-col cols="12">
                <v-switch
                  v-if="field.type === 'boolean'"
                  :model-value="createInputValues[field.name] === true"
                  :label="inputFieldLabel(field)"
                  :hint="field.description"
                  persistent-hint
                  color="primary"
                  @update:model-value="
                    (value: boolean | null) => setCreateInputValue(field.name, value === true)
                  "
                />
                <v-text-field
                  v-else-if="field.type === 'number'"
                  :model-value="textValue(createInputValues[field.name])"
                  type="number"
                  :label="inputFieldLabel(field)"
                  :hint="field.description"
                  persistent-hint
                  @update:model-value="
                    (value: string | number | null) =>
                      setCreateInputValue(field.name, numberValueOf(value))
                  "
                />
                <v-text-field
                  v-else
                  :model-value="textValue(createInputValues[field.name])"
                  :label="inputFieldLabel(field)"
                  :hint="field.description"
                  persistent-hint
                  @update:model-value="
                    (value: string | number | null) =>
                      setCreateInputValue(field.name, String(value ?? ''))
                  "
                />
              </v-col>
            </v-row>
            <!-- 固定字段：整段 JSON 可编辑（含对象 / 数组），未修改的键沿用技能默认值 -->
            <v-row v-if="createHasFixedFields" density="compact">
              <v-col cols="12">
                <json-editor
                  v-model="createFixedText"
                  :label="$t('page.periodicReportPlan.fixedFieldsTitle')"
                  :hint="$t('page.periodicReportPlan.fixedFieldsHint')"
                />
              </v-col>
            </v-row>
          </v-form>
        </template>
      </temp-editable-dialog>

      <!-- 编辑报告计划 -->
      <temp-editable-dialog
        ref="editDialogRef"
        :title="$t('page.periodicReportPlan.edit')"
        :confirm-disabled="!editForm.meta.value.valid"
        prepend-icon="mdi-file-chart-outline"
        @confirm="itemUpdateAction"
      >
        <template #body="{ editable }">
          <v-form>
            <!-- 技能资格提醒：说明可用技能范围与固定字段偏置规则 -->
            <v-row density="compact">
              <v-col cols="12">
                <v-alert type="info">
                  {{ $t("page.periodicReportPlan.scopeHint") }}
                </v-alert>
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <v-text-field
                  v-model="editName"
                  v-bind="editNameProps"
                  :label="$t('model.periodicReportPlan.field.name')"
                  :disabled="!editable"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <v-select
                  v-model="editSkillID"
                  v-bind="editSkillIDProps"
                  :items="skillOptions"
                  item-title="name"
                  item-value="id"
                  :label="$t('model.periodicReportPlan.field.skill')"
                  :disabled="!editable"
                  @update:model-value="onEditSkillChange"
                >
                  <template #item="{ props: itemProps, item }">
                    <v-list-item
                      v-bind="itemProps"
                      :title="item.name"
                      :subtitle="item.description"
                    />
                  </template>
                </v-select>
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="6">
                <cron-field
                  v-model="editCronSpec"
                  v-bind="editCronSpecProps"
                  :label="$t('model.periodicReportPlan.field.cronSpec')"
                  :placeholder="$t('page.periodicReportPlan.cronPlaceholder')"
                  :disabled="!editable"
                />
              </v-col>
              <v-col cols="6">
                <v-text-field
                  v-model="editRetentionDays"
                  v-bind="editRetentionDaysProps"
                  type="number"
                  :label="$t('model.periodicReportPlan.field.retentionDays')"
                  :hint="$t('page.periodicReportPlan.retentionHint')"
                  persistent-hint
                  :disabled="!editable"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <date-field
                  :model-value="editExpiresAtDate"
                  v-bind="editExpiresAtProps"
                  :label="$t('model.periodicReportPlan.field.expiresAt')"
                  :hint="$t('page.periodicReportPlan.expiresHint')"
                  persistent-hint
                  :disabled="!editable"
                  @update:model-value="(value: unknown) => (editExpiresAtDate = isoDateOf(value))"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <v-textarea
                  v-model="editDescription"
                  v-bind="editDescriptionProps"
                  :label="$t('model.periodicReportPlan.field.description')"
                  :disabled="!editable"
                  rows="3"
                />
              </v-col>
            </v-row>
            <!-- nowTime 由系统在运行时注入，计划表单不渲染也不提交 -->
            <v-row v-if="editHasNowTime" density="compact">
              <v-col cols="12">
                <v-alert type="info" variant="tonal" density="compact">
                  {{ $t("page.periodicReportPlan.nowTimeInjected") }}
                </v-alert>
              </v-col>
            </v-row>
            <!-- 输入字段：标题与固定字段编辑器标题同基线 -->
            <v-row v-if="editInputFields.length" density="compact">
              <v-col cols="12">
                <div class="zk-editor-header">
                  <span class="zk-editor-title">
                    {{ $t("page.periodicReportPlan.inputFieldsTitle") }}
                  </span>
                </div>
              </v-col>
            </v-row>
            <!-- 编辑态按当前技能重新渲染参数输入，禁用态随对话框可编辑态联动 -->
            <v-row v-for="field in editInputFields" :key="`edit-${field.name}`" density="compact">
              <v-col cols="12">
                <v-switch
                  v-if="field.type === 'boolean'"
                  :model-value="editInputValues[field.name] === true"
                  :label="inputFieldLabel(field)"
                  :hint="field.description"
                  persistent-hint
                  color="primary"
                  :disabled="!editable"
                  @update:model-value="
                    (value: boolean | null) => setEditInputValue(field.name, value === true)
                  "
                />
                <v-text-field
                  v-else-if="field.type === 'number'"
                  :model-value="textValue(editInputValues[field.name])"
                  type="number"
                  :label="inputFieldLabel(field)"
                  :hint="field.description"
                  persistent-hint
                  :disabled="!editable"
                  @update:model-value="
                    (value: string | number | null) =>
                      setEditInputValue(field.name, numberValueOf(value))
                  "
                />
                <v-text-field
                  v-else
                  :model-value="textValue(editInputValues[field.name])"
                  :label="inputFieldLabel(field)"
                  :hint="field.description"
                  persistent-hint
                  :disabled="!editable"
                  @update:model-value="
                    (value: string | number | null) =>
                      setEditInputValue(field.name, String(value ?? ''))
                  "
                />
              </v-col>
            </v-row>
            <!-- 固定字段：整段 JSON 可编辑（含对象 / 数组），未修改的键沿用技能默认值 -->
            <v-row v-if="editHasFixedFields" density="compact">
              <v-col cols="12">
                <json-editor
                  v-model="editFixedText"
                  :label="$t('page.periodicReportPlan.fixedFieldsTitle')"
                  :hint="$t('page.periodicReportPlan.fixedFieldsHint')"
                  :disabled="!editable"
                />
              </v-col>
            </v-row>
          </v-form>
        </template>
      </temp-editable-dialog>

      <!-- 删除确认 -->
      <confirm-dialog
        ref="deleteConfirmRef"
        :title="$t('model.periodicReportPlan.name')"
        prepend-icon="mdi-delete-outline"
        @confirm="deleteAction"
      >
        <template #body="{ proxy }">
          <confirm-alert
            :action="$t('action.delete')"
            :text="
              $t('notice.deleteConfirm', {
                category: $t('model.periodicReportPlan.name'),
                key: proxy.name,
              })
            "
          />
        </template>
      </confirm-dialog>

      <!-- 立即执行确认 -->
      <confirm-dialog
        ref="runConfirmRef"
        :title="$t('model.periodicReportPlan.name')"
        prepend-icon="mdi-play"
        @confirm="runAction"
      >
        <template #body="{ proxy }">
          <confirm-alert
            :action="$t('page.periodicReportPlan.runNow')"
            :text="$t('page.periodicReportPlan.runNowConfirm', { name: proxy.name })"
          />
        </template>
      </confirm-dialog>

      <!-- 历史报告：该计划的运行记录列表，点击行查看报告正文 -->
      <app-dialog
        v-model="historyDialog"
        :title="$t('page.periodicReportPlan.runsTitle')"
        :subtitle="historyPlanName"
        icon="mdi-history"
        size="lg"
        :loading="runsLoading"
      >
        <div v-if="runs.length === 0 && !runsLoading" class="zk-hint">
          {{ $t("page.periodicReportPlan.noRuns") }}
        </div>
        <v-data-table
          v-else
          :headers="runHeaders"
          :items="runs"
          hide-default-footer
          class="history-table"
        >
          <template #item="{ item }">
            <!-- 自定义 item 插槽直接渲染 <tr> 后，v-data-table 的 click:row 不再派发，
                 点击必须绑在行本身，否则「查看运行记录」里点行没有反应 -->
            <tr class="history-row text-no-wrap" @click="onRunRowClick($event, { item })">
              <td
                v-for="hdr in runHeaders"
                :key="hdr.key"
                :class="{ 'text-center': hdr.key === 'actions' }"
              >
                <template v-if="hdr.key === 'status'">
                  <div class="d-flex align-center ga-1">
                    <v-icon
                      :icon="runStatusMeta(item.status).icon"
                      :color="runStatusMeta(item.status).color"
                      :class="{ 'mdi-spin': runStatusMeta(item.status).spin }"
                      size="small"
                    />
                    <span>{{ enumLabelOf(EnumClass.periodicReportRunStatus, item.status) }}</span>
                  </div>
                </template>
                <template v-else-if="hdr.key === 'error'">
                  <span v-if="item.error" class="zk-error-text history-error" :title="item.error">
                    {{ item.error }}
                  </span>
                  <span v-else class="zk-hint">{{ INVALID_PLACEHOLDER }}</span>
                </template>
                <template v-else-if="hdr.key === 'actions'">
                  <v-btn
                    icon="mdi-message-text-outline"
                    size="small"
                    variant="text"
                    :disabled="!item.sessionID"
                    :title="$t('page.periodicReportPlan.viewSession')"
                    @click.stop="viewRunSession(item)"
                  />
                  <v-btn
                    icon="mdi-delete-outline"
                    size="small"
                    variant="text"
                    color="error"
                    :title="$t('page.periodicReportPlan.deleteRun')"
                    @click.stop="runDeleteConfirmRef?.open(item)"
                  />
                </template>
                <adaptive-cell v-else :header="hdr" :item="item" />
              </td>
            </tr>
          </template>
        </v-data-table>
        <template #footer>
          <v-btn :text="$t('action.close')" variant="text" @click="historyDialog = false" />
        </template>
      </app-dialog>

      <!-- 删除运行记录确认：运行中与已结束的记录都允许删除，报告正文一并删除 -->
      <confirm-dialog
        ref="runDeleteConfirmRef"
        :title="$t('page.periodicReportPlan.deleteRun')"
        prepend-icon="mdi-delete-outline"
        @confirm="deleteRunAction"
      >
        <template #body>
          <confirm-alert
            :action="$t('action.delete')"
            :text="$t('page.periodicReportPlan.deleteRunConfirm')"
          />
        </template>
      </confirm-dialog>

      <!-- 报告正文 -->
      <app-dialog
        v-model="detailDialog"
        :title="$t('page.periodicReportPlan.viewReport')"
        :subtitle="detailTitle"
        icon="mdi-file-document-outline"
        size="xl"
        :loading="detailLoading"
      >
        <assistant-markdown v-if="detailContent" :content="detailContent" />
        <div v-else-if="!detailLoading" class="zk-hint">{{ INVALID_PLACEHOLDER }}</div>
        <template #footer>
          <v-btn :text="$t('action.close')" variant="text" @click="detailDialog = false" />
        </template>
      </app-dialog>

      <!-- 运行会话只读审计视图：运行结束后会话冻结，仅供回顾，不提供任何对话能力 -->
      <report-session-viewer
        v-model="sessionViewerOpened"
        :session-id="sessionViewerID"
        :subtitle="sessionViewerTitle"
      />
    </template>
  </main-layout>
</template>

<route lang="yaml">
name: AI Periodic Report List
meta:
  title: title.aiPeriodicReport
  requiresAuth: true
  requiresPerm: true
</route>

<script setup lang="ts" name="ai-periodic-report-list">
import * as yup from "yup";
import { DateTime } from "luxon";
import { useForm } from "vee-validate";

// 插件 / 工具
import { getResponse, getSimpleResponse } from "@/apis/base";
import {
  periodicReportEnvelopeContent,
  periodicReportPlanCreate,
  periodicReportPlanDelete,
  periodicReportPlanListGet,
  periodicReportPlanListOptions,
  periodicReportPlanRun,
  periodicReportPlanToggle,
  periodicReportPlanUpdate,
  periodicReportRunDelete,
  periodicReportRunDetailGet,
  periodicReportRunListGet,
  periodicReportSkillOptionsGet,
} from "@/apis/periodicReport";
import { useAppStore } from "@/stores/app";
import ReportSessionViewer from "@/components/ai/ReportSessionViewer.vue";
import { BackendDataType, SearchAction } from "@/types";
import { INVALID_PLACEHOLDER } from "@/utils/base";
import { isCronSpecValid, parseCronSpec } from "@/utils/cron";
import { EnumClass, enumLabelOf, ensureEnum } from "@/utils/enums";
import { vuetifyCfg } from "@/utils/form";
import i18n, { localeDatetimeFormat } from "@/utils/i18n";

// 类型
import type { ItemActions, TableHeader } from "@/components/@types/table";
import type { SortItem } from "vuetify/lib/components/VDataTable/composables/sort.mjs";
import type {
  PeriodicReportPlanCreateIn,
  PeriodicReportPlanItem,
  PeriodicReportRunDetailOut,
  PeriodicReportRunItem,
  PeriodicReportRunListOut,
  PeriodicReportSkillInputField,
  PeriodicReportSkillOption,
  PeriodicReportSkillOptionListOut,
} from "@/apis/@types/periodicReport";

const RUN_PAGE_SIZE = 50;

const appStore = useAppStore();

// 错误提示统一沿用全站 appStore.notify 机制
const notifyError = (text: string): void => {
  appStore.notify({ text, timeout: 3000, color: "error" });
};
const notifySuccess = (text: string): void => {
  appStore.notify({ text, timeout: 3000, color: "success" });
};

// ---- 列表 ----
// 通用列表页由 SearchTable 驱动：表头、筛选项、排序、分页与字段投影均取后端 OPTIONS 与搜索契约。
const searchTableRef = useTemplateRef("searchTableRef");

// 默认按创建时间倒序，保持与计划管理页原有展示顺序一致
const defaultOrderBy: SortItem[] = [{ key: "createdAt", order: "desc" }];

// projectFields 字段投影：行操作（编辑/立即执行/历史/删除）依赖以下字段，
// 即使用户隐藏对应列也需随列表返回，避免对话框回填缺值。
const projectFields = (visible: string[]): string[] => [
  "id",
  "name",
  "description",
  "skillID",
  "cronSpec",
  "inputValues",
  "retentionDays",
  "expiresAt",
  "enabled",
  ...visible,
];

// ---- 技能选项 ----
const skillOptions = ref<PeriodicReportSkillOption[]>([]);

const loadSkillOptions = async (): Promise<void> => {
  await getResponse<PeriodicReportSkillOptionListOut>(
    periodicReportSkillOptionsGet(),
    async (rsp) => {
      skillOptions.value = rsp.data.items ?? [];
    },
    async (rsp) => notifyError(rsp.summary),
  );
};

const skillByID = (id: unknown): PeriodicReportSkillOption | undefined =>
  skillOptions.value.find((skill) => skill.id === Number(id));

// inputFieldLabel 技能参数标签：必填字段追加星号，与表单必填校验一致
const inputFieldLabel = (field: PeriodicReportSkillInputField): string =>
  field.required ? `${field.name} *` : field.name;

// ---- 表单校验 ----
// cronSpecMessage 校验失败时展示具体原因（如「分钟字段格式不正确」），而非笼统的默认文案。
const cronSpecMessage = (params: { path: string; value?: unknown }): string =>
  parseCronSpec(String(params.value ?? "")).error ?? i18n.global.t("cron.error.empty");
// isCronSpec 包一层以避开 yup 传入的测试上下文参数（模式由组件 prop 决定，此处为兼容模式）。
const isCronSpec = (value: string | undefined): boolean => isCronSpecValid(value, "both");

// isoDateOf 将 DateField 的取值（Date / ISO 字符串 / null）归一为表单存储的 ISO 日期文本，
// 仅用于提交契约；展示格式由 DateField 按当前语言本地化，页面不再自定日期写法。
// 非法或空值归一为空串，把「无效日期」交给 schema 与后端校验，避免提交半成品日期。
const isoDateOf = (value: unknown): string => {
  if (value === null || value === undefined || value === "") {
    return "";
  }
  const parsed =
    value instanceof Date
      ? DateTime.fromJSDate(value)
      : typeof value === "string"
        ? DateTime.fromISO(value)
        : null;
  return parsed?.isValid ? (parsed.toISODate() ?? "") : "";
};

const planSchema = yup.object({
  name: yup.string().trim().required().max(128).label("model.periodicReportPlan.field.name"),
  description: yup.string().trim().max(512).label("model.periodicReportPlan.field.description"),
  skillID: yup.number().required().min(1).label("model.periodicReportPlan.field.skill"),
  cronSpec: yup
    .string()
    .trim()
    .required()
    .max(64)
    .test("cron", cronSpecMessage, isCronSpec)
    .label("model.periodicReportPlan.field.cronSpec"),
  retentionDays: yup
    .number()
    .required()
    .min(1)
    .max(3650)
    .label("model.periodicReportPlan.field.retentionDays"),
  expiresAt: yup.string().nullable().label("model.periodicReportPlan.field.expiresAt"),
});

const createForm = useForm({ validationSchema: planSchema });
const [createName, createNameProps] = createForm.defineField("name", vuetifyCfg);
const [createDescription, createDescriptionProps] = createForm.defineField(
  "description",
  vuetifyCfg,
);
const [createSkillID, createSkillIDProps] = createForm.defineField("skillID", vuetifyCfg);
const [createCronSpec, createCronSpecProps] = createForm.defineField("cronSpec", vuetifyCfg);
const [createRetentionDays, createRetentionDaysProps] = createForm.defineField(
  "retentionDays",
  vuetifyCfg,
);
const [createExpiresAt, createExpiresAtProps] = createForm.defineField("expiresAt", vuetifyCfg);
// createExpiresAtDate 把 DateField 的取值与表单的 ISO 日期文本双向归一；展示由 DateField 负责。
const createExpiresAtDate = computed<string>({
  get: () => isoDateOf(createExpiresAt.value),
  set: (value) => {
    createExpiresAt.value = value;
  },
});
// 输入状态只保留用户实际填写/选择过（或编辑回填）的键，nowTime 一律不进入状态
const createInputValues = ref<Record<string, unknown>>({});
// 固定字段以整段 JSON 文本编辑：初值取技能默认值，编辑模式下已有偏置覆盖默认值
const createFixedText = ref("");

const editForm = useForm({ validationSchema: planSchema });
const [editName, editNameProps] = editForm.defineField("name", vuetifyCfg);
const [editDescription, editDescriptionProps] = editForm.defineField("description", vuetifyCfg);
const [editSkillID, editSkillIDProps] = editForm.defineField("skillID", vuetifyCfg);
const [editCronSpec, editCronSpecProps] = editForm.defineField("cronSpec", vuetifyCfg);
const [editRetentionDays, editRetentionDaysProps] = editForm.defineField(
  "retentionDays",
  vuetifyCfg,
);
const [editExpiresAt, editExpiresAtProps] = editForm.defineField("expiresAt", vuetifyCfg);
// editExpiresAtDate 与创建表单同口径：表单只承载 ISO 日期文本，展示交给 DateField。
const editExpiresAtDate = computed<string>({
  get: () => isoDateOf(editExpiresAt.value),
  set: (value) => {
    editExpiresAt.value = value;
  },
});
const editInputValues = ref<Record<string, unknown>>({});
// 编辑态固定字段同样以技能默认值回填，已有偏置优先
const editFixedText = ref("");
let editPlanID = 0;

// isRuntimeInjectedField nowTime 由系统在运行时注入：name=nowTime 的输入字段不进入计划表单
// （该字段类型固定为 date，nowTime 不再是字段类型）
const isRuntimeInjectedField = (field: PeriodicReportSkillInputField): boolean =>
  field.name === "nowTime";

// planInputFields 过滤运行时注入字段，得到计划表单可渲染的输入字段
const planInputFields = (
  fields: PeriodicReportSkillInputField[],
): PeriodicReportSkillInputField[] => fields.filter((field) => !isRuntimeInjectedField(field));

// hasRuntimeInjectedField 技能是否含 nowTime 字段，用于展示「由系统注入」只读提示
const hasRuntimeInjectedField = (fields: PeriodicReportSkillInputField[]): boolean =>
  fields.some((field) => isRuntimeInjectedField(field));

// textValue 把带类型的值转为文本控件展示值；空值统一展示为空串
const textValue = (value: unknown): string =>
  value === undefined || value === null ? "" : String(value);

// numberValueOf 把控件值转为 JSON 数字；空值或非有限数字返回 undefined（提交时省略该键）
const numberValueOf = (value: string | number | null | undefined): number | undefined => {
  if (value === null || value === undefined) {
    return undefined;
  }
  const text = String(value).trim();
  if (text === "") {
    return undefined;
  }
  const parsed = Number(text);
  return Number.isFinite(parsed) ? parsed : undefined;
};

// inputValueOf 编辑回填时把已有值归一为字段类型：number 转数字、boolean 转布尔、其余转字符串
const inputValueOf = (field: PeriodicReportSkillInputField, value: unknown): unknown => {
  if (value === undefined || value === null) {
    return undefined;
  }
  switch (field.type) {
    case "boolean":
      return typeof value === "boolean" ? value : String(value).toLowerCase() === "true";
    case "number":
      return typeof value === "number" ? value : numberValueOf(String(value));
    default:
      return typeof value === "string" ? value : String(value);
  }
};

// inputStateOf 按当前技能输入字段构造状态：只保留非运行时字段且已有值的键
const inputStateOf = (
  fields: PeriodicReportSkillInputField[],
  existing: Record<string, unknown>,
): Record<string, unknown> => {
  const state: Record<string, unknown> = {};
  for (const field of fields) {
    if (isRuntimeInjectedField(field)) {
      continue;
    }
    const value = inputValueOf(field, existing[field.name]);
    if (value !== undefined) {
      state[field.name] = value;
    }
  }
  return state;
};

// fixedStateOf 固定字段状态：复制技能默认值后以已有偏置覆盖（已有值优先于默认值）
const fixedStateOf = (
  skill: PeriodicReportSkillOption | undefined,
  existing: Record<string, unknown>,
): Record<string, unknown> => {
  const defaults = skill?.fixedFields ?? {};
  const state: Record<string, unknown> = { ...defaults };
  for (const key of Object.keys(defaults)) {
    const value = existing[key];
    if (value !== undefined) {
      state[key] = value;
    }
  }
  return state;
};

// fixedTextOf 固定字段以 2 空格缩进 JSON 编辑，保留对象的嵌套结构
const fixedTextOf = (values: Record<string, unknown>): string => JSON.stringify(values, null, 2);

// parseFixedFields 解析固定字段 JSON 文本：空文本视为无偏置，语法错误或非对象返回 undefined
const parseFixedFields = (text: string): Record<string, unknown> | undefined => {
  const trimmed = text.trim();
  if (trimmed === "") {
    return {};
  }
  try {
    const parsed: unknown = JSON.parse(trimmed);
    if (parsed === null || typeof parsed !== "object" || Array.isArray(parsed)) {
      return undefined;
    }
    return parsed as Record<string, unknown>;
  } catch {
    return undefined;
  }
};

// hasFixedFields 技能是否声明了固定字段；为空时不展示编辑框
const hasFixedFields = (skill: PeriodicReportSkillOption | undefined): boolean =>
  Object.keys(skill?.fixedFields ?? {}).length > 0;

const createSkill = computed<PeriodicReportSkillOption | undefined>(() =>
  skillByID(createSkillID.value),
);
const createInputFields = computed<PeriodicReportSkillInputField[]>(() =>
  planInputFields(createSkill.value?.inputFields ?? []),
);
const createHasNowTime = computed<boolean>(() =>
  hasRuntimeInjectedField(createSkill.value?.inputFields ?? []),
);
const createHasFixedFields = computed<boolean>(() => hasFixedFields(createSkill.value));

const editSkill = computed<PeriodicReportSkillOption | undefined>(() =>
  skillByID(editSkillID.value),
);
const editInputFields = computed<PeriodicReportSkillInputField[]>(() =>
  planInputFields(editSkill.value?.inputFields ?? []),
);
const editHasNowTime = computed<boolean>(() =>
  hasRuntimeInjectedField(editSkill.value?.inputFields ?? []),
);
const editHasFixedFields = computed<boolean>(() => hasFixedFields(editSkill.value));

const setCreateInputValue = (name: string, value: unknown): void => {
  if (value === undefined) {
    delete createInputValues.value[name];
    return;
  }
  createInputValues.value[name] = value;
};

const setEditInputValue = (name: string, value: unknown): void => {
  if (value === undefined) {
    delete editInputValues.value[name];
    return;
  }
  editInputValues.value[name] = value;
};

// 技能切换时整体重置输入状态，并按新技能回填固定字段默认展示
const onCreateSkillChange = (value: unknown): void => {
  const skill = skillByID(value);
  createInputValues.value = inputStateOf(skill?.inputFields ?? [], {});
  createFixedText.value = fixedTextOf(fixedStateOf(skill, {}));
};

const onEditSkillChange = (value: unknown): void => {
  const skill = skillByID(value);
  editInputValues.value = inputStateOf(skill?.inputFields ?? [], {});
  editFixedText.value = fixedTextOf(fixedStateOf(skill, {}));
};

// isInputValueMissing 必填校验：字符串类去空白判空，数字须为有限数字，布尔只要出现即可
const isInputValueMissing = (field: PeriodicReportSkillInputField, value: unknown): boolean => {
  if (value === undefined || value === null) {
    return true;
  }
  switch (field.type) {
    case "number":
      return typeof value !== "number" || !Number.isFinite(value);
    case "string":
    case "enum":
    case "date":
      return String(value).trim() === "";
    default:
      return false;
  }
};

// firstMissingField 返回首个必填但未填写的技能参数（nowTime 由运行时注入，不计入必填）
const firstMissingField = (
  fields: PeriodicReportSkillInputField[],
  values: Record<string, unknown>,
): PeriodicReportSkillInputField | undefined =>
  fields.find(
    (field) =>
      !isRuntimeInjectedField(field) &&
      field.required &&
      isInputValueMissing(field, values[field.name]),
  );

// buildInputValues 构造提交体：只含填写过的输入字段，以及改动过（与技能默认 JSON 值不同）的
// 固定字段；nowTime 永不提交
const buildInputValues = (
  fields: PeriodicReportSkillInputField[],
  inputValues: Record<string, unknown>,
  skill: PeriodicReportSkillOption | undefined,
  fixedValues: Record<string, unknown>,
): Record<string, unknown> => {
  const values: Record<string, unknown> = {};
  for (const field of fields) {
    if (isRuntimeInjectedField(field)) {
      continue;
    }
    const value = inputValues[field.name];
    if (value === undefined || value === null) {
      continue;
    }
    if (typeof value === "string" && value.trim() === "") {
      continue;
    }
    values[field.name] = value;
  }
  const defaults = skill?.fixedFields ?? {};
  for (const key of Object.keys(defaults)) {
    const value = fixedValues[key];
    if (value === undefined) {
      continue;
    }
    if (JSON.stringify(value) === JSON.stringify(defaults[key])) {
      continue;
    }
    values[key] = value;
  }
  return values;
};

// planPayload 由表单值构造提交体：单行文本去空白，日期转 ISO 时刻，空日期表示长期有效
const planPayload = (
  values: Record<string, unknown>,
  inputValues: Record<string, unknown>,
): PeriodicReportPlanCreateIn => {
  const expiresAt = String(values.expiresAt ?? "").trim();
  return {
    name: String(values.name).trim(),
    description: String(values.description ?? "").trim(),
    skillID: Number(values.skillID),
    cronSpec: String(values.cronSpec).trim(),
    inputValues: { ...inputValues },
    retentionDays: Number(values.retentionDays),
    expiresAt: expiresAt ? DateTime.fromISO(expiresAt).toISO() : null,
  };
};

// ---- 创建对话框 ----
const createDialogRef = useTemplateRef("createDialogRef");
const editDialogRef = useTemplateRef("editDialogRef");
const deleteConfirmRef = useTemplateRef("deleteConfirmRef");
const runConfirmRef = useTemplateRef("runConfirmRef");
const runDeleteConfirmRef = useTemplateRef("runDeleteConfirmRef");

const openCreateDialogAction = (): void => {
  // 复位与默认值同属「准备本次打开的数据」，统一用原子 API resetForm
  createForm.resetForm({
    values: {
      name: "",
      description: "",
      skillID: undefined,
      cronSpec: "",
      retentionDays: 90,
      expiresAt: "",
    },
  });
  createInputValues.value = {};
  createFixedText.value = "";
  createDialogRef.value?.open({});
};

const createAction = createForm.handleSubmit(async (values) => {
  const skill = skillByID(values.skillID);
  const fields = skill?.inputFields ?? [];
  const missing = firstMissingField(fields, createInputValues.value);
  if (missing) {
    notifyError(i18n.global.t("validation.mixed.required", { path: missing.name }));
    return;
  }
  const fixedValues = parseFixedFields(createFixedText.value);
  if (!fixedValues) {
    notifyError(i18n.global.t("page.periodicReportPlan.fixedFieldsInvalid"));
    return;
  }
  await getSimpleResponse(
    periodicReportPlanCreate(
      planPayload(
        values as Record<string, unknown>,
        buildInputValues(fields, createInputValues.value, skill, fixedValues),
      ),
    ),
    async (rsp) => {
      await searchTableRef.value?.refresh();
      notifySuccess(rsp.summary);
    },
    async (rsp) => notifyError(rsp.summary),
  );
});

// ---- 编辑对话框 ----
const openEditDialogAction = (item: PeriodicReportPlanItem): void => {
  editPlanID = item.id;
  editForm.resetForm({
    values: {
      name: item.name,
      description: item.description ?? "",
      skillID: item.skillID,
      cronSpec: item.cronSpec,
      retentionDays: item.retentionDays,
      expiresAt: item.expiresAt ? (DateTime.fromISO(item.expiresAt).toISODate() ?? "") : "",
    },
  });
  const skill = skillByID(item.skillID);
  const existing = item.inputValues ?? {};
  editInputValues.value = inputStateOf(skill?.inputFields ?? [], existing);
  editFixedText.value = fixedTextOf(fixedStateOf(skill, existing));
  editDialogRef.value?.open(item);
};

const itemUpdateAction = editForm.handleSubmit(async (values) => {
  const skill = skillByID(values.skillID);
  const fields = skill?.inputFields ?? [];
  const missing = firstMissingField(fields, editInputValues.value);
  if (missing) {
    notifyError(i18n.global.t("validation.mixed.required", { path: missing.name }));
    return;
  }
  const fixedValues = parseFixedFields(editFixedText.value);
  if (!fixedValues) {
    notifyError(i18n.global.t("page.periodicReportPlan.fixedFieldsInvalid"));
    return;
  }
  await getSimpleResponse(
    periodicReportPlanUpdate(
      editPlanID,
      planPayload(
        values as Record<string, unknown>,
        buildInputValues(fields, editInputValues.value, skill, fixedValues),
      ),
    ),
    async (rsp) => {
      await searchTableRef.value?.refresh();
      notifySuccess(rsp.summary);
    },
    async (rsp) => notifyError(rsp.summary),
  );
});

// ---- 行操作 ----
// 行载荷为动态结构（投影字段），沿用 SearchTable 的 actions 契约。
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const getActions = (item: Record<string, any>): ItemActions => ({
  edit: () => openEditDialogAction(item as unknown as PeriodicReportPlanItem),
  runNow: () => runConfirmRef.value?.open(item),
  history: () => {
    void openHistory(item as unknown as PeriodicReportPlanItem);
  },
  delete: () => deleteConfirmRef.value?.open(item),
});

const deleteAction = async (item: Record<string, unknown>): Promise<void> => {
  await getSimpleResponse(
    periodicReportPlanDelete(Number(item.id)),
    async (rsp) => {
      await searchTableRef.value?.refresh();
      notifySuccess(rsp.summary);
    },
    async (rsp) => notifyError(rsp.summary),
  );
};

const runAction = async (item: Record<string, unknown>): Promise<void> => {
  await getSimpleResponse(
    periodicReportPlanRun(Number(item.id)),
    async (rsp) => {
      await searchTableRef.value?.refresh();
      notifySuccess(rsp.summary);
    },
    async (rsp) => notifyError(rsp.summary),
  );
};

// ---- 历史报告 ----
const historyDialog = ref(false);
const historyPlanID = ref(0);
const historyPlanName = ref("");
const runs = ref<PeriodicReportRunItem[]>([]);
const runsLoading = ref(false);

const runHeaders = ref([
  {
    key: "status",
    title: i18n.global.t("model.periodicReportRun.field.status"),
    type: BackendDataType.STRING,
    align: "start",
  },
  {
    key: "title",
    title: i18n.global.t("model.periodicReportRun.field.title"),
    type: BackendDataType.STRING,
    align: "start",
  },
  {
    key: "triggerAt",
    title: i18n.global.t("model.periodicReportRun.field.triggerAt"),
    type: BackendDataType.DATETIME,
    align: "start",
  },
  {
    key: "createdAt",
    title: i18n.global.t("model.periodicReportRun.field.createdAt"),
    type: BackendDataType.DATETIME,
    align: "start",
  },
  {
    key: "sizeBytes",
    title: i18n.global.t("model.periodicReportRun.field.sizeBytes"),
    type: BackendDataType.DECIMAL_BYTE,
    align: "start",
  },
  {
    key: "error",
    title: i18n.global.t("model.periodicReportRun.field.error"),
    type: BackendDataType.STRING,
    align: "start",
  },
  {
    key: "actions",
    title: i18n.global.t("model.__base.field.__actions"),
    type: BackendDataType.STRING,
    align: "center",
  },
] as Array<TableHeader>);

const runStatusMeta = (status: string): { icon: string; color: string; spin: boolean } => {
  switch (status) {
    case "success":
      return { icon: "mdi-check-circle", color: "success", spin: false };
    case "failed":
      return { icon: "mdi-alert-circle", color: "error", spin: false };
    case "running":
      return { icon: "mdi-loading", color: "primary", spin: true };
    default:
      return { icon: "mdi-help-circle", color: "info", spin: false };
  }
};

const loadRuns = async (planID: number): Promise<void> => {
  runsLoading.value = true;
  await getResponse<PeriodicReportRunListOut>(
    periodicReportRunListGet({
      greedy: false,
      page: 1,
      pageSize: RUN_PAGE_SIZE,
      orderBy: [{ name: "createdAt", order: -1 }],
      filterBy: { planID: { action: SearchAction.EQUAL, value: planID } },
    }),
    async (rsp) => {
      runs.value = rsp.data.items ?? [];
    },
    async (rsp) => notifyError(rsp.summary),
    undefined,
    undefined,
    async () => {
      runsLoading.value = false;
    },
  );
};

const openHistory = async (item: PeriodicReportPlanItem): Promise<void> => {
  historyPlanID.value = item.id;
  historyPlanName.value = item.name;
  runs.value = [];
  historyDialog.value = true;
  await loadRuns(item.id);
};

// deleteRunAction 删除一条运行记录（运行中与已结束均可），随后刷新当前计划的运行列表。
const deleteRunAction = async (item: Record<string, unknown>): Promise<void> => {
  await getSimpleResponse(
    periodicReportRunDelete(Number(item.id)),
    async (rsp) => {
      notifySuccess(rsp.summary);
      await loadRuns(historyPlanID.value);
    },
    async (rsp) => notifyError(rsp.summary),
  );
};

// ---- 报告正文 ----
const detailDialog = ref(false);
const detailTitle = ref("");
const detailContent = ref("");
const detailLoading = ref(false);

const openRunDetail = async (runID: number, title?: string): Promise<void> => {
  detailTitle.value = title ?? "";
  detailContent.value = "";
  detailDialog.value = true;
  detailLoading.value = true;
  await getResponse<PeriodicReportRunDetailOut>(
    periodicReportRunDetailGet(runID),
    async (rsp) => {
      detailTitle.value = rsp.data.title;
      detailContent.value = periodicReportEnvelopeContent(rsp.data.content);
    },
    async (rsp) => {
      detailDialog.value = false;
      notifyError(rsp.summary);
    },
    undefined,
    undefined,
    async () => {
      detailLoading.value = false;
    },
  );
};

// ---- 运行会话（只读审计视图） ----
const sessionViewerOpened = ref(false);
const sessionViewerID = ref(0);
const sessionViewerTitle = ref("");

const onRunRowClick = (_event: Event, data: { item: PeriodicReportRunItem }): void => {
  void openRunDetail(data.item.id, data.item.title);
};

// viewRunSession 打开该次运行的只读会话视图（运行已冻结，仅用于回顾审计，不可继续对话）。
// 历史报告是模态对话框，须先关闭它，避免两层对话框叠压。
const viewRunSession = (item: { sessionID?: number; title?: string }): void => {
  const sessionID = Number(item.sessionID ?? 0);
  if (sessionID <= 0) {
    return;
  }
  historyDialog.value = false;
  sessionViewerID.value = sessionID;
  sessionViewerTitle.value = item.title ?? "";
  sessionViewerOpened.value = true;
};

onMounted(async () => {
  void ensureEnum(EnumClass.periodicReportRunStatus);
  // 列表首屏由 SearchTable 在 OPTIONS 就绪后自动查询；此处仅预取表单所需的技能选项
  await loadSkillOptions();
});
</script>

<style scoped>
.history-row {
  cursor: pointer;
}

.history-error {
  display: inline-block;
  max-width: var(--zk-container-max-width-narrow);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: middle;
}
</style>
