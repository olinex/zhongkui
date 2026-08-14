<template>
  <v-layout>
    <app-header />
    <v-main>
      <v-container class="mx-auto pa-8" style="max-width: 1200px">
        <!-- Welcome area -->
        <div class="mb-8">
          <h1 class="text-h4 font-weight-bold">{{ $t("title.zk") }}</h1>
          <p class="text-h6 text-medium-emphasis mt-2">{{ $t("home.subtitle") }}</p>
        </div>

        <!-- Quick start section -->
        <h2 class="text-h6 mb-4">{{ $t("home.quickStart") }}</h2>

        <!-- Module quick entry cards -->
        <v-row>
          <v-col v-for="item in modules" :key="item.routeName" cols="12" sm="6" md="4" lg="3">
            <v-card class="home-card" @click="goto(item.routeName)">
              <v-card-item>
                <template #prepend>
                  <v-avatar color="primary" variant="tonal">
                    <v-icon :icon="item.icon" />
                  </v-avatar>
                </template>
                <v-card-title>{{ $t(item.title) }}</v-card-title>
              </v-card-item>
            </v-card>
          </v-col>
        </v-row>
      </v-container>
    </v-main>
  </v-layout>
</template>

<route lang="yaml">
name: Home
meta:
  title: title.home
  requiresAuth: false
  requiresPerm: false
</route>

<script setup lang="ts" name="home">
import router from "@/router";

const route = useRoute();

const modules = ref([
  {
    title: "navbar.etl.server",
    icon: "mdi-server",
    routeName: "ETL Server List",
  },
  {
    title: "navbar.etl.task",
    icon: "mdi-timetable",
    routeName: "ETL Task List",
  },
  {
    title: "navbar.admin.user",
    icon: "mdi-account",
    routeName: "Admin User List",
  },
  {
    title: "navbar.admin.group",
    icon: "mdi-account-group",
    routeName: "Admin Group List",
  },
  {
    title: "navbar.admin.namespace",
    icon: "mdi-tag-multiple",
    routeName: "Admin Namespace List",
  },
  {
    title: "navbar.admin.capturer",
    icon: "mdi-lan",
    routeName: "Admin Capturer List",
  },
  {
    title: "navbar.admin.tagRule",
    icon: "mdi-label",
    routeName: "Admin Tag Rule List",
  },
  {
    title: "navbar.admin.tagEventRule",
    icon: "mdi-alert-decagram",
    routeName: "Admin Tag Event Rule List",
  },
]);

const goto = (routeName: string) => {
  // route.params type does not satisfy RouteLocationRaw.params
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  router.push({ name: routeName as any, params: route.params as any });
};
</script>

<style scoped>
.home-card {
  cursor: pointer;
  transition:
    border-color 0.2s ease,
    background-color 0.2s ease;
}

.home-card:hover {
  border-color: rgb(var(--v-theme-primary));
  background-color: rgb(var(--v-theme-primary), 0.06);
}
</style>
