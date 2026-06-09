<template>
  <central-layout>
    <template #prepend>
      <app-header />
    </template>
    <template #body>
      <temp-editable-dialog
        ref="createAccessDialogRef"
        :title="$t(`title.accessKey`)"
        @confirm="accessKeyCreateAction"
        :confirm-disabled="
          !accessKeyCreateForm.meta.value.dirty ||
          !accessKeyCreateForm.meta.value.valid
        "
        prepend-icon="mdi-key"
        force-editable
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
      <v-card
        class="justify-center align-center"
        prepend-avatar="@/assets/logo.png"
        width="80%"
        dense
      >
        <template #title>
          <span class="font-weight-black">{{ userStore.nickleName }}</span>
        </template>
        <template #subtitle>
          <hex-string-chip
            size="x-small"
            :value="userStore.unionID"
            prefix="uid: "
            :length="32"
          />
        </template>
        <v-spacer />
        <v-card-text>
          <v-expansion-panels flat>
            <v-expansion-panel :title="$t('title.profileInfo')">
              <template #text>
                <v-card flat>
                  <template #text>
                    <v-form>
                      <v-text-field
                        v-model="nickleName"
                        v-bind="nickleNameProps"
                        :label="$t('model.user.field.nickleName')"
                      />
                      <v-text-field
                        v-model="email"
                        v-bind="emailProps"
                        :label="$t('model.user.field.email')"
                      />
                    </v-form>
                  </template>
                  <v-card-actions class="d-flex justify-center" flat>
                    <v-btn
                      :disabled="
                        !profileForm.meta.value.dirty ||
                        !profileForm.meta.value.valid
                      "
                      type="submit"
                      color="primary"
                      class="ma-2 pa-2"
                      @click="profileChangeSubmit"
                    >
                      {{ $t("action.update") }}
                    </v-btn>
                  </v-card-actions>
                </v-card>
              </template>
            </v-expansion-panel>
            <v-expansion-panel
              :title="$t('title.accessKey')"
              @group:selected="accessKeyListGetAction"
            >
              <template #text>
                <confirm-dialog
                  ref="confirmDialogRef"
                  :title="$t('model.accessKey.name')"
                  prependIcon="mdi-key-remove"
                  @confirm="accessKeyRemoveAction"
                  double-check
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
                <v-card
                  v-for="key in accessKeys"
                  variant="text"
                  :key="key.id"
                  :subtitle="key.description"
                  density="compact"
                >
                  <template #prepend>
                    <boolean-chip :value="key.enabled" />
                  </template>
                  <template #title>
                    {{ key.accessKeyID }}
                    <copy-btn :value="key.accessKeyID" />
                  </template>
                  <v-card-item>
                    <v-chip-group disabled>
                      <v-chip density="compact">
                        {{ $t("model.accessKey.field.createdAt") }}:
                        {{
                          localeDatetimeFormat(DateTime.fromISO(key.createdAt))
                        }}
                      </v-chip>
                      <v-chip density="compact">
                        {{ $t("model.accessKey.field.updatedAt") }}:
                        {{
                          localeDatetimeFormat(DateTime.fromISO(key.updatedAt))
                        }}
                      </v-chip>
                      <v-chip v-if="key.expiredAt !== null" density="compact">
                        {{ $t("model.accessKey.field.expiredAt") }}:
                        {{
                          localeDatetimeFormat(DateTime.fromISO(key.expiredAt))
                        }}
                      </v-chip>
                      <v-chip
                        v-if="key.lastAccessAt !== null"
                        density="compact"
                      >
                        {{ $t("model.accessKey.field.lastAccessAt") }}:
                        {{
                          localeDatetimeFormat(
                            DateTime.fromISO(key.lastAccessAt)
                          )
                        }}
                      </v-chip>
                    </v-chip-group>
                    <v-divider />
                  </v-card-item>
                  <template #append class="flex-column align-end">
                    <v-btn
                      variant="text"
                      @click="copyToClipboard(key.accessKeySecret)"
                    >
                      {{ $t("action.copySecret") }}
                    </v-btn>
                    <v-btn
                      variant="text"
                      @click="accessKeyEnabledAction(key.id, !key.enabled)"
                      >{{
                        $t(
                          key.enabled ? "action.deactivate" : "action.activate"
                        )
                      }}</v-btn
                    >
                    <v-btn
                      variant="text"
                      color="error"
                      @click="openConfirmDialogAction(key)"
                    >
                      {{ $t("action.delete") }}
                    </v-btn>
                  </template>
                </v-card>
                <v-card-actions class="d-flex justify-center">
                  <v-btn
                    color="primary"
                    @click="openCreateAccessDialogAction"
                    >{{ $t("action.create") }}</v-btn
                  >
                </v-card-actions>
              </template>
            </v-expansion-panel>
            <v-expansion-panel :title="$t('title.passwordReset')">
              <template #text>
                <v-card flat>
                  <template #text>
                    <v-form>
                      <password-field
                        v-model="oldPassword"
                        v-bind="oldPasswordProps"
                        :label="$t('model.user.field.oldPassword')"
                      />
                      <password-field
                        v-model="newPassword"
                        v-bind="newPasswordProps"
                        :label="$t('model.user.field.newPassword')"
                      />
                      <password-field
                        v-model="newPasswordAgain"
                        v-bind="newPasswordAgainProps"
                        :label="$t('model.user.field.newPasswordAgain')"
                      />
                    </v-form>
                  </template>
                  <v-card-actions class="d-flex justify-center" flat>
                    <v-btn
                      :disabled="
                        !pwdResetForm.meta.value.dirty ||
                        !pwdResetForm.meta.value.valid
                      "
                      type="submit"
                      color="primary"
                      class="ma-2 pa-2"
                      @click="passwordResetSubmit"
                    >
                      {{ $t("action.reset") }}
                    </v-btn>
                  </v-card-actions>
                </v-card>
              </template>
            </v-expansion-panel>
          </v-expansion-panels>
        </v-card-text>
      </v-card>
    </template>
  </central-layout>
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
import {
  resetPasswordSchema,
  changeProfileSchema,
  accessKeySchema,
} from "@/schemas/user";
import { useUserStore } from "@/stores/user";
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
import type { accessKeyOut } from "@/apis/@types/user";
import { DateTime } from "luxon";
import { copyToClipboard } from "@/utils/clipboard";

