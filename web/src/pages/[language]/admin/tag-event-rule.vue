<template>
  <main-layout current-nav="admin">
    <template #body>
      <search-table
        ref="searchTableRef"
        :title="$t('model.tagEventRule.name')"
        :list-request="tagEventRuleListGet"
        :options-request="tagEventRuleListOptions"
        :actions="getActions"
        model-name="tagEventRule"
      >
        <template #top>
          <v-btn color="primary" @click="openCreateDialogAction">
            {{ $t("action.create") }}
          </v-btn>
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
        <template #body="{ proxy }">
          <confirm-alert
            :action="$t(toggleTarget === true ? 'action.activate' : 'action.deactivate')"
            :text="
              $t(toggleTarget === true ? 'notice.enableConfirm' : 'notice.disableConfirm', {
                category: $t(`model.tagEventRule.name`),
                key: proxy.name,
              })
            "
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
            <v-row dense>
              <v-col cols="12">
                <v-text-field
                  v-model="createName"
                  v-bind="createNameProps"
                  :label="$t('model.tagEventRule.field.name')"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-textarea
                  v-model="createDescription"
                  v-bind="createDescriptionProps"
                  :label="$t('model.tagEventRule.field.description')"
                  rows="1"
                />
              </v-col>
            </v-row>
            <v-row dense>
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
            <v-row dense align="center">
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
              <v-card-title class="d-flex align-center text-subtitle-2">
                {{ $t("model.tagEventCondition.name") }} #{{ idx + 1 }}
                <v-spacer />
                <v-btn
                  icon="mdi-minus"
                  density="compact"
                  variant="tonal"
                  color="error"
                  :disabled="createConditions.length === 1"
                  @click="removeCreateCondition(idx)"
                />
              </v-card-title>
              <v-card-text>
                <v-row dense>
                  <v-col cols="6">
                    <v-text-field
                      v-model="cond.name"
                      :label="$t('model.tagEventCondition.field.name')"
                      :error="!cond.name.trim()"
                    />
                  </v-col>
                  <v-col cols="6">
                    <v-select
                      v-model="cond.model"
                      :items="modelOptions"
                      :label="$t('model.tagEventCondition.field.model')"
                      clearable
                    />
                  </v-col>
                </v-row>
                <v-row dense>
                  <v-col cols="6">
                    <v-text-field
                      v-model="cond.dimension"
                      :label="$t('model.tagEventCondition.field.dimension')"
                      placeholder="fp_tcp_client_sig / fp_tcp_server_sig ..."
                    />
                  </v-col>
                  <v-col cols="6">
                    <v-text-field
                      v-model="cond.item"
                      :label="$t('model.tagEventCondition.field.item')"
                    />
                  </v-col>
                </v-row>
                <v-row dense>
                  <v-col cols="6">
                    <v-text-field
                      v-model.number="cond.window"
                      type="number"
                      :label="$t('model.tagEventCondition.field.window')"
                      :error="!(cond.window > 0)"
                    />
                  </v-col>
                  <v-col cols="6">
                    <v-text-field
                      v-model.number="cond.threshold"
                      type="number"
                      :label="$t('model.tagEventCondition.field.threshold')"
                      :error="!((cond.threshold ?? 0) > 0)"
                    />
                  </v-col>
                </v-row>
                <v-row dense>
                  <v-col cols="12">
                    <v-text-field
                      v-model="cond.expression"
                      :label="$t('model.tagEventCondition.field.expression')"
                      :error="!!cond.localError"
                      :error-messages="cond.localError || ''"
                      :append-inner-icon="cond.localValid ? 'mdi-check-circle' : ''"
                    />
                  </v-col>
                </v-row>
                <v-row dense align="center">
                  <v-col cols="auto">
                    <v-btn
                      variant="tonal"
                      size="small"
                      :loading="cond.validating"
                      @click="validateConditionExpression(cond)"
                    >
                      {{ $t("action.validate") }}
                    </v-btn>
                  </v-col>
                  <v-col cols="auto">
                    <v-radio-group v-model="cond.connector" inline density="compact" hide-details>
                      <v-radio label="AND" value="AND" />
                      <v-radio label="OR" value="OR" />
                    </v-radio-group>
                  </v-col>
                </v-row>
                <v-row dense>
                  <v-col cols="12">
                    <v-label>{{ $t("action.generateExpression") }}</v-label>
                    <v-chip
                      v-for="tag in tagPool"
                      :key="tag"
                      size="small"
                      variant="tonal"
                      :color="cond.selectedTags.includes(tag) ? 'primary' : ''"
                      class="me-1 my-1"
                      @click="toggleTag(cond, tag)"
                    >
                      {{ tag }}
                    </v-chip>
                    <v-chip
                      v-if="tagPool.length === 0 && !tagPoolLoading"
                      size="small"
                      variant="tonal"
                      disabled
                      class="my-1"
                    >
                      {{ $t("notice.noEnabledRules") }}
                    </v-chip>
                    <v-btn
                      size="small"
                      variant="tonal"
                      class="ms-2"
                      :disabled="cond.selectedTags.length === 0"
                      @click="generateExpression(cond)"
                    >
                      {{ $t("action.generateExpression") }}
                    </v-btn>
                  </v-col>
                </v-row>
                <v-row v-if="suggestTagsFor(cond).length" dense>
                  <v-col cols="12">
                    <v-chip
                      v-for="s in suggestTagsFor(cond)"
                      :key="s"
                      size="small"
                      variant="outlined"
                      class="me-1"
                      @click="appendTag(cond, s)"
                    >
                      {{ s }}
                    </v-chip>
                  </v-col>
                </v-row>
                <v-row dense>
                  <v-col cols="12">
                    <v-textarea
                      v-model="cond.description"
                      :label="$t('model.tagEventCondition.field.description')"
                      rows="1"
                    />
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
            <v-row dense>
              <v-col cols="12">
                <v-text-field
                  :model-value="proxy.name"
                  :label="$t('model.tagEventRule.field.name')"
                  disabled
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-textarea
                  v-model="editDescription"
                  v-bind="editDescriptionProps"
                  :label="$t('model.tagEventRule.field.description')"
                  :disabled="!editable"
                  rows="1"
                />
              </v-col>
            </v-row>
            <v-row dense>
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
            <v-divider class="my-2" />
            <v-row dense align="center">
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
              <v-card-title class="d-flex align-center text-subtitle-2">
                {{ $t("model.tagEventCondition.name") }} #{{ idx + 1 }}
                <v-spacer />
                <v-btn
                  icon="mdi-minus"
                  density="compact"
                  variant="tonal"
                  color="error"
                  :disabled="!editable || editConditions.length === 1"
                  @click="removeEditCondition(idx)"
                />
              </v-card-title>
              <v-card-text>
                <v-row dense>
                  <v-col cols="6">
                    <v-text-field
                      v-model="cond.name"
                      :label="$t('model.tagEventCondition.field.name')"
                      :disabled="!editable"
                      :error="!cond.name.trim()"
                    />
                  </v-col>
                  <v-col cols="6">
                    <v-select
                      v-model="cond.model"
                      :items="modelOptions"
                      :label="$t('model.tagEventCondition.field.model')"
                      :disabled="!editable"
                      clearable
                    />
                  </v-col>
                </v-row>
                <v-row dense>
                  <v-col cols="6">
                    <v-text-field
                      v-model="cond.dimension"
                      :label="$t('model.tagEventCondition.field.dimension')"
                      :disabled="!editable"
                      placeholder="fp_tcp_client_sig / fp_tcp_server_sig ..."
                    />
                  </v-col>
                  <v-col cols="6">
                    <v-text-field
                      v-model="cond.item"
                      :label="$t('model.tagEventCondition.field.item')"
                      :disabled="!editable"
                    />
                  </v-col>
                </v-row>
                <v-row dense>
                  <v-col cols="6">
                    <v-text-field
                      v-model.number="cond.window"
                      type="number"
                      :label="$t('model.tagEventCondition.field.window')"
                      :disabled="!editable"
                      :error="!(cond.window > 0)"
                    />
                  </v-col>
                  <v-col cols="6">
                    <v-text-field
                      v-model.number="cond.threshold"
                      type="number"
                      :label="$t('model.tagEventCondition.field.threshold')"
                      :disabled="!editable"
                      :error="!((cond.threshold ?? 0) > 0)"
                    />
                  </v-col>
                </v-row>
                <v-row dense>
                  <v-col cols="12">
                    <v-text-field
                      v-model="cond.expression"
                      :label="$t('model.tagEventCondition.field.expression')"
                      :disabled="!editable"
                      :error="!!cond.localError"
                      :error-messages="cond.localError || ''"
                      :append-inner-icon="cond.localValid ? 'mdi-check-circle' : ''"
                    />
                  </v-col>
                </v-row>
                <v-row dense align="center">
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
                  <v-col cols="auto">
                    <v-radio-group
                      v-model="cond.connector"
                      inline
                      density="compact"
                      hide-details
                      :disabled="!editable"
                    >
                      <v-radio label="AND" value="AND" />
                      <v-radio label="OR" value="OR" />
                    </v-radio-group>
                  </v-col>
                </v-row>
                <v-row dense>
                  <v-col cols="12">
                    <v-label>{{ $t("action.generateExpression") }}</v-label>
                    <v-chip
                      v-for="tag in tagPool"
                      :key="tag"
                      size="small"
                      variant="tonal"
                      :color="cond.selectedTags.includes(tag) ? 'primary' : ''"
                      :disabled="!editable"
                      class="me-1 my-1"
                      @click="toggleTag(cond, tag)"
                    >
                      {{ tag }}
                    </v-chip>
                    <v-chip
                      v-if="tagPool.length === 0 && !tagPoolLoading"
                      size="small"
                      variant="tonal"
                      disabled
                      class="my-1"
                    >
                      {{ $t("notice.noEnabledRules") }}
                    </v-chip>
                    <v-btn
                      size="small"
                      variant="tonal"
                      class="ms-2"
                      :disabled="!editable || cond.selectedTags.length === 0"
                      @click="generateExpression(cond)"
                    >
                      {{ $t("action.generateExpression") }}
                    </v-btn>
                  </v-col>
                </v-row>
                <v-row v-if="suggestTagsFor(cond).length" dense>
                  <v-col cols="12">
                    <v-chip
                      v-for="s in suggestTagsFor(cond)"
                      :key="s"
                      size="small"
                      variant="outlined"
                      :disabled="!editable"
                      class="me-1"
                      @click="appendTag(cond, s)"
                    >
                      {{ s }}
                    </v-chip>
                  </v-col>
                </v-row>
                <v-row dense>
                  <v-col cols="12">
                    <v-textarea
                      v-model="cond.description"
                      :label="$t('model.tagEventCondition.field.description')"
                      :disabled="!editable"
                      rows="1"
                    />
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
          </v-form>
        </template>
      </temp-editable-dialog>

      <v-dialog v-model="dryRunDialog" max-width="900">
        <v-card>
          <v-card-title>{{ $t("action.dryRun") }}</v-card-title>
          <v-card-text>
            <v-row dense>
              <v-col cols="6">
                <v-text-field v-model="dryRunName" :label="$t('model.tagEventRule.field.name')" />
              </v-col>
              <v-col cols="6">
                <v-switch
                  v-model="dryRunMatchAll"
                  :label="$t('model.tagEventRule.field.matchAll')"
                  color="primary"
                />
              </v-col>
            </v-row>
            <v-row dense align="center">
              <v-col>
                <v-label>{{ $t("model.tagEventRule.field.conditions") }}</v-label>
              </v-col>
              <v-col cols="auto">
                <v-btn
                  variant="tonal"
                  size="small"
                  prepend-icon="mdi-plus"
                  @click="dryRunAddCondition"
                >
                  {{ $t("action.addCondition") }}
                </v-btn>
              </v-col>
            </v-row>
            <v-card
              v-for="(cond, idx) in dryRunConditions"
              :key="cond.key"
              variant="tonal"
              class="my-2"
            >
              <v-card-title class="d-flex align-center text-subtitle-2">
                {{ $t("model.tagEventCondition.name") }} #{{ idx + 1 }}
                <v-spacer />
                <v-btn
                  icon="mdi-minus"
                  density="compact"
                  variant="tonal"
                  color="error"
                  @click="dryRunConditions.splice(idx, 1)"
                />
              </v-card-title>
              <v-card-text>
                <v-row dense>
                  <v-col cols="6">
                    <v-text-field
                      v-model="cond.name"
                      :label="$t('model.tagEventCondition.field.name')"
                    />
                  </v-col>
                  <v-col cols="6">
                    <v-text-field
                      v-model="cond.expression"
                      :label="$t('model.tagEventCondition.field.expression')"
                    />
                  </v-col>
                </v-row>
                <v-row dense>
                  <v-col cols="6">
                    <v-text-field
                      v-model.number="cond.window"
                      type="number"
                      :label="$t('model.tagEventCondition.field.window')"
                    />
                  </v-col>
                  <v-col cols="6">
                    <v-text-field
                      v-model.number="cond.threshold"
                      type="number"
                      :label="$t('model.tagEventCondition.field.threshold')"
                    />
                  </v-col>
                </v-row>
              </v-card-text>
            </v-card>
            <v-alert v-if="dryRunResult" type="success" class="mt-2">
              {{ dryRunResult.ruleName }}
              ({{ $t("model.tagEventRule.field.matchAll") }}: {{ dryRunResult.matchAll }})
            </v-alert>
            <v-list v-if="dryRunResult?.events?.length" density="compact">
              <v-list-item v-for="ev in dryRunResult.events" :key="`${ev.ruleID}-${ev.detectedAt}`">
                <template #title>
                  {{ ev.ruleName }} ({{ localeDatetimeFormat(DateTime.fromISO(ev.detectedAt)) }})
                </template>
                <template #append>
                  <v-chip v-for="hit in ev.hits" :key="hit.conditionID" size="small" class="me-1">
                    {{ hit.conditionName }}: {{ hit.count }}/{{ hit.threshold }}
                  </v-chip>
                </template>
              </v-list-item>
            </v-list>
            <v-alert
              v-else-if="dryRunResult"
              type="info"
              class="mt-2"
              :text="$t('summary.endOfData')"
            />
          </v-card-text>
          <v-card-actions>
            <v-spacer />
            <v-btn variant="plain" @click="dryRunDialog = false">
              {{ $t("action.close") }}
            </v-btn>
            <v-btn color="primary" :loading="dryRunLoading" @click="runDryRun">
              {{ $t("action.run") }}
            </v-btn>
          </v-card-actions>
        </v-card>
      </v-dialog>

      <v-dialog v-model="versionDialog" max-width="800">
        <v-card>
          <v-card-title>{{ $t("action.version") }}</v-card-title>
          <v-card-text>
            <v-list>
              <v-list-item v-for="v in versionItems" :key="v.id">
                <template #title>
                  <v-chip size="small" class="me-2">v{{ v.version }}</v-chip>
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
                  <v-list v-if="expandedVersion === v.version" density="compact" class="rego-view">
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
          </v-card-text>
          <v-card-actions>
            <v-spacer />
            <v-btn variant="plain" @click="versionDialog = false">
              {{ $t("action.close") }}
            </v-btn>
          </v-card-actions>
        </v-card>
      </v-dialog>

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
name: Admin Tag Event Rule List
meta:
  title: title.tagEventRuleAdmin
  requiresAuth: true
  requiresPerm: true
