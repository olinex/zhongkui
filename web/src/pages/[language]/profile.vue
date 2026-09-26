<template>
  <v-layout>
    <app-header />
    <v-main class="d-flex mb-6">
      <v-container class="pa-4 pa-md-6 zk-page-container">
        <temp-editable-dialog
          ref="createAccessDialogRef"
          :title="$t(`title.accessKey`)"
          :confirm-disabled="
            !accessKeyCreateForm.meta.value.dirty || !accessKeyCreateForm.meta.value.valid
          "
          prepend-icon="mdi-key"
          force-editable
          @confirm="accessKeyCreateAction"
        >
          <template #body>
            <v-form>
              <v-row dense>
                <v-col cols="9">
                  <datetime-field
                    v-model="expiredAt"
                    v-bind="expiredAtProps"
                    :label="$t('model.accessKey.field.expiredAt')"
                  />
                </v-col>
                <v-col cols="3" align-self="center">
                  <v-switch
                    v-model="enabled"
                    v-bind="enabledProps"
                    color="success"
                    :label="$t('model.accessKey.field.enabled')"
                  />
                </v-col>
              </v-row>
              <v-row dense>
                <v-col cols="12">
                  <v-textarea
                    v-model="description"
                    v-bind="descriptionProps"
                    :label="$t('model.accessKey.field.description')"
                  />
                </v-col>
              </v-row>
            </v-form>
          </template>
        </temp-editable-dialog>

        <confirm-dialog
          ref="confirmDialogRef"
          :title="$t('model.accessKey.name')"
          prepend-icon="mdi-key-remove"
          double-check
          @confirm="accessKeyRemoveAction"
        >
          <template #body="{ proxy }">
            <confirm-alert
              :action="$t('action.delete')"
              :text="
                $t(`notice.deleteConfirm`, {
                  category: $t(`model.accessKey.name`),
                  key: proxy.accessKeyID,
                })
              "
            />
          </template>
        </confirm-dialog>

        <div class="d-flex align-center mb-5">
          <div>
            <h1 class="text-h5 font-weight-bold mb-0">{{ $t("title.userProfile") }}</h1>
            <p class="text-body-2 text-on-surface-variant mb-0">{{ $t("profile.pageSubtitle") }}</p>
          </div>
          <v-spacer />
          <v-btn
            icon="mdi-arrow-left"
            variant="text"
            size="large"
            :title="$t('action.back')"
            :aria-label="$t('action.back')"
            @click="goBack"
          />
          <v-btn
            icon="mdi-home-outline"
            variant="text"
            size="large"
            :title="$t('title.home')"
            :aria-label="$t('title.home')"
            @click="gotoHome"
          />
        </div>

        <v-row>
          <!-- Profile summary -->
          <v-col cols="12" md="4" xl="3">
            <v-card class="profile-summary">
              <div class="text-center pa-6">
                <v-avatar size="96" color="primary" variant="tonal" class="mx-auto mb-4">
                  <v-img src="@/assets/logo.png" cover />
                </v-avatar>
                <div class="text-h6 font-weight-bold text-truncate">{{ userStore.nickleName }}</div>
                <div class="text-body-2 text-on-surface-variant text-truncate mb-3">
                  {{ userStore.email || "—" }}
                </div>
                <hex-string-chip
                  size="x-small"
                  :value="userStore.unionID"
                  prefix="uid: "
                  :length="32"
                />
              </div>
            </v-card>
          </v-col>

          <!-- Settings sections：面板组裸放在描边盒内，盒内平铺条目（见 AGENTS.md「展开面板族」） -->
          <v-col cols="12" md="8" xl="9">
            <div class="zk-panel pa-4 mb-6">
              <v-expansion-panels>
                <!-- 个人信息 -->
                <v-expansion-panel eager>
                  <expansion-panel-title
                    icon="mdi-account-outline"
                    :title="$t('title.profileInfo')"
                    :subtitle="$t('profile.infoSubtitle')"
                  />
                  <v-expansion-panel-text>
                    <v-form>
                      <v-row dense>
                        <v-col cols="12" sm="6">
                          <v-text-field
                            v-model="nickleName"
                            v-bind="nickleNameProps"
                            :label="$t('model.user.field.nickleName')"
                          />
                        </v-col>
                        <v-col cols="12" sm="6">
                          <v-text-field
                            v-model="email"
                            v-bind="emailProps"
                            :label="$t('model.user.field.email')"
                          />
                        </v-col>
                      </v-row>
                    </v-form>
                    <div class="d-flex justify-end pt-3">
                      <v-btn
                        v-request="profileChangeSubmit"
                        :disabled="!profileForm.meta.value.dirty || !profileForm.meta.value.valid"
                        color="primary"
                        prepend-icon="mdi-content-save-outline"
                      >
                        {{ $t("action.update") }}
                      </v-btn>
                    </div>
                  </v-expansion-panel-text>
                </v-expansion-panel>

                <!-- 访问密钥 -->
                <v-expansion-panel eager>
                  <expansion-panel-title
                    icon="mdi-key-outline"
                    :title="$t('title.accessKey')"
                    :subtitle="$t('profile.accessKeySubtitle')"
                  />
                  <v-expansion-panel-text>
                    <div class="d-flex justify-end mb-2">
                      <v-btn
                        color="primary"
                        size="small"
                        prepend-icon="mdi-plus"
                        @click="openCreateAccessDialogAction"
                      >
                        {{ $t("action.create") }}
                      </v-btn>
                    </div>

                    <!-- Empty state -->
                    <div v-if="accessKeys.length === 0" class="text-center pa-8">
                      <v-icon size="48" class="mb-3 text-on-surface-variant"
                        >mdi-key-off-outline</v-icon
                      >
                      <div class="text-body-2 text-on-surface-variant">
                        {{ $t("profile.emptyAccessKeys") }}
                      </div>
                    </div>

                    <!-- Access key list -->
                    <template v-else>
                      <template v-for="(key, index) in accessKeys" :key="key.id">
                        <v-divider v-if="index > 0" />
                        <div class="py-4">
                          <div class="d-flex align-center ga-3 flex-wrap">
                            <boolean-chip :value="key.enabled" />
                            <span class="font-mono text-body-2 font-weight-medium">{{
                              key.accessKeyID
                            }}</span>
                            <copy-btn :value="key.accessKeyID" />
                            <v-spacer />
                            <v-btn
                              size="small"
                              variant="text"
                              prepend-icon="mdi-content-copy"
                              @click="copyToClipboard(key.accessKeySecret)"
                            >
                              {{ $t("action.copySecret") }}
                            </v-btn>
                            <v-btn
                              size="small"
                              variant="text"
                              :color="key.enabled ? undefined : 'success'"
                              @click="accessKeyEnabledAction(key.id, !key.enabled)"
                            >
                              {{ $t(key.enabled ? "action.deactivate" : "action.activate") }}
                            </v-btn>
                            <v-btn
                              size="small"
                              variant="text"
                              color="error"
                              prepend-icon="mdi-delete-outline"
                              @click="openConfirmDialogAction(key)"
                            >
                              {{ $t("action.delete") }}
                            </v-btn>
                          </div>
                          <div
                            v-if="key.description"
                            class="text-body-2 text-on-surface-variant mt-2"
                          >
                            {{ key.description }}
                          </div>
                          <div class="d-flex flex-wrap ga-2 mt-2">
                            <v-chip density="compact" prepend-icon="mdi-calendar-outline">
                              {{ $t("model.accessKey.field.createdAt") }}:
                              {{ localeDatetimeFormat(DateTime.fromISO(key.createdAt)) }}
                            </v-chip>
                            <v-chip
                              v-if="key.updatedAt"
                              density="compact"
                              prepend-icon="mdi-calendar-refresh-outline"
                            >
                              {{ $t("model.accessKey.field.updatedAt") }}:
                              {{ localeDatetimeFormat(DateTime.fromISO(key.updatedAt)) }}
                            </v-chip>
                            <v-chip
                              v-if="key.expiredAt !== null"
                              density="compact"
                              prepend-icon="mdi-calendar-clock-outline"
                            >
                              {{ $t("model.accessKey.field.expiredAt") }}:
                              {{ localeDatetimeFormat(DateTime.fromISO(key.expiredAt)) }}
                            </v-chip>
                            <v-chip
                              v-if="key.lastAccessAt !== null"
                              density="compact"
                              prepend-icon="mdi-history"
                            >
                              {{ $t("model.accessKey.field.lastAccessAt") }}:
                              {{ localeDatetimeFormat(DateTime.fromISO(key.lastAccessAt)) }}
                            </v-chip>
                          </div>
                        </div>
                      </template>
                    </template>
                  </v-expansion-panel-text>
                </v-expansion-panel>

                <!-- 页面配置同步 -->
                <v-expansion-panel eager>
                  <expansion-panel-title
                    icon="mdi-cloud-sync-outline"
                    :title="$t('title.frontendConfigSync')"
                    :subtitle="$t('profile.configSyncSubtitle')"
                  />
                  <v-expansion-panel-text>
                    <v-switch
                      :model-value="configSyncStore.enabled"
                      color="success"
                      :label="$t('model.user.field.frontendConfigSyncEnabled')"
                      :hint="$t('profile.configSyncHint')"
                      persistent-hint
                      @update:model-value="configSyncEnabledChange"
                    />
                  </v-expansion-panel-text>
                </v-expansion-panel>

                <!-- 重置密码 -->
                <v-expansion-panel eager>
                  <expansion-panel-title
                    icon="mdi-lock-reset"
                    :title="$t('title.passwordReset')"
                    :subtitle="$t('profile.passwordSubtitle')"
                  />
                  <v-expansion-panel-text>
                    <v-form>
                      <v-row dense>
                        <v-col cols="12">
                          <password-field
                            v-model="oldPassword"
                            v-bind="oldPasswordProps"
                            :label="$t('model.user.field.oldPassword')"
                          />
                        </v-col>
                        <v-col cols="12" sm="6">
                          <password-field
                            v-model="newPassword"
                            v-bind="newPasswordProps"
                            :label="$t('model.user.field.newPassword')"
                          />
                        </v-col>
                        <v-col cols="12" sm="6">
                          <password-field
                            v-model="newPasswordAgain"
                            v-bind="newPasswordAgainProps"
                            :label="$t('model.user.field.newPasswordAgain')"
                          />
                        </v-col>
                      </v-row>
                    </v-form>
                    <div class="d-flex justify-end pt-3">
                      <v-btn
                        v-request="passwordResetSubmit"
                        :disabled="!pwdResetForm.meta.value.dirty || !pwdResetForm.meta.value.valid"
                        color="primary"
                        prepend-icon="mdi-lock-reset"
                      >
                        {{ $t("action.reset") }}
                      </v-btn>
                    </div>
                  </v-expansion-panel-text>
                </v-expansion-panel>
              </v-expansion-panels>
            </div>
          </v-col>
        </v-row>
      </v-container>
    </v-main>
  </v-layout>
