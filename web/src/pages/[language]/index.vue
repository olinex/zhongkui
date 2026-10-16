<template>
  <v-layout>
    <app-header />
    <v-main>
      <v-container class="pa-4 pa-md-6 zk-page-container">
        <!-- 欢迎区域 -->
        <div class="mb-8">
          <h1 class="text-headline-large font-weight-bold">{{ $t("title.zk") }}</h1>
          <p class="text-title-large on-surface-variant mt-2">{{ $t("home.subtitle") }}</p>
        </div>

        <!-- 快速开始区域 -->
        <div class="d-flex align-center mb-4">
          <h2 class="text-title-large">{{ $t("home.quickStart.self") }}</h2>
          <v-spacer />
          <v-btn
            :text="$t('home.quickStart.addGroup')"
            prepend-icon="mdi-plus"
            color="primary"
            variant="tonal"
            @click="addGroupAction"
          />
        </div>

        <!-- 自定义导航链接分组 -->
        <div v-for="group in store.groups" :key="group.id" class="mb-8">
          <!-- 分组标题：标题（点击可重命名）+ 操作按钮 -->
          <div class="d-flex align-center mb-3 ga-2">
            <v-icon color="primary">mdi-folder-multiple-outline</v-icon>
            <template v-if="editingGroupID === group.id">
              <v-text-field
                v-model="groupTitleDraft"
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
                class="text-body-large font-weight-bold quick-group-title"
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
              :aria-label="$t('home.quickStart.addLink')"
              @click="openAddLinkDialog(group.id)"
            />
            <v-btn
              v-if="!isBuiltinGroup(group)"
              icon="mdi-delete-outline"
              size="small"
              variant="text"
              :title="$t('home.quickStart.deleteGroup')"
              :aria-label="$t('home.quickStart.deleteGroup')"
              @click="askDeleteGroup(group)"
            />
          </div>

          <!-- 链接以卡片展示；网格按可用宽度自适应排布，宽度足够时一行 4 个 -->
          <div class="quick-link-grid">
            <v-card
              v-for="(link, index) in group.links"
              :key="linkKey(link)"
              class="quick-link-card zk-interactive-card"
              :class="{
                'quick-link-dragging': isDragging(group.id, index),
                'zk-drag-over': isDropTarget(group.id, index),
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
                <v-card-title class="text-body-medium ps-1">{{ $t(link.title) }}</v-card-title>
                <!-- 移除按钮：仅在悬停卡片时可见 -->
                <template #append>
                  <v-btn
                    class="quick-link-remove"
                    icon="mdi-close"
                    size="x-small"
                    variant="text"
                    :title="$t('home.quickStart.removeLink')"
                    :aria-label="$t('home.quickStart.removeLink')"
                    @click.stop="store.removeLink(group.id, index)"
                  />
                </template>
              </v-card-item>
            </v-card>
          </div>

          <!-- 空分组的放置区 -->
          <div
            v-if="group.links.length === 0"
            class="quick-drop-zone zk-panel--subtle zk-hairline zk-hint"
            :class="{ 'zk-drag-over': isDropTarget(group.id, 0) }"
            @dragover.prevent="onDragOver(group.id, 0)"
            @dragleave="onDragLeave($event, group.id, 0)"
            @drop.prevent="onDrop(group.id, 0)"
          >
            {{ $t("home.quickStart.emptyGroup") }}
          </div>

          <!-- 拖拽进行中的尾部放置区 -->
          <div
            v-else-if="dragSource"
            class="quick-drop-zone zk-panel--subtle zk-hairline zk-hint"
            :class="{ 'zk-drag-over': isDropTarget(group.id, group.links.length) }"
            @dragover.prevent="onDragOver(group.id, group.links.length)"
            @dragleave="onDragLeave($event, group.id, group.links.length)"
            @drop.prevent="onDrop(group.id, group.links.length)"
          >
            {{ $t("home.quickStart.dropHere") }}
          </div>
        </div>

        <!-- 添加导航链接对话框 -->
        <app-dialog
          v-model="addLinkDialog"
          :title="$t('home.quickStart.addLink')"
          icon="mdi-link-variant"
          size="sm"
        >
          <v-text-field
            v-model="linkSearch"
            :label="$t('home.quickStart.searchNav')"
            prepend-inner-icon="mdi-magnify"
            clearable
          />
          <v-list density="comfortable">
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
                <v-list-item-title class="text-body-medium">{{
                  $t(child.title)
                }}</v-list-item-title>
                <template #append>
                  <v-icon v-if="linkExists(child)" size="small">mdi-check</v-icon>
                </template>
              </v-list-item>
            </template>
          </v-list>
          <template #footer>
            <v-btn :text="$t('action.close')" variant="text" @click="addLinkDialog = false" />
          </template>
        </app-dialog>

        <!-- 删除分组确认 -->
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
import { filterNavigationByPerm, navigationGroups } from "@/navigation";
import type { NavChild } from "@/navigation";
import { useFeaturesStore } from "@/stores/features";
import { isBuiltinGroup, linkKey, useQuickStartStore } from "@/stores/quickStart";
import type { QuickStartGroup, QuickStartLink } from "@/stores/quickStart";
import { useUserStore } from "@/stores/user";
import i18n from "@/utils/i18n";

