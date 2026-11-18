<template>
  <!-- 网络堆栈对话框：以 HttpTransport 的 traceID 查询组成该 TCP 流的全部网络堆栈记录，
       按 seq 分组为“包”，并根据 AppTailer 的 payload offset 与记录的
       payloadStartOffset/payloadEndOffset 相交，高亮请求包与响应包 -->
  <!-- 未走 AppDialog：本对话框为 fullscreen + dialog-bottom-transition 形态（骨架仅覆盖
       居中卡片式对话框），全屏无宽度档位，故保留原生 VDialog 用法 -->
  <v-dialog
    :model-value="modelValue"
    fullscreen
    transition="dialog-bottom-transition"
    @update:model-value="onUpdate"
  >
    <v-card>
      <v-toolbar>
        <v-toolbar-title class="text-no-wrap">
          {{ $t("model.networkStack.name") }}
        </v-toolbar-title>
        <span
          v-if="transport?.traceID"
          class="font-mono text-body-small on-surface-variant text-truncate"
        >
          {{ transport.traceID }}
        </span>
        <v-spacer />
        <v-btn
          icon="mdi-close"
          variant="text"
          :title="$t('action.close')"
          :aria-label="$t('action.close')"
          @click="onUpdate(false)"
        />
      </v-toolbar>

      <div class="stack-dialog-body px-6 py-4">
        <!-- 传输摘要：请求行 + 高亮图例与计数 -->
        <v-sheet v-if="transport" class="pa-3 mb-3 rounded">
          <div class="d-flex align-center ga-2 flex-wrap">
            <v-chip color="primary">{{ method }}</v-chip>
            <span class="font-mono text-body-medium">{{ host }}{{ path }}</span>
          </div>
          <div class="d-flex align-center ga-3 mt-2 flex-wrap">
            <v-chip color="primary" prepend-icon="mdi-arrow-up-bold">
              {{ $t("model.networkStack.summary.requests") }} {{ requestCount }}
            </v-chip>
            <v-chip color="success" prepend-icon="mdi-arrow-down-bold">
              {{ $t("model.networkStack.summary.responses") }} {{ responseCount }}
            </v-chip>
            <v-chip> {{ $t("model.networkStack.summary.packets") }} {{ packets.length }} </v-chip>
            <span v-if="hasMore" class="text-body-small on-surface-variant">
              {{ $t("model.networkStack.summary.loaded") }} {{ items.length }}/{{ total }}
            </span>
          </div>
        </v-sheet>

        <v-alert v-if="error" type="error" class="mb-3">
          {{ error }}
        </v-alert>
        <div v-if="!transport" class="text-center my-8 on-surface-variant">
          {{ $t("model.networkStack.summary.noTraceID") }}
        </div>
        <div
          v-else-if="!loading && packets.length === 0"
          class="text-center my-8 on-surface-variant"
        >
          {{ $t("model.networkStack.summary.empty") }}
        </div>

        <v-progress-linear v-if="loading && packets.length === 0" indeterminate class="mb-3" />

        <!-- 双栏对比视图：左=客户端→服务端，右=服务端→客户端，按 seq 上下对齐，
             同序号两侧的链路/网络/传输层（含 TCP 头）在同一行左右并排，便于对比 -->
        <div v-if="packets.length > 0" class="stack-compare">
          <div class="zk-stack-compare-header">
            <v-icon size="x-small" class="mr-1">mdi-arrow-up-bold</v-icon>
            {{ $t("model.networkStack.direction.clientToServer") }}
          </div>
          <div class="zk-stack-compare-header">
            <v-icon size="x-small" class="mr-1">mdi-arrow-down-bold</v-icon>
            {{ $t("model.networkStack.direction.serverToClient") }}
          </div>
          <template v-for="cell in stackCells" :key="cell.key">
            <v-card
              v-if="cell.packet"
              :class="packetClass(cell.packet.kind)"
              class="zk-stack-packet"
            >
              <v-card-item>
                <template #prepend>
                  <v-avatar :color="packetColor(cell.packet.kind)" variant="tonal" size="small">
                    <span class="font-mono text-body-small">{{ cell.packet.seq }}</span>
                  </v-avatar>
                </template>
                <template #title>
                  <div class="d-flex align-center ga-2 flex-wrap">
                    <v-chip :color="packetColor(cell.packet.kind)">
                      {{ packetKindText(cell.packet.kind) }}
                    </v-chip>
                    <span class="font-mono text-body-small on-surface-variant">
                      [{{ cell.packet.start }} , {{ cell.packet.end }})
                    </span>
                    <span class="text-body-small on-surface-variant">
                      {{ localeDatetimeFormat(DateTime.fromISO(cell.packet.timestamp)) }}
                    </span>
                  </div>
                </template>
              </v-card-item>
              <v-card-text class="pt-0">
                <v-divider class="mb-2" />
                <v-table density="compact">
                  <tbody>
                    <tr
                      v-for="layer in cell.packet.layers"
                      :key="`${cell.packet.cli2Srv}-${cell.packet.seq}-${layer.level}-${layer.proto}`"
                    >
                      <td class="layer-level-col text-no-wrap">
                        <v-chip size="x-small" variant="flat" color="grey">
                          {{ levelName(layer.level) }}
                        </v-chip>
                      </td>
                      <td class="layer-proto-col text-no-wrap font-mono text-body-medium">
                        {{ protoName(layer.proto) }}
                      </td>
                      <td>
                        <!-- 协议头部：可读摘要 + 逐字段说明表格；未知协议回退原始 JSON -->
                        <network-stack-header
                          :headers="layer.headers || ''"
                          :proto="Number(layer.proto ?? 0)"
                          :title="`${protoName(layer.proto)} · ${levelName(layer.level)}`"
                        />
                      </td>
                    </tr>
                  </tbody>
                </v-table>
              </v-card-text>
            </v-card>
            <div v-else class="zk-stack-compare-empty" />
          </template>
        </div>

        <!-- 加载更多：后端单页上限 100 条（分页 oneof=10 20 50 100），长流分批拉取 -->
        <div v-if="hasMore && !loading" class="text-center my-4">
          <v-btn
            :text="$t('model.networkStack.summary.loadMore')"
            variant="tonal"
            @click="loadMore"
          />
        </div>
        <v-progress-linear v-if="loading && packets.length > 0" indeterminate class="my-2" />
      </div>
    </v-card>
  </v-dialog>
