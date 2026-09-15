<template>
  <v-chip v-if="header.type !== BackendDataType.ACTIONS && item[header.key] === undefined">
    {{ $t("model.__base.value.undefined") }}
  </v-chip>
  <v-chip v-else-if="header.type !== BackendDataType.ACTIONS && item[header.key] === null">
    {{ $t("model.__base.value.null") }}
  </v-chip>
  <v-chip v-else-if="header.type !== BackendDataType.ACTIONS && item[header.key] === ''">
    {{ $t("model.__base.value.blank") }}
  </v-chip>
  <hex-string-chip
    v-else-if="header.type === BackendDataType.UUID"
    color="info"
    suffix="..."
    :value="item[header.key]"
  />
  <hex-string-chip
    v-else-if="header.type === BackendDataType.HEX"
    color="info"
    suffix="..."
    :value="item[header.key]"
  />
  <boolean-chip v-else-if="header.type === BackendDataType.BOOLEAN" :value="item[header.key]" />
  <storage-size-chip
    v-else-if="header.type === BackendDataType.BINARY_BYTE"
    :value="item[header.key]"
    binary
  />
  <storage-size-chip
    v-else-if="header.type === BackendDataType.DECIMAL_BYTE"
    :value="item[header.key]"
  />
  <json-chip
    v-else-if="header.type === BackendDataType.JSON"
    :value="headerJson(item[header.key])"
  />
  <div v-else-if="header.type === BackendDataType.DATETIME">
    {{ localeDatetimeFormat(DateTime.fromISO(item[header.key])) }}
  </div>
  <div v-else-if="header.type === BackendDataType.DATE">
    {{ localeDateFormat(DateTime.fromISO(item[header.key])) }}
  </div>
  <div v-else-if="header.type === BackendDataType.TIME">
    {{ localeTimeFormat(DateTime.fromISO(item[header.key])) }}
  </div>
  <div v-else-if="header.type === BackendDataType.DURATION">
    {{ localDurationFormat(item[header.key]) }}
  </div>
  <div v-else-if="header.enumType && item[header.key] !== undefined && item[header.key] !== null">
    {{ enumLabelOf(header.enumType, item[header.key]) }}
  </div>
  <v-menu
    v-else-if="header.type === BackendDataType.ACTIONS && actions"
    location="bottom right"
    open-on-hover
  >
    <template #activator="{ props }">
      <v-btn icon="mdi-dots-vertical" variant="plain" density="compact" v-bind="props" />
    </template>
    <v-list>
      <v-list-item
        v-for="(action, name) in actions(item)"
        :key="name"
        :vallue="name"
        @click="action(item)"
      >
        <v-list-item-title>{{ $t(`action.${name}`) }}</v-list-item-title>
      </v-list-item>
    </v-list>
  </v-menu>
  <template v-else>
    {{ item[header.key] }}
  </template>
</template>

<script setup lang="ts" name="adaptive-cell">
import type { PropType } from "vue";
import type { ItemActions, TableHeader } from "../@types/table";
import { BackendDataType } from "@/types";
import {
  localDurationFormat,
  localeDateFormat,
  localeDatetimeFormat,
  localeTimeFormat,
} from "@/utils/i18n";
import { enumLabelOf } from "@/utils/enums";
import { DateTime } from "luxon";

defineProps({
  header: {
    type: Object as PropType<TableHeader>,
    required: true,
  },
  item: {
    // Table row values are heterogeneous; column types come from `header.type`.
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    type: Object as PropType<Record<string, any>>,
    required: true,
  },
  actions: {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    type: Function as PropType<(item: Record<string, any>) => ItemActions>,
    default: undefined,
  },
  refresh: {
    type: Function as PropType<() => void>,
    default: () => {},
  },
});

// headerJson 将单元格值归一化为 JSON 字符串
// json-chip 期望接收 JSON 字符串；对象/数组等结构化值经 JSON.stringify 后展示
const headerJson = (value: unknown): string => {
  if (typeof value === "string") {
    return value;
  }
  if (value === null || value === undefined) {
    return "";
  }
  return JSON.stringify(value);
};
</script>
