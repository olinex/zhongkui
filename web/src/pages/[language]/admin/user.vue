<template>
  <main-layout current-nav="admin">
    <template #body>
      <search-table
        ref="searchTableRef"
        :list-request="userListGet"
        :options-request="userListOptions"
        :actions="getActions"
        :batch-actions="getBatchActions"
        :create="openCreateDialogAction"
        :project-fields="projectFields"
        model-name="user"
      >
        <!-- 启用列：开关即时写入，写入期间按行显示 loading 并禁用，失败回退，
             避免重复切换同一行 -->
        <template #cell-active="{ item }">
          <enabled-switch-cell
            v-model="item.active"
            :request="(enabled: boolean) => userSetActive(item.unionID, enabled)"
          />
        </template>
      </search-table>
      <confirm-dialog
        ref="batchConfirmRef"
        :title="$t('model.user.name')"
        prepend-icon="mdi-account-multiple-check"
        @confirm="batchAction"
      >
        <template #body>
          <confirm-alert
            :action="
              $t(batchState?.action === 'activate' ? 'action.activate' : 'action.deactivate')
            "
            :text="$t('notice.batchConfirm', { count: batchState?.items.length ?? 0 })"
          />
        </template>
      </confirm-dialog>
      <temp-editable-dialog
        ref="createDialogRef"
        :title="$t('model.user.name')"
        :confirm-disabled="!createForm.meta.value.dirty || !createForm.meta.value.valid"
        force-editable
        prepend-icon="mdi-account-plus"
        @confirm="createAction"
      >
        <template #body>
          <v-form>
            <v-row density="compact">
              <v-col cols="12">
                <v-text-field
                  v-model="createUsername"
                  v-bind="createUsernameProps"
                  :label="$t('model.user.field.username')"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="6">
                <password-field
                  v-model="createPassword"
                  v-bind="createPasswordProps"
                  :label="$t('model.user.field.password')"
                />
              </v-col>
              <v-col cols="6">
                <password-field
                  v-model="createPasswordAgain"
                  v-bind="createPasswordAgainProps"
                  :label="$t('model.user.field.passwordAgain')"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="6">
                <v-text-field
                  v-model="createNickleName"
                  v-bind="createNickleNameProps"
                  :label="$t('model.user.field.nickleName')"
                />
              </v-col>
              <v-col cols="6">
                <v-text-field
                  v-model="createEmail"
                  v-bind="createEmailProps"
                  :label="$t('model.user.field.email')"
                />
              </v-col>
            </v-row>
          </v-form>
        </template>
      </temp-editable-dialog>
      <temp-editable-dialog
        ref="editDialogRef"
        :title="$t(`model.user.name`)"
        :confirm-disabled="!meta.dirty || !meta.valid"
        prepend-icon="mdi-account"
        @confirm="itemUpdateAction"
      >
        <template #body="{ proxy, editable }">
          <v-form>
            <v-row density="compact">
              <v-col col="12">
                <v-text-field
                  v-model="proxy.username"
                  :label="$t('model.user.field.username')"
                  disabled
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col col="6">
                <v-text-field
                  v-model="proxy.nickleName"
                  v-bind="nickleNameProps"
                  :label="$t('model.user.field.nickleName')"
                  :disabled="!editable"
                />
              </v-col>
              <v-col col="6">
                <v-text-field
                  v-model="proxy.email"
                  v-bind="emailProps"
                  :label="$t('model.user.field.email')"
                  :disabled="!editable"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col col="12">
                <inner-edit-table
                  v-model="proxy.groups"
                  :headers="userPolicyHeaders"
                  :search-headers="userPolicySearchHeaders"
                  :search-request="groupListGet"
                  :list-item-action="true"
                  :editable="editable"
                  order-by-field="createdAt"
                  item-key-field="name"
                >
                  <template #list-item-action="{ item, editable: listEditable, removeItem }">
                    <v-btn
                      :disabled="!listEditable"
                      icon="mdi-minus"
                      variant="tonal"
                      color="error"
                      :title="$t('action.leave')"
                      :aria-label="$t('action.leave')"
                      @click="leaveGroupAction(proxy.unionID, item, removeItem)"
                    />
                  </template>
                  <template #item-subtitle="{ item }">
                    {{ item.description }}
                  </template>
                  <template #item-append="{ item, appendItem, close, editable: listEditable }">
                    <v-select
                      v-model="item.namespace"
                      :items="namespaceOpts"
                      :disabled="!listEditable"
                      hide-details
                      single-line
                      class="join-namespace-select"
                    />
                    <v-btn
                      v-request="
                        () =>
                          joinGroupAction(
                            proxy.unionID,
                            item.namespace,
                            item.name,
                            appendItem,
                            close,
                          )
                      "
                      :disabled="!(listEditable && item.namespace)"
                      :text="$t('action.join')"
                      color="primary"
                      variant="tonal"
                      size="small"
                    />
                  </template>
                </inner-edit-table>
              </v-col>
            </v-row>
          </v-form>
        </template>
      </temp-editable-dialog>
      <temp-editable-dialog
        ref="resetPasswordDialogRef"
        :title="$t('action.resetPassword')"
        :confirm-disabled="resetConfirmDisabled"
        force-editable
        prepend-icon="mdi-lock-reset"
        @confirm="resetPasswordSubmit"
      >
        <template #body>
          <v-form>
            <v-row density="compact">
              <v-col cols="12">
                <v-radio-group v-model="resetMode" inline>
                  <v-radio :label="$t('model.user.field.randomPassword')" value="random" />
                  <v-radio :label="$t('model.user.field.specifyPassword')" value="manual" />
                </v-radio-group>
              </v-col>
            </v-row>
            <v-row v-if="resetMode === 'manual'" density="compact">
              <v-col cols="6">
                <password-field
                  v-model="resetNewPassword"
                  v-bind="resetNewPasswordProps"
                  :label="$t('model.user.field.newPassword')"
                />
              </v-col>
              <v-col cols="6">
                <password-field
                  v-model="resetNewPasswordAgain"
                  v-bind="resetNewPasswordAgainProps"
                  :label="$t('model.user.field.newPasswordAgain')"
                />
              </v-col>
            </v-row>
          </v-form>
        </template>
      </temp-editable-dialog>
      <app-dialog
        v-model="resetResultDialog"
        :title="$t('action.resetPassword')"
        icon="mdi-lock-reset"
        size="sm"
      >
        <v-text-field
          :model-value="resetResultPassword"
          :label="$t('model.user.field.newPassword')"
          readonly
        >
          <template #append-inner>
            <copy-btn :value="resetResultPassword" />
          </template>
        </v-text-field>
        <p class="text-body-medium on-surface-variant mt-2">
          {{ $t("model.user.field.passwordResetNotice") }}
        </p>
        <template #footer>
          <v-btn :text="$t('action.close')" variant="text" @click="resetResultDialog = false" />
        </template>
      </app-dialog>
    </template>
  </main-layout>
