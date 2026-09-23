<template>
  <div>
    <!-- 无法解析或未知协议时回退到原始 JSON chip -->
    <json-chip v-if="!decoded" :value="headers" color="info" density="compact" />
    <!-- 已知协议：显示一行可读摘要，点击打开逐字段说明表格 -->
    <div v-else class="zk-stack-header" @click="dialog = true">
      <div class="zk-stack-header-primary font-mono text-body-2" :title="summary?.primary">
        {{ summary?.primary }}
      </div>
      <div
        v-if="summary?.secondary"
        class="zk-stack-header-secondary text-caption"
        :title="summary?.secondary"
      >
        {{ summary?.secondary }}
      </div>
    </div>

    <app-dialog v-model="dialog" :title="title" size="md">
      <template #header-append>
        <v-spacer />
        <v-btn variant="text" size="small" prepend-icon="mdi-code-json" @click="showRaw = !showRaw">
          {{ $t("model.networkStack.header.rawJson") }}
        </v-btn>
      </template>
      <VueJsonPretty v-if="showRaw" :data="parsed" />
      <template v-else>
        <v-alert v-if="isLoopback" type="info" class="mb-2">
          {{ $t("model.networkStack.header.loopbackNote") }}
        </v-alert>
        <v-table density="compact">
          <thead>
            <tr>
              <th class="col-field">
                {{ $t("model.networkStack.header.col.field") }}
              </th>
              <th class="col-value">
                {{ $t("model.networkStack.header.col.value") }}
              </th>
              <th>
                {{ $t("model.networkStack.header.col.desc") }}
              </th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="row in rows" :key="row.key">
              <td>
                <div class="text-body-2">{{ row.label }}</div>
                <div class="font-mono text-caption text-on-surface-variant">{{ row.key }}</div>
              </td>
              <td class="font-mono text-body-2">{{ row.value }}</td>
              <td class="text-body-2 text-on-surface-variant">{{ row.desc }}</td>
            </tr>
          </tbody>
        </v-table>
      </template>
      <template #footer>
        <v-btn @click="close">
          {{ $t("action.close") }}
        </v-btn>
        <v-btn color="info" @click="copy">
          {{ $t("action.copy") }}
        </v-btn>
      </template>
    </app-dialog>
  </div>
</template>

<script setup lang="ts" name="network-stack-header">
import VueJsonPretty from "vue-json-pretty";
import "vue-json-pretty/lib/styles.css";

import i18n from "@/utils/i18n";
import { copyToClipboard } from "@/utils/clipboard";
import { HEADER_FIELDS, buildHeaderSummary, formatHeaderValue } from "./networkStackHeader";

const props = withDefaults(
  defineProps<{
    headers: string;
    proto: number;
    title?: string;
  }>(),
  { title: "" },
);

const dialog = ref(false);
const showRaw = ref(false);

// parsed 解析 headers JSON 为对象；非对象（数组/标量/非法 JSON）视为不可解析
const parsed = computed<Record<string, unknown> | null>(() => {
  const raw = props.headers.trim();
  if (raw === "") {
    return null;
  }
  try {
    const obj = JSON.parse(raw);
    return obj && typeof obj === "object" && !Array.isArray(obj)
      ? (obj as Record<string, unknown>)
      : null;
  } catch {
    return null;
  }
});

const isLoopback = computed(() => props.proto === 54);

const summary = computed(() =>
  parsed.value ? buildHeaderSummary(props.proto, parsed.value) : null,
);

// decoded 判定是否走可读化展示：已知协议（Loopback 无字段但需展示说明）且解析成功
const decoded = computed(() => !!parsed.value && !!summary.value);

const rows = computed(() => {
  if (!parsed.value) {
    return [];
  }
  const out: Array<{ key: string; label: string; value: string; desc: string }> = [];
  for (const meta of HEADER_FIELDS[props.proto] ?? []) {
    const value = parsed.value[meta.key];
    if (value === undefined) {
      continue;
    }
    out.push({
      key: meta.key,
      label: i18n.global.t(`model.networkStack.header.field.${meta.key}`),
      value: formatHeaderValue(meta.kind, value),
      desc: i18n.global.t(`model.networkStack.header.desc.${meta.key}`),
    });
  }
  return out;
});

const close = (): void => {
  dialog.value = false;
};

const copy = async (): Promise<void> => {
  await copyToClipboard(props.headers);
  close();
};
</script>

<style scoped>
/* 表头列宽提示：字段列/值列限定宽度，说明列自适应占满剩余空间 */
.col-field {
  width: 40%;
}

.col-value {
  width: 30%;
}
</style>