</template>

<route lang="yaml">
name: Profile
meta:
  title: title.userProfile
  requiresAuth: true
  requiresPerm: false
</route>

<script setup lang="ts">
import { useForm } from "vee-validate";
import { resetPasswordSchema, changeProfileSchema, accessKeySchema } from "@/schemas/user";
import { useUserStore } from "@/stores/user";
import { useConfigSyncStore } from "@/stores/configSync";
import { getResponse, getSimpleResponse } from "@/apis/base";
import {
  passwordReset,
  profileChange,
  accessKeyListGet,
  accessKeyCreate,
  accessKeyRemove,
  accessKeyEnabled,
} from "@/apis/user";
import i18n, { localeDatetimeFormat } from "@/utils/i18n";
import { vuetifyCfg } from "@/utils/form";
import { useAppStore } from "@/stores/app";
import router from "@/router";
import type { accessKeyOut } from "@/apis/@types/user";
import { DateTime } from "luxon";
import { copyToClipboard } from "@/utils/clipboard";

const route = useRoute();
const appStore = useAppStore();
const userStore = useUserStore();
const configSyncStore = useConfigSyncStore();
const createAccessDialogRef = useTemplateRef("createAccessDialogRef");
const confirmDialogRef = useTemplateRef("confirmDialogRef");

