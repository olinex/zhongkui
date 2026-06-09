<template>
  <v-dialog v-model="dialog" max-width="600">
    <template #default>
      <v-card :prepend-icon="prependIcon" :title="title">
        <v-card-text>
          <slot name="body" :proxy="proxy" />
          <v-text-field
            v-show="doubleCheck"
            v-model="value"
            variant="underlined"
          >
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
          <v-btn
            color="info"
            @click="confirm"
            :disabled="doubleCheck && random !== value"
          >
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
const proxy = ref({} as Record<string, any>);
const value = ref("");
const random = ref("");

const open = (item: Record<string, any>) => {
  proxy.value = { ...item };
  dialog.value = true;
  value.value = "";
  random.value = generateRandomString(6);
};

const close = () => {
  dialog.value = false;
};

const confirm = async () => {
  emits("confirm", proxy.value);
  close();
};

const emits = defineEmits(["confirm"]);

const props = defineProps({
  prependIcon: {
    type: String,
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
