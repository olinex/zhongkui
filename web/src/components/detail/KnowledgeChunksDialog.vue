<template>
  <!-- 知识库分块明细对话框：展示文档级索引摘要（分块统计 / 切片参数 / 向量模型与维度 /
       索引新鲜度）与逐块构建明细。分块行点击展开正文，结构单元 chip 的取值与文案由后端
       枚举类 chunkUnitKind 下发；全部信息由后端读时派生，前端不做二次推算。 -->
  <app-dialog
    :model-value="modelValue"
    :title="$t('page.aiKnowledge.chunks')"
    icon="mdi-file-document-multiple-outline"
    size="lg"
    :subtitle="documentTitle"
    @update:model-value="onUpdate"
  >
    <v-skeleton-loader v-if="loading" type="article" min-height="220" />
    <v-alert v-else-if="errorText" type="error">
      {{ errorText }}
    </v-alert>
    <template v-else-if="loaded">
      <!-- 索引摘要：面板头（区块标题 + 状态 chip）+ 盒内平铺字段表。
           文档标题已作为对话框标题的次级标题给出，此处不再重复成行 -->
      <section class="zk-panel">
        <div class="section-head">
          <div class="zk-section-title">{{ $t("page.aiKnowledge.chunkSummary") }}</div>
          <v-chip :color="indexed ? 'success' : 'warning'" size="small">
            {{
              indexed ? $t("page.aiKnowledge.chunkIndexed") : $t("page.aiKnowledge.chunkNotIndexed")
            }}
          </v-chip>
          <v-chip v-if="stale" color="warning" size="small">
            {{ $t("page.aiKnowledge.chunkStale") }}
          </v-chip>
        </div>
        <div class="zk-hairline-top">
          <v-table density="compact">
            <tbody>
              <tr v-for="field in summaryFields" :key="field.key">
                <td class="field-label-col text-no-wrap font-weight-medium">
                  {{ field.label }}
                </td>
                <td>
                  <hex-string-chip
                    v-if="field.key === 'hash' && hash"
                    :value="hash"
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

      <!-- 分块列表：沿用与索引摘要同一「面板头 + 盒内平铺条目」结构，
           行内给出序号 / 标题路径 / 结构单元 / 字符数与行数，点击展开正文 -->
      <section class="zk-panel chunk-section">
        <div class="section-head">
          <div class="zk-section-title">{{ $t("page.aiKnowledge.chunkList") }}</div>
          <v-chip size="small">{{ chunks.length }}</v-chip>
        </div>
        <div class="zk-hairline-top">
          <v-alert
            v-if="chunks.length === 0"
            type="info"
            :text="$t('page.aiKnowledge.chunkNotIndexedHint')"
            class="chunk-empty"
          />
          <div v-else class="chunk-list">
            <div
              v-for="(chunk, index) in chunks"
              :key="chunk.index"
              class="chunk-item"
              :class="{ 'zk-hairline-top': index > 0 }"
            >
              <div
                class="chunk-row zk-row-hover"
                :class="{ 'zk-row-selected': expanded.has(chunk.index) }"
                @click="toggleChunk(chunk.index)"
              >
                <span class="chunk-index font-mono text-body-small">#{{ chunk.index }}</span>
                <span class="chunk-path text-body-medium">
                  {{ chunk.headingPath || blankText }}
                </span>
                <v-chip v-for="kind in chunk.kinds" :key="kind" size="small">
                  {{ enumLabelOf(EnumClass.chunkUnitKind, kind) }}
                </v-chip>
                <v-spacer />
                <span class="text-body-small on-surface-variant">
                  {{ chunkMetaTextOf(chunk) }}
                </span>
                <v-icon
                  :icon="expanded.has(chunk.index) ? 'mdi-chevron-up' : 'mdi-chevron-down'"
                  size="small"
                />
              </div>
              <v-expand-transition>
                <pre
                  v-show="expanded.has(chunk.index)"
                  class="zk-code-preview chunk-body font-mono text-body-small"
                  >{{ chunk.content }}</pre>
              </v-expand-transition>
            </div>
          </div>
        </div>
      </section>
    </template>
  </app-dialog>
