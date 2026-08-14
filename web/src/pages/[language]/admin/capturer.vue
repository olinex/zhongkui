<template>
  <main-layout current-nav="admin">
    <template #body>
      <search-table
        ref="searchTableRef"
        :title="$t('model.capturer.name')"
        :list-request="capturerListGet"
        :options-request="capturerListOptions"
        :actions="getActions"
        model-name="capturer"
      >
        <template #top>
          <v-btn color="primary" @click="openCreateDialogAction">
            {{ $t("action.create") }}
          </v-btn>
        </template>
      </search-table>
      <confirm-dialog
        ref="confirmDialogRef"
        :title="$t(`model.capturer.name`)"
        prepend-icon="mdi-owl"
        @confirm="itemDeleteAction"
      >
        <template #body="{ proxy }">
          <v-alert
            type="warning"
            icon="mdi-table-row-remove"
            :title="$t(`action.delete`)"
            :text="
              $t(`notice.deleteConfirm`, {
                category: $t(`model.capturer.name`),
                key: `${proxy.namespace}-${proxy.name}:${proxy.version}`,
              })
            "
          />
        </template>
      </confirm-dialog>
      <temp-editable-dialog
        ref="createDialogRef"
        :title="$t(`model.capturer.name`)"
        :confirm-disabled="!createForm.meta.value.dirty || !createForm.meta.value.valid"
        prepend-icon="mdi-owl"
        force-editable
        @confirm="createAction"
      >
        <template #body>
          <v-form>
            <v-row dense>
              <v-col cols="12">
                <chip-search-field
                  v-model="createNamespace"
                  v-bind="createNamespaceProps"
                  :label="$t('model.capturer.field.namespace')"
                  item-title="name"
                  item-value="name"
                  :search-headers="namespaceSearchHeaders"
                  :search-request="namespaceListGet"
                  order-by-field="name"
                  clearable
                  init
                >
                  <template #chip="{ props, item }">
                    <v-chip v-bind="props">{{ item.raw.name }}</v-chip>
                  </template>
                  <template #item="{ props, item }">
                    <v-list-item
                      v-bind="props"
                      :title="item.raw.name"
                      :subtitle="item.raw.description"
                    />
                  </template>
                </chip-search-field>
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="9">
                <v-text-field
                  v-model="createName"
                  v-bind="createNameProps"
                  :label="$t('model.capturer.field.name')"
                />
              </v-col>
              <v-col cols="3">
                <v-text-field
                  v-model="createVersion"
                  v-bind="createVersionProps"
                  :label="$t('model.capturer.field.version')"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-text-field
                  v-model="createDevice"
                  v-bind="createDeviceProps"
                  :label="$t('model.capturer.field.device')"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-text-field
                  v-model="createBpfExpr"
                  v-bind="createBpfExprProps"
                  :label="$t('model.capturer.field.bpfExpr')"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="6">
                <v-number-input
                  v-model="createSnapLen"
                  v-bind="createSnapLenProps"
                  :label="$t('model.capturer.field.snapLen')"
                />
              </v-col>
              <v-col cols="6">
                <v-number-input
                  v-model="createTimeout"
                  v-bind="createTimeoutProps"
                  :label="$t('model.capturer.field.timeout')"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="6">
                <v-switch
                  v-model="createPromisc"
                  v-bind="createPromiscProps"
                  color="success"
                  :label="$t('model.capturer.field.promisc')"
                />
              </v-col>
              <v-col cols="6">
                <v-switch
                  v-model="createEnabled"
                  v-bind="createEnabledProps"
                  color="success"
                  :label="$t('model.capturer.field.enabled')"
                />
              </v-col>
            </v-row>
          </v-form>
        </template>
      </temp-editable-dialog>
      <temp-editable-dialog
        ref="editDialogRef"
        :title="$t(`model.capturer.name`)"
        :confirm-disabled="!updateForm.meta.value.dirty || !updateForm.meta.value.valid"
        prepend-icon="mdi-owl"
        @confirm="itemUpdateAction"
      >
        <template #body="{ proxy, editable }">
          <v-form>
            <v-row dense>
              <v-col cols="12">
                <chip-search-field
                  v-model="proxy.namespace"
                  item-title="name"
                  item-value="name"
                  :search-headers="namespaceSearchHeaders"
                  :search-request="namespaceListGet"
                  order-by-field="name"
                  :label="$t('model.capturer.field.namespace')"
                  clearable
                  disabled
                >
                  <template #chip="{ props, item }">
                    <v-chip v-bind="props" :label="item.raw.name" />
                  </template>
                </chip-search-field>
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="9">
                <v-text-field
                  v-model="proxy.name"
                  :label="$t('model.capturer.field.name')"
                  disabled
                />
              </v-col>
              <v-col cols="3">
                <v-text-field
                  v-model="proxy.version"
                  :label="$t('model.capturer.field.version')"
                  disabled
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-text-field
                  v-model="updateDevice"
                  v-bind="updateDeviceProps"
                  :label="$t('model.capturer.field.device')"
                  :disabled="!editable"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-text-field
                  v-model="updateBpfExpr"
                  v-bind="updateBpfExprProps"
                  :label="$t('model.capturer.field.bpfExpr')"
                  :disabled="!editable"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="6">
                <v-number-input
                  v-model="updateSnapLen"
                  v-bind="updateSnapLenProps"
                  :label="$t('model.capturer.field.snapLen')"
                  :disabled="!editable"
                />
              </v-col>
              <v-col cols="6">
                <v-number-input
                  v-model="updateTimeout"
                  v-bind="updateTimeoutProps"
                  :label="$t('model.capturer.field.timeout')"
                  :disabled="!editable"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="6">
                <v-switch
                  v-model="updatePromisc"
                  v-bind="updatePromiscProps"
                  color="success"
                  :label="$t('model.capturer.field.promisc')"
                  :disabled="!editable"
                />
              </v-col>
              <v-col cols="6">
                <v-switch
                  v-model="updateEnabled"
                  v-bind="updateEnabledProps"
                  color="success"
                  :label="$t('model.capturer.field.enabled')"
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
name: Admin Capturer List
meta:
  title: title.capturerAdmin
  requiresAuth: true
  requiresPerm: true
