<template>
  <main-layout current-nav="tag-event">
    <template #body>
      <search-table
        ref="searchTableRef"
        :list-request="tagEventRuleListGet"
        :options-request="tagEventRuleListOptions"
        :actions="getActions"
        :batch-actions="getBatchActions"
        :create="openCreateDialogAction"
        :project-fields="projectFields"
        model-name="tagEventRule"
      >
        <!-- 启用态：开关即时写入（乐观更新，失败回退） -->
        <template #cell-enabled="{ item }">
          <enabled-switch-cell
            v-model="item.enabled"
            :request="(enabled: boolean) => tagEventRuleSetEnabled(item.id, enabled)"
          />
        </template>
      </search-table>

      <confirm-dialog
        ref="deleteConfirmRef"
        :title="$t('model.tagEventRule.name')"
        prepend-icon="mdi-tag-multiple"
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
              $t(`notice.deleteConfirm`, {
                category: $t(`model.tagEventRule.name`),
                key: proxy.name,
              })
            "
          />
        </template>
      </confirm-dialog>

      <confirm-dialog
        ref="toggleConfirmRef"
        :title="$t('model.tagEventRule.name')"
        prepend-icon="mdi-toggle-switch"
        @confirm="itemToggleAction"
      >
        <template #body>
          <confirm-alert
            v-if="batchState"
            :action="
              $t(batchState?.action === 'activate' ? 'action.activate' : 'action.deactivate')
            "
            :text="$t('notice.batchConfirm', { count: batchState?.items.length ?? 0 })"
          />
        </template>
      </confirm-dialog>

      <temp-editable-dialog
        ref="createDialogRef"
        :title="$t('model.tagEventRule.name')"
        :confirm-disabled="!createForm.meta.value.valid || !createConditionsValid"
        force-editable
        prepend-icon="mdi-tag-multiple"
        @confirm="createAction"
      >
        <template #body>
          <v-form>
            <v-row density="compact">
              <v-col cols="12">
                <v-text-field
                  v-model="createName"
                  v-bind="createNameProps"
                  :label="$t('model.tagEventRule.field.name')"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <v-textarea
                  v-model="createDescription"
                  v-bind="createDescriptionProps"
                  :label="$t('model.tagEventRule.field.description')"
                  rows="3"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <v-select
                  v-model="createNotificationDelivery"
                  :items="notificationOptions"
                  item-title="title"
                  item-value="value"
                  :label="$t('model.tagEventRule.field.notification')"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="6">
                <v-text-field
                  v-model.number="createSuppressWindow"
                  v-bind="createSuppressWindowProps"
                  type="number"
                  :label="$t('model.tagEventRule.field.suppressWindow')"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="6">
                <v-switch
                  v-model="createMatchAll"
                  v-bind="createMatchAllProps"
                  :label="$t('model.tagEventRule.field.matchAll')"
                  color="primary"
                />
              </v-col>
              <v-col cols="6">
                <v-switch
                  v-model="createEnabled"
                  v-bind="createEnabledProps"
                  :label="$t('model.tagEventRule.field.enabled')"
                  color="primary"
                />
              </v-col>
            </v-row>
            <v-divider class="my-2" />
            <v-row density="compact" class="align-center">
              <v-col>
                <v-label>{{ $t("model.tagEventRule.field.conditions") }}</v-label>
              </v-col>
              <v-col cols="auto">
                <v-btn
                  variant="tonal"
                  size="small"
                  prepend-icon="mdi-plus"
                  @click="addCreateCondition"
                >
                  {{ $t("action.addCondition") }}
                </v-btn>
              </v-col>
            </v-row>
            <v-card
              v-for="(cond, idx) in createConditions"
              :key="cond.key"
              variant="tonal"
              class="my-2"
            >
              <v-card-title class="d-flex align-center text-title-small">
                {{ $t("model.tagEventCondition.name") }} #{{ idx + 1 }}
                <v-spacer />
                <v-btn
                  icon="mdi-minus"
                  variant="tonal"
                  color="error"
                  :disabled="createConditions.length === 1"
                  :title="$t('action.removeCondition')"
                  :aria-label="$t('action.removeCondition')"
                  @click="removeCreateCondition(idx)"
                />
              </v-card-title>
              <v-card-text>
                <v-row density="compact">
                  <v-col cols="6">
                    <v-text-field
                      v-model="cond.name"
                      :label="$t('model.tagEventCondition.field.name')"
                      :error-messages="cond.touched ? cond.fieldErrors.name : ''"
                      @update:model-value="markTouched(cond)"
                    />
                  </v-col>
                  <v-col cols="6">
                    <v-select
                      v-model="cond.model"
                      :items="modelOptions"
                      :label="$t('model.tagEventCondition.field.model')"
                      :error-messages="cond.touched ? cond.fieldErrors.model : ''"
                      clearable
                    />
                  </v-col>
                </v-row>
                <v-row density="compact">
                  <v-col cols="6">
                    <v-select
                      :model-value="cond.dimension"
                      :items="dimensionOptions(cond.model)"
                      :label="$t('model.tagEventCondition.field.dimension')"
                      :error-messages="cond.touched ? cond.fieldErrors.dimension : ''"
                      clearable
                      @update:model-value="onDimensionChange(cond, $event)"
                    />
                  </v-col>
                  <v-col cols="6">
                    <v-text-field
                      v-model="cond.item"
                      :label="$t('model.tagEventCondition.field.item')"
                      :error-messages="cond.touched ? cond.fieldErrors.item : ''"
                    />
                  </v-col>
                </v-row>
                <v-row density="compact">
                  <v-col cols="6">
                    <v-text-field
                      v-model.number="cond.window"
                      type="number"
                      :label="$t('model.tagEventCondition.field.window')"
                      :error-messages="cond.touched ? cond.fieldErrors.window : ''"
                      @update:model-value="markTouched(cond)"
                    />
                  </v-col>
                  <v-col cols="6">
                    <v-text-field
                      v-model.number="cond.threshold"
                      type="number"
                      :label="$t('model.tagEventCondition.field.threshold')"
                      :error-messages="cond.touched ? cond.fieldErrors.threshold : ''"
                      @update:model-value="markTouched(cond)"
                    />
                  </v-col>
                </v-row>
                <v-row density="compact">
                  <v-col cols="12">
                    <v-textarea
                      v-model="cond.description"
                      :label="$t('model.tagEventCondition.field.description')"
                      :error-messages="cond.touched ? cond.fieldErrors.description : ''"
                      rows="3"
                    />
                  </v-col>
                </v-row>
                <v-row density="compact">
                  <v-col cols="12">
                    <tag-expr-editor
                      v-model="cond.expression"
                      :tags="tagPool"
                      :valid="cond.touched && cond.localValid && !cond.fieldErrors.expression"
                      :error-message="
                        cond.touched ? cond.localError || cond.fieldErrors.expression : ''
                      "
                      :min-height="'var(--zk-editor-min-height)'"
                      @update:model-value="markTouched(cond)"
                    />
                  </v-col>
                </v-row>
                <v-row density="compact" class="align-center">
                  <v-col cols="auto">
                    <v-btn
                      v-request="
                        async () => {
                          await validateConditionExpression(cond);
                        }
                      "
                      variant="tonal"
                      size="small"
                    >
                      {{ $t("action.validate") }}
                    </v-btn>
                  </v-col>
                </v-row>
                <v-switch
                  v-model="cond.enabled"
                  :label="$t('model.tagEventCondition.field.enabled')"
                  color="primary"
                  density="compact"
                  hide-details
                />
              </v-card-text>
            </v-card>
            <v-divider class="my-2" />
            <v-row density="compact" class="align-center">
              <v-col cols="auto">
                <v-btn
                  color="primary"
                  size="small"
                  :loading="createDryRunRef?.loading"
                  @click="createDryRunRef?.run()"
                >
                  {{ $t("action.dryRun") }}
                </v-btn>
              </v-col>
            </v-row>
            <tag-event-rule-dry-run-panel
              ref="createDryRunRef"
              :rule-name="createName"
              :match-all="createMatchAll"
              :conditions="createDryRunConditions"
            />
          </v-form>
        </template>
      </temp-editable-dialog>

      <temp-editable-dialog
        ref="editDialogRef"
        :title="$t('model.tagEventRule.name')"
        :confirm-disabled="!updateForm.meta.value.valid || !editConditionsValid"
        prepend-icon="mdi-tag-multiple"
        @confirm="itemUpdateAction"
      >
        <template #body="{ proxy, editable }">
          <v-form>
            <v-row density="compact">
              <v-col cols="12">
                <v-text-field
                  :model-value="proxy.name"
                  :label="$t('model.tagEventRule.field.name')"
                  disabled
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <v-textarea
                  v-model="editDescription"
                  v-bind="editDescriptionProps"
                  :label="$t('model.tagEventRule.field.description')"
                  :disabled="!editable"
                  rows="3"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <v-select
                  v-model="editNotificationDelivery"
                  :items="notificationOptions"
                  item-title="title"
                  item-value="value"
                  :label="$t('model.tagEventRule.field.notification')"
                  :disabled="!editable"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="6">
                <v-switch
                  v-model="editMatchAll"
                  v-bind="editMatchAllProps"
                  :label="$t('model.tagEventRule.field.matchAll')"
                  color="primary"
                  :disabled="!editable"
                />
              </v-col>
              <v-col cols="6">
                <v-switch
                  v-model="editEnabled"
                  v-bind="editEnabledProps"
                  :label="$t('model.tagEventRule.field.enabled')"
                  color="primary"
                  :disabled="!editable"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="6">
                <v-text-field
                  v-model.number="editSuppressWindow"
                  v-bind="editSuppressWindowProps"
                  type="number"
                  :label="$t('model.tagEventRule.field.suppressWindow')"
                  :disabled="!editable"
                />
              </v-col>
            </v-row>
            <v-divider class="my-2" />
            <v-row density="compact" class="align-center">
              <v-col>
                <v-label>{{ $t("model.tagEventRule.field.conditions") }}</v-label>
              </v-col>
              <v-col cols="auto">
                <v-btn
                  variant="tonal"
                  size="small"
                  prepend-icon="mdi-plus"
                  :disabled="!editable"
                  @click="addEditCondition"
                >
                  {{ $t("action.addCondition") }}
                </v-btn>
              </v-col>
            </v-row>
            <v-card
              v-for="(cond, idx) in editConditions"
              :key="cond.key"
              variant="tonal"
              class="my-2"
            >
              <v-card-title class="d-flex align-center text-title-small">
                {{ $t("model.tagEventCondition.name") }} #{{ idx + 1 }}
                <v-spacer />
                <v-btn
                  icon="mdi-minus"
                  variant="tonal"
                  color="error"
                  :disabled="!editable || editConditions.length === 1"
                  :title="$t('action.removeCondition')"
                  :aria-label="$t('action.removeCondition')"
                  @click="removeEditCondition(idx)"
                />
              </v-card-title>
              <v-card-text>
                <v-row density="compact">
                  <v-col cols="6">
                    <v-text-field
                      v-model="cond.name"
                      :label="$t('model.tagEventCondition.field.name')"
                      :disabled="!editable"
                      :error-messages="cond.touched ? cond.fieldErrors.name : ''"
                      @update:model-value="markTouched(cond)"
                    />
                  </v-col>
                  <v-col cols="6">
                    <v-select
                      v-model="cond.model"
                      :items="modelOptions"
                      :label="$t('model.tagEventCondition.field.model')"
                      :disabled="!editable"
                      :error-messages="cond.touched ? cond.fieldErrors.model : ''"
                      clearable
                    />
                  </v-col>
                </v-row>
                <v-row density="compact">
                  <v-col cols="6">
                    <v-select
                      :model-value="cond.dimension"
                      :items="dimensionOptions(cond.model)"
                      :label="$t('model.tagEventCondition.field.dimension')"
                      :disabled="!editable"
                      :error-messages="cond.touched ? cond.fieldErrors.dimension : ''"
                      clearable
                      @update:model-value="onDimensionChange(cond, $event)"
                    />
                  </v-col>
                  <v-col cols="6">
                    <v-text-field
                      v-model="cond.item"
                      :label="$t('model.tagEventCondition.field.item')"
                      :disabled="!editable"
                      :error-messages="cond.touched ? cond.fieldErrors.item : ''"
                    />
                  </v-col>
                </v-row>
                <v-row density="compact">
                  <v-col cols="6">
                    <v-text-field
                      v-model.number="cond.window"
                      type="number"
                      :label="$t('model.tagEventCondition.field.window')"
                      :disabled="!editable"
                      :error-messages="cond.touched ? cond.fieldErrors.window : ''"
                      @update:model-value="markTouched(cond)"
                    />
                  </v-col>
                  <v-col cols="6">
                    <v-text-field
                      v-model.number="cond.threshold"
                      type="number"
                      :label="$t('model.tagEventCondition.field.threshold')"
                      :disabled="!editable"
                      :error-messages="cond.touched ? cond.fieldErrors.threshold : ''"
                      @update:model-value="markTouched(cond)"
                    />
                  </v-col>
                </v-row>
                <v-row density="compact">
                  <v-col cols="12">
                    <v-textarea
                      v-model="cond.description"
                      :label="$t('model.tagEventCondition.field.description')"
                      :disabled="!editable"
                      :error-messages="cond.touched ? cond.fieldErrors.description : ''"
                      rows="3"
                    />
                  </v-col>
                </v-row>
                <v-row density="compact">
                  <v-col cols="12">
                    <tag-expr-editor
                      v-model="cond.expression"
                      :tags="tagPool"
                      :valid="cond.touched && cond.localValid && !cond.fieldErrors.expression"
                      :error-message="
                        cond.touched ? cond.localError || cond.fieldErrors.expression : ''
                      "
                      :disabled="!editable"
                      :min-height="'var(--zk-editor-min-height)'"
                      @update:model-value="markTouched(cond)"
                    />
                  </v-col>
                </v-row>
                <v-row density="compact" class="align-center">
                  <v-col cols="auto">
                    <v-btn
                      variant="tonal"
                      size="small"
                      :disabled="!editable"
                      :loading="cond.validating"
                      @click="validateConditionExpression(cond)"
                    >
                      {{ $t("action.validate") }}
                    </v-btn>
                  </v-col>
                </v-row>
                <v-switch
                  v-model="cond.enabled"
                  :label="$t('model.tagEventCondition.field.enabled')"
                  color="primary"
                  density="compact"
                  hide-details
                  :disabled="!editable"
                />
              </v-card-text>
            </v-card>
            <v-divider class="my-2" />
            <v-row density="compact" class="align-center">
              <v-col cols="auto">
                <v-btn
                  v-request="
                    async () => {
                      await editDryRunRef?.run();
                    }
                  "
                  color="primary"
                  size="small"
                  :disabled="!editable"
                >
                  {{ $t("action.dryRun") }}
                </v-btn>
              </v-col>
            </v-row>
            <tag-event-rule-dry-run-panel
              ref="editDryRunRef"
              :rule-name="proxy.name"
              :match-all="editMatchAll"
              :conditions="editDryRunConditions"
            />
          </v-form>
        </template>
      </temp-editable-dialog>

      <app-dialog v-model="versionDialog" :title="$t('action.version')" size="md">
        <v-alert v-if="versionItems.length === 0" type="info" :text="$t('notice.noVersion')" />
        <v-list v-else>
          <v-list-item v-for="v in versionItems" :key="v.id">
            <template #title>
              <v-chip class="me-2">v{{ v.version }}</v-chip>
              {{ v.name }}
            </template>
            <template #subtitle>
              {{ localeDatetimeFormat(DateTime.fromISO(v.createdAt)) }}
            </template>
            <template #append>
              <v-btn
                variant="tonal"
                size="small"
                class="me-2"
                @click="expandedVersion = expandedVersion === v.version ? null : v.version"
              >
                {{ $t("action.expand") }}
              </v-btn>
              <v-btn variant="tonal" size="small" color="error" @click="openRollbackConfirm(v)">
                {{ $t("action.rollback") }}
              </v-btn>
            </template>
            <v-expand-transition>
              <v-list v-if="expandedVersion === v.version" class="zk-code-preview text-body-medium">
                <v-list-item
                  v-for="c in v.conditions"
                  :key="c.id"
                  :title="`${c.name} (${c.expression})`"
                  :subtitle="`${$t('model.tagEventCondition.field.window')}: ${localDurationFormat(c.window)}`"
                />
              </v-list>
            </v-expand-transition>
          </v-list-item>
        </v-list>
        <template #footer>
          <v-btn variant="text" @click="versionDialog = false">
            {{ $t("action.close") }}
          </v-btn>
        </template>
      </app-dialog>

      <confirm-dialog
        ref="rollbackConfirmRef"
        :title="$t('action.rollback')"
        prepend-icon="mdi-history"
        @confirm="itemRollbackAction"
      >
        <template #body="{ proxy }">
          <confirm-alert
            :action="$t('action.rollback')"
            :text="
              $t(`notice.rollbackConfirm`, {
                category: $t(`model.tagEventRule.name`),
                key: proxy.name,
                version: proxy.version,
              })
            "
          />
        </template>
      </confirm-dialog>
    </template>
  </main-layout>
