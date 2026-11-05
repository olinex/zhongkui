<template>
  <main-layout current-nav="ai">
    <template #body>
      <!-- 本页由引用溯源 / 系统知识库列表以新标签页打开，未登记在侧边栏导航中，故路由守卫
           不做功能门禁；页面自行判定 AI 能力是否启用，未启用时不发起任何详情请求。 -->
      <v-empty-state
        v-if="!aiEnabled"
        icon="mdi-lock-outline"
        color="warning"
        :title="$t('page.aiAssistant.disabled')"
      >
        <template #actions>
          <v-btn variant="text" prepend-icon="mdi-arrow-left" @click="goBackPrevious">
            {{ $t("action.backPrevious") }}
          </v-btn>
        </template>
      </v-empty-state>

      <template v-else>
        <!-- 顶部返回：回到进入本页前所处的页面，无历史可回退时退化为知识库列表 -->
        <div class="d-flex align-center mb-4">
          <v-btn variant="text" prepend-icon="mdi-arrow-left" @click="goBackPrevious">
            {{ $t("action.backPrevious") }}
          </v-btn>
        </div>

        <v-skeleton-loader v-if="loading" type="article" min-height="220" />

        <!-- 不可读态：文档不存在 / 不在可读范围 / 无权限时后端返回统一错误封套 -->
        <v-alert v-else-if="errorText" type="error">
          {{ errorText }}
        </v-alert>

        <template v-else-if="doc">
          <!-- 标题与状态：面板头承载文档标题与处理状态 chip -->
          <section class="zk-panel">
            <div class="section-head">
              <div class="zk-section-title">{{ doc.title }}</div>
              <v-chip :color="statusColor(doc.status)" size="small">
                {{ enumLabelOf(EnumClass.knowledgeDocStatus, doc.status) }}
              </v-chip>
            </div>
            <div class="zk-hairline-top">
              <v-table density="compact">
                <tbody>
                  <tr v-for="field in metaFields" :key="field.key">
                    <td class="field-label-col text-no-wrap font-weight-medium">
                      {{ field.label }}
                    </td>
                    <td>
                      <hex-string-chip
                        v-if="field.key === 'hash' && doc.hash"
                        :value="doc.hash"
                        color="info"
                        :length="10"
                      />
                      <span v-else class="text-body-medium">{{ field.display }}</span>
                    </td>
                  </tr>
                </tbody>
              </v-table>
            </div>
          </section>

          <!-- 正文只读区块：等宽展示，沿用 .zk-code-preview 的底色 / 换行 / 断词 -->
          <section class="zk-panel content-section">
            <div class="section-head">
              <div class="zk-section-title">{{ $t("model.aiDocument.field.content") }}</div>
            </div>
            <div class="zk-hairline-top">
              <pre class="zk-code-preview doc-content font-mono text-body-small">{{ doc.content }}</pre>
            </div>
          </section>

          <!-- 分块明细：只读复用分块视图组件，请求接口按来源命名空间切换 -->
          <knowledge-chunks-view
            :documentID="docID"
            :chunks-request="chunksRequest"
            class="chunks-section"
          />
        </template>
      </template>
    </template>
  </main-layout>
</template>

<route lang="yaml">
name: AI Document Detail
meta:
  title: title.aiDocumentDetail
  requiresAuth: true
  requiresPerm: true
</route>

<script setup lang="ts" name="ai-document-detail">
import { DateTime } from "luxon";
import type { AxiosPromise } from "axios";

import {
  knowledgeDocumentChunksGet,
  knowledgeDocumentDetailGet,
  knowledgeSystemDocumentChunksGet,
  knowledgeSystemDocumentDetailGet,
} from "@/apis/ai";
import { getResponse } from "@/apis/base";

import type { KnowledgeDocumentDetail } from "@/apis/@types/ai";
import router from "@/router";
import { useAppStore } from "@/stores/app";
import { FEATURE_AI, useFeaturesStore } from "@/stores/features";
import { isSystemKnowledgeSource } from "@/utils/ai";
import { EnumClass, enumLabelOf, ensureEnum } from "@/utils/enums";
import i18n, { localeDatetimeFormat } from "@/utils/i18n";

const route = useRoute();
const appStore = useAppStore();
const featuresStore = useFeaturesStore();

// AI 能力可用性：本页未经路由守卫做功能门禁，须自行判定；不可用时不发起详情请求
const aiEnabled = computed<boolean>(() => featuresStore.isEnable(FEATURE_AI));

const docID = computed<number>(() => Number(route.params.id));
// 来源命名空间取自跳转链接的 query（系统知识库 / 引用溯源均携带），缺省为空
const namespaceParam = computed<string>(() => String(route.query.namespace ?? ""));

// 来源判定：命名空间非空且不等于当前活动命名空间即系统共享来源，据此选择只读接口。
// 系统命名空间由后端常量定义，前端以命名空间比较判定，禁止硬编码 "system"
const systemSource = computed<boolean>(() =>
  isSystemKnowledgeSource(namespaceParam.value, appStore.namespace),
);

// 详情 / 分块请求按来源切换：系统来源走只读系统接口（数据范围恒为系统命名空间）
const detailRequest = (id: number): AxiosPromise =>
  systemSource.value ? knowledgeSystemDocumentDetailGet(id) : knowledgeDocumentDetailGet(id);
const chunksRequest = (id: number): AxiosPromise =>
  systemSource.value ? knowledgeSystemDocumentChunksGet(id) : knowledgeDocumentChunksGet(id);