</route>

<script setup lang="ts">
import {
  capturerCreate,
  capturerUpdate,
  capturerDelete,
  capturerDetailGet,
  capturerListGet,
  capturerListOptions,
  namespaceListGet,
} from "@/apis/admin";
import { getResponse, getSimpleResponse } from "@/apis/base";
import type { ItemActions } from "@/components/@types/table";
import type { CapturerDetailOut } from "./@types";
import { useAppStore } from "@/stores/app";
import type { CapturerConfigIn } from "@/apis/@types/admin";
import { useForm } from "vee-validate";
import { createCapturerSchema, updateCapturerSchema } from "@/schemas/capturer";
import { vuetifyCfg } from "@/utils/form";
import { SearchAction } from "@/types";

const createForm = useForm({
  validationSchema: createCapturerSchema,
});

const updateForm = useForm({
  validationSchema: updateCapturerSchema,
});

const [createNamespace, createNamespaceProps] = createForm.defineField("namespace", vuetifyCfg);

const [createName, createNameProps] = createForm.defineField("name", vuetifyCfg);

const [createVersion, createVersionProps] = createForm.defineField("version", vuetifyCfg);

const [createDevice, createDeviceProps] = createForm.defineField("device", vuetifyCfg);

const [createBpfExpr, createBpfExprProps] = createForm.defineField("bpfExpr", vuetifyCfg);

const [createSnapLen, createSnapLenProps] = createForm.defineField("snapLen", vuetifyCfg);

const [createTimeout, createTimeoutProps] = createForm.defineField("timeout", vuetifyCfg);

const [createPromisc, createPromiscProps] = createForm.defineField("promisc", vuetifyCfg);

const [createEnabled, createEnabledProps] = createForm.defineField("enabled", vuetifyCfg);

const [updateDevice, updateDeviceProps] = updateForm.defineField("device", vuetifyCfg);

const [updateBpfExpr, updateBpfExprProps] = updateForm.defineField("bpfExpr", vuetifyCfg);

const [updateSnapLen, updateSnapLenProps] = updateForm.defineField("snapLen", vuetifyCfg);

const [updateTimeout, updateTimeoutProps] = updateForm.defineField("timeout", vuetifyCfg);

const [updatePromisc, updatePromiscProps] = updateForm.defineField("promisc", vuetifyCfg);

const [updateEnabled, updateEnabledProps] = updateForm.defineField("enabled", vuetifyCfg);

const appStore = useAppStore();
const searchTableRef = useTemplateRef("searchTableRef");
const confirmDialogRef = useTemplateRef("confirmDialogRef");
const editDialogRef = useTemplateRef("editDialogRef");
const createDialogRef = useTemplateRef("createDialogRef");

const namespaceSearchHeaders: Record<string, SearchAction> = {
  name: SearchAction.CONTAIN,
};

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
  await getResponse<CapturerDetailOut>(
    capturerDetailGet(item.id as number),
    async (rsp) => {
      updateForm.resetField("device", { value: rsp.data.config.device });
      updateForm.resetField("bpfExpr", { value: rsp.data.config.bpfExpr });
      updateForm.resetField("snapLen", { value: rsp.data.config.snapLen });
      updateForm.resetField("timeout", { value: rsp.data.config.timeout });
      updateForm.resetField("promisc", { value: rsp.data.config.promisc });
      updateForm.resetField("enabled", { value: rsp.data.config.enabled });
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
const itemDeleteAction = async (item: Record<string, any>): Promise<void> => {
  await getSimpleResponse(
    capturerDelete(item.id as number),
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
  const defaults = {
    namespace: undefined,
    name: "",
    version: "",
    device: "eth0",
    snapLen: 1600,
    promisc: false,
    timeout: 5,
    bpfExpr: "tcp port 80",
    enabled: true,
  };
  createForm.resetField("namespace", { value: defaults.namespace });
  createForm.resetField("name", { value: defaults.name });
  createForm.resetField("version", { value: defaults.version });
  createForm.resetField("device", { value: defaults.device });
  createForm.resetField("snapLen", { value: defaults.snapLen });
  createForm.resetField("promisc", { value: defaults.promisc });
  createForm.resetField("timeout", { value: defaults.timeout });
  createForm.resetField("bpfExpr", { value: defaults.bpfExpr });
  createForm.resetField("enabled", { value: defaults.enabled });
  createDialogRef.value?.open(defaults);
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const itemUpdateAction = (proxy: Record<string, any>) => {
  updateForm.handleSubmit(async (values) => {
    await getSimpleResponse(
      capturerUpdate(proxy.id, values as CapturerConfigIn),
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

const createAction = createForm.handleSubmit(async (values): Promise<void> => {
  await getSimpleResponse(
    capturerCreate({
      namespace: values.namespace,
      name: values.name,
      version: values.version,
      config: {
        device: values.device,
        snapLen: values.snapLen,
        promisc: values.promisc,
        timeout: values.timeout,
        bpfExpr: values.bpfExpr,
        enabled: values.enabled,
      },
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
