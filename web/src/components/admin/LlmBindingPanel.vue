<template>
  <div class="pa-4">
    <!-- 说明以 banner 常驻提示；刷新控件收在其尾部，仍保持单行布局 -->
    <v-alert type="info" class="mb-3">
      {{ $t("page.llmBindingAdmin.hint") }}
      <template #append>
        <v-btn
          icon="mdi-refresh"
          variant="text"
          size="small"
          :loading="loading"
          :title="$t('action.refresh')"
          :aria-label="$t('action.refresh')"
          @click="refreshAction(true)"
        />
      </template>
    </v-alert>

    <v-table density="comfortable">
      <thead>
        <tr>
          <th class="zk-label-caps">
            {{ $t("model.llmBinding.field.slot") }}
          </th>
          <th class="zk-label-caps">
            {{ $t("model.llmBinding.field.modelName") }}
          </th>
          <th class="zk-label-caps">
            {{ $t("model.llmBinding.field.kind") }}
          </th>
          <th class="zk-label-caps">
            {{ $t("model.llmBinding.field.dimensions") }}
          </th>
          <th class="zk-label-caps">
            {{ $t("model.llmBinding.field.available") }}
          </th>
          <th class="zk-label-caps">
            {{ $t("model.__base.field.action") }}
          </th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="row in rows" :key="row.slot">
          <td>
            <div class="text-on-surface">{{ enumLabelOf(EnumClass.llmSlot, row.slot) }}</div>
          </td>
          <td>
            <div v-if="row.bound" class="text-on-surface">{{ row.modelName }}</div>
            <div v-else class="zk-hint">
              {{ $t("page.llmBindingAdmin.unbound") }}
            </div>
            <div v-if="row.bound" class="zk-hint">
              {{ row.providerModel }}
            </div>
          </td>
          <td>
            <span v-if="row.bound" class="text-on-surface">
              {{ enumLabelOf(EnumClass.llmModelKind, row.kind) }}
            </span>
            <span v-else class="zk-hint">—</span>
          </td>
          <td>
            <span v-if="row.bound" class="text-on-surface">{{ row.dimensions }}</span>
            <span v-else class="zk-hint">—</span>
          </td>
          <td>
            <boolean-chip :value="row.available" />
          </td>
          <td>
            <div class="d-flex align-center">
              <v-select
                v-model="selection[row.slot]"
                :items="candidatesOf(row)"
                item-title="title"
                item-value="value"
                :placeholder="$t('page.llmBindingAdmin.selectModel')"
                hide-details
                class="slot-select"
              />
              <v-btn
                color="primary"
                variant="tonal"
                size="small"
                class="ms-2"
                :loading="submittingSlot === row.slot"
                :disabled="!canBind(row)"
                @click="bindAction(row.slot)"
              >
                {{ $t("action.bind") }}
              </v-btn>
              <v-btn
                v-if="row.bound"
                color="error"
                variant="text"
                size="small"
                class="ms-1"
                :disabled="submittingSlot === row.slot"
                @click="openUnbindConfirm(row)"
              >
                {{ $t("action.unbind") }}
              </v-btn>
            </div>
          </td>
        </tr>
      </tbody>
    </v-table>
  </div>

  <!-- 解绑确认对话框：解绑后该档位不可用 -->
  <confirm-dialog
    ref="unbindConfirmRef"
    :title="$t('model.llmBinding.name')"
    prepend-icon="mdi-link-variant-off"
    @confirm="unbindAction"
  >
    <template #body>
      <confirm-alert
        :action="$t('action.unbind')"
        :text="
          $t('notice.llmBinding.unbindConfirm', {
            key: enumLabelOf(EnumClass.llmSlot, unbindTarget?.slot),
          })
        "
      />
    </template>
  </confirm-dialog>
</template>

<script setup lang="ts" name="llm-binding-panel">
import { llmBindingListGet, llmBindingSet, llmBindingUnbind, llmModelListGet } from "@/apis/llm";
import type {
  LlmBindingListOut,
  LlmModelItem,
  LlmModelListOut,
  LlmSlotBindingOut,
} from "@/apis/@types/llm";
import { getResponse, getSimpleResponse } from "@/apis/base";
import { useAppStore } from "@/stores/app";
import { EnumClass, enumLabelOf, ensureEnum } from "@/utils/enums";
import i18n from "@/utils/i18n";

const appStore = useAppStore();
const unbindConfirmRef = useTemplateRef("unbindConfirmRef");

const bindingState = ref<LlmBindingListOut | null>(null);
const models = ref<LlmModelItem[]>([]);
const loading = ref(false);
const submittingSlot = ref<string | null>(null);
// selection 各档位下拉当前选中的模型 ID（null 表示未选择）
const selection = ref<Record<string, number | null>>({});
// unbindTarget 当前待解绑的档位行（确认对话框正文用）
const unbindTarget = ref<LlmSlotBindingOut | null>(null);

