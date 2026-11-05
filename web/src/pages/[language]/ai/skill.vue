<template>
  <main-layout current-nav="ai">
    <template #body>
      <search-table
        ref="searchTableRef"
        :list-request="agentSkillListGet"
        :options-request="agentSkillListOptions"
        :actions="getActions"
        :batch-actions="getBatchActions"
        :create="openCreateDialogAction"
        :project-fields="projectFields"
        model-name="agentSkill"
      >
        <!-- 作用域提醒：内置技能为代码声明的全局能力，仅可启停；自定义技能按命名空间隔离。
             置于列表卡片内，与表格共享同一表面，避免独立悬浮造成的边缘错位 -->
        <template #banner>
          <div class="px-4 pt-4 pb-2">
            <v-alert type="info">
              {{ $t("page.aiSkill.scopeHint") }}
            </v-alert>
          </div>
        </template>

        <!-- 来源列：label 经枚举 store 渲染；内置来源只读（可启停），以锁形图标标识 -->
        <template #cell-source="{ item }">
          <div class="d-flex align-center justify-center ga-1">
            <span>{{ enumLabelOf(EnumClass.agentSkillSource, item.source) }}</span>
            <v-icon
              v-if="readonlySkill(item)"
              icon="mdi-lock-outline"
              size="small"
              :title="$t('page.aiSkill.builtinReadonly')"
              :aria-label="$t('page.aiSkill.builtinReadonly')"
            />
          </div>
        </template>

        <!-- 启停列：开关即时写入（乐观更新，失败回退）；内置技能同样可启停 -->
        <template #cell-enabled="{ item }">
          <enabled-switch-cell
            :model-value="item.enabled"
            :request="(enabled: boolean) => agentSkillStatusSet(item.id, { enabled })"
            @update:model-value="(v: boolean) => (item.enabled = v)"
          />
        </template>

        <!-- 周期能力 / 输入字段 / 固定字段均不做页面级定制：后端 OPTIONS 元数据标注列类型与
             kind，由自适应单元格统一渲染（布尔走布尔图标，JSON 走 JSON 胶囊） -->
      </search-table>

      <!-- 删除确认对话框：单条删除与批量删除共用 -->
      <confirm-dialog
        ref="deleteConfirmRef"
        :title="$t('model.agentSkill.name')"
        prepend-icon="mdi-lightning-bolt-outline"
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
                category: $t('model.agentSkill.name'),
                key: proxy.name,
              })
            "
          />
        </template>
      </confirm-dialog>

      <!-- 新建技能对话框：可直接粘贴 Markdown 正文，也可从本地导入（导入仅回填名称与正文） -->
      <temp-editable-dialog
        ref="createDialogRef"
        :title="$t('page.aiSkill.create')"
        :confirm-disabled="!createForm.meta.value.valid"
        force-editable
        prepend-icon="mdi-lightning-bolt-outline"
        @confirm="createAction"
      >
        <template #body>
          <v-form>
            <v-row density="compact">
              <v-col cols="12">
                <v-text-field
                  v-model="createName"
                  v-bind="createNameProps"
                  :label="$t('model.agentSkill.field.name')"
                  :placeholder="$t('page.aiSkill.namePlaceholder')"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <v-textarea
                  v-model="createDescription"
                  v-bind="createDescriptionProps"
                  :label="$t('model.agentSkill.field.description')"
                  :placeholder="$t('page.aiSkill.descriptionPlaceholder')"
                  rows="3"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <v-file-input
                  v-model="importFile"
                  accept=".md,.markdown,.txt"
                  :label="$t('page.aiSkill.importMarkdown')"
                  :hint="$t('page.aiSkill.importHint')"
                  persistent-hint
                  show-size
                  @update:model-value="importMarkdownAction"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <markdown-editor
                  v-model="createContent"
                  :label="$t('model.agentSkill.field.content')"
                  :placeholder-text="$t('page.aiSkill.contentPlaceholder')"
                  :error-message="createForm.errors.value.content"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <div class="zk-panel--subtle pa-3 d-flex flex-column ga-3">
                  <v-switch
                    v-model="createPeriodicCapable"
                    color="primary"
                    density="compact"
                    hide-details
                    :label="$t('model.agentSkill.field.periodicCapable')"
                  />
                  <SkillInputFieldsEditor
                    v-model="createInputFields"
                    :periodic="createPeriodicCapable"
                  />
                  <JsonEditor
                    v-model="createFixedFieldsText"
                    :label="$t('model.agentSkill.field.fixedFields')"
                    :hint="$t('page.aiSkill.fixedFieldsHint')"
                  />
                </div>
              </v-col>
            </v-row>
          </v-form>
        </template>
      </temp-editable-dialog>

      <!-- 编辑技能对话框：先取详情回填，名称 / 描述 / 正文均可修改 -->
      <temp-editable-dialog
        ref="editDialogRef"
        :title="$t('page.aiSkill.edit')"
        :confirm-disabled="!updateForm.meta.value.valid"
        prepend-icon="mdi-lightning-bolt-outline"
        @confirm="itemUpdateAction"
      >
        <template #body="{ editable }">
          <v-form>
            <v-row density="compact">
              <v-col cols="12">
                <v-text-field
                  v-model="editName"
                  v-bind="editNameProps"
                  :label="$t('model.agentSkill.field.name')"
                  :disabled="!editable"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <v-textarea
                  v-model="editDescription"
                  v-bind="editDescriptionProps"
                  :label="$t('model.agentSkill.field.description')"
                  :disabled="!editable"
                  rows="3"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <markdown-editor
                  v-model="editContent"
                  :label="$t('model.agentSkill.field.content')"
                  :placeholder-text="$t('page.aiSkill.contentPlaceholder')"
                  :error-message="updateForm.errors.value.content"
                  :disabled="!editable"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <div class="zk-panel--subtle pa-3 d-flex flex-column ga-3">
                  <v-switch
                    v-model="editPeriodicCapable"
                    color="primary"
                    density="compact"
                    hide-details
                    :disabled="!editable"
                    :label="$t('model.agentSkill.field.periodicCapable')"
                  />
                  <SkillInputFieldsEditor
                    v-model="editInputFields"
                    :periodic="editPeriodicCapable"
                    :disabled="!editable"
                  />
                  <JsonEditor
                    v-model="editFixedFieldsText"
                    :label="$t('model.agentSkill.field.fixedFields')"
                    :hint="$t('page.aiSkill.fixedFieldsHint')"
                    :disabled="!editable"
                  />
                </div>
              </v-col>
            </v-row>
          </v-form>
        </template>
      </temp-editable-dialog>
    </template>
  </main-layout>