</template>

<script setup lang="ts" name="network-stack-dialog">
import { DateTime } from "luxon";

import { getResponse } from "@/apis/base";
import type { NetworkStackListItem, NetworkStackListOut } from "@/apis/@types/traffic";
import { trafficStackListGet } from "@/apis/traffic";
import type { FilterByItem, SearchIn } from "@/components/@types/table";
import { SearchAction } from "@/types";
import i18n, { localeDatetimeFormat } from "@/utils/i18n";

const PAGE_SIZE = 100;
// 时间窗口余量：以 AppTailer 时间锚点（请求/响应段起止采集时间）向两侧扩展，
// 覆盖整条 TCP 流的握手/确认等非载荷包与入库延迟；traceID 已限定到单条流，
// 窗口放宽不影响结果正确性，仅影响查询命中的小时分区数量
const TIME_MARGIN = { hours: 1 };

// gopacket LayerType 数值 → 协议名（gopacket v1.6.0 layers/layertypes.go）
const PROTO_NAMES: Record<number, string> = {
  0: "Payload",
  10: "ARP",
  15: "Dot1Q",
  17: "Ethernet",
  19: "ICMPv4",
  20: "IPv4",
  21: "IPv6",
  22: "LLC",
  23: "SNAP",
  25: "PPP",
  26: "PPPoE",
  44: "TCP",
  45: "UDP",
  54: "Loopback",
  57: "ICMPv6",
  64: "RadioTap",
  65: "Dot11",
  67: "Dot11Data",
  68: "Dot11DataCFAck",
  69: "Dot11DataCFPoll",
  70: "Dot11DataCFAckPoll",
  71: "Dot11DataNull",
  72: "Dot11DataCFAckNoData",
  73: "Dot11DataCFPollNoData",
  74: "Dot11DataCFAckPollNoData",
  75: "Dot11DataQOSData",
  76: "Dot11DataQOSDataCFAck",
  77: "Dot11DataQOSDataCFPoll",
  78: "Dot11DataQOSDataCFAckPoll",
  79: "Dot11DataQOSNull",
  80: "Dot11DataQOSCFPollNoData",
  81: "Dot11DataQOSCFAckPollNoData",
  106: "Dot11WEP",
  113: "Linux SLL",
  115: "Prism",
  276: "Linux SLL2",
};

