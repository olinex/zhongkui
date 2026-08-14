<template>
  <main-layout current-nav="admin">
    <template #body>
      <search-table
        ref="searchTableRef"
        :title="$t(`model.namespace.name`)"
        :list-request="namespaceListGet"
        :options-request="namespaceListOptions"
        :actions="getActions"
        model-name="namespace"
      >
        <template #top>
          <v-btn color="primary" @click="openCreateDialogAction">
            {{ $t("action.create") }}
          </v-btn>
        </template>
      </search-table>
      <confirm-dialog
        ref="confirmDialogRef"
        :title="$t(`model.namespace.name`)"
        prepend-icon="mdi-group"
        @confirm="itemDeleteAction"
      >
        <template #body="{ proxy }">
          <confirm-alert
            :action="$t('action.delete')"
            :text="
              $t(`notice.deleteConfirm`, {
                category: $t(`model.namespace.name`),
                key: proxy.name,
              })
            "
          />
        </template>
      </confirm-dialog>
      <temp-editable-dialog
        ref="createDialogRef"
        :title="$t(`model.namespace.name`)"
        :confirm-disabled="!createForm.meta.value.dirty || !createForm.meta.value.valid"
        force-editable
        prepend-icon="mdi-group"
        @confirm="createAction"
      >
        <template #body>
          <v-form>
            <v-row dense>
              <v-col cols="12">
                <v-text-field
                  v-model="createName"
                  v-bind="createNameProps"
                  :label="$t('model.namespace.field.name')"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-textarea
                  v-model="createDescription"
                  v-bind="createDescriptionProps"
                  :label="$t('model.namespace.field.description')"
                />
              </v-col>
            </v-row>
          </v-form>
        </template>
      </temp-editable-dialog>
      <temp-editable-dialog
        ref="editDialogRef"
        :title="$t(`model.namespace.name`)"
        :confirm-disabled="!updateForm.meta.value.dirty || !updateForm.meta.value.valid"
        prepend-icon="mdi-group"
        @confirm="itemUpdateAction"
      >
        <template #body="{ proxy, editable }">
          <v-form>
            <v-row dense>
              <v-col cols="12">
                <v-text-field
                  v-model="proxy.name"
                  :label="$t('model.namespace.field.name')"
                  disabled
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-textarea
                  v-model="updateDescription"
                  v-bind="updateDescriptionProps"
                  :label="$t('model.namespace.field.description')"
                  :disabled="!editable"
                />
              </v-col>
            </v-row>
          </v-form>
        </template>
      </temp-editable-dialog>
    </template>
  </main-layout>
</template>

<route lang="yaml">
name: Admin Namespace List
meta:
  title: title.namespaceAdmin
  requiresAuth: true
  requiresPerm: true
</route>

<script setup lang="ts">
import {
  namespaceCreate,
  namespaceDetailGet,
  namespaceDelete,
  namespaceListGet,
  namespaceListOptions,
  namespaceUpdate,
} from "@/apis/admin";
import { getResponse, getSimpleResponse } from "@/apis/base";
import type { ItemActions } from "@/components/@types/table";
import { useAppStore } from "@/stores/app";
import type { NamespaceDetailOut } from "./@types";
import { useForm } from "vee-validate";
import { createNamespaceSchema, updateNamespaceSchema } from "@/schemas/namespace";
import { vuetifyCfg } from "@/utils/form";

const updateForm = useForm({
  validationSchema: updateNamespaceSchema,
});

const createForm = useForm({
  validationSchema: createNamespaceSchema,
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
const editDialogRef = useTemplateRef("editDialogRef");
const createDialogRef = useTemplateRef("createDialogRef");

const getActions = (): ItemActions => {
  const actions: ItemActions = {
    edit: detailGetAction,
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    delete: function (item: Record<string, any>): void {
      confirmDialogRef.value?.open(item);
    },
  };
  return actions;
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const detailGetAction = async (item: Record<string, any>): Promise<void> => {
  await getResponse<NamespaceDetailOut>(
    namespaceDetailGet(item.name as string),
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
      namespaceUpdate(proxy.name, {
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
    namespaceDelete(item.name as string),
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
    namespaceCreate({
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
</script>
