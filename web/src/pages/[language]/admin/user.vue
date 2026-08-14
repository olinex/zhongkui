<template>
  <main-layout current-nav="admin">
    <template #body>
      <search-table
        ref="searchTableRef"
        :title="$t(`model.user.name`)"
        :list-request="userListGet"
        :options-request="userListOptions"
        :actions="getActions"
        model-name="user"
      />
      <temp-editable-dialog
        ref="editDialogRef"
        :title="$t(`model.user.name`)"
        :confirm-disabled="!meta.dirty || !meta.valid"
        prepend-icon="mdi-account"
        @confirm="itemUpdateAction"
      >
        <template #body="{ proxy, editable }">
          <v-form>
            <v-row dense>
              <v-col col="12">
                <v-text-field
                  v-model="proxy.username"
                  :label="$t('model.user.field.username')"
                  disabled
                />
              </v-col>
            </v-row>
            <v-row dense>
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
            <v-row dense>
              <v-col col="12">
                <inner-edit-table
                  v-model="proxy.groups"
                  :headers="userPolicyHeaders"
                  :search-headers="userPolicySearchHeaders"
                  :search-request="groupListGet"
                  :list-item-action="true"
                  :editable="editable"
                  order-by-field="createdAt"
                >
                  <template #list-item-action="{ item, editable: listEditable, removeItem }">
                    <v-btn
                      :disabled="!listEditable"
                      icon="mdi-minus"
                      variant="tonal"
                      color="error"
                      density="compact"
                      @click="leaveGroupAction(proxy.unionID, item, removeItem)"
                    />
                  </template>
                  <template #search="{ items, close, appendItem }">
                    <v-list :items="items">
                      <v-list-item v-for="item in items" :key="item.id">
                        <v-card color="surface-variant" variant="tonal" density="compact">
                          <v-card-title>
                            {{ item.name }}
                          </v-card-title>
                          <v-card-subtitle>
                            {{ item.description }}
                          </v-card-subtitle>
                          <v-card-text>
                            <v-form>
                              <v-select
                                v-model="item.namespace"
                                :items="namespaceOpts"
                                :disabled="!editable"
                                variant="outlined"
                                density="compact"
                                rounded
                              />
                            </v-form>
                          </v-card-text>
                          <v-card-actions>
                            <v-btn
                              :disabled="!(editable && item.namespace)"
                              :text="$t('action.join')"
                              color="primary"
                              density="compact"
                              variant="tonal"
                              block
                              @click="
                                joinGroupAction(
                                  proxy.unionID,
                                  item.namespace,
                                  item.name,
                                  appendItem,
                                  close,
                                )
                              "
                            />
                          </v-card-actions>
                        </v-card>
                      </v-list-item>
                    </v-list>
                  </template>
                </inner-edit-table>
              </v-col>
            </v-row>
          </v-form>
        </template>
      </temp-editable-dialog>
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
  userSetActive,
  userDetailGet,
  userGroupJoin,
  userGroupLeave,
  userListGet,
  userListOptions,
  userUpdate,
} from "@/apis/admin";
import { getResponse, getSimpleResponse } from "@/apis/base";
import type { ItemActions, TableHeader } from "@/components/@types/table";
import TempEditableDialog from "@/components/forms/TempEditableDialog.vue";
import { useAppStore } from "@/stores/app";
import type { UserDetailOut, UserRbacPolicyDetailOut } from "./@types";
import i18n from "@/utils/i18n";
import { BackendDataType, SearchAction } from "@/types";
import { useUserStore } from "@/stores/user";
import { useForm } from "vee-validate";
import { changeProfileSchema } from "@/schemas/user";
import { vuetifyCfg } from "@/utils/form";

const { defineField, handleSubmit, meta, resetField } = useForm({
  validationSchema: changeProfileSchema,
});

const [nickleName, nickleNameProps] = defineField("nickleName", vuetifyCfg);
const [email, emailProps] = defineField("email", vuetifyCfg);

const appStore = useAppStore();
const userStore = useUserStore();
const searchTableRef = useTemplateRef("searchTableRef");
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

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const getActions = (item: Record<string, any>): ItemActions => {
  const actions: ItemActions = {
    edit: detailGetAction,
  };
  switch (item.active) {
    case false:
      actions.activate = activateAction;
      break;
    case true:
      actions.deactivate = deactivateAction;
      break;
  }
  return actions;
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const detailGetAction = async (item: Record<string, any>): Promise<void> => {
  await getResponse<UserDetailOut>(
    userDetailGet(item.unionID),
    async (rsp) => {
      resetField("nickleName", { value: rsp.data.nickleName });
      resetField("email", { value: rsp.data.email });
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
        nickleName: values.nickleName,
        email: values.email,
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

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const activateAction = async (item: Record<string, any>): Promise<void> => {
  await getSimpleResponse(
    userSetActive(item.unionID, true),
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

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const deactivateAction = async (item: Record<string, any>): Promise<void> => {
  await getSimpleResponse(
    userSetActive(item.unionID, false),
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
