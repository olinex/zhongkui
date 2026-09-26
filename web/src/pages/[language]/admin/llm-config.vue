<template>
  <main-layout current-nav="admin">
    <template #body>
      <!-- AI 核心配置（模型注册表 / 档位绑定 / Agent 参数 / MCP 服务器 / 工具）经 tab 拆分到同一页面：
           消除多入口拆分带来的上下文割裂，各 tab 内容由独立面板组件承载 -->
      <tab-view
        v-model="tab"
        :items="[
          { key: 'model', label: $t('model.llmModel.name') },
          { key: 'binding', label: $t('model.llmBinding.name') },
          { key: 'param', label: $t('model.agentParam.name') },
          { key: 'mcp', label: $t('model.mcpServer.name') },
          { key: 'tool', label: $t('model.llmTool.name') },
        ]"
      >
        <template #model>
          <llm-model-panel />
        </template>

        <template #binding>
          <llm-binding-panel />
        </template>

        <template #param>
          <agent-param-panel :active="tab === 'param'" />
        </template>

        <template #mcp>
          <mcp-server-panel />
        </template>

        <template #tool>
          <llm-tool-panel />
        </template>
      </tab-view>
    </template>
  </main-layout>
</template>

<route lang="yaml">
name: Admin LLM Config
meta:
  title: title.llmConfigAdmin
  requiresAuth: true
  requiresPerm: true
</route>

<script setup lang="ts" name="llm-config">
// 当前激活的 tab：model（模型注册表）/ binding（档位绑定）/ param（Agent 参数）
const tab = ref("model");
</script>