</template>

<route lang="yaml">
name: Tag Event Rule List
meta:
  title: title.tagEventRuleTagEvent
  requiresAuth: true
  requiresPerm: true
</route>

<script setup lang="ts">
import { reactive } from "vue";
import { DateTime } from "luxon";
import * as yup from "yup";

import {
  tagEventRuleCreate,
  tagEventRuleDelete,
  tagEventRuleDetailGet,
  tagEventRuleListGet,
  tagEventRuleListOptions,
  tagEventRuleSetEnabled,
  tagEventRuleUpdate,
  tagEventRuleValidate,
  tagEventRuleVersionList,
  tagEventRuleVersionRollback,
  tagRuleDetailGet,
  tagRuleListGet,
  tagRuleModelMetadata,
} from "@/apis/tagEvent";
import { notificationGroupListGet, notificationTargetListGet } from "@/apis/notification";
import type {
  NotificationGroupListOut,
  NotificationTargetListOut,
} from "@/apis/@types/notification";
import type { BatchResultOut, TagEventRuleConditionIn } from "@/apis/@types/admin";
import { getResponse, getSimpleResponse } from "@/apis/base";
import type { BatchActions, ItemActions } from "@/components/@types/table";
import { useAppStore } from "@/stores/app";
import {
  createTagEventRuleSchema,
  tagEventConditionSchema,
  updateTagEventRuleSchema,
} from "@/schemas/tagEventRule";
import { parseBooleanExpr } from "@/utils/booleanExpr";
import { durationToMinutes, minutesToDurationString } from "@/utils/duration";
import { vuetifyCfg } from "@/utils/form";
import i18n, { localeDatetimeFormat, localDurationFormat } from "@/utils/i18n";
import { EnumClass, enumLabelOf, enumOptionsOf, ensureEnum } from "@/utils/enums";
import { extractTagLiterals } from "@/utils/rego";
import { useForm } from "vee-validate";

