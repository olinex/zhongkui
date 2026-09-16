<template>
  <v-expansion-panels v-model="expanded" class="guide-panel" variant="accordion" density="compact">
    <v-expansion-panel>
      <v-expansion-panel-title class="panel-titlebar">
        <div class="panel-head">
          <v-icon size="18" class="panel-head-icon">mdi-database-search</v-icon>
          <span class="panel-title">{{ $t("guide.title") }}</span>
        </div>
      </v-expansion-panel-title>
      <v-expansion-panel-text>
        <!-- 数据源速查 -->
        <div class="guide-section">
          <div class="section-head">
            <span class="section-badge">01</span>
            <span class="section-title">{{ $t("guide.datasource") }}</span>
          </div>
          <div v-if="datasources.length === 0" class="guide-empty">
            <v-icon size="16" start>mdi-database-off</v-icon>
            <span>{{ $t("guide.datasourceEmpty") }}</span>
          </div>
          <template v-else>
            <div v-for="ds in datasources" :key="ds.name" class="ds-card">
              <div class="ds-head">
                <div class="ds-ident">
                  <v-avatar size="28" color="primary" variant="tonal" class="ds-avatar">
                    <v-icon size="16">mdi-database</v-icon>
                  </v-avatar>
                  <div class="ds-ident-text">
                    <code class="ds-name">{{ ds.name }}</code>
                    <span class="ds-count">{{ ds.columns.length }} {{ $t("guide.columns") }}</span>
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
                  size="small"
                  variant="outlined"
                  class="ds-col"
                  :title="$t('guide.clickInsertColumn')"
                  @click="insertColumn(ds, col)"
                >
                  <span class="ds-col-name">{{ col.name }}</span>
                  <span class="ds-col-type">{{ $t(columnTypeLabel(col.type)) }}</span>
                </v-chip>
              </div>
            </div>
          </template>
        </div>

        <!-- 场景样例：一键插入可运行片段 -->
        <div class="guide-section">
          <div class="section-head">
            <span class="section-badge">02</span>
            <span class="section-title">{{ $t("guide.samples") }}</span>
          </div>
          <div v-if="samples.length === 0" class="guide-empty">
            <v-icon size="16" start>mdi-flask-outline</v-icon>
            <span>{{ $t("guide.sampleEmpty") }}</span>
          </div>
          <v-card
            v-for="s in samples"
            :key="s.titleKey"
            variant="outlined"
            class="sample-card"
            :aria-label="$t('guide.clickInsertSample')"
            @click="insert(s.snippet)"
          >
            <v-avatar size="34" color="primary" variant="tonal" class="sample-avatar">
              <v-icon size="18">{{ sampleIcon(s.titleKey) }}</v-icon>
            </v-avatar>
            <div class="sample-main">
              <div class="sample-title">{{ $t(s.titleKey) }}</div>
              <div class="sample-desc">{{ $t(s.descKey) }}</div>
            </div>
            <v-icon size="18" class="sample-arrow">mdi-chevron-right</v-icon>
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
.guide-panel {
  border: 1px solid rgb(var(--v-theme-border));
  border-radius: 8px;
  overflow: hidden;
}

/* 参考栏已固定为全屏右侧 340px，无需再压缩内边距；
   保留适度左右留白，避免内容与参考栏边框紧贴 */
/* stylelint-disable-next-line selector-class-pattern */
.guide-panel :deep(.v-expansion-panel-text__wrapper) {
  padding: 4px 12px 12px;
}

.panel-titlebar {
  min-height: 40px;
}

.panel-head {
  display: flex;
  align-items: center;
  gap: 8px;
}

.panel-head-icon {
  color: rgb(var(--v-theme-primary));
}

.panel-title {
  font-size: 13px;
  font-weight: 600;
  color: rgb(var(--v-theme-on-surface));
}

.guide-section {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.guide-section + .guide-section {
  margin-top: 14px;
  padding-top: 14px;
  border-top: 1px dashed rgb(var(--v-theme-border));
}

.section-head {
  display: flex;
  align-items: center;
  gap: 8px;
}

.section-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 20px;
  height: 20px;
  padding: 0 5px;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 700;
  color: rgb(var(--v-theme-on-primary));
  background: rgb(var(--v-theme-primary));
}

.section-title {
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.03em;
  text-transform: uppercase;
  color: rgb(var(--v-theme-on-surface-variant));
}

.guide-empty {
  display: flex;
  align-items: center;
  font-size: 12px;
  line-height: 1.5;
  color: rgb(var(--v-theme-on-surface-variant));
}

.ds-card {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 12px 14px;
  border: 1px solid rgb(var(--v-theme-border));
  border-radius: 8px;
  background: rgb(var(--v-theme-surface));
  transition: border-color 0.16s ease;
}

.ds-card:hover {
  border-color: rgb(var(--v-theme-primary));
}

.ds-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.ds-ident {
  display: flex;
  align-items: center;
  gap: 8px;
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
  font-size: 13px;
  font-weight: 600;
  color: rgb(var(--v-theme-primary));
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ds-count {
  font-size: 11px;
  color: rgb(var(--v-theme-on-surface-variant));
  white-space: nowrap;
}

.ds-cols {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.ds-col {
  cursor: pointer;
}

.ds-col-name {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-weight: 500;
}

.ds-col-type {
  font-size: 11px;
  color: rgb(var(--v-theme-on-surface-variant));
}

.sample-card {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 14px;
  cursor: pointer;
  transition:
    border-color 0.16s ease,
    background-color 0.16s ease;
}

.sample-card:hover {
  border-color: rgb(var(--v-theme-primary));
  background: rgb(var(--v-theme-surface-variant), 0.4);
}

.sample-avatar {
  flex-shrink: 0;
}

.sample-main {
  display: flex;
  flex-direction: column;
  gap: 1px;
  min-width: 0;
  flex: 1;
}

.sample-title {
  font-size: 13px;
  font-weight: 600;
  color: rgb(var(--v-theme-on-surface));
}

.sample-desc {
  font-size: 12px;
  line-height: 1.4;
  color: rgb(var(--v-theme-on-surface-variant));
  overflow-wrap: break-word;
}

.sample-arrow {
  flex-shrink: 0;
  color: rgb(var(--v-theme-on-surface-variant));
  opacity: 0.7;
}

.sample-card:hover .sample-arrow {
  opacity: 1;
  color: rgb(var(--v-theme-primary));
}
</style>
