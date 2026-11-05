<template>
  <div>
    <v-chip :color="color" :variant="variant" :density="density" @click="dialog = true">
      <span class="font-mono">{{ short }}</span>
    </v-chip>
    <!-- 宽度来自 AppDialog 的尺寸档位（sm = 560px）；内容区经 AppDialog 的 scrollable
         滚动，故无需本地 max-height。 -->
    <app-dialog v-model="dialog" :title="$t('action.detail')" size="sm">
      <json-viewer :data="value && JSON.parse(value)" />
      <template #footer>
        <v-btn color="info" @click="copy">
          {{ $t("action.copy") }}
        </v-btn>
      </template>
    </app-dialog>
  </div>
</template>

<script setup lang="ts" name="json-chip">
import i18n from "@/utils/i18n";
import type { Density, Variant } from "../@types/base";
import { copyToClipboard } from "@/utils/clipboard";

const dialog = ref(false);

const short = computed(() => {
  const raw = props.value.trim();
  if (raw === "") {
    return i18n.global.t("model.__base.value.blank");
  }
  const firstChar = raw.charAt(0);
  switch (firstChar) {
    case "[":
      return "[...]";
    case "{":
      return "{...}";
    default:
      return raw;
  }
});

const close = () => {
  dialog.value = false;
};

const copy = async () => {
  copyToClipboard(props.value);
  close();
};

// 默认保持 undefined，由主题层统一 VChip 基线（small + tonal）；
// 调用方仅在确有语义差异时覆盖。
const props = defineProps({
  value: {
    type: String,
    required: true,
  },
  color: {
    type: String,
    default: undefined,
  },
  variant: {
    type: String as PropType<Variant>,
    default: undefined,
  },
  density: {
    type: String as PropType<Density>,
    default: undefined,
  },
});
</script>
