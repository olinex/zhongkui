<template>
  <v-layout>
    <v-main>
      <slot name="prepend" />
      <v-container
        class="d-flex justify-center align-center pa-4"
        width="100%"
        height="100%"
        style="max-width: 480px"
      >
        <slot name="body" />
      </v-container>
      <slot name="append" />
    </v-main>
  </v-layout>
</template>

<script setup lang="ts" name="central-layout">
//
</script>
