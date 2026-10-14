<template>
  <!-- 列表启用态开关单元格：即时写入 + 乐观更新 + 按行 pending。
       展示态经 modelValue 传入（父级用 v-model 绑定行字段），写入经 request(next) 发起；
       请求期间仅该行显示 loading 并禁用，失败时回退到原状态并提示，成功后无需整表刷新。
       size 写 Vuetify 档位名 small（即「sm」档，无 sm 字面值；全站已完成同值登记，
       此处显式声明以固定列表开关档位）。 -->
  <v-switch
    :model-value="modelValue"
    size="small"
    color="primary"
    hide-details
    :loading="pending"
    :disabled="pending || disabled"
    @update:model-value="onToggle"
  />
</template>

<script setup lang="ts" name="enabled-switch-cell">
import type { PropType } from "vue";
import type { AxiosPromise } from "axios";
import { getSimpleResponse } from "@/apis/base";
import { useAppStore } from "@/stores/app";

const props = defineProps({
  modelValue: {
    type: Boolean,
    required: true,
  },
  // 写入请求：入参为开关目标态；允许返回 Promise（如先取详情再以整表单更新的复合请求）
  request: {
    type: Function as PropType<(next: boolean) => AxiosPromise>,
    required: true,
  },
  disabled: {
    type: Boolean,
    default: false,
  },
});

const emits = defineEmits<{ "update:modelValue": [boolean] }>();

const appStore = useAppStore();
// 该行写入进行中标记：只作用于本行实例，不影响其他行与整表
const pending = ref(false);

const onToggle = async (next: boolean | null): Promise<void> => {
  const target = next === true;
  if (target === props.modelValue) {
    return;
  }
  const previous = props.modelValue;
  // 乐观更新：先落 UI，失败时回退
  emits("update:modelValue", target);
  pending.value = true;
  try {
    await getSimpleResponse(
      props.request(target),
      async (rsp) => {
        appStore.notify({
          text: rsp.summary,
          timeout: 3000,
          color: "success",
        });
      },
      async (rsp) => {
        emits("update:modelValue", previous);
        appStore.notify({
          text: rsp.summary,
          timeout: 3000,
          color: "error",
        });
      },
    );
  } finally {
    pending.value = false;
  }
};
</script>
