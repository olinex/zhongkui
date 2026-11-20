<template>
  <!-- 明文查看对话框：加密字段解密 / S3 body 按对象键读取的明文展示，
       支持文本/十六进制预览与下载；open() 由调用方在行详情与列表单元格触发 -->
  <app-dialog v-model="dialog" :title="title" size="lg">
    <v-alert v-if="!isText" type="info" class="mb-2">
      {{ $t("notice.plaintextBinary") }}
    </v-alert>
    <div class="plaintext-preview">
      <pre class="font-mono">{{ isText ? text : hex }}</pre>
    </div>
    <div v-if="truncated" class="zk-hint mt-1">
      {{ $t("notice.plaintextTruncated") }}
    </div>
    <template #footer>
      <v-btn
        :text="$t('action.download')"
        prepend-icon="mdi-download"
        variant="tonal"
        @click="downloadAction"
      />
      <v-btn :text="$t('action.close')" variant="text" @click="dialog = false" />
    </template>
  </app-dialog>
</template>

<script setup lang="ts" name="plaintext-dialog">
import axios from "axios";

import { plaintextCryptoGet, plaintextS3Get } from "@/apis/traffic";
import type { DetailField } from "@/components/@types/detail";
import { useAppStore } from "@/stores/app";
import i18n from "@/utils/i18n";

const TEXT_PREVIEW_LIMIT = 100000;
const HEX_PREVIEW_LIMIT = 1024;
// 控制字符（除 \t \n \r 外）出现即视为二进制内容
const BINARY_CONTROL_RE = /[\u0000-\u0008\u000B\u000C\u000E-\u001F]/;

// truncateChars 按字符（码点）截断文本。
// 先完整解码再截断字符，避免按字节切片把多字节字符/代理对切断，
// 导致预览尾部出现 U+FFFD 乱码；for...of 按码点迭代，不会拆开 emoji 等代理对。
const truncateChars = (value: string, limit: number): string => {
  let result = "";
  let count = 0;
  for (const char of value) {
    if (count >= limit) {
      break;
    }
    result += char;
    count += 1;
  }
  return result;
};

const appStore = useAppStore();

const dialog = ref(false);
const title = ref("");
const fieldKey = ref("");
const text = ref("");
const hex = ref("");
const isText = ref(true);
const truncated = ref(false);
const blobUrl = ref("");

const parseErrorBody = (body: string): string => {
  try {
    const json = JSON.parse(body) as { summary?: string };
    return json.summary || body;
  } catch {
    return body;
  }
};

const parsePlaintextError = (err: unknown): string => {
  if (axios.isAxiosError(err) && err.response) {
    const data = err.response.data;
    if (data instanceof ArrayBuffer) {
      return parseErrorBody(new TextDecoder("utf-8").decode(data));
    }
    if (typeof data === "string") {
      return parseErrorBody(data);
    }
  }
  return i18n.global.t("notice.plaintextLoadFailed");
};

const toHex = (bytes: Uint8Array, limit: number): string => {
  return Array.from(bytes.slice(0, limit), (byte) => {
    return byte.toString(16).padStart(2, "0");
  }).join(" ");
};

// open 打开明文查看对话框：field 为详情字段（kind 决定接口分支，value 为密文/对象键），
// model 为模型注册名（与后端权限点 plaintext:{crypto|s3}:{model}:{field} 对应），label 为字段标题
const open = async (field: DetailField, model: string, label: string): Promise<void> => {
  try {
    const rsp =
      field.kind === "s3"
        ? await plaintextS3Get({
            model,
            field: field.key,
            object_key: String(field.value),
          })
        : await plaintextCryptoGet({
            model,
            field: field.key,
            ciphertext: String(field.value),
          });
    const bytes = new Uint8Array(rsp.data as ArrayBuffer);
    // 文本判定用流式解码的样本：stream 模式会暂存样本末尾不完整的多字节序列，
    // 避免按字节截断处被误判为乱码，把正常文本当成二进制
    const sample = new TextDecoder("utf-8").decode(bytes.slice(0, TEXT_PREVIEW_LIMIT), {
      stream: true,
    });
    isText.value = !sample.includes("\uFFFD") && !BINARY_CONTROL_RE.test(sample);
    if (isText.value) {
      // 文本预览：完整解码后按字符截断
      const fullText = new TextDecoder("utf-8").decode(bytes);
      text.value = truncateChars(fullText, TEXT_PREVIEW_LIMIT);
      hex.value = "";
      truncated.value = text.value.length !== fullText.length;
    } else {
      // 二进制预览：十六进制仅展示前 HEX_PREVIEW_LIMIT 字节
      text.value = "";
      hex.value = toHex(bytes, HEX_PREVIEW_LIMIT);
      truncated.value = bytes.length > HEX_PREVIEW_LIMIT;
    }
    fieldKey.value = field.key;
    title.value = label;
    if (blobUrl.value) {
      URL.revokeObjectURL(blobUrl.value);
    }
    blobUrl.value = URL.createObjectURL(new Blob([bytes]));
    dialog.value = true;
  } catch (err) {
    appStore.notify({
      text: parsePlaintextError(err),
      timeout: 3000,
      color: "error",
    });
  }
};

const downloadAction = (): void => {
  const url = blobUrl.value;
  if (!url) {
    return;
  }
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = `${fieldKey.value}.bin`;
  anchor.click();
};

watch(dialog, (open) => {
  if (!open && blobUrl.value) {
    URL.revokeObjectURL(blobUrl.value);
    blobUrl.value = "";
  }
});

defineExpose({ open });
</script>

<style scoped>
/* 明文预览区：内容过长时纵向滚动。 */
.plaintext-preview {
  max-height: var(--zk-preview-max-height);
  overflow-y: auto;
}

.plaintext-preview pre {
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
