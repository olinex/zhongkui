<template>
  <central-layout>
    <template #prepend>
      <app-header />
    </template>
    <template #body>
      <v-card prepend-avatar="@/assets/logo-gradient.svg">
        <template #title>
          <span class="font-weight-black">{{ $t("title.passwordReset") }}</span>
        </template>
        <template #text>
          <v-spacer />
          <!-- 缺少令牌：提示用户重新发起申请 -->
          <template v-if="missingToken">
            <v-alert type="warning" class="mb-4">{{ $t("notice.resetTokenMissing") }}</v-alert>
            <v-card-actions class="d-flex justify-center">
              <v-btn
                type="button"
                variant="text"
                color="primary"
                @click="$router.push(`/${appStore.language}/auth/forgot-password`)"
                >{{ $t("action.pwdForget") }}</v-btn
              >
            </v-card-actions>
          </template>
          <v-form v-else @submit.prevent="submit">
            <password-field
              v-model="newPassword"
              v-bind="newPasswordProps"
              :label="$t('model.user.field.newPassword')"
              autofocus
            />
            <password-field
              v-model="newPasswordAgain"
              v-bind="newPasswordAgainProps"
              :label="$t('model.user.field.newPasswordAgain')"
            />
            <!-- 将提交按钮放入表单内，使其成为默认按钮，
                 这样在任一输入框内按回车即可隐式提交表单 -->
            <v-card-actions class="d-flex justify-space-between">
              <v-btn
                type="button"
                variant="text"
                color="primary"
                @click="$router.push(`/${appStore.language}/auth/sign-in`)"
                >{{ $t("action.forgotBackToSignIn") }}</v-btn
              >
              <v-btn
                v-request="submit"
                :disabled="!meta.dirty || !meta.valid"
                type="submit"
                color="primary"
                variant="flat"
                >{{ $t("action.resetSubmit") }}</v-btn
              >
            </v-card-actions>
          </v-form>
        </template>
      </v-card>
    </template>
  </central-layout>
</template>

<route lang="yaml">
name: Reset Password
meta:
  title: title.passwordReset
  requiresAuth: false
  requiresPerm: false
</route>

<script setup lang="ts">
import { useForm } from "vee-validate";
import router from "@/router";
import { resetPasswordByTokenSchema } from "@/schemas/user";
import { passwordResetByToken } from "@/apis/user";
import { getResponse } from "@/apis/base";
import { useAppStore } from "@/stores/app";
import { vuetifyCfg } from "@/utils/form";

// 令牌由邮件链接携带（?token=...），自浏览器地址栏解析
const route = useRoute();
const token = (route.query.token as string) ?? "";
const missingToken = computed(() => !token);

const { defineField, handleSubmit, meta } = useForm({
  validationSchema: resetPasswordByTokenSchema,
});

// 令牌不渲染为输入框，仅作为表单隐藏字段随请求提交；初值自 URL 读取
const [tokenField] = defineField("token");
tokenField.value = token;

const [newPassword, newPasswordProps] = defineField("newPassword", vuetifyCfg);
const [newPasswordAgain, newPasswordAgainProps] = defineField("newPasswordAgain", vuetifyCfg);

const appStore = useAppStore();

const submit = handleSubmit(async (values) => {
  await getResponse(
    passwordResetByToken({
      token: (values.token as string).trim(),
      newPassword: values.newPassword as string,
    }),
    async () => {
      // 重置成功后跳转到引导说明页（倒计时 3 秒后自动进入登录页），
      // 而非直接跳转登录页并弹提示，便于用户明确获知重置结果。
      await router.push(`/${appStore.language}/auth/reset-success`);
    },
    async (rsp) => {
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "error",
      });
    },
  );
});
</script>