import type {
  TagEventRuleDetailOut,
  TagEventRuleValidateOut,
  TagEventRuleVersionOut,
  TagRuleDetailOut,
  TagRuleListOut,
  TagRuleModelMetadataOut,
} from "./@types";

interface ConditionDraft {
  key: number;
  name: string;
  model: string;
  dimension: string;
  item: string;
  /** 抑制窗口，单位为分钟（提交时转换为 duration 字符串） */
  window: number;
  threshold: number | null;
  expression: string;
  enabled: boolean;
  description: string;
  localValid: boolean;
  localError: string;
  backendValid: boolean | null;
  validating: boolean;
  /** 用户是否已与条件交互或尝试提交；为 false 时不展示字段错误，避免空表单一进来就标红 */
  touched: boolean;
  /** 各字段的 yup 校验错误信息（expression 除外，由 localError 独立展示） */
  fieldErrors: Record<string, string>;
}

const appStore = useAppStore();
const searchTableRef = useTemplateRef("searchTableRef");
const createDialogRef = useTemplateRef("createDialogRef");
const editDialogRef = useTemplateRef("editDialogRef");
const createDryRunRef = useTemplateRef("createDryRunRef");
const editDryRunRef = useTemplateRef("editDryRunRef");
const deleteConfirmRef = useTemplateRef("deleteConfirmRef");
const toggleConfirmRef = useTemplateRef("toggleConfirmRef");
const rollbackConfirmRef = useTemplateRef("rollbackConfirmRef");