</template>

<script setup lang="ts" name="knowledge-chunks-dialog">
import { DateTime } from "luxon";

import { knowledgeDocumentChunksGet } from "@/apis/ai";
import { getResponse } from "@/apis/base";

import type { KnowledgeDocumentChunk, KnowledgeDocumentChunksOut } from "@/apis/@types/ai";
import { useAppStore } from "@/stores/app";
import { EnumClass, ensureEnum, enumLabelOf } from "@/utils/enums";
import i18n, { localeDatetimeFormat } from "@/utils/i18n";

const props = defineProps<{
  modelValue: boolean;
  documentId: number;
  documentTitle: string;
}>();

const emits = defineEmits<{
  "update:modelValue": [value: boolean];
}>();

const onUpdate = (value: boolean): void => {
  emits("update:modelValue", value);
};

const appStore = useAppStore();

const data = ref<KnowledgeDocumentChunksOut | null>(null);
const loading = ref(false);
const errorText = ref("");
// 展开的分块序号集合：一次请求返回全部块，展开态仅体现在前端
const expanded = ref(new Set<number>());

// 打开或切换文档时重新拉取明细并清空上次的展开态与错误；
// 结构单元取值的展示文案由后端枚举类下发，随语言切换重取
watch(
  () => [props.modelValue, props.documentId] as const,
  ([open, documentId]) => {
    if (!open || !documentId) {
      return;
    }
    void ensureEnum(EnumClass.chunkUnitKind);
    data.value = null;
    errorText.value = "";
    expanded.value = new Set<number>();
    loading.value = true;
    void getResponse<KnowledgeDocumentChunksOut>(
      knowledgeDocumentChunksGet(documentId),
      async (rsp) => {
        data.value = rsp.data ?? null;
        loading.value = false;
      },
      async (rsp) => {
        loading.value = false;
        errorText.value = rsp.summary ?? "";
        appStore.notify({
          text: rsp.summary,
          timeout: 3000,
          color: "error",
        });
      },
    );
  },
  { immediate: true },
);

// ---- 派生展示值：模板不对可空响应做解引用，未加载完成时取中性默认值 ----
const loaded = computed<boolean>(() => data.value !== null);
const chunks = computed<KnowledgeDocumentChunk[]>(() => data.value?.chunks ?? []);
const indexed = computed<boolean>(() => data.value?.summary.indexed ?? false);
const stale = computed<boolean>(() => data.value?.summary.stale ?? false);
const hash = computed<string>(() => data.value?.summary.hash ?? "");
const blankText = computed<string>(() => i18n.global.t("model.__base.value.blank"));

const toggleChunk = (index: number): void => {
  const set = expanded.value;
  if (set.has(index)) {
    set.delete(index);
  } else {
    set.add(index);
  }
};

// chunkMetaTextOf 单块的规模提示（字符数 · 行数），与摘要区的字符口径一致
const chunkMetaTextOf = (chunk: KnowledgeDocumentChunk): string =>
  `${i18n.global.t("page.aiKnowledge.chunkCharsValue", { count: chunk.chars })} · ` +
  i18n.global.t("page.aiKnowledge.chunkLinesValue", { count: chunk.lines });

// indexedAtText 索引时间为 Go 零值（尚未索引）时回退空白占位，避免展示公元 1 年
const indexedAtText = (value: string): string => {
  const dt = DateTime.fromISO(value);
  if (!value || !dt.isValid || dt.year <= 1) {
    return i18n.global.t("model.__base.value.blank");
  }
  return localeDatetimeFormat(dt);
};

interface SummaryField {
  key: string;
  label: string;
  display: string;
}