</template>

<route lang="yaml">
name: Admin User List
meta:
  title: title.userAdmin
  requiresAuth: true
  requiresPerm: true
</route>

<script setup lang="ts">
import {
  groupListGet,
  userCreate,
  userPasswordReset,
  userSetActive,
  userDetailGet,
  userFrontendConfigReset,
  userGroupJoin,
  userGroupLeave,
  userListGet,
  userListOptions,
  userUpdate,
} from "@/apis/admin";
import { getResponse, getSimpleResponse } from "@/apis/base";
import type { BatchActions, ItemActions, TableHeader } from "@/components/@types/table";
import TempEditableDialog from "@/components/forms/TempEditableDialog.vue";
import { useAppStore } from "@/stores/app";
import type { UserDetailOut, UserRbacPolicyDetailOut } from "./@types";
import type { BatchResultOut, UserPasswordResetOut } from "@/apis/@types/admin";
import i18n from "@/utils/i18n";
import { BackendDataType, SearchAction } from "@/types";
import { useUserStore } from "@/stores/user";
import { useForm } from "vee-validate";
import { adminResetPasswordSchema, changeProfileSchema, createUserSchema } from "@/schemas/user";
import { vuetifyCfg } from "@/utils/form";

const { defineField, handleSubmit, meta, resetForm } = useForm({
  validationSchema: changeProfileSchema,
});

const createForm = useForm({
  validationSchema: createUserSchema,
});

const [nickleName, nickleNameProps] = defineField("nickleName", vuetifyCfg);
const [email, emailProps] = defineField("email", vuetifyCfg);