const profileForm = useForm({
  validationSchema: changeProfileSchema,
  initialValues: {
    nickleName: userStore.nickleName,
    email: userStore.email,
  },
});
const pwdResetForm = useForm({
  validationSchema: resetPasswordSchema,
});
const accessKeyCreateForm = useForm({
  validationSchema: accessKeySchema,
});

const [nickleName, nickleNameProps] = profileForm.defineField("nickleName", vuetifyCfg);
const [email, emailProps] = profileForm.defineField("email", vuetifyCfg);
const [oldPassword, oldPasswordProps] = pwdResetForm.defineField("oldPassword", vuetifyCfg);
const [newPassword, newPasswordProps] = pwdResetForm.defineField("newPassword", vuetifyCfg);
const [newPasswordAgain, newPasswordAgainProps] = pwdResetForm.defineField(
  "newPasswordAgain",
  vuetifyCfg,
);
const [enabled, enabledProps] = accessKeyCreateForm.defineField("enabled", vuetifyCfg);
const [description, descriptionProps] = accessKeyCreateForm.defineField("description", vuetifyCfg);
const [expiredAt, expiredAtProps] = accessKeyCreateForm.defineField("expiredAt", vuetifyCfg);

const accessKeys = ref([] as accessKeyOut[]);

onMounted(() => {
  accessKeyListGetAction();
});

