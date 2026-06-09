<template>
  <v-layout>
    <app-header />
    <v-navigation-drawer
      color="grey-darken-2"
      width="180"
      expand-on-hover
      permanent
      rail
    >
      <v-list nav>
        <v-list-item
          v-for="(item, index) in navigation"
          @click="mainNav = item.value"
          :key="index"
          :title="$t(item.title)"
          :active="mainNav == item.value"
          color="success"
          nav
        >
          <template #prepend>
            <v-icon>{{ item.icon }}</v-icon>
          </template>
        </v-list-item>
      </v-list>
    </v-navigation-drawer>
    <v-navigation-drawer color="grey-darken-1" width="150" permanent>
      <v-list
        v-for="(parent, index) in navigation"
        :key="index"
        v-show="parent.value == mainNav"
        nav
      >
        <v-list-item
          v-for="(child, cindex) in parent.childrens"
          :key="`${index}-${cindex}`"
          :title="$t(child.title)"
          :to="{ name: child.routeName, params: route.params }"
        />
      </v-list>
    </v-navigation-drawer>
    <v-main class="d-flex mb-6">
      <v-container>
        <slot name="prepend" />
        <v-sheet>
          <slot name="body" />
        </v-sheet>
        <slot name="append" />
      </v-container>
    </v-main>
  </v-layout>
</template>

<script setup lang="ts" name="main-layout">
const route = useRoute();

const props = defineProps({
  currentNav: {
    type: String,
    required: true,
  },
});

const mainNav = ref(props.currentNav);

const navigation = ref([
  {
    title: "navbar.etl.self",
    icon: "mdi-timetable",
    value: "etl",
    childrens: [
      {
        title: "navbar.etl.server",
        routeName: "ETL Server List",
      },
      {
        title: "navbar.etl.task",
        routeName: "ETL Task List",
      },
    ],
  },
  {
    title: "navbar.admin.self",
    icon: "mdi-wrench",
    value: "admin",
    childrens: [
      {
        title: "navbar.admin.user",
        routeName: "Admin User List",
      },
      {
        title: "navbar.admin.group",
        routeName: "Admin Group List",
      },
      {
        title: "navbar.admin.namespace",
        routeName: "Admin Namespace List",
      },
      {
        title: "navbar.admin.capturer",
        routeName: "Admin Capturer List",
      },
    ],
  },
]);
</script>