// 模型下拉选项：统一经 tagRuleModel 枚举类从枚举 store 解析（value 为完整类型键，
// title 为按当前语言翻译的 label）
const modelOptions = computed(() => enumOptionsOf(EnumClass.tagRuleModel));

// 页面依赖 tagRuleModel 枚举类（模型下拉选项）；仅需确保缓存，store 响应式驱动下拉刷新
onMounted(() => {
  void ensureEnum(EnumClass.tagRuleModel);
});

let condKeySeq = 1;

const newCondition = (): ConditionDraft =>
  reactive({
    key: condKeySeq++,
    name: "",
    model: "",
    dimension: "",
    item: "",
    window: 5,
    threshold: null,
    expression: "",
    enabled: true,
    description: "",
    localValid: false,
    localError: "",
    backendValid: null,
    validating: false,
    touched: false,
    fieldErrors: {},
  });

// 用户编辑过字段或触发校验后，条件进入 touched 状态，错误才展示
const markTouched = (cond: ConditionDraft): void => {
  cond.touched = true;
};

// 提交失败时将所有条件标记为 touched，使校验错误可见
const markConditionsTouched = (list: ConditionDraft[]): void => {
  list.forEach((c) => {
    c.touched = true;
  });
};

// ---- dimension 下拉可选项：目标模型元信息中的合法维度（与 tagengine 注册表/物化视图一致）----
const modelMetaCache = reactive<Record<string, TagRuleModelMetadataOut>>({});

