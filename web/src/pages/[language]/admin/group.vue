<template>
  <main-layout current-nav="admin">
    <template #body>
      <search-table
        ref="searchTableRef"
        :list-request="groupListGet"
        :options-request="groupListOptions"
        :actions="getActions"
        :create="openCreateDialogAction"
        model-name="group"
      />
      <confirm-dialog
        ref="confirmDialogRef"
        :title="$t(`model.group.name`)"
        prepend-icon="mdi-group"
        @confirm="itemDeleteAction"
      >
        <template #body="{ proxy }">
          <confirm-alert
            :action="$t('action.delete')"
            :text="
              $t(`notice.deleteConfirm`, {
                category: $t(`model.group.name`),
                key: proxy.name,
              })
            "
          />
        </template>
      </confirm-dialog>
      <temp-editable-dialog
        ref="createDialogRef"
        :title="$t('model.group.name')"
        :confirm-disabled="!createForm.meta.value.dirty || !createForm.meta.value.valid"
        force-editable
        prepend-icon="mdi-account-multiple"
        @confirm="createAction"
      >
        <template #body>
          <v-form>
            <v-row dense>
              <v-col cols="12">
                <v-text-field
                  v-model="createName"
                  v-bind="createNameProps"
                  :label="$t('model.group.field.name')"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-textarea
                  v-model="createDescription"
                  v-bind="createDescriptionProps"
                  :label="$t('model.group.field.description')"
                />
              </v-col>
            </v-row>
          </v-form>
        </template>
      </temp-editable-dialog>
      <temp-editable-dialog
        ref="editDialogRef"
        :title="$t('model.group.name')"
        :confirm-disabled="!updateForm.meta.value.dirty || !updateForm.meta.value.valid"
        prepend-icon="mdi-account-multiple"
        @confirm="itemUpdateAction"
      >
        <template #body="{ proxy, editable }">
          <v-form>
            <v-row dense>
              <v-col cols="12">
                <v-text-field v-model="proxy.name" :label="$t('model.group.field.name')" disabled />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-textarea
                  v-model="updateDescription"
                  v-bind="updateDescriptionProps"
                  :label="$t('model.group.field.description')"
                  :disabled="!editable"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <inner-edit-table
                  v-model="proxy.policies"
                  :headers="groupPolicyHeaders"
                  :search-headers="groupPolicySearchHeaders"
                  :search-request="rbacPolicyTemplateListGet"
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
                      @click="removePolicyAction(proxy.name, item, removeItem)"
                    />
                  </template>
                  <template #search="{ items, close, appendItem }">
                    <v-list :items="items">
                      <v-list-item v-for="item in items" :key="item.id">
                        <v-card color="surface-variant" variant="tonal" density="compact">
                          <v-card-title> {{ item.action }} {{ item.target }} </v-card-title>
                          <v-card-text>
                            <v-form>
                              <v-select
                                v-model="item.effect"
                                :items="['allow', 'deny']"
                                :disabled="!editable"
                                variant="outlined"
                                density="compact"
                                rounded
                              />
                            </v-form>
                          </v-card-text>
                          <v-card-actions>
                            <v-btn
                              :disabled="!(editable && item.effect)"
                              :text="$t('action.add')"
                              color="primary"
                              density="compact"
                              variant="tonal"
                              block
                              @click="
                                addPolicyAction(
                                  proxy.name,
                                  item.target,
                                  item.action,
                                  item.effect,
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
name: Admin Group List
meta:
  title: title.groupAdmin
  requiresAuth: true
  requiresPerm: true
</route>

<script setup lang="ts">
import {
  groupCreate,
  groupDelete,
  groupDetailGet,
  groupListGet,
  groupListOptions,
  groupRbacPolicyAdd,
  groupRbacPolicyRemove,
  groupUpdate,
  rbacPolicyTemplateListGet,
} from "@/apis/admin";
import { getResponse, getSimpleResponse } from "@/apis/base";
import type { ItemActions, TableHeader } from "@/components/@types/table";
import type { GroupDetailOut, GroupRbacPolicyDetailOut } from "./@types";
import { useAppStore } from "@/stores/app";
import { BackendDataType, SearchAction } from "@/types";
import i18n from "@/utils/i18n";
import { useForm } from "vee-validate";
import { updateGroupSchema, createGroupSchema } from "@/schemas/group";
import { vuetifyCfg } from "@/utils/form";

const updateForm = useForm({
  validationSchema: updateGroupSchema,
});

const createForm = useForm({
  validationSchema: createGroupSchema,
});

const [updateDescription, updateDescriptionProps] = updateForm.defineField(
  "description",
  vuetifyCfg,
);

const [createName, createNameProps] = createForm.defineField("name", vuetifyCfg);

const [createDescription, createDescriptionProps] = createForm.defineField(
  "description",
  vuetifyCfg,
);

const appStore = useAppStore();
const searchTableRef = useTemplateRef("searchTableRef");
const confirmDialogRef = useTemplateRef("confirmDialogRef");
const createDialogRef = useTemplateRef("createDialogRef");
const editDialogRef = useTemplateRef("editDialogRef");
const groupPolicyHeaders: Array<TableHeader> = [
  {
    key: "target",
    title: i18n.global.t("model.policy.field.target"),
    align: "start",
    type: BackendDataType.STRING,
    sortable: false,
  },
  {
    key: "action",
    title: i18n.global.t("model.policy.field.action"),
    align: "start",
    type: BackendDataType.STRING,
    sortable: false,
  },
  {
    key: "effect",
    title: i18n.global.t("model.policy.field.effect"),
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

const groupPolicySearchHeaders: Record<string, SearchAction> = {
  target: SearchAction.CONTAIN,
  action: SearchAction.CONTAIN,
};

const getActions = (): ItemActions => {
  return {
    edit: detailGetAction,
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    delete: function (item: Record<string, any>): void {
      confirmDialogRef.value?.open(item);
    },
  };
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const detailGetAction = async (item: Record<string, any>): Promise<void> => {
  await getResponse<GroupDetailOut>(
    groupDetailGet(item.name),
    async (rsp) => {
      updateForm.resetField("description", { value: rsp.data.description });
      editDialogRef.value?.open(rsp.data);
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
  updateForm.handleSubmit(async (values) => {
    await getSimpleResponse(
      groupUpdate(proxy.name, {
        description: values.description,
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
const itemDeleteAction = async (item: Record<string, any>): Promise<void> => {
  await getSimpleResponse(
    groupDelete(item.name as string),
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

const openCreateDialogAction = () => {
  createDialogRef.value?.open({ name: "", description: "" });
  createForm.resetField("name", { value: "" });
  createForm.resetField("description", { value: "" });
};

const createAction = createForm.handleSubmit(async (values) => {
  await getSimpleResponse(
    groupCreate({
      name: values.name,
      description: values.description,
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

const addPolicyAction = async (
  groupName: string,
  target: string,
  action: string,
  effect: string,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  appendItem: (item: Record<string, any>) => void,
  close: () => void,
): Promise<void> => {
  await getResponse<GroupRbacPolicyDetailOut>(
    groupRbacPolicyAdd(groupName, {
      target,
      action,
      effect,
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

const removePolicyAction = async (
  groupName: string,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  item: Record<string, any>,
  callback: () => void,
): Promise<void> => {
  await getSimpleResponse(
    groupRbacPolicyRemove(groupName, {
      target: item.target,
      action: item.action,
      effect: item.effect,
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