</template>

<route lang="yaml">
name: AI Skill List
meta:
  title: title.aiSkill
  requiresAuth: true
  requiresPerm: true
</route>

<script setup lang="ts" name="ai-skill-list">
import * as yup from "yup";
import { useForm } from "vee-validate";

import {
  agentSkillCreate,
  agentSkillDelete,
  agentSkillDetailGet,
  agentSkillListGet,
  agentSkillListOptions,
  agentSkillStatusSet,
  agentSkillUpdate,
} from "@/apis/ai";
import { getResponse, getSimpleResponse } from "@/apis/base";
import type { BatchResultOut } from "@/apis/@types/admin";
import type { AgentSkillDetailOut, AgentSkillInputFieldOut } from "@/apis/@types/ai";
import JsonEditor from "@/components/forms/JsonEditor.vue";
import SkillInputFieldsEditor from "@/components/forms/SkillInputFieldsEditor.vue";
import type { BatchActions, ItemActions } from "@/components/@types/table";
import { useAppStore } from "@/stores/app";
import { EnumClass, enumEntriesOf, enumLabelOf, ensureEnum } from "@/utils/enums";
import { vuetifyCfg } from "@/utils/form";
import i18n from "@/utils/i18n";

// ---- 表单校验 schema（正文为多行文本域，内容中的空白具有语义，不做 trim）----
const skillSchema = yup.object({
  name: yup.string().trim().required().min(1).max(64).label("model.agentSkill.field.name"),
  // 描述可空，仅限制长度
  description: yup.string().trim().max(256).label("model.agentSkill.field.description"),
  content: yup.string().required().min(1).max(32768).label("model.agentSkill.field.content"),
});

const appStore = useAppStore();

