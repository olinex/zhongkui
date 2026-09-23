<template>
  <!-- .zk-flat-panels 提供分组面板的扁平外观（透明背景 / 去阴影 / 淡色 hover） -->
  <div class="pa-4 zk-flat-panels">
    <!-- 说明以 banner 常驻提示；待保存计数与操作控件收在其尾部，仍保持单行布局 -->
    <v-alert type="info" class="mb-3">
      {{ $t("page.agentParamAdmin.hint") }}
      <template #append>
        <div class="d-flex align-center">
          <v-chip v-if="dirtyItems.length > 0" color="primary" class="me-2">
            {{ $t("page.agentParamAdmin.dirtyCount", { count: dirtyItems.length }) }}
          </v-chip>
          <v-btn
            icon="mdi-refresh"
            variant="text"
            size="small"
            :loading="loading"
            :title="$t('action.refresh')"
            :aria-label="$t('action.refresh')"
            @click="loadParamsAction(true)"
          />
          <v-btn
            color="primary"
            variant="tonal"
            size="small"
            class="ms-2"
            :loading="saving"
            :disabled="dirtyItems.length === 0"
            @click="saveAction"
          >
            {{ $t("action.save") }}
          </v-btn>
        </div>
      </template>
    </v-alert>

    <!-- 扩展面板为手风琴式：同一时刻只展开一个分组（multiple 会突破该语义，禁止声明） -->
    <v-expansion-panels v-model="expandedGroup" class="param-groups">
      <v-expansion-panel v-for="group in groups" :key="group" :value="group">
        <v-expansion-panel-title>
          {{ groupLabel(group) }}
          <span class="zk-hint ms-2">
            {{ $t("page.agentParamAdmin.groupCount", { count: groupItems(group).length }) }}
          </span>
          <!-- actions 槽会顶替默认展开图标，故自行渲染 chevron；按钮需 stop 以免连带切换分组 -->
          <template #actions="{ expanded }">
            <v-btn
              variant="text"
              size="small"
              color="error"
              :disabled="!hasOverride(group)"
              @click.stop="openResetGroupConfirm(group)"
            >
              {{ $t("action.restoreDefault") }}
            </v-btn>
            <v-icon
              size="small"
              :icon="expanded ? 'mdi-chevron-up' : 'mdi-chevron-down'"
              class="ms-1"
            />
          </template>
        </v-expansion-panel-title>

        <v-expansion-panel-text>
          <div v-for="item in groupItems(group)" :key="item.key" class="param-item zk-hairline-top">
            <div class="d-flex align-center mb-1">
              <span class="text-body-2 text-on-surface font-weight-500">{{ item.label }}</span>
              <!-- 参数键与出厂值放进 popover：tooltip 离开 activator 即收起，承载不了可点击的复制按钮 -->
              <v-menu
                location="top"
                :open-on-hover="true"
                :close-delay="300"
                :close-on-content-click="false"
              >
                <template #activator="{ props: menuProps }">
                  <v-icon
                    v-bind="menuProps"
                    icon="mdi-information-outline"
                    size="x-small"
                    color="primary"
                    class="ms-1"
                  />
                </template>
                <!-- 参数键对使用者是噪音，但对运维需与 fixture / 数据库对齐，故保留并提供一键复制 -->
                <!-- 须用 v-sheet 承载：VMenu 仅对其直接子元素 .v-card/.v-sheet/.v-list 上背景与阴影 -->
                <v-sheet class="pa-3 param-tip">
                  <div class="d-flex align-center mb-1">
                    <span class="param-tip-key font-mono">{{ item.key }}</span>
                    <copy-btn :value="item.key" color="primary" density="compact" class="ms-1" />
                  </div>
                  <div class="default-value-tip">
                    {{
                      $t("page.agentParamAdmin.defaultValue", { value: formatValue(item.default) })
                    }}
                  </div>
                </v-sheet>
              </v-menu>
              <v-chip v-if="item.overridden" size="x-small" color="primary" class="ms-2">
                {{ $t("model.agentParam.field.overridden") }}
              </v-chip>
              <v-spacer />
              <v-btn
                variant="text"
                size="small"
                color="primary"
                :disabled="!item.overridden"
                @click="resetKeyAction(item)"
              >
                {{ $t("action.reset") }}
              </v-btn>
            </div>
            <div v-if="item.comment" class="zk-hint mb-1">
              {{ item.comment }}
            </div>

            <!-- 值控件按参数类型渲染：多行文本域不 trim，单行文本提交前 trim -->
            <v-textarea
              v-if="item.kind === 'text'"
              v-model="drafts[item.key]"
              rows="2"
              auto-grow
              hide-details
            />
            <!-- 后端注册表声明了封闭取值清单的字符串参数（如 agent.defaultSlot）渲染为下拉框，
                 避免手工输入出清单外的值（写入接口同样会拒绝）；
                 草稿值为跨类型联合（string | number | boolean），故拆开 model-value 与 update 事件绑定 -->
            <v-select
              v-else-if="item.kind === 'string' && item.options?.length"
              :model-value="String(drafts[item.key] ?? '')"
              :items="optionsOf(item)"
              hide-details
              @update:model-value="drafts[item.key] = $event"
            />
            <v-text-field
              v-else-if="item.kind === 'string'"
              v-model="drafts[item.key]"
              hide-details
            />
            <v-text-field
              v-else-if="item.kind === 'int' || item.kind === 'float'"
              v-model.number="drafts[item.key]"
              type="number"
              :step="item.kind === 'float' ? 0.1 : 1"
              hide-details
            />
            <v-switch
              v-else-if="item.kind === 'bool'"
              v-model="drafts[item.key]"
              color="primary"
              density="compact"
              hide-details
            />
          </div>
        </v-expansion-panel-text>
      </v-expansion-panel>
    </v-expansion-panels>
  </div>

  <!-- 整组恢复出厂值确认对话框 -->
  <confirm-dialog
    ref="resetGroupConfirmRef"
    :title="$t('model.agentParam.name')"
    prepend-icon="mdi-restore"
    @confirm="resetGroupAction"
  >
    <template #body>
      <confirm-alert
        :action="$t('action.restoreDefault')"
        :text="
          $t('notice.agentParam.resetGroupConfirm', {
            key: groupLabel(resetGroupTarget),
          })
        "
      />
    </template>
  </confirm-dialog>