const doc = ref<KnowledgeDocumentDetail | null>(null);
const loading = ref(false);
const errorText = ref("");

// 处理状态徽标语义色（未登记状态回退默认色），与知识库列表同一展示映射
const STATUS_COLOR: Record<string, string | undefined> = {
  pending: undefined,
  parsing: "info",
  ready: "success",
  failed: "error",
};
const statusColor = (status: string | undefined): string | undefined => STATUS_COLOR[status ?? ""];

const blankText = computed<string>(() => i18n.global.t("model.__base.value.blank"));

// datetimeText 时间字段展示：Go 零值 / 空值时回退空白占位，避免展示公元 1 年
const datetimeText = (value: string | null): string => {
  if (!value) {
    return blankText.value;
  }
  const dt = DateTime.fromISO(value);
  if (!dt.isValid || dt.year <= 1) {
    return blankText.value;
  }
  return localeDatetimeFormat(dt);
};

// formatBytes 将字节数格式化为人类可读文本（1024 进制，保留一位小数）；无原件（0 字节）回退空白
const formatBytes = (value: number): string => {
  const bytes = Number(value ?? 0);
  if (!Number.isFinite(bytes) || bytes <= 0) {
    return blankText.value;
  }
  const units = ["B", "KB", "MB", "GB", "TB"];
  let size = bytes;
  let unit = 0;
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024;
    unit += 1;
  }
  const text = unit === 0 ? String(size) : size.toFixed(1);
  return `${text} ${units[unit]}`;
};

interface MetaField {
  key: string;
  label: string;
  display: string;
}

// 元信息字段行：标签取 model.aiDocument.field.*，空值一律回退空白占位
const metaFields = computed<MetaField[]>(() => {
  const item = doc.value;
  if (!item) {
    return [];
  }
  const label = (key: string): string => i18n.global.t(`model.aiDocument.field.${key}`);
  return [
    {
      key: "format",
      label: label("format"),
      display: enumLabelOf(EnumClass.documentFormat, item.format) || blankText.value,
    },
    { key: "sizeBytes", label: label("sizeBytes"), display: formatBytes(item.sizeBytes) },
    { key: "chunkCount", label: label("chunkCount"), display: String(item.chunkCount) },
    { key: "useCount", label: label("useCount"), display: String(item.useCount) },
    { key: "lastUsedAt", label: label("lastUsedAt"), display: datetimeText(item.lastUsedAt) },
    { key: "createdAt", label: label("createdAt"), display: datetimeText(item.createdAt) },
    { key: "updatedAt", label: label("updatedAt"), display: datetimeText(item.updatedAt) },
    { key: "hash", label: label("hash"), display: blankText.value },
    {
      key: "originalName",
      label: label("originalName"),
      display: item.originalName || blankText.value,
    },
    { key: "parseError", label: label("parseError"), display: item.parseError || blankText.value },
  ];
});

// loadDetail 拉取详情：id 非正时不请求；失败（不存在 / 不在可读范围 / 无权限）时以
// 后端统一错误封套的 summary 提示，缺失时回退到本地化不可读文案
const loadDetail = (): void => {
  const id = docID.value;
  if (!aiEnabled.value) {
    return;
  }
  if (!id || id <= 0) {
    doc.value = null;
    errorText.value = "";
    loading.value = false;
    return;
  }
  void ensureEnum(EnumClass.knowledgeDocStatus);
  void ensureEnum(EnumClass.documentFormat);
  doc.value = null;
  errorText.value = "";
  loading.value = true;
  void getResponse<KnowledgeDocumentDetail>(
    detailRequest(id),
    async (rsp) => {
      doc.value = rsp.data ?? null;
      loading.value = false;
    },
    async (rsp) => {
      loading.value = false;
      errorText.value = rsp.summary || i18n.global.t("page.aiKnowledge.documentUnreadable");
    },
  );
};

watch([docID, systemSource, aiEnabled], loadDetail, { immediate: true });

// goBackPrevious 返回上一级：有历史可回退时回退，直接以 URL 打开本页（新标签页）无历史，
// 退化为回到知识库列表，避免按钮无响应或把用户带出应用
const goBackPrevious = (): void => {
  const state = window.history.state as { back?: string | null } | null;
  if (state?.back) {
    router.back();
    return;
  }
  router.push({ name: "AI Knowledge List", params: { language: appStore.language } });
};
</script>

<style scoped>
/* 仅布局定位：外观一律走 .zk-* 外壳类与 Vuetify 排版类 */

/* 面板头：内边距与盒内平铺内容的左起对齐（表格单元格 / 正文均为 16px） */
.section-head {
  display: flex;
  align-items: center;
  gap: var(--zk-space-2);
  padding: var(--zk-space-3) var(--zk-space-4);
}

/* 元信息标签列宽提示：固定标签列宽度，值列自适应占满剩余空间 */
.field-label-col {
  width: 180px;
}

/* 正文 / 分块区块：与标题面板上下留白，形成独立分区 */
.content-section,
.chunks-section {
  margin-top: var(--zk-space-4);
}

/* 正文只读展示：等宽走 .font-mono，底色 / 换行 / 断词走 .zk-code-preview；
   此处仅按面板头同样的左右留白内缩并限高滚动 */
.doc-content {
  margin: var(--zk-space-3) var(--zk-space-4) var(--zk-space-4);
  max-height: var(--zk-preview-max-height);
  overflow: auto;
}
</style>
