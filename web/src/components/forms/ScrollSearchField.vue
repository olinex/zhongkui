<template>
  <v-text-field
    :model-value="modelValue"
    :label="$t(`model.__base.field.search`)"
    prepend-inner-icon="mdi-magnify"
    variant="outlined"
    density="compact"
    hide-details
    single-line
    rounded
    @update:model-value="emits('update:modelValue', $event)"
  >
    <template #append-inner>
      <v-menu
        v-model="open"
        location="bottom end"
        transition="scale-transition"
        :close-on-content-click="false"
      >
        <template #activator="{ props: activatorProps }">
          <v-icon v-bind="activatorProps" color="primary" @click="more">mdi-more</v-icon>
        </template>
        <v-card>
          <v-card-text>
            <v-infinite-scroll
              load-more-text="summary.dataLoading"
              empty-text="summary.endOfData"
              side="end"
              max-height="300px"
              @load="load"
            >
              <slot name="items" :items="items" :close="() => (open = false)" />
              <template #error="{ props: errorProps }">
                <v-alert type="error" variant="tonal" density="compact">
                  <div class="d-flex justify-space-between align-center">
                    {{ $t("summary.unknownError") }}
                    <v-btn
                      v-bind="errorProps"
                      color="white"
                      size="small"
                      variant="outlined"
                      icon="mdi-refresh"
                      density="compact"
                    />
                  </div>
                </v-alert>
              </template>
            </v-infinite-scroll>
          </v-card-text>
        </v-card>
      </v-menu>
    </template>
  </v-text-field>
</template>

<script setup lang="ts" name="scroll-search-field">
import { getResponse } from "@/apis/base";
import { SearchAction } from "@/types";
import type { AxiosPromise } from "axios";
import type { PropType } from "vue";
import type { InfiniteScrollStatus } from "vuetify/lib/components/VInfiniteScroll/VInfiniteScroll.mjs";
import type { FilterByItem, SearchIn, SearchOut } from "../@types/table";
import { useAppStore } from "@/stores/app";

const appStore = useAppStore();
const open = ref(false);
const page = ref(1);
// Server response items are dynamic.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const items = ref([] as Array<Record<string, any>>);

const more = () => {
  page.value = 1;
  items.value = [];
  open.value = true;
};

const load = ({ done }: { done: (status: InfiniteScrollStatus) => void }) => {
  done("loading");
  // Server response items are dynamic.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  getResponse<SearchOut<Record<string, any>>>(
    props.searchRequest({
      greedy: true,
      page: page.value,
      pageSize: 10,
      orderBy: [{ name: props.orderByField, order: 1 }],
      filterBy: Object.entries(props.searchHeaders).reduce(
        (prev, [field, action]) => {
          if (props.modelValue === "") {
            return prev;
          }
          prev[field] = {
            action: action,
            value: props.modelValue,
          };
          return prev;
        },
        {} as Record<string, FilterByItem>,
      ),
    }),
    async (rsp) => {
      if (rsp.data.items.length === 0 || rsp.data.total === 0) {
        done("empty");
        return;
      }
      items.value = items.value.concat(rsp.data.items);
      page.value++;
      done("ok");
    },
    async (rsp) => {
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "error",
      });
      done("error");
    },
  );
};

const props = defineProps({
  modelValue: {
    type: String,
    required: true,
  },
  orderByField: {
    type: String,
    default: "id",
  },
  searchHeaders: {
    type: Object as PropType<Record<string, SearchAction>>,
    required: true,
  },
  searchRequest: {
    type: Function as PropType<(req: SearchIn) => AxiosPromise>,
    required: true,
  },
});

const emits = defineEmits(["update:modelValue"]);
</script>