</template>

<script setup lang="ts" name="agent-param-panel">
import { llmParamListGet, llmParamReset, llmParamSet } from "@/apis/llm";
import type { LlmParamItemOut, LlmParamListOut } from "@/apis/@types/llm";
import { getResponse, getSimpleResponse } from "@/apis/base";
import { useAppStore } from "@/stores/app";
import { ensureEnum, enumLabelOf } from "@/utils/enums";
import i18n from "@/utils/i18n";

const props = defineProps<{
  /** 该 tab 是否处于激活态（面板挂载后常驻，需据此在离开时重置展开状态） */
  active: boolean;
}>();

// PARAM_GROUP_ORDER 参数分组的固定展示顺序（前端固定，不依赖后端返回顺序）
const PARAM_GROUP_ORDER = ["agent", "retrieval", "session", "chunking", "eval", "sanitize"];

const appStore = useAppStore();
const resetGroupConfirmRef = useTemplateRef("resetGroupConfirmRef");

const items = ref<LlmParamItemOut[]>([]);
// drafts 各参数的编辑草稿值（按参数键索引），提交时按参数类型归一化
const drafts = ref<Record<string, string | number | boolean>>({});
// expandedGroup 当前展开的分组名（v-expansion-panels 的 v-model）：面板为手风琴式，
// 同一时刻只允许展开一个分组，故为单值而非列表，空串表示全部分组收起
const expandedGroup = ref<string>("");

// 面板经 tab 切换仅隐藏（组件常驻），故显式监听激活态：
// 离开该 tab 时收起全部分组，避免下次进入时残留上次的展开状态
watch(
  () => props.active,
  (isActive, wasActive) => {
    if (wasActive && !isActive) {
      expandedGroup.value = "";
    }
  },
);
// resetGroupTarget 当前待整组恢复出厂值的分组名（确认对话框正文用）
const resetGroupTarget = ref("");
const loading = ref(false);
const saving = ref(false);

onMounted(() => {
  void loadParamsAction(true);
});