</route>

<script setup lang="ts">
import { reactive } from "vue";
import { DateTime } from "luxon";

import {
  tagEventRuleCreate,
  tagEventRuleDelete,
  tagEventRuleDetailGet,
  tagEventRuleDryRun,
  tagEventRuleListGet,
  tagEventRuleListOptions,
  tagEventRuleSetEnabled,
  tagEventRuleUpdate,
  tagEventRuleValidate,
  tagEventRuleVersionList,
  tagEventRuleVersionRollback,
  tagRuleDetailGet,
  tagRuleListGet,
} from "@/apis/admin";
import type { TagEventRuleConditionIn } from "@/apis/@types/admin";
import { getResponse, getSimpleResponse } from "@/apis/base";
import type { ItemActions } from "@/components/@types/table";
import { useAppStore } from "@/stores/app";
import { createTagEventRuleSchema, updateTagEventRuleSchema } from "@/schemas/tagEventRule";
import { parseBooleanExpr } from "@/utils/booleanExpr";
import { durationToMinutes, minutesToDurationString } from "@/utils/duration";
import { vuetifyCfg } from "@/utils/form";
import i18n, { localeDatetimeFormat, localDurationFormat } from "@/utils/i18n";
import { extractTagLiterals, HTTP_TRANSPORT_MODEL_KEY } from "@/utils/rego";
import { useForm } from "vee-validate";

