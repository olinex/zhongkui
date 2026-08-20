<template>
  <v-layout>
    <app-header />
    <v-main>
      <v-container class="mx-auto pa-8" style="max-width: 1200px">
        <!-- Welcome area -->
        <div class="mb-8">
          <h1 class="text-h4 font-weight-bold">{{ $t("title.zk") }}</h1>
          <p class="text-h6 text-medium-emphasis mt-2">{{ $t("home.subtitle") }}</p>
        </div>

        <!-- Quick start section -->
        <div class="d-flex align-center mb-4">
          <h2 class="text-h6">{{ $t("home.quickStart.self") }}</h2>
          <v-spacer />
          <v-btn
            :text="$t('home.quickStart.addGroup')"
            prepend-icon="mdi-plus"
            color="primary"
            variant="tonal"
            @click="addGroupAction"
          />
        </div>

        <!-- Custom groups of navigation links -->
        <div v-for="group in store.groups" :key="group.id" class="mb-8">
          <!-- Group heading: title (click to rename) + actions -->
          <div class="d-flex align-center mb-3 ga-2">
            <v-icon color="primary">mdi-folder-multiple-outline</v-icon>
            <template v-if="editingGroupId === group.id">
              <v-text-field
                v-model="groupTitleDraft"
                density="compact"
                variant="underlined"
                hide-details
                autofocus
                class="quick-group-name-input"
                :label="$t('home.quickStart.groupName')"
                @keydown.enter="commitRename(group.id)"
                @keydown.esc="cancelRename"
                @blur="commitRename(group.id)"
              />
            </template>
            <template v-else>
              <h3
                class="text-subtitle-1 font-weight-bold quick-group-title"
                :title="$t('home.quickStart.renameGroup')"
                @click="startRename(group)"
              >
                {{ displayTitle(group.title) }}
              </h3>
            </template>
            <v-spacer />
            <v-btn
              icon="mdi-plus"
              size="small"
              variant="text"
              :title="$t('home.quickStart.addLink')"
              @click="openAddLinkDialog(group.id)"
            />
            <v-btn
              v-if="!isBuiltinGroup(group)"
              icon="mdi-delete-outline"
              size="small"
              variant="text"
              :title="$t('home.quickStart.deleteGroup')"
              @click="askDeleteGroup(group)"
            />
          </div>

          <!-- Links as cards, same style as the pre-group quick start -->
          <v-row dense>
            <v-col
              v-for="(link, index) in group.links"
              :key="linkKey(link)"
              cols="12"
              sm="6"
              md="4"
              lg="3"
            >
              <v-card
                class="quick-link-card"
                :class="{
                  'quick-link-dragging': isDragging(group.id, index),
                  'quick-drop-before': isDropTarget(group.id, index),
                }"
                draggable="true"
                @click="goto(link)"
                @dragstart="onDragStart($event, group.id, index)"
                @dragover.prevent="onDragOver(group.id, index)"
                @dragleave="onDragLeave($event, group.id, index)"
                @drop.prevent="onDrop(group.id, index)"
                @dragend="onDragEnd"
              >
                <v-card-item class="pa-2">
                  <template #prepend>
                    <v-avatar color="primary" variant="tonal" size="36">
                      <v-icon size="small" :icon="link.icon" />
                    </v-avatar>
                  </template>
                  <v-card-title class="text-body-2 ps-1">{{ $t(link.title) }}</v-card-title>
                  <!-- Remove button: only visible when hovering the card -->
                  <template #append>
                    <v-btn
                      class="quick-link-remove"
                      icon="mdi-close"
                      size="x-small"
                      variant="text"
                      :title="$t('home.quickStart.removeLink')"
                      @click.stop="store.removeLink(group.id, index)"
                    />
                  </template>
                </v-card-item>
              </v-card>
            </v-col>
          </v-row>

          <!-- Empty group drop zone -->
          <div
            v-if="group.links.length === 0"
            class="quick-drop-zone"
            :class="{ 'quick-drop-active': isDropTarget(group.id, 0) }"
            @dragover.prevent="onDragOver(group.id, 0)"
            @dragleave="onDragLeave($event, group.id, 0)"
            @drop.prevent="onDrop(group.id, 0)"
          >
            {{ $t("home.quickStart.emptyGroup") }}
          </div>

          <!-- Trailing drop zone while a drag is active -->
          <div
            v-else-if="dragSource"
            class="quick-drop-zone"
            :class="{ 'quick-drop-active': isDropTarget(group.id, group.links.length) }"
            @dragover.prevent="onDragOver(group.id, group.links.length)"
            @dragleave="onDragLeave($event, group.id, group.links.length)"
            @drop.prevent="onDrop(group.id, group.links.length)"
          >
            {{ $t("home.quickStart.dropHere") }}
          </div>
        </div>

        <!-- Add navigation link dialog -->
        <v-dialog v-model="addLinkDialog" max-width="560">
          <v-card :title="$t('home.quickStart.addLink')" prepend-icon="mdi-link-variant">
            <v-card-text>
              <v-text-field
                v-model="linkSearch"
                density="compact"
                variant="outlined"
                :label="$t('home.quickStart.searchNav')"
                prepend-inner-icon="mdi-magnify"
                clearable
              />
              <v-list density="comfortable" class="quick-picker-list">
                <template v-for="navGroup in filteredNavigation" :key="navGroup.value">
                  <v-list-subheader>{{ $t(navGroup.title) }}</v-list-subheader>
                  <v-list-item
                    v-for="child in navGroup.childrens"
                    :key="linkKey(child)"
                    :disabled="linkExists(child)"
                    @click="addLinkToCurrent(child)"
                  >
                    <template #prepend>
                      <v-icon size="small">{{ child.icon }}</v-icon>
                    </template>
                    <v-list-item-title class="text-body-2">{{ $t(child.title) }}</v-list-item-title>
                    <template #append>
                      <v-icon v-if="linkExists(child)" size="small">mdi-check</v-icon>
                    </template>
                  </v-list-item>
                </template>
              </v-list>
            </v-card-text>
            <v-card-actions>
              <v-spacer />
              <v-btn :text="$t('action.close')" variant="text" @click="addLinkDialog = false" />
            </v-card-actions>
          </v-card>
        </v-dialog>

        <!-- Delete group confirmation -->
        <confirm-dialog
          ref="confirmDialogRef"
          prepend-icon="mdi-delete-outline"
          :title="$t('home.quickStart.deleteGroup')"
          @confirm="deleteGroupAction"
        >
          <template #body="{ proxy }">
            {{
              $t("home.quickStart.deleteGroupConfirm", {
                group: displayTitle((proxy.group as QuickStartGroup).title),
              })
            }}
          </template>
        </confirm-dialog>
      </v-container>
    </v-main>
  </v-layout>