// OSI 层级（LayerLevel）→ i18n 键
const LEVEL_I18N: Record<number, string> = {
  0: "model.networkStack.level.unknown",
  2: "model.networkStack.level.link",
  3: "model.networkStack.level.network",
  4: "model.networkStack.level.transport",
  7: "model.networkStack.level.application",
};

type PacketKind = "request" | "response" | "other";

interface PacketView {
  seq: number;
  cli2Srv: boolean;
  kind: PacketKind;
  timestamp: string;
  start: number;
  end: number;
  layers: NetworkStackListItem[];
}

const props = defineProps<{
  modelValue: boolean;
  transport?: Record<string, unknown> | null;
}>();

const emits = defineEmits<{
  "update:modelValue": [value: boolean];
}>();

const onUpdate = (value: boolean): void => {
  emits("update:modelValue", value);
};

const page = ref(1);
const items = ref<NetworkStackListItem[]>([]);
const total = ref(0);
const loading = ref(false);
const error = ref("");
// seenRows 记录已加载行的唯一键（cli2Srv:seq:level:proto），防御分页边界重复行
const seenRows = ref(new Set<string>());

// 每次打开时重置分页状态并重新查询
watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      page.value = 1;
      items.value = [];
      total.value = 0;
      error.value = "";
      seenRows.value = new Set();
      void fetchPage();
    }
  },
);

const hasMore = computed(() => {
  return items.value.length < total.value;
});

// AppTailer 时间锚点：请求/响应段在各自方向的起止采集时间 + 传输自身时间
const anchorTimes = computed<DateTime[]>(() => {
  const t = props.transport ?? {};
  const out: DateTime[] = [];
  for (const key of [
    "timestamp",
    "clientStartTimestamp",
    "clientEndTimestamp",
    "serverStartTimestamp",
    "serverEndTimestamp",
    "createdAt",
  ]) {
    const raw = t[key];
    if (typeof raw === "string" && raw !== "") {
      const dt = DateTime.fromISO(raw);
      if (dt.isValid) {
        out.push(dt);
      }
    }
  }
  return out;
});

// buildSearchIn 构造 stack/list 查询：traceID 必填 + createdAt 范围（分区裁剪），timestamp 同时提供以按采集时间精确过滤
const buildSearchIn = (): SearchIn => {
  const t = props.transport ?? {};
  let start = DateTime.now().minus({ day: 1 });
  let end = DateTime.now();
  if (anchorTimes.value.length > 0) {
    const sorted = [...anchorTimes.value].sort((a, b) => a.toMillis() - b.toMillis());
    start = sorted[0].minus(TIME_MARGIN);
    end = sorted[sorted.length - 1].plus(TIME_MARGIN);
  }
  const toIso = (dt: DateTime): string => dt.toUTC().toISO() ?? "";
  const filterBy: Record<string, FilterByItem> = {
    timestampFrom: { action: SearchAction.GREATER_EQUAL, value: toIso(start) },
    timestampTo: { action: SearchAction.LESS_EQUAL, value: toIso(end) },
    createdAtFrom: { action: SearchAction.GREATER_EQUAL, value: toIso(start) },
    createdAtTo: { action: SearchAction.LESS_EQUAL, value: toIso(end) },
    traceID: { action: SearchAction.EQUAL, value: String(t.traceID) },
  };
  if (typeof t.namespace === "string" && t.namespace !== "") {
    filterBy.namespace = { action: SearchAction.EQUAL, value: t.namespace };
  }
  return {
    greedy: false,
    page: page.value,
    pageSize: PAGE_SIZE,
    // 按采集时间升序分页；同一包的三层记录共享同一 timestamp（双向同 seq 的包时间戳也相同），
    // 仅按 timestamp 排序时 ClickHouse 对并列键不保证稳定顺序，OFFSET 分页会令边界行重复/漏跳，
    // 因此追加 cli2Srv/seq/level/proto 决胜列构成全序，保证分页边界确定
    orderBy: [
      { name: "timestamp", order: 1 },
      { name: "cli2Srv", order: 1 },
      { name: "seq", order: 1 },
      { name: "level", order: 1 },
      { name: "proto", order: 1 },
    ],
    filterBy,
  };
};

