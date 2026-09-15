<template>
  <v-expansion-panels v-model="expanded" class="guide-panel" variant="accordion">
    <v-expansion-panel>
      <v-expansion-panel-title>
        <span class="panel-title">{{ $t("guide.title") }}</span>
      </v-expansion-panel-title>
      <v-expansion-panel-text>
        <!-- 数据源速查 -->
        <div class="guide-section">
          <div class="guide-section-title">{{ $t("guide.datasource") }}</div>
          <div v-if="datasources.length === 0" class="guide-empty">
            {{ $t("guide.datasourceEmpty") }}
          </div>
          <template v-else>
            <div v-for="ds in datasources" :key="ds.name" class="ds-block">
              <div class="ds-head">
                <code class="ds-name">{{ ds.name }}</code>
                <v-btn size="small" variant="tonal" @click="insertSelect(ds)">
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
          <div class="guide-section-title">{{ $t("guide.samples") }}</div>
          <div v-if="samples.length === 0" class="guide-empty">
            {{ $t("guide.sampleEmpty") }}
          </div>
          <v-card
            v-for="s in samples"
            :key="s.titleKey"
            variant="outlined"
            class="sample-card"
            :title="$t('guide.clickInsertSample')"
            @click="insert(s.snippet)"
          >
            <div class="sample-title">{{ $t(s.titleKey) }}</div>
            <div class="sample-desc">{{ $t(s.descKey) }}</div>
          </v-card>
        </div>
      </v-expansion-panel-text>
    </v-expansion-panel>
  </v-expansion-panels>
</template>

<script setup lang="ts" name="datasource-guide-panel">
// Third-party / Plugins

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
</script>

<style scoped>
.guide-panel {
  border: 1px solid rgb(0 0 0 / 24%);
  border-radius: 4px;
}

.panel-title {
  font-size: 13px;
  font-weight: 600;
}

.guide-section {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.guide-section + .guide-section {
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px dashed rgb(0 0 0 / 12%);
}

.guide-section-title {
  font-size: 12px;
  font-weight: 600;
  color: rgb(var(--v-theme-on-surface-variant));
}

.guide-empty {
  font-size: 12px;
  color: rgb(var(--v-theme-on-surface-variant));
}

.ds-block {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.ds-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.ds-name {
  font-size: 13px;
  font-weight: 600;
  color: rgb(var(--v-theme-primary));
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
}

.ds-col-type {
  font-size: 11px;
  color: rgb(var(--v-theme-on-surface-variant));
}

.sample-card {
  padding: 6px 10px;
  cursor: pointer;
}

.sample-card:hover {
  border-color: rgb(var(--v-theme-primary));
}

.sample-title {
  font-size: 13px;
  font-weight: 600;
}

.sample-desc {
  font-size: 12px;
  color: rgb(var(--v-theme-on-surface-variant));
}
</style>
