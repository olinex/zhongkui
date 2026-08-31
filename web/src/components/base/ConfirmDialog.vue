<template>
  <v-dialog v-model="dialog" max-width="600">
    <template #default>
      <v-card :prepend-icon="prependIcon" :title="title">
        <v-card-text>
          <slot name="body" :proxy="proxy" />
          <v-text-field v-show="doubleCheck" v-model="value" variant="underlined">
            <template #append-inner>
              <v-chip variant="text">
                {{ $t("model.__base.content.doubleCheck", { random }) }}
              </v-chip>
            </template>
          </v-text-field>
        </v-card-text>
        <v-card-actions>
          <v-btn @click="close">
            {{ $t("action.close") }}
          </v-btn>
          <v-btn v-request="confirm" color="info" :disabled="doubleCheck && random !== value">
            {{ $t("action.confirm") }}
          </v-btn>
        </v-card-actions>
      </v-card>
    </template>
  </v-dialog>
</template>

<script setup lang="ts" name="confirm-dialog">
import { generateRandomString } from "@/utils/base";

const dialog = ref(false);
const proxy = ref({} as Record<string, unknown>);
const value = ref("");
const random = ref("");

const open = (item: object) => {
  proxy.value = { ...item };
  dialog.value = true;
  value.value = "";
  random.value = generateRandomString(6);
};

const close = () => {
  dialog.value = false;
};

// confirm 等待父级 @confirm 处理器返回的 Promise（若为异步请求）结束后再关窗，
// 配合 v-request 指令实现"点击后禁用 + 加载中动画"，防止慢接口重复提交。
const confirm = async () => {
  const result = emits("confirm", proxy.value);
  await Promise.resolve(result);
  close();
};

const emits = defineEmits(["confirm"]);

defineProps({
  prependIcon: {
    type: String,
    default: "",
  },
  title: {
    type: String,
    required: true,
  },
  doubleCheck: {
    type: Boolean,
    deafult: false,
  },
});

defineExpose({ open });
</script>
