<template>
  <central-layout>
    <template #prepend>
      <app-header />
    </template>
    <template #body>
      <v-card prepend-avatar="@/assets/logo.png" style="width: min(420px, 100%)">
        <template #title>
          <span class="font-weight-black">{{ $t("title.zk") }}</span>
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
            <password-field
              v-model="password"
              v-bind="passwordProps"
              :label="$t('model.user.field.password')"
            />
            <!-- 将提交按钮放入表单内，使其成为默认按钮，
                 这样在任一输入框内按回车即可隐式提交表单 -->
            <v-card-actions class="d-flex justify-space-between">
              <v-btn type="button" variant="text" color="primary" class="ma-2 pa-2">{{
                $t("action.pwdForget")
              }}</v-btn>
              <v-btn
                v-request="submit"
                :disabled="!meta.dirty || !meta.valid"
                type="submit"
                color="primary"
                variant="flat"
                class="ma-2 pa-2"
                >{{ $t("action.signIn") }}</v-btn
              >
            </v-card-actions>
          </v-form>
        </template>
      </v-card>
    </template>
  </central-layout>
</template>

<route lang="yaml">
name: Sign In
meta:
  title: title.signIn
  requiresAuth: false
  requiresPerm: false
</route>

<script setup lang="ts">
import { useForm } from "vee-validate";
import router from "@/router";
import { loginSchema } from "@/schemas/user";
import { passwordLogin } from "@/apis/user";
import { getResponse } from "@/apis/base";
import { useUserStore } from "@/stores/user";
import { localManager } from "@/stores/local";
import { vuetifyCfg } from "@/utils/form";
import i18n from "@/utils/i18n";
import { useAppStore } from "@/stores/app";

// Types
import type { UserTokenOut } from "@/apis/@types/user";

const route = useRoute();
const { defineField, handleSubmit, setFieldError, meta } = useForm({
  validationSchema: loginSchema,
});

const [username, usernameProps] = defineField("username", vuetifyCfg);
const [password, passwordProps] = defineField("password", vuetifyCfg);

const appStore = useAppStore();
const userStore = useUserStore();

const submit = handleSubmit(async (values) => {
  await getResponse<UserTokenOut>(
    passwordLogin({
      // 用户名单行文本提交前去除首尾空白，避免误输入空白导致登录失败
      // （密码字段按规范豁免 trim：首尾空白是密码的合法字符）
      username: (values.username as string).trim(),
      password: values.password as string,
    }),
    async (rsp) => {
      const redirect = route.query.redirect as string;
      const next = redirect || "/";
      setTimeout(async () => {
        // 将登录返回的令牌持久化到本地存储，避免依赖受主机作用域限制的 Cookie
        localManager.setJwt(rsp.data.token);
        // 等待用户信息加载完成后再跳转，否则路由守卫会误判未登录而跳转 401
        await userStore.load();
        router.push(next);
        appStore.notify({
          text: i18n.global.t("notice.wellcome"),
          timeout: 1000,
          color: "success",
        });
      }, 1000);
    },
    async (rsp) => {
      switch (typeof rsp.details) {
        case "string":
        case "undefined":
          break;
        default:
          Object.entries(rsp.details).forEach(([key, value]) => {
            setFieldError(key, value);
          });
      }
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "error",
      });
    },
  );
});
</script>