// groups 分组列表：按前端固定顺序排列，未登记的分组按出现顺序追加在末尾
const groups = computed((): string[] => {
  const seen = new Set<string>();
  const ordered: string[] = [];
  for (const group of PARAM_GROUP_ORDER) {
    if (items.value.some((i) => i.group === group)) {
      seen.add(group);
      ordered.push(group);
    }
  }
  for (const item of items.value) {
    if (!seen.has(item.group)) {
      seen.add(item.group);
      ordered.push(item.group);
    }
  }
  return ordered;
});

const groupItems = (group: string): LlmParamItemOut[] =>
  items.value.filter((i) => i.group === group);

// groupLabel 分组展示名：翻译缺失时回退为分组原始名
const groupLabel = (group: string): string => {
  const key = `model.agentParam.type.${group}`;
  const label = i18n.global.t(key);
  return label === key ? group : label;
};

const hasOverride = (group: string): boolean => groupItems(group).some((i) => i.overridden);

// optionsOf 构造封闭取值清单的下拉项：参数声明了枚举类时以该类条目的本地化 label 展示
// （取值与文案均来自后端），未声明时字面量本身即展示文案（如 retrieval.fusion）
const optionsOf = (item: LlmParamItemOut): Array<{ title: string; value: string }> =>
  (item.options ?? []).map((value) => ({
    title: item.enumType ? enumLabelOf(item.enumType, value) : value,
    value,
  }));

// formatValue 参数值展示：空值显示为占位文案，布尔值显示为本地化的是/否
const formatValue = (value: unknown): string => {
  if (value === null || value === undefined || value === "") {
    return i18n.global.t("model.__base.value.blank");
  }
  if (typeof value === "boolean") {
    return i18n.global.t(value ? "model.__base.value.true" : "model.__base.value.false");
  }
  return String(value);
};

// draftFromValue 由参数当前值构造草稿初值（按类型归一化，便于与提交值比较）
const draftFromValue = (item: LlmParamItemOut): string | number | boolean => {
  switch (item.kind) {
    case "int":
    case "float": {
      const num = Number(item.value);
      return Number.isFinite(num) ? num : 0;
    }
    case "bool":
      return item.value === true || String(item.value) === "true";
    default:
      return item.value === null || item.value === undefined ? "" : String(item.value);
  }
};

// normalizeDraft 提交前归一化草稿值：int/float 转数字，bool 转布尔，
// string 去首尾空白（多行 text 保留原样）
const normalizeDraft = (item: LlmParamItemOut): string | number | boolean => {
  const raw = drafts.value[item.key];
  switch (item.kind) {
    case "int": {
      const num = Number(raw);
      return Number.isFinite(num) ? Math.trunc(num) : 0;
    }
    case "float": {
      const num = Number(raw);
      return Number.isFinite(num) ? num : 0;
    }
    case "bool":
      return raw === true || String(raw) === "true";
    case "text":
      return raw === null || raw === undefined ? "" : String(raw);
    default:
      return String(raw ?? "").trim();
  }
};

// isDirty 判定单个参数是否已改动（与当前生效值比较）
const isDirty = (item: LlmParamItemOut): boolean => {
  const draft = normalizeDraft(item);
  const current = item.value;
  if (typeof draft === "boolean" || typeof current === "boolean") {
    return Boolean(draft) !== Boolean(current);
  }
  if (typeof draft === "number" || typeof current === "number") {
    return Number(draft) !== Number(current);
  }
  return String(draft ?? "") !== String(current ?? "");
};

const dirtyItems = computed((): LlmParamItemOut[] => items.value.filter(isDirty));

const notifyError = (text: string): void => {
  appStore.notify({ text, timeout: 3000, color: "error" });
};

// loadParamsAction 拉取参数元信息与当前生效值，并重建草稿与分组折叠态
const loadParamsAction = async (showLoading = false): Promise<void> => {
  if (showLoading) {
    loading.value = true;
  }
  try {
    await getResponse<LlmParamListOut>(
      llmParamListGet(),
      async (rsp) => {
        items.value = rsp.data.items ?? [];
        const nextDrafts: Record<string, string | number | boolean> = {};
        for (const item of items.value) {
          nextDrafts[item.key] = draftFromValue(item);
          // 枚举类取值的选项文案由枚举 store 提供，先确保已按当前语言缓存
          if (item.enumType) {
            void ensureEnum(item.enumType);
          }
        }
        drafts.value = nextDrafts;
      },
      async (rsp) => {
        notifyError(rsp.summary);
      },
    );
  } finally {
    loading.value = false;
  }
};