import type {
  TagEventRuleDetailOut,
  TagEventRuleDryRunOut,
  TagEventRuleValidateOut,
  TagEventRuleVersionOut,
  TagRuleDetailOut,
  TagRuleListOut,
} from "./@types";

interface ConditionDraft {
  key: number;
  name: string;
  model: string;
  dimension: string;
  item: string;
  /** window in minutes (converted to duration string on submit) */
  window: number;
  threshold: number | null;
  expression: string;
  enabled: boolean;
  description: string;
  localValid: boolean;
  localError: string;
  backendValid: boolean | null;
  validating: boolean;
  selectedTags: string[];
  connector: "AND" | "OR";
}

const appStore = useAppStore();
const searchTableRef = useTemplateRef("searchTableRef");
const createDialogRef = useTemplateRef("createDialogRef");
const editDialogRef = useTemplateRef("editDialogRef");
const deleteConfirmRef = useTemplateRef("deleteConfirmRef");
const toggleConfirmRef = useTemplateRef("toggleConfirmRef");
const rollbackConfirmRef = useTemplateRef("rollbackConfirmRef");

const modelOptions = [
  {
    title: "HTTP Transport",
    value: HTTP_TRANSPORT_MODEL_KEY,
  },
];

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
    selectedTags: [] as string[],
    connector: "AND" as const,
  });

