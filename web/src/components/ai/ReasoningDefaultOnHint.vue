<template>
  <!-- 思考来源说明：当前档位模型未配置思考开关（开关已隐藏），却仍产出思考内容，说明是该模型默认
       开启思考。以警示图标 + 悬浮提示解释来源，并指引管理员在「LLM 模型」高级配置中填写思考开关
       字段与开 / 关取值，从而手动控制思考。所有思考折叠块（末个流式块与更早各轮）共用此入口，
       避免提示只出现在末个思考块上。 -->
  <v-tooltip
    location="top"
    :max-width="maxWidth"
    :text="$t('page.aiAssistant.reasoningDefaultOnHint')"
  >
    <template #activator="{ props }">
      <v-icon v-bind="props" icon="mdi-alert-circle-outline" size="x-small" color="warning" />
    </template>
  </v-tooltip>
</template>

<script setup lang="ts" name="reasoning-default-on-hint">
withDefaults(
  defineProps<{
    /** 气泡最大宽度（px）：说明含「原因 + 如何控制」两段文案，需折成多行才易读 */
    maxWidth?: number;
  }>(),
  { maxWidth: 320 },
);
</script>