// saveAction 提交全部改动的参数（空串等价于把该键写回出厂值）
const saveAction = async (): Promise<void> => {
  const changed = dirtyItems.value;
  if (changed.length === 0) {
    return;
  }
  saving.value = true;
  try {
    await getSimpleResponse(
      llmParamSet({
        items: changed.map((item) => ({ key: item.key, value: normalizeDraft(item) })),
      }),
      async (rsp) => {
        await loadParamsAction();
        appStore.notify({
          text: rsp.summary || i18n.global.t("notice.agentParam.saveSuccess"),
          timeout: 3000,
          color: "success",
        });
      },
      async (rsp) => {
        notifyError(rsp.summary);
      },
    );
  } finally {
    saving.value = false;
  }
};

// resetKeyAction 把单个参数写回出厂值（default_value）
const resetKeyAction = async (item: LlmParamItemOut): Promise<void> => {
  await getSimpleResponse(
    llmParamReset({ keys: [item.key] }),
    async (rsp) => {
      await loadParamsAction();
      appStore.notify({
        text: rsp.summary || i18n.global.t("notice.agentParam.resetSuccess"),
        timeout: 3000,
        color: "success",
      });
    },
    async (rsp) => {
      notifyError(rsp.summary);
    },
  );
};

const openResetGroupConfirm = (group: string): void => {
  resetGroupTarget.value = group;
  resetGroupConfirmRef.value?.open({});
};

// resetGroupAction 把整组参数写回出厂值（default_value）
const resetGroupAction = async (): Promise<void> => {
  const group = resetGroupTarget.value;
  if (!group) {
    return;
  }
  await getSimpleResponse(
    llmParamReset({ group }),
    async (rsp) => {
      await loadParamsAction();
      appStore.notify({
        text: rsp.summary || i18n.global.t("notice.agentParam.resetSuccess"),
        timeout: 3000,
        color: "success",
      });
    },
    async (rsp) => {
      notifyError(rsp.summary);
    },
  );
};
</script>

<style scoped>
/* 分组展开面板：扁平外观（透明背景 / 去阴影 / hover 高亮）由全局 .zk-flat-panels 提供，
   此处只保留布局定位 */

/* 展开时不再浮起：取消默认的 16px 间距，并保持分隔线始终可见 */
.param-groups :deep(.v-expansion-panel:not(:first-child)),
.param-groups :deep(.v-expansion-panel + .v-expansion-panel) {
  margin-top: 0;
}

.param-groups :deep(.v-expansion-panel:not(:first-child)::after) {
  opacity: 1;
}

/* 标题去掉左右内缩并固定高度，使分组名与参数项左边缘对齐、展开时不跳动
   （权重需高于 Vuetify 展开态标题的 64px 规则，故带上 button 元素选择器；
   高度取紧凑工具栏令牌，足够容纳「恢复默认」按钮，折叠/展开两种状态一致） */
.param-groups :deep(.v-expansion-panel > button.v-expansion-panel-title) {
  min-height: var(--zk-toolbar-height-compact);
  padding: var(--zk-space-1) 0;
}

/* stylelint-disable-next-line selector-class-pattern -- Vuetify 内部类名 */
.param-groups :deep(.v-expansion-panel-text__wrapper) {
  padding: 0 0 var(--zk-space-2);
}

.param-item + .param-item {
  margin-top: var(--zk-space-2);
  padding-top: var(--zk-space-2);
}

.param-tip {
  max-width: var(--zk-popover-max-width);
}

.param-tip-key {
  overflow-wrap: anywhere;
}

/* tooltip 内的出厂值：保留原始换行与缩进，超长时限制高度可滚动 */
.default-value-tip {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  max-height: var(--zk-popover-max-height);
  overflow-y: auto;
}
</style>
