<template>
  <v-expansion-panels
    v-model="expanded"
    class="guide-panel zk-panel"
  >
    <v-expansion-panel>
      <v-expansion-panel-title class="panel-titlebar">
        <div class="panel-head">
          <v-icon size="18" color="primary">mdi-database-search</v-icon>
          <span class="zk-editor-title">{{ $t("guide.title") }}</span>
        </div>
      </v-expansion-panel-title>
      <v-expansion-panel-text>
        <!-- 数据源速查 -->
        <div class="guide-section">
          <div class="section-head">
            <span class="zk-step-badge">01</span>
            <span class="zk-label-caps">{{ $t("guide.datasource") }}</span>
          </div>
          <div v-if="datasources.length === 0" class="guide-empty zk-hint">
            <v-icon size="16" start>mdi-database-off</v-icon>
            <span>{{ $t("guide.datasourceEmpty") }}</span>
          </div>
          <template v-else>
            <div
              v-for="ds in datasources"
              :key="ds.name"
              class="ds-card zk-panel zk-interactive-card"
            >
              <div class="ds-head">
                <div class="ds-ident">
                  <v-avatar size="28" color="primary" variant="tonal" class="ds-avatar">
                    <v-icon size="16">mdi-database</v-icon>
                  </v-avatar>
                  <div class="ds-ident-text">
                    <code class="ds-name zk-mono-emphasis">{{ ds.name }}</code>
                    <span class="ds-count zk-hint"
                      >{{ ds.columns.length }} {{ $t("guide.columns") }}</span
                    >
                  </div>
                </div>
                <v-btn
                  class="ds-insert-btn"
                  size="small"
                  variant="tonal"
                  prepend-icon="mdi-shape-plus-outline"
                  @click="insertSelect(ds)"
                >
                  {{ $t("guide.insertSelect") }}
                </v-btn>
              </div>
              <div class="ds-cols">
                <v-chip
                  v-for="col in ds.columns"
                  :key="col.name"
                  variant="outlined"
                  class="ds-col"
                  :title="$t('guide.clickInsertColumn')"
                  @click="insertColumn(ds, col)"
                >
                  <span class="font-mono">{{ col.name }}</span>
                  <span class="zk-hint">{{ $t(columnTypeLabel(col.type)) }}</span>
                </v-chip>
              </div>
            </div>
          </template>
        </div>

        <!-- 场景样例：一键插入可运行片段 -->
        <div class="guide-section zk-hairline-top">
          <div class="section-head">
            <span class="zk-step-badge">02</span>
            <span class="zk-label-caps">{{ $t("guide.samples") }}</span>
          </div>
          <div v-if="samples.length === 0" class="guide-empty zk-hint">
            <v-icon size="16" start>mdi-flask-outline</v-icon>
            <span>{{ $t("guide.sampleEmpty") }}</span>
          </div>
          <v-card
            v-for="s in samples"
            :key="s.titleKey"
            variant="outlined"
            class="sample-card zk-interactive-card"
            :aria-label="$t('guide.clickInsertSample')"
            @click="insert(s.snippet)"
          >
            <v-avatar size="34" color="primary" variant="tonal" class="sample-avatar">
              <v-icon size="18">{{ sampleIcon(s.titleKey) }}</v-icon>
            </v-avatar>
            <div class="sample-main">
              <div class="zk-editor-title">{{ $t(s.titleKey) }}</div>
              <div class="sample-desc zk-hint">{{ $t(s.descKey) }}</div>
            </div>
            <v-icon size="18" class="sample-arrow text-on-surface-variant"
              >mdi-chevron-right</v-icon
            >
          </v-card>
        </div>
      </v-expansion-panel-text>
    </v-expansion-panel>
  </v-expansion-panels>
</template>

<script setup lang="ts" name="datasource-guide-panel">
// Internal
import {
  buildDatasourceSamples,
  type DatasourceSample,
  type InputFieldMeta,
  type SqliteColumnMeta,
  type SqliteDatasourceMeta,
} from "@/utils/rego";

const props = withDefaults(
  defineProps<{
    /** 当前命名空间已注册数据源（表名 + 列），取自 /sqlite/meta，同 RegoEditor 的 sqlite-datasources */
    datasources: SqliteDatasourceMeta[];
    /** 输入模型字段元信息，用于为场景样例挑选一个示例 input.<字段> */
    fields?: InputFieldMeta[];
    /** 场景样例生成的默认打标名 */
    tag?: string;
    /** 面板初始是否展开 */
    defaultOpen?: boolean;
  }>(),
  { fields: () => [], tag: "tag", defaultOpen: true },
);