// 索引摘要字段行（文档标题由对话框工具栏承载，不在此重复）。未索引文档的向量模型 /
// 维度 / 索引时间均无意义，统一回退空白占位，避免出现"0 维"这类误导性数值。
const summaryFields = computed<SummaryField[]>(() => {
  const summary = data.value?.summary;
  if (!summary) {
    return [];
  }
  const label = (key: string): string => i18n.global.t(`page.aiKnowledge.${key}`);
  return [
    { key: "chunkCount", label: label("chunkStatCount"), display: String(summary.chunkCount) },
    {
      key: "contentChars",
      label: label("chunkStatContentChars"),
      display: String(summary.contentChars),
    },
    {
      key: "chunkCharsTotal",
      label: label("chunkStatCharsTotal"),
      display: String(summary.chunkCharsTotal),
    },
    {
      key: "chunkCharsAvg",
      label: label("chunkStatCharsAvg"),
      display: String(summary.chunkCharsAvg),
    },
    {
      key: "chunkCharsMin",
      label: label("chunkStatCharsMin"),
      display: String(summary.chunkCharsMin),
    },
    {
      key: "chunkCharsMax",
      label: label("chunkStatCharsMax"),
      display: String(summary.chunkCharsMax),
    },
    {
      key: "headingPathCount",
      label: label("chunkStatHeadingPaths"),
      display: String(summary.headingPathCount),
    },
    {
      key: "atomicChunkCount",
      label: label("chunkStatAtomic"),
      display: String(summary.atomicChunkCount),
    },
    { key: "chunkSize", label: label("chunkSize"), display: String(summary.chunkSize) },
    { key: "chunkOverlap", label: label("chunkOverlap"), display: String(summary.chunkOverlap) },
    {
      key: "embeddingModel",
      label: label("chunkEmbeddingModel"),
      display: summary.embeddingModel || blankText.value,
    },
    {
      key: "embeddingDimensions",
      label: label("chunkEmbeddingDimensions"),
      display:
        summary.embeddingDimensions > 0 ? String(summary.embeddingDimensions) : blankText.value,
    },
    { key: "hash", label: label("chunkHash"), display: blankText.value },
    { key: "indexedAt", label: label("chunkIndexedAt"), display: indexedAtText(summary.indexedAt) },
  ];
});
</script>

<style scoped>
/* 仅布局定位：外观一律走 .zk-* 外壳类与 Vuetify 排版类 */

/* 面板头：内边距与盒内平铺内容的左起对齐（表格单元格 / 分块行均为 16px），
   使区块标题、字段名、分块序号落在同一竖直基准线上 */
.section-head {
  display: flex;
  align-items: center;
  gap: var(--zk-space-2);
  padding: var(--zk-space-3) var(--zk-space-4);
}

/* 摘要标签列宽提示：固定标签列宽度，值列自适应占满剩余空间 */
.field-label-col {
  width: 180px;
}

.chunk-section {
  margin-top: var(--zk-space-4);

  /* 分块行的 hover / 选中底色铺满盒宽，裁掉四角以贴合面板圆角 */
  overflow: hidden;
}

/* 未建索引时的提示：盒内仅此一项，按面板头同样的左右留白内缩 */
.chunk-empty {
  margin: var(--zk-space-3) var(--zk-space-4);
}

.chunk-list {
  display: flex;
  flex-direction: column;
}

/* 分块行：可点击展开；hover 走 .zk-row-hover、展开态高亮走 .zk-row-selected */
.chunk-row {
  display: flex;
  align-items: center;
  gap: var(--zk-space-2);
  padding: var(--zk-space-2) var(--zk-space-4);
  cursor: pointer;
}

.chunk-index {
  flex: 0 0 auto;
}

.chunk-path {
  flex: 0 1 auto;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 分块正文：等宽走 .font-mono，底色 / 换行 / 断词 / 内边距走 .zk-code-preview；
   此处只负责它在分块行下方的落位——
   · 左右缩进 16px：盒边与分块行文字、面板头同处一条内容列，四处左对齐统一到 16px；
     叠加 .zk-code-preview 自带的 8px 内边距后正文文字落在 24px，比父行低一级，形成从属关系
   · 上间距 12px < 下间距 16px：按邻近性把正文归到它所属的分块行，而不是下一行
     （行的选中底色止于行内边距处，与正文之间的可见空隙即此处上边距） */
.chunk-body {
  margin: var(--zk-space-3) var(--zk-space-4) var(--zk-space-4);
  max-height: var(--zk-preview-max-height);
  overflow: auto;
}
</style>
