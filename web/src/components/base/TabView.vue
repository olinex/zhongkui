<template>
  <!-- TabView 通用 tab 分页容器：统一底部下划线、分隔线与窗口切换；
       actions 插槽可将操作控件（如 RefreshControl）置于 tab 栏下方右侧的独立操作行，避免与 tab 栏挤作一行 -->
  <v-card flat class="zk-tabs-embedded">
    <v-tabs v-model="active" color="primary">
      <v-tab v-for="item in items" :key="item.key" :value="item.key">
        {{ item.label }}
      </v-tab>
    </v-tabs>
    <v-divider />
    <div v-if="$slots.actions" class="d-flex justify-end px-4 py-2">
      <slot name="actions" />
    </div>
    <v-tabs-window v-model="active">
      <v-tabs-window-item v-for="item in items" :key="item.key" :value="item.key">
        <slot :name="item.key" />
      </v-tabs-window-item>
    </v-tabs-window>
  </v-card>
</template>

<script setup lang="ts" name="tab-view">
// TabView 通用 tab 分页容器组件：
// - items 定义各 tab（key + label），v-model 绑定当前激活的 tab key；
// - 每个 tab 的内容经同名具名插槽（:name="key"）渲染；
// - actions 插槽用于在 tab 栏下方右侧放置操作控件（如 RefreshControl）。
interface TabItem {
  /** tab key，同时作为具名插槽名 */
  key: string;
  /** tab 显示标签（已翻译） */
  label: string;
}

defineProps<{
  items: TabItem[];
}>();

const active = defineModel<string>({ required: true });
</script>