// 按模型拉取元信息并缓存（一次拉取后复用），模型为空或已缓存时跳过
const loadModelMetadata = async (model: string): Promise<void> => {
  if (!model || modelMetaCache[model]) {
    return;
  }
  await getResponse<TagRuleModelMetadataOut>(
    tagRuleModelMetadata({ model }),
    async (rsp) => {
      modelMetaCache[model] = rsp.data;
    },
    async () => {
      modelMetaCache[model] = { model, fields: [], dimensions: [] };
    },
  );
};

/** dimension 下拉选项：后端登记的目标模型合法标签维度（选项值仍按模型从元信息接口取，
 * 展示名统一经 tagEventDimension 枚举类按 enum.tagEventDimension.<取值> 翻译，
 * 与标签事件记录详情的维度列、通知模板一致）。 */
const dimensionOptions = (model: string): Array<{ title: string; value: string }> =>
  (modelMetaCache[model]?.dimensions ?? []).map((d) => ({
    title: enumLabelOf(EnumClass.tagEventDimension, d.value),
    value: d.value,
  }));

// 清除下拉时 v-select 回传 null，统一落回空串（空串表示维度不限制，符合后端语义）
const onDimensionChange = (cond: ConditionDraft, value: string | null): void => {
  cond.dimension = value ?? "";
  markTouched(cond);
};

// ---- 通知方式（规则绑定通知组/通知目标）----
// 命名空间默认通知组（__default__）与直投目标互斥，二者至多由一个编码字符串决定：
//   __default__ ：不绑定（后端使用命名空间默认组）
//   group:<id>  ：绑定指定通知组
//   target:<id> ：绑定指定通知目标（优先级高于通知组）
const DEFAULT_DELIVERY = "__default__";

const notificationGroupItems = ref<NotificationGroupListOut["items"]>([]);
const notificationTargetItems = ref<NotificationTargetListOut["items"]>([]);

// 通知组/通知目标候选列表（下拉数据源），从通知管理模块列表接口装载
const loadNotificationOptions = async (): Promise<void> => {
  await getResponse<NotificationGroupListOut>(
    notificationGroupListGet({ greedy: false, page: 1, pageSize: 100 }),
    async (rsp) => {
      notificationGroupItems.value = rsp.data.items ?? [];
    },
    async () => {
      notificationGroupItems.value = [];
    },
  );
  await getResponse<NotificationTargetListOut>(
    notificationTargetListGet({ greedy: false, page: 1, pageSize: 100 }),
    async (rsp) => {
      notificationTargetItems.value = rsp.data.items ?? [];
    },
    async () => {
      notificationTargetItems.value = [];
    },
  );
};