const goBack = () => {
  router.back();
};

const gotoHome = () => {
  // route.params type does not satisfy RouteLocationRaw.params
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  router.push({ name: "Home", params: route.params as any });
};

const openConfirmDialogAction = (key: accessKeyOut) => {
  confirmDialogRef.value?.open(key);
};

const openCreateAccessDialogAction = () => {
  const defaults = {
    enabled: true,
    expiredAt: null,
    description: "",
  };
  accessKeyCreateForm.resetField("enabled", { value: defaults.enabled });
  accessKeyCreateForm.resetField("expiredAt", { value: defaults.expiredAt });
  accessKeyCreateForm.resetField("description", {
    value: defaults.description,
  });
  createAccessDialogRef.value?.open(defaults);
};

const profileChangeSubmit = profileForm.handleSubmit(async (values) => {
  await getSimpleResponse(
    profileChange({
      // 单行文本提交前去除首尾空白，避免误输入空白导致数据异常
      nickleName: (values.nickleName as string).trim(),
      email: (values.email as string).trim(),
    }),
    async () => {
      appStore.notify({
        text: i18n.global.t("notice.profileUpdated"),
        timeout: 3000,
        color: "success",
      });
      await userStore.load();
      profileForm.resetField("nickleName", { value: values.nickleName });
      profileForm.resetField("email", { value: values.email });
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

const passwordResetSubmit = pwdResetForm.handleSubmit(async (values) => {
  await getSimpleResponse(
    passwordReset({
      oldPassword: values.oldPassword as string,
      newPassword: values.newPassword as string,
    }),
    async () => {
      appStore.notify({
        text: i18n.global.t("notice.passwordReset"),
        timeout: 3000,
        color: "success",
      });
      pwdResetForm.resetForm();
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

const accessKeyListGetAction = async () => {
  await getResponse<accessKeyOut[]>(
    accessKeyListGet(),
    async (rsp) => {
      accessKeys.value = rsp.data;
    },
    async (rsp) => {
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "error",
      });
    },
  );
};

const configSyncEnabledChange = async (enabled: boolean | null): Promise<void> => {
  await configSyncStore.setEnabled(enabled === true);
};

const accessKeyCreateAction = accessKeyCreateForm.handleSubmit(async (values): Promise<void> => {
  await getResponse<accessKeyOut>(
    accessKeyCreate({
      enabled: values.enabled,
      description: values.description,
      expiredAt: values.expiredAt,
    }),
    async (rsp) => {
      accessKeys.value.push(rsp.data);
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "success",
      });
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

const accessKeyRemoveAction = async (key: accessKeyOut): Promise<void> => {
  await getSimpleResponse(
    accessKeyRemove(key.id),
    async (rsp) => {
      accessKeys.value = accessKeys.value.filter((item) => item.id !== key.id);
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "success",
      });
    },
    async (rsp) => {
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "error",
      });
    },
  );
};

const accessKeyEnabledAction = async (id: number, enabled: boolean): Promise<void> => {
  await getResponse<accessKeyOut>(
    accessKeyEnabled(id, enabled),
    async (rsp) => {
      accessKeys.value = accessKeys.value.map((item) => (item.id !== id ? item : rsp.data));
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "success",
      });
    },
    async (rsp) => {
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "error",
      });
    },
  );
};
</script>