const fetchPage = async (): Promise<boolean> => {
  if (!props.transport?.traceID) {
    return false;
  }
  loading.value = true;
  try {
    let ok = false;
    await getResponse<NetworkStackListOut>(
      trafficStackListGet(buildSearchIn()),
      async (rsp) => {
        ok = true;
        total.value = rsp.data.total;
        // 按 (cli2Srv, seq, level, proto) 唯一键去重，防御分页边界重复行
        for (const item of rsp.data.items) {
          const key = `${Boolean(item.cli2Srv)}:${Number(item.seq ?? 0)}:${Number(item.level ?? 0)}:${Number(item.proto ?? 0)}`;
          if (!seenRows.value.has(key)) {
            seenRows.value.add(key);
            items.value.push(item);
          }
        }
      },
      async (rsp) => {
        error.value = rsp.summary;
      },
    );
    return ok;
  } finally {
    loading.value = false;
  }
};

const loadMore = async (): Promise<void> => {
  page.value += 1;
  const ok = await fetchPage();
  if (!ok) {
    // 失败时回滚页码，避免跳过一页数据
    page.value -= 1;
  }
};

// intersect 判断包载荷区间 [start,end) 与目标区间 [targetStart,targetEnd) 相交；
// 空载荷包（start == end）不在调用方传入，而按偏移是否落在目标区间内部单独判断
const intersect = (start: number, end: number, targetStart: number, targetEnd: number): boolean => {
  return start < targetEnd && end > targetStart && end > start;
};

// packets 按包分组为包视图。包标识 = 方向 + seq：
// 客户端/服务端两侧流各自从 seq=0 开始独立编号（Side 各自维护 PayloadOffsets），
// 仅按 seq 分组会把客户端 seq=0 与服务端 seq=0 误并为同一个包，因此必须连同 cli2Srv 分组；
// 层内记录按 level 升序，高亮状态按 offset 相交算法计算
const packets = computed<PacketView[]>(() => {
  const t = props.transport ?? {};
  const clientStart = Number(t.clientStartPayloadOffset ?? 0);
  const clientEnd = Number(t.clientEndPayloadOffset ?? 0);
  const serverStart = Number(t.serverStartPayloadOffset ?? 0);
  const serverEnd = Number(t.serverEndPayloadOffset ?? 0);
  const groups = new Map<string, NetworkStackListItem[]>();
  for (const item of items.value) {
    const cli2Srv = Boolean(item.cli2Srv);
    const key = `${cli2Srv ? "c" : "s"}:${Number(item.seq ?? 0)}`;
    const list = groups.get(key);
    if (list) {
      list.push(item);
    } else {
      groups.set(key, [item]);
    }
  }
  const out: PacketView[] = [];
  for (const layers of groups.values()) {
    layers.sort((a, b) => Number(a.level ?? 0) - Number(b.level ?? 0));
    const first = layers[0];
    const start = Number(first.payloadStartOffset ?? 0);
    const end = Number(first.payloadEndOffset ?? 0);
    const cli2Srv = Boolean(first.cli2Srv);
    const empty = end === start;
    let kind: PacketKind = "other";
    if (cli2Srv) {
      // 非空包：载荷区间与请求字节区间相交；空包：偏移严格落在请求区间“中间”时同样归入请求段
      const hit = empty
        ? clientStart < start && start < clientEnd
        : intersect(start, end, clientStart, clientEnd);
      if (hit) kind = "request";
    } else {
      const hit = empty
        ? serverStart < start && start < serverEnd
        : intersect(start, end, serverStart, serverEnd);
      if (hit) kind = "response";
    }
    out.push({
      seq: Number(first.seq ?? 0),
      cli2Srv,
      kind,
      timestamp: first.timestamp,
      start,
      end,
      layers,
    });
  }
  // 跨方向按采集时间升序排列，同一方向内部 seq 递增与时序一致
  out.sort((a, b) => tsMillis(a.timestamp) - tsMillis(b.timestamp) || a.seq - b.seq);
  return out;
});

