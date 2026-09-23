<template>
  <central-layout>
    <template #prepend>
      <app-header />
    </template>
    <template #body>
      <v-card prepend-avatar="@/assets/logo.png">
        <template #title>
          <span class="font-weight-black">{{ $t("title.pwdForget") }}</span>
        </template>
        <template #text>
          <v-spacer />
          <v-form @submit.prevent="submit">
            <v-text-field
              v-model="username"
              v-bind="usernameProps"
              :label="$t('model.user.field.username')"
              autofocus
            />
            <v-text-field
              v-model="email"
              v-bind="emailProps"
              :label="$t('model.user.field.email')"
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
                >{{ $t("action.forgotSend") }}</v-btn
              >
            </v-card-actions>
          </v-form>
        </template>
      </v-card>
    </template>
  </central-layout>
</template>

<route lang="yaml">
name: Forgot Password
meta:
  title: title.pwdForget
  requiresAuth: false
  requiresPerm: false
</route>

<script setup lang="ts">
import { useForm } from "vee-validate";
import router from "@/router";
import { forgotPasswordSchema } from "@/schemas/user";
import { passwordForgot } from "@/apis/user";
import { getResponse } from "@/apis/base";
import { useAppStore } from "@/stores/app";
import { vuetifyCfg } from "@/utils/form";
import i18n from "@/utils/i18n";

const { defineField, handleSubmit, meta } = useForm({
  validationSchema: forgotPasswordSchema,
});

const [username, usernameProps] = defineField("username", vuetifyCfg);
const [email, emailProps] = defineField("email", vuetifyCfg);

const appStore = useAppStore();

// 为避免账号枚举，后端无论账号/邮箱是否匹配均返回成功，前端统一展示发送提示
const submit = handleSubmit(async (values) => {
  await getResponse(
    passwordForgot({
      // 单行文本提交前去除首尾空白，避免误输入空白导致校验失败
      username: (values.username as string).trim(),
      email: (values.email as string).trim(),
    }),
    async () => {
      appStore.notify({
        text: i18n.global.t("notice.forgotEmailSent"),
        timeout: 5000,
        color: "success",
      });
      // 提示后返回登录页，由用户在邮箱完成后续重置
      await router.push(`/${appStore.language}/auth/sign-in`);
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