onMounted(() => {
  void ensureEnum(EnumClass.llmSlot);
  void ensureEnum(EnumClass.llmModelKind);
  void refreshAction(true);
});

// rows 档位行：直接采用后端返回的槽位清单（其顺序即档位清单顺序），前端不另行声明档位字面量
const rows = computed((): LlmSlotBindingOut[] => bindingState.value?.slots ?? []);

// CandidateOption 候选模型下拉项；未启用模型保留展示但不可选
interface CandidateOption {
  title: string;
  value: number;
  props?: { disabled: boolean };
}

// candidatesOf 按档位要求的类型（后端返回的 requiredKind）过滤候选模型（未启用模型禁用选择）
const candidatesOf = (row: LlmSlotBindingOut): CandidateOption[] => {
  const required = row.requiredKind;
  return models.value
    .filter((m) => m.kind === required)
    .map((m) => ({
      title: `${m.name} · ${m.model}`,
      value: m.id,
      props: { disabled: !m.enabled },
    }));
};

// canBind 选中模型且与当前绑定不同才允许提交
const canBind = (row: LlmSlotBindingOut): boolean => {
  const selected = selection.value[row.slot];
  if (selected === null || selected === undefined) {
    return false;
  }
  return !(row.bound && row.modelId === selected);
};

const notifyError = (text: string): void => {
  appStore.notify({ text, timeout: 3000, color: "error" });
};

// loadBindings 拉取四个档位的绑定摘要，并把下拉初始化为当前绑定模型
const loadBindings = async (): Promise<void> => {
  await getResponse<LlmBindingListOut>(
    llmBindingListGet(),
    async (rsp) => {
      bindingState.value = rsp.data;
      const next: Record<string, number | null> = {};
      for (const slot of rsp.data.slots ?? []) {
        next[slot.slot] = slot.bound ? slot.modelId : null;
      }
      selection.value = next;
    },
    async (rsp) => {
      notifyError(rsp.summary);
    },
  );
};

// loadModels 拉取注册表全量模型，供各档位按类型过滤候选
const loadModels = async (): Promise<void> => {
  await getResponse<LlmModelListOut>(
    llmModelListGet({ greedy: true }),
    async (rsp) => {
      models.value = rsp.data.items ?? [];
    },
    async (rsp) => {
      notifyError(rsp.summary);
    },
  );
};

// refreshAction 重新加载绑定摘要与候选模型；showLoading 为 true 时点亮刷新按钮
const refreshAction = async (showLoading = false): Promise<void> => {
  if (showLoading) {
    loading.value = true;
  }
  try {
    await Promise.all([loadBindings(), loadModels()]);
  } finally {
    loading.value = false;
  }
};

const bindAction = async (slot: string): Promise<void> => {
  const modelId = selection.value[slot];
  if (modelId === null || modelId === undefined) {
    return;
  }
  submittingSlot.value = slot;
  try {
    await getSimpleResponse(
      llmBindingSet(slot, { modelId }),
      async (rsp) => {
        await loadBindings();
        appStore.notify({
          text: rsp.summary || i18n.global.t("notice.llmBinding.bindSuccess"),
          timeout: 3000,
          color: "success",
        });
      },
      async (rsp) => {
        // 绑定失败原因（类型不匹配 / 模型未启用 / 模型不存在）由后端以英文错误返回
        notifyError(rsp.summary);
        await loadBindings();
      },
    );
  } finally {
    submittingSlot.value = null;
  }
};

const openUnbindConfirm = (row: LlmSlotBindingOut): void => {
  unbindTarget.value = row;
  unbindConfirmRef.value?.open({});
};

const unbindAction = async (): Promise<void> => {
  const target = unbindTarget.value;
  if (!target) {
    return;
  }
  submittingSlot.value = target.slot;
  try {
    await getSimpleResponse(
      llmBindingUnbind(target.slot),
      async (rsp) => {
        await loadBindings();
        appStore.notify({
          text: rsp.summary || i18n.global.t("notice.llmBinding.unbindSuccess"),
          timeout: 3000,
          color: "success",
        });
      },
      async (rsp) => {
        notifyError(rsp.summary);
        await loadBindings();
      },
    );
  } finally {
    submittingSlot.value = null;
  }
};
</script>

<style scoped>
/* 候选模型下拉：限定最小/最大宽度，避免模型名过长挤压操作按钮 */
.slot-select {
  min-width: var(--zk-select-min-width);
  max-width: var(--zk-select-max-width);
}
</style>
