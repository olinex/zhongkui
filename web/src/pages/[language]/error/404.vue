<template>
  <central-layout>
    <template #body>
      <v-empty-state headline="404" :title="$t('summary.pageNotFound')" />
    </template>
  </central-layout>
</template>

<route lang="yaml">
name: "404"
meta:
  title: title.err404
  requiresAuth: false
  requiresPerm: false
</route>

<script setup lang="ts"></script>
