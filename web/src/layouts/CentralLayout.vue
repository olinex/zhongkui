<template>
  <v-layout>
    <v-main>
      <slot name="prepend" />
      <!-- 全屏居中骨架：尺寸与滚动由外壳层统一承载
           （styles/main.scss 的 .zk-centered-layout / .zk-centered-surface），
           认证页与错误页只提供内容，不再各自书写宽度或高度 -->
      <div class="zk-centered-layout">
        <div class="zk-centered-surface">
          <slot name="body" />
        </div>
      </div>
      <slot name="append" />
    </v-main>
  </v-layout>
</template>

<script setup lang="ts" name="central-layout">
//
</script>
