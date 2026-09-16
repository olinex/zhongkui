<template>
  <div>
    <!-- 无法解析或未知协议时回退到原始 JSON chip -->
    <json-chip v-if="!decoded" :value="headers" color="info" density="compact" />
    <!-- 已知协议：显示一行可读摘要，点击打开逐字段说明表格 -->
    <div v-else class="stack-header" @click="dialog = true">
      <div class="stack-header-primary font-mono text-body-2" :title="summary?.primary">
        {{ summary?.primary }}
      </div>
      <div
        v-if="summary?.secondary"
        class="stack-header-secondary text-caption"
        :title="summary?.secondary"
      >
        {{ summary?.secondary }}
      </div>
    </div>

    <v-dialog v-model="dialog" max-width="760">
      <v-card>
        <v-toolbar density="compact">
          <v-toolbar-title class="text-body-2 text-no-wrap text-truncate">
            {{ title }}
          </v-toolbar-title>
          <v-spacer />
          <v-btn
            variant="text"
            size="small"
            prepend-icon="mdi-code-json"
            @click="showRaw = !showRaw"
          >
            {{ $t("model.networkStack.header.rawJson") }}
          </v-btn>
        </v-toolbar>
        <v-card-text>
          <VueJsonPretty v-if="showRaw" :data="parsed" />
          <template v-else>
            <v-alert v-if="isLoopback" type="info" density="compact" class="mb-2">
              {{ $t("model.networkStack.header.loopbackNote") }}
            </v-alert>
            <v-table density="compact">
              <thead>
                <tr>
                  <th style="width: 40%">
                    {{ $t("model.networkStack.header.col.field") }}
                  </th>
                  <th style="width: 30%">
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
        </v-card-text>
        <v-card-actions>
          <v-btn @click="close">
            {{ $t("action.close") }}
          </v-btn>
          <v-btn color="info" @click="copy">
            {{ $t("action.copy") }}
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
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
.stack-header {
  display: inline-block;
  max-width: 100%;
  padding: 2px 8px;
  border-radius: 4px;
  cursor: pointer;
  line-height: 1.35;
}

.stack-header:hover {
  background-color: rgb(var(--v-theme-surface-variant));
}

.stack-header-primary {
  color: rgb(var(--v-theme-on-surface));
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 摘要第二行：信息片段（协议 · TTL · 标志 · 长度）较多，窄列下允许换行，
   长 token（MAC/IPv6/端口列表）用 overflow-wrap 断行，保证信息完整可见 */
.stack-header-secondary {
  color: rgb(var(--v-theme-on-surface-variant));
  white-space: normal;
  overflow-wrap: anywhere;
}
</style>