interface NotificationDeliveryOption {
  title: string;
  value: string;
}

// 下拉选项：默认组 + 各通知组 + 各通知目标（标题带类型前缀加以区分）
const notificationOptions = computed<NotificationDeliveryOption[]>(() => [
  {
    title: i18n.global.t("model.tagEventRule.content.notificationDefault"),
    value: DEFAULT_DELIVERY,
  },
  ...notificationGroupItems.value.map((g) => ({
    title: `${i18n.global.t("model.tagEventRule.content.notificationGroup")} · ${g.name}`,
    value: `group:${g.id}`,
  })),
  ...notificationTargetItems.value.map((t) => ({
    title: `${i18n.global.t("model.tagEventRule.content.notificationTarget")} · ${t.name}`,
    value: `target:${t.id}`,
  })),
]);

// 将通知方式编码字符串解为提交入参的绑定字段；默认组时两者均缺省
const deliveryToPayload = (
  v: string,
): { notificationGroupID?: number; notificationTargetID?: number } => {
  if (v.startsWith("group:")) {
    return { notificationGroupID: Number(v.slice("group:".length)) };
  }
  if (v.startsWith("target:")) {
    return { notificationTargetID: Number(v.slice("target:".length)) };
  }
  return {};
};

const createNotificationDelivery = ref<string>(DEFAULT_DELIVERY);
const editNotificationDelivery = ref<string>(DEFAULT_DELIVERY);

// 用 tagEventConditionSchema 校验条件字段，将各字段错误信息写入 cond.fieldErrors。
// expression 同样纳入 fieldErrors（required/max 约束），编辑器展示时优先取语法错误
// localError，其次回退到 schema 错误。
const validateConditionFields = (cond: ConditionDraft): void => {
  const errors: Record<string, string> = {};
  try {
    // 校验对象浅拷贝，避免 yup 的默认 cast 副作用污染响应式草稿
    tagEventConditionSchema().validateSync({ ...cond }, { abortEarly: false });
  } catch (err) {
    if (err instanceof yup.ValidationError && err.inner) {
      for (const e of err.inner) {
        if (e.path && !errors[e.path]) {
          errors[e.path] = e.message;
        }
      }
    }
  }
  cond.fieldErrors = errors;
};

// 条件数据变化时（含加载编辑数据）实时校验字段约束；
// 含 expression 以同步其 required/max 约束（语法由 watchConditionExpression 负责）
const watchConditionFields = (cond: ConditionDraft): void => {
  watch(
    () => [
      cond.name,
      cond.model,
      cond.dimension,
      cond.item,
      cond.window,
      cond.threshold,
      cond.enabled,
      cond.description,
      cond.expression,
    ],
    () => validateConditionFields(cond),
    { immediate: true },
  );
  // 模型变化（或编辑加载时 immediate）→ 拉取该模型的元信息（dimension 可选项）
  watch(
    () => cond.model,
    (model) => {
      void loadModelMetadata(model);
    },
    { immediate: true },
  );
};

const toConditionIn = (c: ConditionDraft): TagEventRuleConditionIn => ({
  // 单行文本字段统一去除首尾空白，避免误输入空白导致查询失败或数据异常
  // （expression 为表达式、description 为多行文本域，空白具有语义，豁免 trim）
  name: c.name.trim(),
  model: c.model?.trim(),
  dimension: c.dimension?.trim(),
  item: c.item?.trim(),
  window: minutesToDurationString(c.window),
  threshold: c.threshold ?? 0,
  expression: c.expression,
  enabled: c.enabled,
  description: c.description,
});

// ---- 表单 ----
const createForm = useForm({ validationSchema: createTagEventRuleSchema });
const updateForm = useForm({ validationSchema: updateTagEventRuleSchema });

const [createName, createNameProps] = createForm.defineField("name", vuetifyCfg);
const [createDescription, createDescriptionProps] = createForm.defineField(
  "description",
  vuetifyCfg,
);
const [createMatchAll, createMatchAllProps] = createForm.defineField("matchAll", vuetifyCfg);
const [createSuppressWindow, createSuppressWindowProps] = createForm.defineField(
  "suppressWindow",
  vuetifyCfg,
);
const [createEnabled, createEnabledProps] = createForm.defineField("enabled", vuetifyCfg);

const [editDescription, editDescriptionProps] = updateForm.defineField("description", vuetifyCfg);
const [editMatchAll, editMatchAllProps] = updateForm.defineField("matchAll", vuetifyCfg);
const [editSuppressWindow, editSuppressWindowProps] = updateForm.defineField(
  "suppressWindow",
  vuetifyCfg,
);
const [editEnabled, editEnabledProps] = updateForm.defineField("enabled", vuetifyCfg);

// ---- 标签池（启发式扩展来源） ----
const tagPool = ref<string[]>([]);
const tagPoolLoading = ref(false);
const tagPoolLoaded = ref(false);