const toConditionIn = (c: ConditionDraft): TagEventRuleConditionIn => ({
  name: c.name,
  model: c.model,
  dimension: c.dimension,
  item: c.item,
  window: minutesToDurationString(c.window),
  threshold: c.threshold ?? 0,
  expression: c.expression,
  enabled: c.enabled,
  description: c.description,
});

// ---- forms ----
const createForm = useForm({ validationSchema: createTagEventRuleSchema });
const updateForm = useForm({ validationSchema: updateTagEventRuleSchema });

const [createName, createNameProps] = createForm.defineField("name", vuetifyCfg);
const [createDescription, createDescriptionProps] = createForm.defineField(
  "description",
  vuetifyCfg,
);
const [createMatchAll, createMatchAllProps] = createForm.defineField("matchAll", vuetifyCfg);
const [createEnabled, createEnabledProps] = createForm.defineField("enabled", vuetifyCfg);

const [editDescription, editDescriptionProps] = updateForm.defineField("description", vuetifyCfg);
const [editMatchAll, editMatchAllProps] = updateForm.defineField("matchAll", vuetifyCfg);
const [editEnabled, editEnabledProps] = updateForm.defineField("enabled", vuetifyCfg);

// ---- tag pool (heuristic expansion source) ----
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

// ---- condition helpers ----
const toggleTag = (cond: ConditionDraft, tag: string): void => {
  const idx = cond.selectedTags.indexOf(tag);
  if (idx >= 0) {
    cond.selectedTags.splice(idx, 1);
  } else {
    cond.selectedTags.push(tag);
  }
};

