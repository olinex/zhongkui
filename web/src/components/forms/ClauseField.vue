<template>
  <v-confirm-edit :model-value="props.modelValue" @update:model-value="addSelection">
    <template #default="{ model: proxyModel, actions }">
      <v-card class="zk-clause-card mx-auto">
        <template #text>
          <v-select
            :model-value="choiceField"
            :items="fieldOptions"
            :disabled="props.modify"
            :label="$t('model.__base.field.field')"
            item-title="title"
            item-value="value"
            return-object
            @update:model-value="(value) => updateField(proxyModel, value)"
          />
          <v-select
            :model-value="choiceAction"
            :items="actionOptions"
            :label="$t('model.__base.field.action')"
            item-title="title"
            item-value="value"
            return-object
            @update:model-value="(value) => updateAction(proxyModel, value)"
          />
          <adaptive-field
            :model-value="choiceValue"
            :input-type="inputType"
            :number-props="numberProps"
            :options="choiceOptions"
            @update:model-value="(value) => updateValue(proxyModel, value)"
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
import {
  SearchInputType,
  fromBackendDataType,
  type Option,
  type SearchItem,
} from "../@types/forms";
import type { FilterByOption } from "../@types/table";
import { BackendDataType, SearchAction, type NumberRange } from "@/types";
import { DateTime } from "luxon";

const choiceField = ref({ value: "", title: "" });
const choiceAction = ref({ value: "", title: "" });
// The selected value type depends on the field's backend data type.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const choiceValue = ref(null as any);

const choiceOption = computed((): FilterByOption | null => {
  const field = choiceField.value;
  if (field.value && props.headers[field.value]) {
    return props.headers[field.value];
  } else {
    return null;
  }
});

// 枚举字段的下拉可选项（title 为 i18n 键，展示时经翻译）
const choiceOptions = computed<Option[]>(() => choiceOption.value?.options ?? []);

const fieldOptions = computed((): Option[] => {
  return Object.entries(props.headers)
    .filter(([key]) => {
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
  const option = choiceOption.value;
  if (option) {
    return option.actions
      .filter(
        (action) =>
          !props.selectedFieldActions.some(
            (couple) => couple.field === choiceField.value.value && couple.action === action,
          ),
      )
      .map((action) => ({
        value: action,
        title: i18n.global.t(`option.search.${action}`),
      }));
  }
  return [];
});

const numberProps = computed((): NumberRange => {
  const option = choiceOption.value;
  const defaultProps = DataTypeRanges[BackendDataType.FLOAT64] as NumberRange;
  if (option === undefined || option === null) {
    return defaultProps;
  } else {
    const tmp = DataTypeRanges[option.type];
    if (tmp === null) {
      return defaultProps;
    }
    return tmp;
  }
});

const inputType = computed((): SearchInputType => {
  const field = choiceField.value.value;
  if (!field) {
    return SearchInputType.UNKNOWN;
  }
  const action = choiceAction.value.value;
  if (!action) {
    return SearchInputType.UNKNOWN;
  }
  const option = choiceOption.value;
  if (!option) {
    return SearchInputType.UNKNOWN;
  }
  return fromBackendDataType(option.type, action as SearchAction);
});

const addSelection = (item: SearchItem) => {
  emits("update:modelValue", {
    field: { value: item.field.value, title: item.field.title },
    action: { value: item.action.value, title: item.action.title },
    value: { value: item.value.value, title: item.value.title },
  });
};

const updateField = (proxy: globalThis.Ref<SearchItem, SearchItem>, value: Option) => {
  choiceField.value = value;
  choiceAction.value = { value: "", title: "" };
  choiceValue.value = null;
  if (!props.modify) {
    // 字段确定后，若该字段剩余可选动作唯一，则自动勾选该动作并直接展示值输入
    if (actionOptions.value.length === 1) {
      choiceAction.value = actionOptions.value[0];
      proxy.value.action = choiceAction.value;
    }
    return;
  }
  proxy.value.field = choiceField.value;
  proxy.value.action = { value: "", title: "" };
  proxy.value.value = { value: null, title: "" };
};

const updateAction = (proxy: globalThis.Ref<SearchItem, SearchItem>, value: Option) => {
  choiceAction.value = value;
  choiceValue.value = null;
  if (value.value === SearchAction.NIL || value.value === SearchAction.NOT_NIL) {
    proxy.value.field = choiceField.value;
    proxy.value.action = choiceAction.value;
    proxy.value.value = { value: null, title: "" };
  } else if (props.modify) {
    proxy.value.action = choiceAction.value;
    proxy.value.value = { value: null, title: "" };
  }
};

// The updated value is heterogeneous: Date for datetime fields, string/number otherwise.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const updateValue = (proxy: globalThis.Ref<SearchItem, SearchItem>, value: any) => {
  if (!props.modify) {
    proxy.value.field = choiceField.value;
    proxy.value.action = choiceAction.value;
  }
  const option = choiceOption.value;
  if (!option) {
    return;
  }
  const title = value;
  switch (option.type) {
    case BackendDataType.DATE:
      const date = localeDateFormat(DateTime.fromJSDate(value));
      proxy.value.value = {
        value: date,
        title: date,
      };
      choiceValue.value = date;
      break;
    case BackendDataType.DATETIME:
      const datetime = localeDatetimeFormat(DateTime.fromJSDate(value));
      proxy.value.value = {
        value: value,
        title: datetime,
      };
      choiceValue.value = value;
      break;
    case BackendDataType.ENUM:
      // value 为 v-select 选中的选项对象（title 为 i18n 键），保存原始键与翻译后的展示名
      proxy.value.value = {
        value: value?.value,
        title: i18n.global.t(String(value?.title ?? "")),
      };
      choiceValue.value = value?.value;
      break;
    case BackendDataType.BOOLEAN:
      // 布尔筛选值保存原始布尔值，title 统一存翻译后的“是/否”，使 chip 展示与其他类型一致
      proxy.value.value = {
        value: value,
        title: i18n.global.t(
          value === true || value === "true" ? "model.__base.value.yes" : "model.__base.value.no",
        ),
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
  // 枚举/布尔字段在编辑时回填当前选中值，使下拉框展示已有过滤条件
  if (
    choiceOption.value?.type === BackendDataType.ENUM ||
    choiceOption.value?.type === BackendDataType.BOOLEAN
  ) {
    choiceValue.value = props.modelValue.value.value;
  }
});

const props = defineProps({
  modelValue: {
    type: Object as PropType<SearchItem>,
    default: () =>
      ({
        field: { value: "", title: "" },
        action: { value: "", title: "" },
        value: { value: null, title: "" },
      }) as SearchItem,
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

<style scoped>
/* 字段/动作/值三段输入控件统一宽度：
   v-menu 内容按内容收缩时，值输入可能因 min-width（如日期 180px、日期时间 245px）
   或内容撑宽而与其他两段不一致；卡片最小/最大宽度走全局 .zk-clause-card，
   此处仅让所有输入控件撑满，保证三段（字段/动作/值）始终等宽 */
.zk-clause-card :deep(.v-input) {
  width: 100%;

  /* 统一输入控件高度（compact），保证字段/动作/值三段等高的兜底约束 */
  --v-input-control-height: var(--zk-control-height-compact);
}
</style>