const loadTagPool = async (): Promise<void> => {
  if (tagPoolLoaded.value) {
    return;
  }
  tagPoolLoading.value = true;
  try {
    await getResponse<{ items: TagRuleListOut[] }>(
      tagRuleListGet({ greedy: false, page: 1, pageSize: 100 }),
      async (rsp) => {
        const enabledRules = rsp.data.items.filter((r) => r.enabled);
        const tags = new Set<string>();
        for (const rule of enabledRules) {
          await getResponse<TagRuleDetailOut>(tagRuleDetailGet(rule.id), async (r2) => {
            for (const t of extractTagLiterals(r2.data.rego)) {
              tags.add(t);
            }
          });
        }
        tagPool.value = [...tags].sort();
        tagPoolLoaded.value = true;
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
    tagPoolLoading.value = false;
  }
};

const validateConditionExpression = async (cond: ConditionDraft): Promise<void> => {
  cond.validating = true;
  // 主动触发校验也视为已交互，校验结果（含错误）立即展示
  markTouched(cond);
  try {
    await getResponse<TagEventRuleValidateOut>(
      tagEventRuleValidate({ expression: cond.expression }),
      async (rsp) => {
        if (rsp.data.valid) {
          appStore.notify({
            text: i18n.global.t("action.validatePassed"),
            timeout: 3000,
            color: "success",
          });
          cond.backendValid = true;
          if (rsp.data.refined) {
            cond.expression = rsp.data.refined;
          }
          const p = parseBooleanExpr(cond.expression);
          cond.localValid = p.ok;
          cond.localError = p.ok ? "" : (p.error ?? "");
        } else {
          cond.backendValid = false;
          cond.localValid = false;
          cond.localError = rsp.data.error || "invalid expression";
        }
      },
      async () => {
        cond.backendValid = null;
      },
    );
  } finally {
    cond.validating = false;
  }
};

const watchConditionExpression = (cond: ConditionDraft): void => {
  watch(
    () => cond.expression,
    (val) => {
      const p = parseBooleanExpr(val);
      cond.localValid = p.ok;
      cond.localError = p.ok ? "" : (p.error ?? "");
    },
    { immediate: true },
  );
};

// ---- 创建对话框 ----
const createConditions = ref<ConditionDraft[]>([]);

const addCreateCondition = (): void => {
  const cond = newCondition();
  watchConditionExpression(cond);
  watchConditionFields(cond);
  createConditions.value.push(cond);
};

const removeCreateCondition = (idx: number): void => {
  createConditions.value.splice(idx, 1);
};

const createConditionsValid = computed<boolean>(
  () =>
    createConditions.value.length > 0 &&
    createConditions.value.every((c) => Object.keys(c.fieldErrors).length === 0 && c.localValid),
);

const openCreateDialogAction = (): void => {
  // 复位用原子 API resetForm：完整枚举全部 defineField 字段，值 + 校验状态一次性重置
  createForm.resetForm({
    values: {
      name: "",
      description: "",
      matchAll: false,
      suppressWindow: 60,
      enabled: true,
    },
  });
  createNotificationDelivery.value = DEFAULT_DELIVERY;
  void loadNotificationOptions();
  createConditions.value = [];
  addCreateCondition();
  void loadTagPool();
  createDryRunRef.value?.reset();
  createDialogRef.value?.open({});
};

const createAction = createForm.handleSubmit(async (values) => {
  if (!createConditionsValid.value) {
    // 提交失败：展示各条件字段的校验错误
    markConditionsTouched(createConditions.value);
    appStore.notify({
      text: i18n.global.t("notice.expressionInvalid"),
      timeout: 3000,
      color: "error",
    });
    return;
  }
  await getSimpleResponse(
    tagEventRuleCreate({
      // 名称单行文本提交前去除首尾空白，避免误输入空白创建出看似相同实则不同的记录
      name: (values.name as string).trim(),
      description: values.description,
      matchAll: values.matchAll,
      suppressWindow: values.suppressWindow * 60e9,
      enabled: values.enabled,
      conditions: createConditions.value.map(toConditionIn),
      ...deliveryToPayload(createNotificationDelivery.value),
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
let editRuleID = 0;
const editConditions = ref<ConditionDraft[]>([]);

const addEditCondition = (): void => {
  const cond = newCondition();
  watchConditionExpression(cond);
  watchConditionFields(cond);
  editConditions.value.push(cond);
};

const removeEditCondition = (idx: number): void => {
  editConditions.value.splice(idx, 1);
};

const editConditionsValid = computed<boolean>(
  () =>
    editConditions.value.length > 0 &&
    editConditions.value.every((c) => Object.keys(c.fieldErrors).length === 0 && c.localValid),
);

// ---- 试运行入参：直接取自表单当前草稿，随表单编辑实时变化 ----
const createDryRunConditions = computed<TagEventRuleConditionIn[]>(() =>
  createConditions.value.map(toConditionIn),
);
const editDryRunConditions = computed<TagEventRuleConditionIn[]>(() =>
  editConditions.value.map(toConditionIn),
);

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const detailGetAction = async (item: Record<string, any>): Promise<void> => {
  void loadTagPool();
  editDryRunRef.value?.reset();
  await getResponse<TagEventRuleDetailOut>(
    tagEventRuleDetailGet(item.id),
    async (rsp) => {
      editRuleID = rsp.data.id;
      // 复位与回填同属「准备本次打开的数据」：完整枚举 defineField 字段，原子重置值 + 校验状态
      updateForm.resetForm({
        values: {
          description: rsp.data.description,
          matchAll: rsp.data.matchAll,
          suppressWindow: durationToMinutes(rsp.data.suppressWindow),
          enabled: rsp.data.enabled,
        },
      });
      // 回填通知方式：直投目标优先，其次通知组，否则命名空间默认组
      editNotificationDelivery.value =
        rsp.data.notificationTargetID != null
          ? `target:${rsp.data.notificationTargetID}`
          : rsp.data.notificationGroupID != null
            ? `group:${rsp.data.notificationGroupID}`
            : DEFAULT_DELIVERY;
      void loadNotificationOptions();
      editConditions.value = (rsp.data.conditions ?? []).map((c) => {
        const cond = newCondition();
        cond.name = c.name;
        cond.model = c.model;
        cond.dimension = c.dimension;
        cond.item = c.item;
        cond.window = durationToMinutes(c.window);
        cond.threshold = c.threshold;
        cond.expression = c.expression;
        cond.enabled = c.enabled;
        cond.description = c.description;
        watchConditionExpression(cond);
        watchConditionFields(cond);
        return cond;
      });
      if (editConditions.value.length === 0) {
        addEditCondition();
      }
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
    if (!editConditionsValid.value) {
      // 提交失败：展示各条件字段的校验错误
      markConditionsTouched(editConditions.value);
      appStore.notify({
        text: i18n.global.t("notice.expressionInvalid"),
        timeout: 3000,
        color: "error",
      });
      return;
    }
    await getSimpleResponse(
      tagEventRuleUpdate(editRuleID, {
        description: values.description,
        matchAll: values.matchAll,
        suppressWindow: values.suppressWindow * 60e9,
        enabled: values.enabled,
        conditions: editConditions.value.map(toConditionIn),
        ...deliveryToPayload(editNotificationDelivery.value),
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
const projectFields = (visible: string[]): string[] => ["id", "name", "enabled", ...visible];

const getActions = (): ItemActions => {
  const actions: ItemActions = {
    edit: detailGetAction,
    version: versionAction,
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    delete: (i: Record<string, any>) => {
      batchState.value = null;
      deleteConfirmRef.value?.open(i);
    },
  };
  return actions;
};

// ---- 批量删除/启用/禁用 ----
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
type BatchState = { items: Record<string, any>[]; action: "delete" | "activate" | "deactivate" };
const batchState = ref<BatchState | null>(null);

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const getBatchActions = (items: Record<string, any>[]): BatchActions => {
  return {
    delete: () => {
      batchState.value = { items, action: "delete" };
      deleteConfirmRef.value?.open({});
    },
    activate: () => {
      batchState.value = { items, action: "activate" };
      toggleConfirmRef.value?.open({});
    },
    deactivate: () => {
      batchState.value = { items, action: "deactivate" };
      toggleConfirmRef.value?.open({});
    },
  };
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const batchDeleteAction = async (items: Record<string, any>[]): Promise<void> => {
  const ids = items.map((i) => i.id as number);
  await getResponse<BatchResultOut>(
    tagEventRuleDelete(ids),
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
const batchToggleAction = async (items: Record<string, any>[], enabled: boolean): Promise<void> => {
  const ids = items.map((i) => i.id as number);
  await getResponse<BatchResultOut>(
    tagEventRuleSetEnabled(ids, enabled),
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
    tagEventRuleDelete(proxy.id),
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

const itemToggleAction = async (): Promise<void> => {
  const state = batchState.value;
  if (state) {
    batchState.value = null;
    await batchToggleAction(state.items, state.action === "activate");
  }
};

// ---- 版本列表与回滚 ----
const versionDialog = ref(false);
const versionItems = ref<TagEventRuleVersionOut[]>([]);
const expandedVersion = ref<number | null>(null);

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const versionAction = async (item: Record<string, any>): Promise<void> => {
  versionDialog.value = true;
  versionItems.value = [];
  expandedVersion.value = null;
  await getResponse<{ items: TagEventRuleVersionOut[] }>(
    tagEventRuleVersionList(item.id),
    async (rsp) => {
      versionItems.value = rsp.data.items ?? [];
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

const openRollbackConfirm = (v: TagEventRuleVersionOut): void => {
  rollbackConfirmRef.value?.open(v);
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const itemRollbackAction = async (proxy: Record<string, any>): Promise<void> => {
  await getSimpleResponse(
    tagEventRuleVersionRollback(proxy.ruleID, proxy.version),
    async (rsp) => {
      await searchTableRef.value?.refresh();
      versionDialog.value = false;
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
</script>