</template>

<route lang="yaml">
name: Home
meta:
  title: title.home
  requiresAuth: false
  requiresPerm: false
</route>

<script setup lang="ts" name="home">
import router from "@/router";
import { navigationGroups } from "@/navigation";
import type { NavChild } from "@/navigation";
import { isBuiltinGroup, linkKey, useQuickStartStore } from "@/stores/quickStart";
import type { QuickStartGroup, QuickStartLink } from "@/stores/quickStart";
import i18n from "@/utils/i18n";

const store = useQuickStartStore();
const route = useRoute();

// --- Group operations ---
// displayTitle 组标题支持 i18n key 与纯文本：key 存在时翻译，否则原样返回
const displayTitle = (title: string): string => {
  return i18n.global.te(title) ? i18n.global.t(title) : title;
};

const addGroupAction = () => {
  const id = store.addGroup("home.quickStart.newGroup");
  const group = store.groups.find((g) => g.id === id);
  if (group) {
    startRename(group);
  }
};

const editingGroupId = ref("");
const groupTitleDraft = ref("");

const startRename = (group: QuickStartGroup) => {
  editingGroupId.value = group.id;
  groupTitleDraft.value = displayTitle(group.title);
};

const cancelRename = () => {
  editingGroupId.value = "";
};

const commitRename = (id: string) => {
  if (!editingGroupId.value) {
    return;
  }
  const title = groupTitleDraft.value.trim();
  if (title) {
    store.renameGroup(id, title);
  }
  editingGroupId.value = "";
};

const confirmDialogRef = useTemplateRef("confirmDialogRef");

const askDeleteGroup = (group: QuickStartGroup) => {
  confirmDialogRef.value?.open({ group });
};

const deleteGroupAction = (payload: { group: QuickStartGroup }) => {
  store.removeGroup(payload.group.id);
};

// --- Add link dialog ---
const addLinkDialog = ref(false);
const addLinkTargetGroupId = ref("");
const linkSearch = ref("");

const openAddLinkDialog = (groupId: string) => {
  addLinkTargetGroupId.value = groupId;
  linkSearch.value = "";
  addLinkDialog.value = true;
};