const store = useQuickStartStore();
const route = useRoute();

// --- 分组操作 ---
// displayTitle 组标题支持 i18n key 与纯文本：key 存在时翻译，否则原样返回
const displayTitle = (title: string): string => {
  return i18n.global.te(title) ? i18n.global.t(title) : title;
};

// addGroupAction 新增一个快速开始分组并立即进入重命名状态。
const addGroupAction = () => {
  const id = store.addGroup("home.quickStart.newGroup");
  const group = store.groups.find((g) => g.id === id);
  if (group) {
    startRename(group);
  }
};

const editingGroupID = ref("");
const groupTitleDraft = ref("");

// startRename 进入指定分组标题的编辑态，草稿预填当前显示名。
const startRename = (group: QuickStartGroup) => {
  editingGroupID.value = group.id;
  groupTitleDraft.value = displayTitle(group.title);
};

// cancelRename 退出分组标题编辑态。
const cancelRename = () => {
  editingGroupID.value = "";
};

// commitRename 提交分组重命名：标题为空则放弃修改。
const commitRename = (id: string) => {
  if (!editingGroupID.value) {
    return;
  }
  const title = groupTitleDraft.value.trim();
  if (title) {
    store.renameGroup(id, title);
  }
  editingGroupID.value = "";
};

const confirmDialogRef = useTemplateRef("confirmDialogRef");

// askDeleteGroup 打开删除该分组的确认对话框。
const askDeleteGroup = (group: QuickStartGroup) => {
  confirmDialogRef.value?.open({ group });
};

// deleteGroupAction 确认后删除分组。
const deleteGroupAction = (payload: { group: QuickStartGroup }) => {
  store.removeGroup(payload.group.id);
};

// --- 添加链接对话框 ---
const addLinkDialog = ref(false);
const addLinkTargetGroupID = ref("");
const linkSearch = ref("");

// openAddLinkDialog 打开添加链接对话框并重置搜索词。
const openAddLinkDialog = (groupID: string) => {
  addLinkTargetGroupID.value = groupID;
  linkSearch.value = "";
  addLinkDialog.value = true;
};

// 按功能开关与权限点过滤导航组：设置 feature 且开关关闭的分组（如 AI 助手）不展示，
// 子项需满足其声明的权限点，分组内子项全部隐藏时该分组一并隐藏。
const visibleNavigationGroups = computed(() => {
  const featuresStore = useFeaturesStore();
  const userStore = useUserStore();
  const groups = navigationGroups.filter(
    (group) => !group.feature || featuresStore.isEnable(group.feature),
  );
  return filterNavigationByPerm(groups, (perm) => userStore.can(perm.obj, perm.act));
});