const emit = defineEmits<{ (e: "insert", text: string): void }>();

// 面板展开态：数组元素为展开的面板索引（仅一页，0=展开）
const expanded = ref<number[]>(props.defaultOpen ? [0] : []);

const insert = (text: string): void => emit("insert", text);

/** 挑选一个非敏感字符串字段作为场景样例的参数 placeholder，缺省返回空串。 */
const pickField = (): string =>
  props.fields.find((f) => f.type === "string" && !f.sensitive)?.field ?? "";

/** 基于选中的第一个数据源组装场景样例；无数据源时为空（提示先创建）。 */
const samples = computed<DatasourceSample[]>(() => {
  const ds = props.datasources[0];
  if (!ds) {
    return [];
  }
  return buildDatasourceSamples(ds, props.tag ?? "tag", pickField());
});

/** 插入一句基础 SELECT 查询行，供用户在规则体内按列名进一步编写。 */
const insertSelect = (ds: SqliteDatasourceMeta): void => {
  insert(`rows := sqlite.query("SELECT * FROM ${ds.name} WHERE 1 LIMIT 10", [])\n`);
};

/** 插入 `表名.列名`，用于拼接 SELECT / WHERE。 */
const insertColumn = (ds: SqliteDatasourceMeta, col: SqliteColumnMeta): void => {
  insert(`${ds.name}.${col.name}`);
};

/** 列类型徽标文案键，复用数据源 schema 编辑器的类型翻译。 */
const columnTypeLabel = (type: string): string => `model.datasource.columnType.${type}`;

/** 场景样例标题键 → 装饰性图标，按三个固定样例标题键映射，其余回退通用图标。 */
const sampleIcon = (titleKey: string): string => {
  switch (titleKey) {
    case "guide.sample.exists":
      return "mdi-check-circle-outline";
    case "guide.sample.value":
      return "mdi-arrow-right-box";
    default:
      return "mdi-chart-bar";
  }
};
</script>

<style scoped>
/* 参考栏外壳（hairline 描边 / 圆角 / surface 背景）走全局 .zk-panel，
   卡片 hover 反馈走全局 .zk-interactive-card，此处只保留布局定位 */
.guide-panel {
  overflow: hidden;
}

/* 参考栏已固定为全屏右侧 340px，无需再压缩内边距；
   保留适度左右留白，避免内容与参考栏边框紧贴 */
/* stylelint-disable-next-line selector-class-pattern */
.guide-panel :deep(.v-expansion-panel-text__wrapper) {
  padding: var(--zk-space-1) var(--zk-space-3) var(--zk-space-3);
}

.panel-titlebar {
  min-height: var(--zk-control-height-compact);
}

.panel-head {
  display: flex;
  align-items: center;
  gap: var(--zk-space-2);
}

.guide-section {
  display: flex;
  flex-direction: column;
  gap: var(--zk-space-2);
}

/* 分区之间的分隔线走 .zk-hairline-top */
.guide-section + .guide-section {
  margin-top: var(--zk-space-3);
  padding-top: var(--zk-space-3);
}

.section-head {
  display: flex;
  align-items: center;
  gap: var(--zk-space-2);
}

.guide-empty {
  display: flex;
  align-items: center;
}

.ds-card {
  display: flex;
  flex-direction: column;
  gap: var(--zk-space-2);
  padding: var(--zk-space-3);
}

.ds-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--zk-space-2);
}

.ds-ident {
  display: flex;
  align-items: center;
  gap: var(--zk-space-2);
  flex: 1;
  min-width: 0;
}

.ds-insert-btn {
  flex-shrink: 0;
}

.ds-avatar {
  flex-shrink: 0;
}

.ds-ident-text {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.ds-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ds-count {
  white-space: nowrap;
}

.ds-cols {
  display: flex;
  flex-wrap: wrap;
  gap: var(--zk-space-1);
}

.ds-col {
  cursor: pointer;
}

.sample-card {
  display: flex;
  align-items: center;
  gap: var(--zk-space-3);
  padding: var(--zk-space-3);
  cursor: pointer;
}

.sample-avatar {
  flex-shrink: 0;
}

.sample-main {
  display: flex;
  flex-direction: column;
  gap: var(--zk-space-1);
  min-width: 0;
  flex: 1;
}

.sample-desc {
  overflow-wrap: break-word;
}

.sample-arrow {
  flex-shrink: 0;
}
</style>