const filteredNavigation = computed(() => {
  const keyword = linkSearch.value.trim().toLowerCase();
  if (!keyword) {
    return navigationGroups;
  }
  return navigationGroups
    .map((group) => {
      return {
        ...group,
        childrens: group.childrens.filter((child) => {
          const name = i18n.global.t(child.title).toLowerCase();
          return (
            name.includes(keyword) ||
            child.routeName.toLowerCase().includes(keyword) ||
            linkKey(child).toLowerCase().includes(keyword)
          );
        }),
      };
    })
    .filter((group) => group.childrens.length > 0);
});

const linkExists = (child: NavChild): boolean => {
  const group = store.groups.find((g) => g.id === addLinkTargetGroupId.value);
  if (!group) {
    return false;
  }
  return group.links.some((existing) => linkKey(existing) === linkKey(child));
};

const addLinkToCurrent = (child: NavChild) => {
  store.addLink(addLinkTargetGroupId.value, child);
};

// --- Navigation ---
const goto = (link: QuickStartLink) => {
  // route.params type does not satisfy RouteLocationRaw.params
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  router.push({ name: link.routeName as any, params: { ...route.params, ...link.params } as any });
};

// --- Drag & drop reorder (native HTML5 DnD) ---
const dragSource = ref<{ groupId: string; index: number } | null>(null);
const dropTarget = ref<{ groupId: string; index: number } | null>(null);

const isDragging = (groupId: string, index: number): boolean => {
  return dragSource.value?.groupId === groupId && dragSource.value?.index === index;
};

const isDropTarget = (groupId: string, index: number): boolean => {
  return dropTarget.value?.groupId === groupId && dropTarget.value?.index === index;
};

const onDragStart = (event: DragEvent, groupId: string, index: number) => {
  dragSource.value = { groupId, index };
  if (event.dataTransfer) {
    event.dataTransfer.effectAllowed = "move";
    event.dataTransfer.setData("text/plain", `${groupId}:${index}`);
  }
};

const onDragOver = (groupId: string, index: number) => {
  const current = dropTarget.value;
  if (current?.groupId !== groupId || current?.index !== index) {
    dropTarget.value = { groupId, index };
  }
};

const onDragLeave = (event: DragEvent, groupId: string, index: number) => {
  // 子元素间移动会触发 dragleave，目标仍在其内部时忽略
  const related = event.relatedTarget as Node | null;
  const currentTarget = event.currentTarget as HTMLElement | null;
  if (related && currentTarget?.contains(related)) {
    return;
  }
  if (dropTarget.value?.groupId === groupId && dropTarget.value?.index === index) {
    dropTarget.value = null;
  }
};

const onDrop = (groupId: string, index: number) => {
  const source = dragSource.value;
  if (source) {
    store.moveLink(source.groupId, source.index, groupId, index);
  }
  dragSource.value = null;
  dropTarget.value = null;
};

const onDragEnd = () => {
  dragSource.value = null;
  dropTarget.value = null;
};
</script>

<style scoped>
.quick-group-title {
  cursor: pointer;
  transition: color 0.2s ease;
}

.quick-group-title:hover {
  color: rgb(var(--v-theme-primary));
}

.quick-group-name-input {
  max-width: 320px;
}

/* Link cards, matching the pre-group quick start style. */
.quick-link-card {
  position: relative;
  cursor: grab;
  transition:
    border-color 0.2s ease,
    background-color 0.2s ease;
}

.quick-link-card:hover {
  border-color: rgb(var(--v-theme-primary));
  background-color: rgb(var(--v-theme-primary), 0.06);
}

.quick-link-card:active {
  cursor: grabbing;
}

.quick-link-dragging {
  opacity: 0.4;
}

/* Remove button: only visible while hovering the whole card. */
.quick-link-remove {
  opacity: 0;
  transition: opacity 0.15s ease;
}

.quick-link-card:hover .quick-link-remove {
  opacity: 1;
}

/* Insertion indicator above a card. */
.quick-drop-before {
  box-shadow: inset 0 2px 0 rgb(var(--v-theme-primary));
}

/* Drop zone (empty group / trailing) highlight. */
.quick-drop-zone {
  min-height: 44px;
  margin-top: 8px;
  border: 1px dashed rgb(var(--v-theme-on-surface), 0.25);
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: rgb(var(--v-theme-on-surface), 0.6);
  font-size: 0.875rem;
}

.quick-drop-active {
  border-style: solid;
  border-color: rgb(var(--v-theme-primary));
  background-color: rgb(var(--v-theme-primary), 0.08);
}

.quick-picker-list {
  max-height: 480px;
  overflow-y: auto;
}
</style>