const appStore = useAppStore();
const userStore = useUserStore();
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

const [nickleName, nickleNameProps] = profileForm.defineField(
  "nickleName",
  vuetifyCfg
);
const [email, emailProps] = profileForm.defineField("email", vuetifyCfg);
const [oldPassword, oldPasswordProps] = pwdResetForm.defineField(
  "oldPassword",
  vuetifyCfg
);
const [newPassword, newPasswordProps] = pwdResetForm.defineField(
  "newPassword",
  vuetifyCfg
);
const [newPasswordAgain, newPasswordAgainProps] = pwdResetForm.defineField(
  "newPasswordAgain",
  vuetifyCfg
);
const [enabled, enabledProps] = accessKeyCreateForm.defineField(
  "enabled",
  vuetifyCfg
);
const [description, descriptionProps] = accessKeyCreateForm.defineField(
  "description",
  vuetifyCfg
);
const [expiredAt, expiredAtProps] = accessKeyCreateForm.defineField(
  "expiredAt",
  vuetifyCfg
);

const accessKeys = ref([] as accessKeyOut[]);

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
      nickleName: values.nickleName as string,
      email: values.email as string,
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
    }
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
    }
  );
});

const accessKeyListGetAction = async (val: { value: boolean }) => {
  if (!val.value) {
    return;
  }
  await getResponse<accessKeyOut[]>(
    accessKeyListGet(),
    async (rsp) => {
      accessKeys.value = rsp.data;
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
    }
  );
};

const accessKeyCreateAction = accessKeyCreateForm.handleSubmit(
  async (values): Promise<void> => {
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
      }
    );
  }
);

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
    }
  );
};

const accessKeyEnabledAction = async (
  id: number,
  enabled: boolean
): Promise<void> => {
  await getResponse<accessKeyOut>(
    accessKeyEnabled(id, enabled),
    async (rsp) => {
      accessKeys.value = accessKeys.value.map((item) =>
        item.id !== id ? item : rsp.data
      );
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
    }
  );
};
</script>