// ---- 技能定义字段（周期能力 / 输入字段 / 固定参数）----
const createPeriodicCapable = ref(false);
const createInputFields = ref<AgentSkillInputFieldOut[]>([]);
const createFixedFieldsText = ref("{}");
const editPeriodicCapable = ref(false);
const editInputFields = ref<AgentSkillInputFieldOut[]>([]);
const editFixedFieldsText = ref("{}");

// parseFixedFields 把固定字段 JSON 文本解析为对象：空文本按空对象处理，非对象 / 非法 JSON
// 以错误提示返回 null，调用方据此中止提交。
const parseFixedFields = (text: string): Record<string, unknown> | null => {
  if (!text.trim()) {
    return {};
  }
  try {
    const parsed: unknown = JSON.parse(text);
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
      throw new Error("fixed fields must be an object");
    }
    return parsed as Record<string, unknown>;
  } catch {
    appStore.notify({
      text: i18n.global.t("page.aiSkill.invalidFixedFields"),
      timeout: 3000,
      color: "error",
    });
    return null;
  }
};

// validateSkillDefinition 前端兜底校验：输入字段名不能为空；周期技能必须包含 nowTime/date 字段。
// 固定参数契约（timeOffset / timeWindow 等）由后端统一校验。
const validateSkillDefinition = (periodic: boolean, fields: AgentSkillInputFieldOut[]): boolean => {
  if (fields.some((field) => !field.name.trim())) {
    appStore.notify({
      text: i18n.global.t("page.aiSkill.inputFieldNameRequired"),
      timeout: 3000,
      color: "error",
    });
    return false;
  }
  if (
    periodic &&
    !fields.some((field) => field.name.trim() === "nowTime" && field.type === "date")
  ) {
    appStore.notify({
      text: i18n.global.t("page.aiSkill.nowTimeRequired"),
      timeout: 3000,
      color: "error",
    });
    return false;
  }
  return true;
};

// 来源枚举 label 由后端枚举注册表下发，页面挂载时确保按当前语言缓存
onMounted(() => {
  void ensureEnum(EnumClass.agentSkillSource);
});

// builtinSkillSource 从后端枚举注册表取内置来源的存储值，避免前端硬编码枚举字面量
const builtinSkillSource = computed(
  () => enumEntriesOf(EnumClass.agentSkillSource)?.find((e) => e.key === "builtin")?.value,
);

// readonlySkill 内置技能由代码注册表维护，只读（仍可启停）
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const readonlySkill = (item: Record<string, any>): boolean =>
  builtinSkillSource.value !== undefined && item.source === builtinSkillSource.value;

const searchTableRef = useTemplateRef("searchTableRef");
const createDialogRef = useTemplateRef("createDialogRef");
const editDialogRef = useTemplateRef("editDialogRef");
const deleteConfirmRef = useTemplateRef("deleteConfirmRef");

// ---- 表单 ----
const createForm = useForm({ validationSchema: skillSchema });
const updateForm = useForm({ validationSchema: skillSchema });

const [createName, createNameProps] = createForm.defineField("name", vuetifyCfg);
const [createDescription, createDescriptionProps] = createForm.defineField(
  "description",
  vuetifyCfg,
);
// 正文由 MarkdownEditor 承载，错误信息经 errorMessage 单独下发，不再透传 vuetify 字段配置
const [createContent] = createForm.defineField("content", vuetifyCfg);

const [editName, editNameProps] = updateForm.defineField("name", vuetifyCfg);
const [editDescription, editDescriptionProps] = updateForm.defineField("description", vuetifyCfg);
const [editContent] = updateForm.defineField("content", vuetifyCfg);

// ---- 新建对话框 ----
// importFile 为「导入 Markdown」的选中文件：读取后仅回填名称与正文，不再重复提交该文件
const importFile = ref<File | null>(null);

const openCreateDialogAction = (): void => {
  // 复位用原子 API resetForm：完整枚举全部 defineField 字段，值 + 校验状态一次性重置
  createForm.resetForm({ values: { name: "", description: "", content: "" } });
  createPeriodicCapable.value = false;
  createInputFields.value = [];
  createFixedFieldsText.value = "{}";
  importFile.value = null;
  createDialogRef.value?.open({});
};

// fileNameWithoutExt 取文件名去扩展名，用作未填写名称时的预填值
const fileNameWithoutExt = (filename: string): string => filename.replace(/\.[^.]+$/, "");

