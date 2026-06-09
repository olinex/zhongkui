<template>
  <central-layout>
    <template #prepend>
      <app-header />
    </template>
    <template #body>
      <v-card
        class="justify-center align-center"
        prepend-avatar="@/assets/logo.png"
        width="30%"
      >
        <template #title>
          <span class="font-weight-black">{{ $t("title.zk") }}</span>
        </template>
        <template #text>
          <v-spacer />
          <v-form>
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
          </v-form>
        </template>

        <v-card-actions class="d-flex justify-space-between">
          <v-btn color="third" class="ma-2 pa-2">{{
            $t("action.pwdForget")
          }}</v-btn>
          <v-btn
            :disabled="!meta.dirty || !meta.valid"
            @click="submit"
            @keyup.enter="submit"
            type="submit"
            color="primary"
            class="ma-2 pa-2"
            >{{ $t("action.signIn") }}</v-btn
          >
        </v-card-actions>
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
import { getSimpleResponse } from "@/apis/base";
import { useUserStore } from "@/stores/user";
import { vuetifyCfg } from "@/utils/form";
import i18n from "@/utils/i18n";
import { useAppStore } from "@/stores/app";

const route = useRoute();
const { defineField, handleSubmit, setFieldError, meta } = useForm({
  validationSchema: loginSchema,
});

const [username, usernameProps] = defineField("username", vuetifyCfg);
const [password, passwordProps] = defineField("password", vuetifyCfg);

const appStore = useAppStore();
const userStore = useUserStore();

const submit = handleSubmit(async (values) => {
  await getSimpleResponse(
    passwordLogin({
      username: values.username as string,
      password: values.password as string,
    }),
    async () => {
      let redirect = route.query.redirect as string;
      let next = redirect || "/";
      setTimeout(() => {
        userStore.load();
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
    }
  );
});
</script>
