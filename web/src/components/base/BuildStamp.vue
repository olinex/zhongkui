<template>
  <!-- 构建标识：贴靠视口右下角的极淡文本，悬停加深，点击复制完整标识（tag + commit）。
       层级取最高档令牌，避免被对话框 / 编辑器浮层等覆盖。 -->
  <button
    type="button"
    class="build-stamp zk-build-stamp"
    :title="buildTitle"
    @click="copyToClipboard(buildTitle)"
  >
    {{ buildInfo.tag }} · {{ shortCommit }}
  </button>
</template>

<script setup lang="ts" name="build-stamp">
import { copyToClipboard } from "@/utils/clipboard";

// 编译期常量：由 vite.config.mts 读取环境变量并兜底 build.config.ts 的默认值后注入
const buildInfo = __BUILD_INFO__;
// 默认展示短哈希；完整 commit 保留在 title 与复制内容中，便于粘贴核对
const shortCommit = computed(() =>
  buildInfo.commit.length > 7 ? buildInfo.commit.slice(0, 7) : buildInfo.commit,
);
const buildTitle = `${buildInfo.tag} (${buildInfo.commit})`;
</script>

<style scoped>
/* 仅布局定位：贴视口右下角，落在「直达底部」按钮 24px 边距的下方 */
.build-stamp {
  position: fixed;
  right: var(--zk-space-2);
  bottom: var(--zk-space-1);
  z-index: var(--zk-z-tooltip);
  white-space: nowrap;
  user-select: none;
}
</style>
