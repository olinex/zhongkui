<template>
  <!-- JSON 树视图统一入口：行号用于按行定位，容器折叠图标用于收起无关分支；
       容器边界旁的箭头可在起始 / 结束括号之间快速跳转 -->
  <VueJsonPretty
    class="zk-json-viewer"
    :data="viewerData"
    show-line-number
    show-icon
    :render-node-actions="renderNodeActions"
  />
</template>

<script setup lang="ts" name="json-viewer">
import VueJsonPretty from "vue-json-pretty";
import type { JSONDataType } from "vue-json-pretty/types/utils";
import "vue-json-pretty/lib/styles.css";

import i18n from "@/utils/i18n";

const props = withDefaults(defineProps<{ data?: JSONDataType }>(), { data: null });

const viewerData = computed(() => props.data);

// 边界符号 → 跳转方向：起始括号向下找匹配的结束括号，结束括号向上回到起始括号
const BOUNDARY_DIRECTIONS: Record<string, "up" | "down"> = {
  objectStart: "down",
  arrayStart: "down",
  objectEnd: "up",
  arrayEnd: "up",
};

// 括号对深度的贡献：起始 +1、结束 -1；折叠态的 "{...}" / "[...]" 不含整行括号，计 0
const BRACKET_DELTA: Record<string, number> = { "{": 1, "[": 1, "}": -1, "]": -1 };

const bracketDeltaOf = (row: Element): number => {
  let delta = 0;
  for (const bracket of row.querySelectorAll(".vjs-tree-brackets")) {
    delta += BRACKET_DELTA[bracket.textContent?.trim() ?? ""] ?? 0;
  }
  return delta;
};

// jumpToBoundary 从当前行按括号深度逐行查找匹配的另一侧边界，并滚动到该行：
// 向下跳时对齐可视区底部，向上跳时对齐顶部，便于把容器两端的对照内容一次留在视野内
const jumpToBoundary = (event: MouseEvent, direction: "up" | "down"): void => {
  event.stopPropagation();
  const row = (event.currentTarget as HTMLElement).closest(".vjs-tree-node");
  const tree = row?.closest(".vjs-tree");
  if (!row || !tree) {
    return;
  }
  const rows = Array.from(tree.querySelectorAll(".vjs-tree-node"));
  const start = rows.indexOf(row);
  if (start < 0) {
    return;
  }
  const step = direction === "down" ? 1 : -1;
  let depth = bracketDeltaOf(row);
  let target: Element | null = null;
  for (let i = start + step; i >= 0 && i < rows.length; i += step) {
    depth += bracketDeltaOf(rows[i]);
    if (depth === 0) {
      target = rows[i];
      break;
    }
  }
  target?.scrollIntoView({
    behavior: "smooth",
    block: direction === "down" ? "end" : "start",
  });
};

// renderNodeActions 仅在容器边界行渲染跳转按钮；其余行的动作容器为空，由样式隐藏
const renderNodeActions = ({ node }: { node: { type: string } }): unknown => {
  const direction = BOUNDARY_DIRECTIONS[node.type];
  if (!direction) {
    return null;
  }
  const label = i18n.global.t(
    direction === "up" ? "action.jumpToContainerTop" : "action.jumpToContainerBottom",
  );
  return h(
    "button",
    {
      type: "button",
      class: "zk-json-boundary-jump",
      title: label,
      "aria-label": label,
      onClick: (event: MouseEvent) => jumpToBoundary(event, direction),
    },
    [h("i", { class: `mdi mdi-arrow-collapse-${direction}`, "aria-hidden": "true" })],
  );
};
</script>