// importMarkdownAction 读取所选 Markdown 文本并写入正文；名称为空时以文件名预填名称
const importMarkdownAction = async (file: File | File[] | null): Promise<void> => {
  const selected = Array.isArray(file) ? file[0] : file;
  if (!selected) {
    return;
  }
  try {
    createContent.value = await selected.text();
    if (!createName.value.trim()) {
      createName.value = fileNameWithoutExt(selected.name);
    }
  } catch {
    appStore.notify({
      text: i18n.global.t("page.aiSkill.importFailed", { name: selected.name }),
      timeout: 3000,
      color: "error",
    });
  }
};

const createAction = createForm.handleSubmit(async (values) => {
  const fixedFields = parseFixedFields(createFixedFieldsText.value);
  if (
    !fixedFields ||
    !validateSkillDefinition(createPeriodicCapable.value, createInputFields.value)
  ) {
    return;
  }
  await getSimpleResponse(
    agentSkillCreate({
      // 名称 / 描述为单行文本，提交前去除首尾空白；正文保留原样
      name: (values.name as string).trim(),
      description: (values.description as string).trim(),
      content: values.content,
      periodicCapable: createPeriodicCapable.value,
      inputFields: createInputFields.value,
      fixedFields,
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
let editSkillID = 0;
// editDetail 详情回填对象：同时承载周期能力、输入字段与固定参数的编辑副本
const editDetail = ref<AgentSkillDetailOut | null>(null);

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const detailGetAction = async (item: Record<string, any>): Promise<void> => {
  await getResponse<AgentSkillDetailOut>(
    agentSkillDetailGet(item.id),
    async (rsp) => {
      editSkillID = rsp.data.id;
      editDetail.value = rsp.data;
      // 复位与回填同属「准备本次打开的数据」：完整枚举 defineField 字段，原子重置值 + 校验状态
      updateForm.resetForm({
        values: {
          name: rsp.data.name,
          description: rsp.data.description,
          content: rsp.data.content,
        },
      });
      editPeriodicCapable.value = rsp.data.periodicCapable;
      editInputFields.value = rsp.data.inputFields ?? [];
      editFixedFieldsText.value = JSON.stringify(rsp.data.fixedFields ?? {}, null, 2);
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
    const fixedFields = parseFixedFields(editFixedFieldsText.value);
    if (
      !fixedFields ||
      !validateSkillDefinition(editPeriodicCapable.value, editInputFields.value)
    ) {
      return;
    }
    await getSimpleResponse(
      agentSkillUpdate(editSkillID, {
        name: (values.name as string).trim(),
        description: (values.description as string).trim(),
        content: values.content,
        periodicCapable: editPeriodicCapable.value,
        inputFields: editInputFields.value,
        fixedFields,
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
// projectFields 字段投影：行操作依赖 id/name/source/enabled 恒定投影，
// 避免用户隐藏对应列后只读判定与开关失效
const projectFields = (visible: string[]): string[] => [
  "id",
  "name",
  "source",
  "enabled",
  ...visible,
];

// getActions 行操作：内置技能由代码注册表维护，不提供编辑与删除入口（仍可启停）
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const getActions = (item: Record<string, any>): ItemActions => {
  if (readonlySkill(item)) {
    return {};
  }
  return {
    edit: () => detailGetAction(item),
    delete: () => {
      batchState.value = null;
      deleteConfirmRef.value?.open(item);
    },
  };
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
type BatchState = { items: Record<string, any>[] };
const batchState = ref<BatchState | null>(null);

// getBatchActions 批量操作：选中项含内置技能时返回空集（后端整批拒绝删除），
// 即内置技能不提供任何删除入口
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const getBatchActions = (items: Record<string, any>[]): BatchActions => {
  if (items.some(readonlySkill)) {
    return {};
  }
  return {
    delete: () => {
      batchState.value = { items };
      deleteConfirmRef.value?.open({});
    },
  };
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const batchDeleteAction = async (items: Record<string, any>[]): Promise<void> => {
  const ids = items.map((i) => i.id as number);
  await getResponse<BatchResultOut>(
    agentSkillDelete(ids),
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
    agentSkillDelete([proxy.id]),
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
</script>
