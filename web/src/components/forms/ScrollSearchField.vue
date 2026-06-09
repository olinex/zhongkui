<template>
  <v-text-field
    :model-value="modelValue"
    @update:model-value="emits('update:modelValue', $event)"
    :label="$t(`model.__base.field.search`)"
    prepend-inner-icon="mdi-magnify"
    variant="outlined"
    density="compact"
    hide-details
    single-line
    rounded
  >
    <template #append-inner>
      <v-menu
        v-model="open"
        location="bottom end"
        transition="scale-transition"
        :close-on-content-click="false"
      >
        <template #activator="{ props }">
          <v-icon v-bind="props" @click="more" color="success">mdi-more</v-icon>
        </template>
        <v-card>
          <v-card-text>
            <v-infinite-scroll
              @load="load"
              load-more-text="summary.dataLoading"
              empty-text="summary.endOfData"
              side="end"
              max-height="300px"
            >
              <slot name="items" :items="items" :close="() => (open = false)" />
              <template #error="{ props }">
                <v-alert type="error" variant="tonal" density="compact">
                  <div class="d-flex justify-space-between align-center">
                    {{ $t("summary.unknownError") }}
                    <v-btn
                      v-bind="props"
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
import type {
  InfiniteScrollSide,
  InfiniteScrollStatus,
} from "vuetify/lib/components/VInfiniteScroll/VInfiniteScroll.mjs";
import type { FilterByItem, SearchIn, SearchOut } from "../@types/table";
import { useAppStore } from "@/stores/app";

const appStore = useAppStore();
const open = ref(false);
const page = ref(1);
const items = ref([] as Array<Record<string, any>>);

const more = () => {
  page.value = 1;
  items.value = [];
  open.value = true;
};

const load = ({
  side,
  done,
}: {
  side: InfiniteScrollSide;
  done: (status: InfiniteScrollStatus) => void;
}) => {
  done("loading");
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
        {} as Record<string, FilterByItem>
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
    }
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
