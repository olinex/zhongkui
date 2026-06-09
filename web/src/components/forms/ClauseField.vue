<template>
  <v-confirm-edit
    :model-value="props.modelValue"
    @update:model-value="addSelection"
  >
    <template #default="{ model: proxyModel, actions }">
      <v-card class="mx-auto">
        <template #text>
          <v-select
            :model-value="choiceField"
            @update:model-value="(value) => updateField(proxyModel, value)"
            :items="fieldOptions"
            :disabled="props.modify"
            :label="$t('model.__base.field.field')"
            item-title="title"
            item-value="value"
            return-object
          />
          <v-select
            :model-value="choiceAction"
            @update:model-value="(value) => updateAction(proxyModel, value)"
            :items="actionOptions"
            :label="$t('model.__base.field.action')"
            item-title="title"
            item-value="value"
            return-object
          />
          <adaptive-field
            :model-value="choiceValue"
            @update:model-value="(value) => updateValue(proxyModel, value)"
            :input-type="inputType"
            :number-props="numberProps"
          />
        </template>
        <template #actions>
          <v-spacer></v-spacer>
          <component :is="actions"></component>
        </template>
      </v-card>
    </template>
  </v-confirm-edit>
</template>

<script setup lang="ts" name="clause-field">
import i18n, { localeDateFormat, localeDatetimeFormat } from "@/utils/i18n";
import { DataTypeRanges } from "@/utils/options";
import { SearchInputType, type Option, type SearchItem } from "../@types/forms";
import type { FilterByOption } from "../@types/table";
import { BackendDataType, SearchAction, type NumberRange } from "@/types";
import { DateTime } from "luxon";

const choiceField = ref({ value: "", title: "" });
const choiceAction = ref({ value: "", title: "" });
const choiceValue = ref(null as any);

const choiceOption = computed((): FilterByOption | null => {
  let field = choiceField.value;
  if (field.value && props.headers[field.value]) {
    return props.headers[field.value];
  } else {
    return null;
  }
});

const fieldOptions = computed((): Option[] => {
  return Object.entries(props.headers)
    .filter(([key, _]) => {
      return (
        props.selectedFieldActions.length === 0 ||
        !props.selectedFieldActions.some((couple) => couple.field === key)
      );
    })
    .map(([key, header]) => ({
      title: header.title,
      value: key,
    }));
});

const actionOptions = computed((): Option[] => {
  let option = choiceOption.value;
  if (option) {
    return option.actions
      .filter(
        (action) =>
          !props.selectedFieldActions.some(
            (couple) =>
              couple.field === choiceField.value.value &&
              couple.action === action
          )
      )
      .map((action) => ({
        value: action,
        title: i18n.global.t(`option.search.${action}`),
      }));
  }
  return [];
});

const numberProps = computed((): NumberRange => {
  let option = choiceOption.value;
  let defaultProps = DataTypeRanges[BackendDataType.FLOAT64] as NumberRange;
  if (option === undefined || option === null) {
    return defaultProps;
  } else {
    let tmp = DataTypeRanges[option.type];
    if (tmp === null) {
      return defaultProps;
    }
    return tmp;
  }
});

const inputType = computed((): SearchInputType => {
  let field = choiceField.value.value;
  if (!field) {
    return SearchInputType.UNKNOWN;
  }
  let action = choiceAction.value.value;
  if (!action) {
    return SearchInputType.UNKNOWN;
  }
  let option = choiceOption.value;
  if (!option) {
    return SearchInputType.UNKNOWN;
  }
  return SearchInputType.fromBackendDataType(
    option.type,
    action as SearchAction
  );
});

const addSelection = (item: SearchItem) => {
  emits("update:modelValue", {
    field: { value: item.field.value, title: item.field.title },
    action: { value: item.action.value, title: item.action.title },
    value: { value: item.value.value, title: item.value.title },
  });
};

const updateField = (
  proxy: globalThis.Ref<SearchItem, SearchItem>,
  value: Option
) => {
  choiceField.value = value;
  choiceAction.value = { value: "", title: "" };
  choiceValue.value = null;
  if (!props.modify) {
    return;
  }
  proxy.value.field = choiceField.value;
  proxy.value.action = { value: "", title: "" };
  proxy.value.value = { value: null, title: "" };
};

const updateAction = (
  proxy: globalThis.Ref<SearchItem, SearchItem>,
  value: Option
) => {
  choiceAction.value = value;
  choiceValue.value = null;
  if (
    value.value === SearchAction.NIL ||
    value.value === SearchAction.NOT_NIL
  ) {
    proxy.value.field = choiceField.value;
    proxy.value.action = choiceAction.value;
    proxy.value.value = { value: null, title: "" };
  } else if (props.modify) {
    proxy.value.action = choiceAction.value;
    proxy.value.value = { value: null, title: "" };
  }
};

const updateValue = (
  proxy: globalThis.Ref<SearchItem, SearchItem>,
  value: any
) => {
  if (!props.modify) {
    proxy.value.field = choiceField.value;
    proxy.value.action = choiceAction.value;
  }
  let option = choiceOption.value;
  if (!option) {
    return;
  }
  let title = value;
  switch (option.type) {
    case BackendDataType.DATE:
      let date = localeDateFormat(DateTime.fromJSDate(value));
      proxy.value.value = {
        value: date,
        title: date,
      };
      choiceValue.value = date;
      break;
    case BackendDataType.DATETIME:
      let datetime = localeDatetimeFormat(DateTime.fromJSDate(value));
      proxy.value.value = {
        value: value,
        title: datetime,
      };
      choiceValue.value = value;
      break;
    default:
      proxy.value.value = { value: value, title: title };
      choiceValue.value = value;
  }
};

onMounted(() => {
  if (!props.modify) {
    return;
  }
  choiceField.value.value = props.modelValue.field.value;
  choiceField.value.title = props.modelValue.field.title;
  choiceAction.value.value = props.modelValue.action.value;
  choiceAction.value.title = props.modelValue.action.title;
});

const props = defineProps({
  modelValue: {
    type: Object as PropType<SearchItem>,
    default: () =>
      ({
        field: { value: "", title: "" },
        action: { value: "", title: "" },
        value: { value: null, title: "" },
      } as SearchItem),
  },
  selectedFieldActions: {
    type: Array<{
      field: string;
      action: string;
    }>,
    required: true,
  },
  headers: {
    type: Object as PropType<Record<string, FilterByOption>>,
    required: true,
  },
  modify: { type: Boolean, default: false },
});

const emits = defineEmits(["update:modelValue"]);
</script>