const [createUsername, createUsernameProps] = createForm.defineField("username", vuetifyCfg);
const [createPassword, createPasswordProps] = createForm.defineField("password", vuetifyCfg);
const [createPasswordAgain, createPasswordAgainProps] = createForm.defineField(
  "passwordAgain",
  vuetifyCfg,
);
const [createNickleName, createNickleNameProps] = createForm.defineField("nickleName", vuetifyCfg);
const [createEmail, createEmailProps] = createForm.defineField("email", vuetifyCfg);

const appStore = useAppStore();
const userStore = useUserStore();
const searchTableRef = useTemplateRef("searchTableRef");
const createDialogRef = useTemplateRef("createDialogRef");
const editDialogRef = useTemplateRef("editDialogRef");
const userPolicyHeaders: Array<TableHeader> = [
  {
    key: "namespace",
    title: i18n.global.t("model.policy.field.namespace"),
    align: "start",
    type: BackendDataType.STRING,
    sortable: false,
  },
  {
    key: "name",
    title: i18n.global.t("model.policy.field.name"),
    align: "start",
    type: BackendDataType.STRING,
    sortable: false,
  },
  {
    key: "createdAt",
    title: i18n.global.t("model.policy.field.createdAt"),
    align: "start",
    type: BackendDataType.DATETIME,
    sortable: false,
  },
  {
    key: "updatedAt",
    title: i18n.global.t("model.policy.field.updatedAt"),
    align: "start",
    type: BackendDataType.DATETIME,
    sortable: false,
  },
];
const userPolicySearchHeaders: Record<string, SearchAction> = {
  name: SearchAction.CONTAIN,
};

const namespaceOpts = computed(() => {
  return ["*", ...userStore.namespaces];
});

// projectFields 字段投影：行操作依赖字段恒定投影（unionID 定位、active 启用态开关），
// 避免用户隐藏对应列后行操作失效
const projectFields = (visible: string[]): string[] => ["id", "unionID", "active", ...visible];