const appendTag = (cond: ConditionDraft, tag: string): void => {
  const expr = cond.expression;
  const tail = /([a-zA-Z][a-zA-Z0-9_-]*)$/.exec(expr);
  if (tail && expr.slice(0, tail.index).trim()) {
    cond.expression = `${expr.slice(0, tail.index)}${tag}`;
    return;
  }
  const sep = expr && !/[\s(]$/.test(expr) ? ` ${cond.connector} ` : "";
  cond.expression = `${expr}${sep}${tag}`;
};

const generateExpression = (cond: ConditionDraft): void => {
  if (cond.selectedTags.length === 0) {
    return;
  }
  cond.expression = cond.selectedTags.join(` ${cond.connector} `);
  void validateConditionExpression(cond);
};

const suggestTagsFor = (cond: ConditionDraft): string[] => {
  const tail = /(?:^|[\s(]+)([a-zA-Z][a-zA-Z0-9_-]*)$/.exec(cond.expression);
  if (!tail) {
    return [];
  }
  return tagPool.value.filter((t) => t.startsWith(tail[1])).slice(0, 8);
};

const validateConditionExpression = async (cond: ConditionDraft): Promise<void> => {
  cond.validating = true;
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

// ---- create dialog ----
const createConditions = ref<ConditionDraft[]>([]);

const addCreateCondition = (): void => {
  const cond = newCondition();
  watchConditionExpression(cond);
  createConditions.value.push(cond);
};

const removeCreateCondition = (idx: number): void => {
  createConditions.value.splice(idx, 1);
};

const createConditionsValid = computed<boolean>(
  () =>
    createConditions.value.length > 0 &&
    createConditions.value.every(
      (c) =>
        c.name.trim() !== "" &&
        c.expression.trim() !== "" &&
        c.window > 0 &&
        (c.threshold ?? 0) > 0 &&
        c.localValid,
    ),
);

const openCreateDialogAction = (): void => {
  createForm.resetField("name", { value: "" });
  createForm.resetField("description", { value: "" });
  createForm.resetField("matchAll", { value: false });
  createForm.resetField("enabled", { value: true });
  createConditions.value = [];
  addCreateCondition();
  void loadTagPool();
  createDialogRef.value?.open({});
};

const createAction = createForm.handleSubmit(async (values) => {
  if (!createConditionsValid.value) {
    appStore.notify({
      text: i18n.global.t("notice.expressionInvalid"),
      timeout: 3000,
      color: "error",
    });
    return;
  }
  await getSimpleResponse(
    tagEventRuleCreate({
      name: values.name,
      description: values.description,
      matchAll: values.matchAll,
      enabled: values.enabled,
      conditions: createConditions.value.map(toConditionIn),
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

// ---- edit dialog ----
let editRuleId = 0;
const editConditions = ref<ConditionDraft[]>([]);

const addEditCondition = (): void => {
  const cond = newCondition();
  watchConditionExpression(cond);
  editConditions.value.push(cond);
};

const removeEditCondition = (idx: number): void => {
  editConditions.value.splice(idx, 1);
};

const editConditionsValid = computed<boolean>(
  () =>
    editConditions.value.length > 0 &&
    editConditions.value.every(
      (c) =>
        c.name.trim() !== "" &&
        c.expression.trim() !== "" &&
        c.window > 0 &&
        (c.threshold ?? 0) > 0 &&
        c.localValid,
    ),
);

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const detailGetAction = async (item: Record<string, any>): Promise<void> => {
  void loadTagPool();
  await getResponse<TagEventRuleDetailOut>(
    tagEventRuleDetailGet(item.id),
    async (rsp) => {
      editRuleId = rsp.data.id;
      updateForm.resetField("description", { value: rsp.data.description });
      updateForm.resetField("matchAll", { value: rsp.data.matchAll });
      updateForm.resetField("enabled", { value: rsp.data.enabled });
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
      appStore.notify({
        text: i18n.global.t("notice.expressionInvalid"),
        timeout: 3000,
        color: "error",
      });
      return;
    }
    await getSimpleResponse(
      tagEventRuleUpdate(editRuleId, {
        description: values.description,
        matchAll: values.matchAll,
        enabled: values.enabled,
        conditions: editConditions.value.map(toConditionIn),
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

// ---- list actions ----
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const getActions = (item: Record<string, any>): ItemActions => {
  const actions: ItemActions = {
    edit: detailGetAction,
    version: versionAction,
    dryRun: openDryRunAction,
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    delete: (i: Record<string, any>) => deleteConfirmRef.value?.open(i),
  };
  switch (item.enabled) {
    case false:
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      actions.activate = (i: Record<string, any>) => toggleAction(i, true);
      break;
    case true:
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      actions.deactivate = (i: Record<string, any>) => toggleAction(i, false);
      break;
  }
  return actions;
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const itemDeleteAction = async (item: Record<string, any>): Promise<void> => {
  await getSimpleResponse(
    tagEventRuleDelete(item.id),
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

const toggleTarget = ref<boolean | null>(null);

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const toggleAction = (item: Record<string, any>, enabled: boolean): void => {
  toggleTarget.value = enabled;
  toggleConfirmRef.value?.open(item);
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const itemToggleAction = async (item: Record<string, any>): Promise<void> => {
  if (toggleTarget.value === null) {
    return;
  }
  await getSimpleResponse(
    tagEventRuleSetEnabled(item.id, toggleTarget.value),
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

// ---- dry run ----
const dryRunDialog = ref(false);
const dryRunName = ref("");
const dryRunMatchAll = ref(false);
const dryRunConditions = ref<ConditionDraft[]>([]);
const dryRunLoading = ref(false);
const dryRunResult = ref<TagEventRuleDryRunOut | null>(null);

const dryRunAddCondition = (): void => {
  dryRunConditions.value.push(newCondition());
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const openDryRunAction = (item: Record<string, any>): void => {
  dryRunName.value = item.name || "";
  dryRunMatchAll.value = !!item.matchAll;
  dryRunConditions.value = [];
  dryRunAddCondition();
  dryRunResult.value = null;
  dryRunDialog.value = true;
};

const runDryRun = async (): Promise<void> => {
  if (dryRunConditions.value.length === 0) {
    return;
  }
  dryRunResult.value = null;
  dryRunLoading.value = true;
  try {
    await getResponse<TagEventRuleDryRunOut>(
      tagEventRuleDryRun({
        name: dryRunName.value,
        matchAll: dryRunMatchAll.value,
        enabled: true,
        conditions: dryRunConditions.value.map(toConditionIn),
      }),
      async (rsp) => {
        dryRunResult.value = rsp.data;
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
    dryRunLoading.value = false;
  }
};

// ---- version list & rollback ----
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
    tagEventRuleVersionRollback(proxy.ruleId, proxy.version),
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

<style scoped>
.rego-view {
  white-space: pre-wrap;
  word-break: break-all;
  background-color: rgb(var(--v-theme-surface-variant));
  border-radius: 4px;
  padding: 8px;
  font-size: 13px;
  max-height: 320px;
  overflow: auto;
}
</style>