// tsMillis 解析采集时间为毫秒时间戳，非法值回退为 0（避免 NaN 破坏排序）
const tsMillis = (value: string): number => {
  const dt = DateTime.fromISO(value);
  return dt.isValid ? dt.toMillis() : 0;
};

// stackCells 双栏对比视图：左=客户端方向(cli2Srv=true)、右=服务端方向(cli2Srv=false)，
// 两侧流各自从 seq=0 连续编号，按 seq 上下对齐后展平为 [左0,右0,左1,右1,...]，
// 配合 .stack-compare 的 grid 两列布局，实现同序号两侧（含 TCP 层）同一行左右并排；
// 某一侧缺少该序号的包时以空占位保持行对齐
const stackCells = computed<Array<{ key: string; packet: PacketView | null }>>(() => {
  const left = new Map<number, PacketView>();
  const right = new Map<number, PacketView>();
  for (const p of packets.value) {
    (p.cli2Srv ? left : right).set(p.seq, p);
  }
  const maxSeq = Math.max(0, ...Array.from(left.keys()), ...Array.from(right.keys()));
  const cells: Array<{ key: string; packet: PacketView | null }> = [];
  for (let seq = 0; seq <= maxSeq; seq++) {
    cells.push({ key: `c${seq}`, packet: left.get(seq) ?? null });
    cells.push({ key: `s${seq}`, packet: right.get(seq) ?? null });
  }
  return cells;
});

const requestCount = computed(() => packets.value.filter((p) => p.kind === "request").length);
const responseCount = computed(() => packets.value.filter((p) => p.kind === "response").length);

const method = computed(() => String(props.transport?.method ?? ""));
const host = computed(() => String(props.transport?.host ?? ""));
const path = computed(() => String(props.transport?.path ?? ""));

const packetColor = (kind: PacketKind): string => {
  return kind === "request" ? "primary" : kind === "response" ? "success" : "grey";
};

const packetKindText = (kind: PacketKind): string => {
  const key =
    kind === "request"
      ? "model.networkStack.packet.request"
      : kind === "response"
        ? "model.networkStack.packet.response"
        : "model.networkStack.packet.other";
  return i18n.global.t(key);
};

const levelName = (value: unknown): string => {
  const v = Number(value);
  return i18n.global.t(LEVEL_I18N[v] ?? "model.networkStack.level.unknown");
};

const protoName = (value: unknown): string => {
  const v = Number(value);
  return PROTO_NAMES[v] ?? `#${v}`;
};

// packetClass 高亮样式：命中的请求/响应包叠加 .zk-stack-request/.zk-stack-response，
// 以左侧色条 + 顶部色带 + 高透明度底色与普通包形成强烈区分
const packetClass = (kind: PacketKind): Record<string, boolean> => {
  return {
    "zk-stack-request": kind === "request",
    "zk-stack-response": kind === "response",
  };
};
</script>

<style scoped>
/* 全屏对话框主体：视口高度减去紧凑工具栏高度（令牌值），并自行纵向滚动 */
.stack-dialog-body {
  height: calc(100vh - var(--zk-toolbar-height-compact));
  overflow-y: auto;
}

/* 双栏对比网格：两列布局，[左0,右0,左1,右1,...] 依序排入，每对元素占一行，
   实现同序号两侧（客户端/服务端）上下对齐、左右并排 */
.stack-compare {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  gap: var(--zk-space-3);
  align-items: start;
}

/* 层表列宽提示：层级列 / 协议列限定宽度，协议头部列自适应占满剩余空间 */
.layer-level-col {
  width: 96px;
}

.layer-proto-col {
  width: 120px;
}
</style>