const getActions = (): ItemActions => {
  const actions: ItemActions = {
    edit: detailGetAction,
    resetPassword: resetPasswordAction,
    resetFrontendConfig: resetFrontendConfigAction,
  };
  return actions;
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const detailGetAction = async (item: Record<string, any>): Promise<void> => {
  await getResponse<UserDetailOut>(
    userDetailGet(item.unionID),
    async (rsp) => {
      // 复位与回填同属打开动作：原子重置全部字段与校验状态
      resetForm({
        values: {
          nickleName: rsp.data.nickleName,
          email: rsp.data.email,
        },
      });
      editDialogRef.value?.open({
        ...rsp.data,
        nickleName,
        email,
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

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const itemUpdateAction = (proxy: Record<string, any>) => {
  handleSubmit(async (values) => {
    await getSimpleResponse(
      userUpdate(proxy.unionID, {
        // 单行文本提交前去除首尾空白，避免误输入空白导致数据异常
        nickleName: (values.nickleName as string).trim(),
        email: (values.email as string).trim(),
      }),
      async (rsp) => {
        await searchTableRef.value?.refresh();
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
  })();
};

const openCreateDialogAction = () => {
  // 打开动作里原子复位全部字段；密码等凭据字段留空
  const defaults = {
    username: "",
    password: "",
    passwordAgain: "",
    nickleName: "",
    email: "",
  };
  createForm.resetForm({ values: defaults });
  createDialogRef.value?.open(defaults);
};

const createAction = createForm.handleSubmit(async (values) => {
  await getSimpleResponse(
    userCreate({
      // 单行文本提交前去除首尾空白，避免误输入空白导致登录失败或数据异常
      // （密码字段按规范豁免 trim：首尾空白是密码的合法字符）
      username: (values.username as string).trim(),
      password: values.password,
      nickleName: (values.nickleName as string).trim(),
      email: (values.email as string).trim(),
    }),
    async (rsp) => {
      await searchTableRef.value?.refresh();
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

// ---- 批量启用/禁用 ----
// applyBatchResult 统一处理 BatchResultOut 响应：存在失败条目时以 error 提示失败数，否则沿用成功摘要
const applyBatchResult = (summary: string, result: BatchResultOut): void => {
  if (result.failed.length > 0) {
    appStore.notify({
      text: i18n.global.t("notice.batchResult", {
        succeeded: result.succeeded,
        failed: result.failed.length,
      }),
      timeout: 3000,
      color: "error",
    });
    return;
  }
  appStore.notify({
    text: summary,
    timeout: 3000,
    color: "success",
  });
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type BatchState = { items: Record<string, any>[]; action: "activate" | "deactivate" };
const batchState = ref<BatchState | null>(null);
const batchConfirmRef = useTemplateRef("batchConfirmRef");

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const getBatchActions = (items: Record<string, any>[]): BatchActions => {
  return {
    activate: () => {
      batchState.value = { items, action: "activate" };
      batchConfirmRef.value?.open({});
    },
    deactivate: () => {
      batchState.value = { items, action: "deactivate" };
      batchConfirmRef.value?.open({});
    },
  };
};

const batchAction = async (): Promise<void> => {
  const state = batchState.value;
  if (!state) {
    return;
  }
  batchState.value = null;
  const unionIDs = state.items.map((i) => i.unionID as string);
  await getResponse<BatchResultOut>(
    userSetActive(unionIDs, state.action === "activate"),
    async (rsp) => {
      applyBatchResult(rsp.summary, rsp.data);
      searchTableRef.value?.clearSelection?.();
      await searchTableRef.value?.refresh();
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

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const resetFrontendConfigAction = async (item: Record<string, any>): Promise<void> => {
  await getSimpleResponse(
    userFrontendConfigReset(item.unionID),
    async (rsp) => {
      await searchTableRef.value?.refresh();
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

const resetMode = ref("random");

const resetPasswordForm = useForm({
  // 仅指定密码模式需要强密码校验；随机生成模式无需填写密码直接提交
  validationSchema: computed(() =>
    resetMode.value === "manual" ? adminResetPasswordSchema : undefined,
  ),
});

const [resetNewPassword, resetNewPasswordProps] = resetPasswordForm.defineField(
  "newPassword",
  vuetifyCfg,
);
const [resetNewPasswordAgain, resetNewPasswordAgainProps] = resetPasswordForm.defineField(
  "newPasswordAgain",
  vuetifyCfg,
);

const resetPasswordDialogRef = useTemplateRef("resetPasswordDialogRef");
const resetUnionID = ref("");
const resetResultDialog = ref(false);
const resetResultPassword = ref("");

const resetConfirmDisabled = computed(() => {
  // 随机生成模式无需表单校验；指定密码模式必须填写且通过强密码校验
  return (
    resetMode.value === "manual" &&
    (!resetPasswordForm.meta.value.dirty || !resetPasswordForm.meta.value.valid)
  );
});

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const resetPasswordAction = async (item: Record<string, any>): Promise<void> => {
  resetMode.value = "random";
  // 打开重置密码弹窗的动作：原子复位密码字段（凭据留空）
  resetPasswordForm.resetForm({ values: { newPassword: "", newPasswordAgain: "" } });
  resetUnionID.value = item.unionID;
  resetPasswordDialogRef.value?.open({});
};

const resetPasswordSubmit = resetPasswordForm.handleSubmit(async () => {
  await getResponse<UserPasswordResetOut>(
    userPasswordReset(resetUnionID.value, {
      password: resetMode.value === "manual" ? resetNewPassword.value : undefined,
    }),
    async (rsp) => {
      resetResultPassword.value = rsp.data.password;
      resetResultDialog.value = true;
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

const joinGroupAction = async (
  unionID: string,
  namespace: string,
  groupName: string,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  appendItem: (item: Record<string, any>) => void,
  close: () => void,
): Promise<void> => {
  await getResponse<UserRbacPolicyDetailOut>(
    userGroupJoin(unionID, {
      namespace,
      groupName,
    }),
    async (rsp) => {
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "success",
      });
      appendItem(rsp.data);
      close();
    },
    async (rsp) => {
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "error",
      });
      close();
    },
  );
};

const leaveGroupAction = async (
  unionID: string,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  item: Record<string, any>,
  callback: () => void,
): Promise<void> => {
  await getSimpleResponse(
    userGroupLeave(unionID, {
      namespace: item.namespace,
      groupName: item.name,
    }),
    async (rsp) => {
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "success",
      });
      callback();
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

<style scoped>
/* 命名空间下拉固定宽度，使行内的追加操作按钮保持对齐。 */
.join-namespace-select {
  width: 140px;
}
</style>