// 按搜索关键词过滤可见导航组：仅保留命中子项的分组。
const filteredNavigation = computed(() => {
  const keyword = linkSearch.value.trim().toLowerCase();
  const groups = visibleNavigationGroups.value;
  if (!keyword) {
    return groups;
  }
  return groups
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

// linkExists 判断某导航项是否已存在于目标分组中（用于禁用重复添加）。
const linkExists = (child: NavChild): boolean => {
  const group = store.groups.find((g) => g.id === addLinkTargetGroupID.value);
  if (!group) {
    return false;
  }
  return group.links.some((existing) => linkKey(existing) === linkKey(child));
};

// addLinkToCurrent 把选中的导航项添加到目标分组。
const addLinkToCurrent = (child: NavChild) => {
  store.addLink(addLinkTargetGroupID.value, child);
};

// --- 导航跳转 ---
const goto = (link: QuickStartLink) => {
  // route.params 类型不满足 RouteLocationRaw.params 的要求
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  router.push({ name: link.routeName as any, params: { ...route.params, ...link.params } as any });
};

// --- 拖拽排序（原生 HTML5 DnD） ---
const dragSource = ref<{ groupID: string; index: number } | null>(null);
const dropTarget = ref<{ groupID: string; index: number } | null>(null);

// isDragging 判断指定链接是否为当前拖拽源。
const isDragging = (groupID: string, index: number): boolean => {
  return dragSource.value?.groupID === groupID && dragSource.value?.index === index;
};

// isDropTarget 判断指定链接是否为当前放置目标。
const isDropTarget = (groupID: string, index: number): boolean => {
  return dropTarget.value?.groupID === groupID && dropTarget.value?.index === index;
};

// onDragStart 记录拖拽源并设置 dataTransfer 数据。
const onDragStart = (event: DragEvent, groupID: string, index: number) => {
  dragSource.value = { groupID, index };
  if (event.dataTransfer) {
    event.dataTransfer.effectAllowed = "move";
    event.dataTransfer.setData("text/plain", `${groupID}:${index}`);
  }
};

// onDragOver 更新当前放置目标（变更时）。
const onDragOver = (groupID: string, index: number) => {
  const current = dropTarget.value;
  if (current?.groupID !== groupID || current?.index !== index) {
    dropTarget.value = { groupID, index };
  }
};

const onDragLeave = (event: DragEvent, groupID: string, index: number) => {
  // 子元素间移动会触发 dragleave，目标仍在其内部时忽略
  const related = event.relatedTarget as Node | null;
  const currentTarget = event.currentTarget as HTMLElement | null;
  if (related && currentTarget?.contains(related)) {
    return;
  }
  if (dropTarget.value?.groupID === groupID && dropTarget.value?.index === index) {
    dropTarget.value = null;
  }
};

// onDrop 将拖拽源移动到放置目标位置。
const onDrop = (groupID: string, index: number) => {
  const source = dragSource.value;
  if (source) {
    store.moveLink(source.groupID, source.index, groupID, index);
  }
  dragSource.value = null;
  dropTarget.value = null;
};

// onDragEnd 拖拽结束时清理拖拽源与放置目标状态。
const onDragEnd = () => {
  dragSource.value = null;
  dropTarget.value = null;
};
</script>

<style scoped>
.quick-group-title {
  cursor: pointer;
}

.quick-group-name-input {
  max-width: var(--zk-container-max-width-narrow);
}

/* 快捷入口网格：按可用宽度自适应排布，一行最多 4 个。
   轨道最小宽度取「--zk-quick-link-min-width」与「(100% - 3 × 间距) / 4」中的较大值：
   后者保证可用宽度足够时恰好 4 列（自动填充不会溢出为第 5 列），
   不足时回落到固定最小宽度，逐级减少为 3 / 2 / 1 列。 */
.quick-link-grid {
  display: grid;
  grid-template-columns: repeat(
    auto-fill,
    minmax(max(var(--zk-quick-link-min-width), calc((100% - 3 * var(--zk-space-2)) / 4)), 1fr)
  );
  gap: var(--zk-space-2);
}

/* 链接卡片：悬停反馈来自全局的 .zk-interactive-card 外壳。 */
.quick-link-card {
  position: relative;
  cursor: grab;
}

.quick-link-card:active {
  cursor: grabbing;
}

.quick-link-dragging {
  opacity: 0.4;
}

/* 移除按钮：仅在悬停整个卡片时可见。 */
.quick-link-remove {
  opacity: 0;
  transition: opacity var(--zk-transition-fast) ease;
}

.quick-link-card:hover .quick-link-remove {
  opacity: 1;
}

/* 放置区（空分组 / 尾部）：仅负责布局，外观来自
   .zk-panel--subtle + .zk-hairline，高亮来自 .zk-drag-over。 */
.quick-drop-zone {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: var(--zk-control-height-compact);
  margin-top: var(--zk-space-2);
}
</style>
