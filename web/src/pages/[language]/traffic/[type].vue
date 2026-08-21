<template>
  <main-layout current-nav="traffic">
    <template #body>
      <v-container>
        <v-form class="filter-form">
          <v-row dense align-center>
            <!-- Time range filters: capture time (timestamp) and ingestion time (createdAt),
                 each a single Vuetify range picker (start ~ end). 245px min-width range
                 fields cannot fit 25% columns until the lg breakpoint (container ≥ 1200px),
                 so keep the row inline only from lg up and stack below to avoid overlapping -->
            <v-col cols="12" sm="6" lg="4">
              <date-time-range-field
                v-model="timestampRange"
                v-bind="timestampRangeProps"
                :label="$t('model.__base.field.timestampRange')"
              />
            </v-col>
            <v-col cols="12" sm="6" lg="4">
              <date-time-range-field
                v-model="createdAtRange"
                v-bind="createdAtRangeProps"
                :label="$t('model.__base.field.createdAtRange')"
              />
            </v-col>
            <v-col cols="auto" class="ml-auto">
              <div class="d-flex align-center ga-2">
                <!-- 高级筛选开关：紧凑文本按钮内联于操作区，不再独占整行；
                     与其他图标按钮同高（variant=text + density=compact），hover 高亮保持一致 -->
                <v-btn
                  :append-icon="advancedFilterOpen ? 'mdi-chevron-up' : 'mdi-chevron-down'"
                  variant="text"
                  density="compact"
                  :aria-label="$t('model.__base.field.advancedFilter')"
                  @click="advancedFilterOpen = !advancedFilterOpen"
                >
                  {{ $t("model.__base.field.advancedFilter") }}
                </v-btn>
                <!-- 查询按钮常驻展示：收起状态下仍可直接发起查询 -->
                <v-btn
                  :loading="loading"
                  icon="mdi-magnify"
                  color="primary"
                  variant="text"
                  density="compact"
                  :title="$t('action.query')"
                  :aria-label="$t('action.query')"
                  @click="submitQuery"
                />
                <!-- 其余操作默认收起：刷新常驻，列设置展开后显示 -->
                <action-button-group :refresh="refreshAction" :loading="loading" collapsible>
                  <column-visibility-menu
                    v-model="visibleKeys"
                    v-model:order="columnOrder"
                    :headers="allToggleable"
                  />
                </action-button-group>
              </div>
            </v-col>
          </v-row>
          <!-- 高级筛选字段：可选重要字段（五元组/URI 等），仅精确匹配 eq；
               后端配套 bloom_filter/set 跳过索引，在必填时间窗口内按数据块剪枝 -->
          <v-expand-transition>
            <div v-if="advancedFilterOpen" class="mt-2">
              <v-row dense>
                <v-col
                  v-for="field in optionalFilterFields"
                  :key="field.key"
                  cols="12"
                  sm="6"
                  md="4"
                  lg="3"
                >
                  <v-text-field
                    v-model="optionalFilters[field.key]"
                    :label="trafficFieldTitle(field.key)"
                    :type="field.kind"
                    density="compact"
                    hide-details
                  />
                </v-col>
              </v-row>
            </div>
          </v-expand-transition>
        </v-form>
        <v-divider class="my-2" />
        <v-row dense>
          <v-col cols="12">
            <v-data-table-server
              v-model:page="page"
              v-model:items-per-page="pageSize"
              v-model:sort-by="sortBy"
              :headers="visibleHeaders"
              :items="items"
              :items-length="total"
              :items-per-page-options="pageSizeOptions"
              :loading="loading"
              hover
              @click:row="onClickRow"
            >
              <template #item="{ item }">
                <tr class="text-no-wrap" style="cursor: pointer">
                  <td
                    v-for="hdr in visibleHeaders"
                    :key="hdr.key"
                    :class="
                      hdr.fixed === 'end'
                        ? 'v-data-table-column--fixed-end v-data-table-column--first-fixed-end'
                        : undefined
                    "
                  >
                    <!-- 加密字段与详情页展示形式统一：占位“已加密”，可点击直接解密查看明文；
                         S3 字段展示对象键与存储大小，均不直接暴露 base64 密文 -->
                    <template v-if="cryptoKeys.has(hdr.key)">
                      <v-btn
                        :text="$t('model.__base.field.encrypted')"
                        size="small"
                        variant="tonal"
                        color="warning"
                        :loading="plaintextLoading === hdr.key"
                        @click.stop="viewPlaintextCrypto(item, hdr.key)"
                      />
                    </template>
                    <template v-else-if="s3Keys.has(hdr.key)">
                      <span class="font-mono">{{ item[hdr.key] }}</span>
                      <storage-size-chip
                        class="ml-2"
                        :value="Number(item[`${hdr.key}Length`] ?? 0)"
                        binary
                        density="comfortable"
                      />
                      <v-btn
                        :text="$t('action.viewPlaintext')"
                        class="ml-2"
                        size="small"
                        variant="tonal"
                        color="primary"
                        :loading="plaintextLoading === hdr.key"
                        @click.stop="viewPlaintextS3(item, hdr.key)"
                      />
                    </template>
                    <adaptive-cell v-else :header="hdr" :item="item" :actions="rowActions" />
                  </td>
                </tr>
              </template>
            </v-data-table-server>
          </v-col>
        </v-row>
      </v-container>

      <!-- 行详情对话框：列表已返回全字段，直接展示行数据，无需再发起详情请求；
           只读明细以全屏对话框展示，便于浏览大字段与长报文 -->
      <model-detail-dialog
        v-model="detailDialog"
        :title="$t('title.trafficDetail')"
        :fields="detailFields"
        :plaintext-loading="plaintextLoading"
        @view-plaintext="viewPlaintextAction"
      />

      <!-- 明文查看对话框（共享组件：加密/S3 字段解密展示与下载） -->
      <plaintext-dialog ref="plaintextDialogRef" />

      <!-- 网络堆栈对话框：以选中传输的 traceId 展示其所在 TCP 流的网络堆栈，
           并按 AppTailer payload offset 相交高亮请求/响应包 -->
      <network-stack-dialog v-model="stackDialog" :transport="stackTransport" />
    </template>
  </main-layout>
</template>

<route lang="yaml">
name: Traffic List
meta:
  title: title.traffic
  requiresAuth: true
  requiresPerm: true
</route>

<script setup lang="ts">
import { DateTime } from "luxon";
import { useForm } from "vee-validate";

import { getResponse, getSimpleResponse } from "@/apis/base";
import { tagRuleSampleCreate } from "@/apis/admin";
import {
  TrafficCryptoFields,
  TrafficModelKeys,
  TrafficS3Fields,
  trafficListGet,
} from "@/apis/traffic";
import type { TrafficListOut, TrafficListItem, TrafficType } from "@/apis/@types/traffic";
import type { DetailField } from "@/components/@types/detail";
import type { FilterByItem, ItemActions, SearchIn, TableHeader } from "@/components/@types/table";
import { trafficFilterSchema } from "@/schemas/traffic";
import { useAppStore } from "@/stores/app";
import { BackendDataType, SearchAction } from "@/types";
import { vuetifyCfg } from "@/utils/form";
import i18n, { localeDatetimeFormat } from "@/utils/i18n";
import { modelFieldTitle } from "@/utils/modelField";
import { useColumnVisibility } from "@/utils/column";

const DATETIME_FIELD_KEYS = new Set(["timestamp", "createdAt"]);

// 高级筛选可选字段配置：按流量类型罗列常用重要字段（五元组 / 会话 ID / URI 等）。
// 与后端 search.toml 的 filterBy 名称一致，仅精确匹配（eq）——配合 bloom_filter/set 跳过索引，
// 在必填时间窗口内按数据块剪枝，避免十亿级数据逐行扫描。
const OPTIONAL_FILTER_FIELDS: Record<
  TrafficType,
  Array<{ key: string; kind: "text" | "number" }>
> = {
  "ipv4-tcp": [
    { key: "ipv4Src", kind: "text" },
    { key: "ipv4Dst", kind: "text" },
    { key: "srcPort", kind: "number" },
    { key: "dstPort", kind: "number" },
    { key: "sessionId", kind: "text" },
  ],
  "ipv6-tcp": [
    { key: "ipv6Src", kind: "text" },
    { key: "ipv6Dst", kind: "text" },
    { key: "srcPort", kind: "number" },
    { key: "dstPort", kind: "number" },
    { key: "sessionId", kind: "text" },
  ],
  "http-transport": [
    { key: "clientIp", kind: "text" },
    { key: "serverIp", kind: "text" },
    { key: "clientPort", kind: "number" },
    { key: "serverPort", kind: "number" },
    { key: "method", kind: "text" },
    { key: "host", kind: "text" },
    { key: "path", kind: "text" },
    { key: "httpVersion", kind: "text" },
    { key: "fileExt", kind: "text" },
  ],
};

// 流量模型类型与路由段一一对应（/traffic/:type），侧边栏各模型入口分别携带 type 参数
const isTrafficType = (value: unknown): value is TrafficType => {
  return value === "ipv4-tcp" || value === "ipv6-tcp" || value === "http-transport";
};

const route = useRoute();
const router = useRouter();

// 当前流量模型由路由参数决定；非法参数回退默认模型，保证 URL 与侧边栏高亮一致
const type = computed<TrafficType>(() => {
  return isTrafficType(route.params.type) ? route.params.type : "ipv4-tcp";
});

watch(
  () => route.params.type,
  (raw) => {
    if (!isTrafficType(raw)) {
      router.replace({ name: "Traffic List", params: { ...route.params, type: "ipv4-tcp" } });
    }
  },
  { immediate: true },
);

const pageSizeOptions = [
  { title: "10", value: 10 },
  { title: "20", value: 20 },
  { title: "50", value: 50 },
  { title: "100", value: 100 },
];

const appStore = useAppStore();

const page = ref(1);
const pageSize = ref(10);
const loading = ref(false);
const items = ref([] as TrafficListItem[]);
const total = ref(0);
const sortBy = ref<Array<{ key: string; order?: boolean | "asc" | "desc" }>>([
  { key: "timestamp", order: "desc" },
]);

const detailDialog = ref(false);
const detailItem = ref<Record<string, unknown> | null>(null);

// 网络堆栈对话框：选中 http-transport 行（携带 traceId 与 AppTailer offset）后打开
const stackDialog = ref(false);
const stackTransport = ref<Record<string, unknown> | null>(null);
const openStack = (item: Record<string, unknown>): void => {
  stackTransport.value = item;
  stackDialog.value = true;
};

const plaintextDialogRef = useTemplateRef("plaintextDialogRef");
const plaintextLoading = ref("");

// 高级筛选状态：面板开合 + 可选字段值（按 type 动态渲染，仅非空值参与查询）
const advancedFilterOpen = ref(false);
const optionalFilters = reactive<Record<string, string>>({});
const optionalFilterFields = computed(() => OPTIONAL_FILTER_FIELDS[type.value]);

// 初始查询范围：近一天（起始 ~ 截止）
const initialRange = (): [DateTime, DateTime] => {
  return [DateTime.now().minus({ day: 1 }), DateTime.now()];
};

const { defineField, handleSubmit } = useForm({
  validationSchema: trafficFilterSchema,
  initialValues: {
    timestampRange: initialRange(),
    createdAtRange: initialRange(),
  },
});
const [timestampRange, timestampRangeProps] = defineField("timestampRange", vuetifyCfg);
const [createdAtRange, createdAtRangeProps] = defineField("createdAtRange", vuetifyCfg);

// 字段渲染类型补充表：仅覆盖需要特殊渲染的非常用字段，其余缺省 STRING
// 类型依据 internal/models/network 下三个模型（PacketIPv4TCP / PacketIPv6TCP / HttpTransport）的 JSON tag 逐一核对
const EXTRA_TYPES: Partial<Record<string, BackendDataType>> = {
  // 布尔标志：捕获状态与 TCP 扩展标志
  truncated: BackendDataType.BOOLEAN,
  bitEce: BackendDataType.BOOLEAN,
  bitCwr: BackendDataType.BOOLEAN,
  bitNs: BackendDataType.BOOLEAN,
  // 复合结构：JSON 预览
  tags: BackendDataType.JSON,
  options: BackendDataType.JSON,
  queries: BackendDataType.JSON,
  layerStack: BackendDataType.JSON,
  ipv4Options: BackendDataType.JSON,
  ipv6HopByHop: BackendDataType.JSON,
  // 时间戳
  clientStartTimestamp: BackendDataType.DATETIME,
  clientEndTimestamp: BackendDataType.DATETIME,
  serverStartTimestamp: BackendDataType.DATETIME,
  serverEndTimestamp: BackendDataType.DATETIME,
  // 字节数与负载偏移（uint32 / uint64）
  captureLength: BackendDataType.UINT32,
  clientConsumed: BackendDataType.UINT64,
  clientStartPayloadOffset: BackendDataType.UINT64,
  clientEndPayloadOffset: BackendDataType.UINT64,
  serverConsumed: BackendDataType.UINT64,
  serverStartPayloadOffset: BackendDataType.UINT64,
  serverEndPayloadOffset: BackendDataType.UINT64,
  // TCP 数值字段
  dataOffset: BackendDataType.UINT8,
  checksum: BackendDataType.UINT16,
  urgent: BackendDataType.UINT16,
  // IPv4 数值字段
  ipv4Version: BackendDataType.UINT8,
  ipv4Ihl: BackendDataType.UINT8,
  ipv4Tos: BackendDataType.UINT8,
  ipv4Id: BackendDataType.UINT16,
  ipv4Flags: BackendDataType.UINT8,
  ipv4FragOffset: BackendDataType.UINT16,
  ipv4Checksum: BackendDataType.UINT16,
  // IPv6 数值字段
  ipv6Version: BackendDataType.UINT8,
  ipv6TrafficClass: BackendDataType.UINT8,
  ipv6FlowLabel: BackendDataType.UINT32,
  ipv6LastHeader: BackendDataType.UINT8,
  // 链路层数值字段
  ethType: BackendDataType.UINT16,
  ethLength: BackendDataType.UINT16,
  // HTTP 传输数值字段
  clientIsn: BackendDataType.UINT32,
  serverIsn: BackendDataType.UINT32,
  streamIndex: BackendDataType.UINT32,
  ipVersion: BackendDataType.UINT8,
  fpTcpMtu: BackendDataType.UINT16,
  // 指纹置信度（float64，0-1 区间）
  fpOsQuality: BackendDataType.FLOAT64,
  fpTcpClientOsQuality: BackendDataType.FLOAT64,
  fpTcpServerOsQuality: BackendDataType.FLOAT64,
  fpHttpBrowserQuality: BackendDataType.FLOAT64,
  fpHttpServerQuality: BackendDataType.FLOAT64,
};

// 加密与 S3 body 字段清单（随类型切换）：列表单元格与详情页统一占位展示，不直接暴露密文
const cryptoKeys = computed(() => new Set(TrafficCryptoFields[type.value] ?? []));
const s3Keys = computed(() => new Set(TrafficS3Fields[type.value] ?? []));

// 非常用字段清单：列设置中罗列全部模型字段、默认隐藏；常用审计列单独定义于 headers
const IPV4_TCP_EXTRA_KEYS = [
  "truncated",
  "captureLength",
  "layerStack",
  "payload",
  "ethDstMac",
  "ethSrcMac",
  "ethType",
  "ethLength",
  "ipv4Version",
  "ipv4Ihl",
  "ipv4Tos",
  "ipv4Id",
  "ipv4Flags",
  "ipv4FragOffset",
  "ipv4Checksum",
  "ipv4Options",
  "ipv4Padding",
  "dataOffset",
  "bitEce",
  "bitCwr",
  "bitNs",
  "checksum",
  "urgent",
  "options",
  "fpOsQuality",
];

const IPV6_TCP_EXTRA_KEYS = [
  "truncated",
  "captureLength",
  "layerStack",
  "payload",
  "ethDstMac",
  "ethSrcMac",
  "ethType",
  "ethLength",
  "ipv6Version",
  "ipv6TrafficClass",
  "ipv6FlowLabel",
  "ipv6HopByHop",
  "ipv6LastHeader",
  "dataOffset",
  "bitEce",
  "bitCwr",
  "bitNs",
  "checksum",
  "urgent",
  "options",
  "fpOsQuality",
];

const HTTP_EXTRA_KEYS = [
  "clientIsn",
  "serverIsn",
  "streamIndex",
  "ipVersion",
  "url",
  "scheme",
  "fileExt",
  "queries",
  "fragment",
  "reqBody",
  "reqBodyHash",
  "reqHdrAuthorization",
  "reqHdrProxyAuthorization",
  "reqHdrCacheControl",
  "reqHdrIfMatch",
  "reqHdrIfNoneMatch",
  "reqHdrIfModifiedSince",
  "reqHdrIfUnmodifiedSince",
  "reqHdrIfRange",
  "reqHdrConnection",
  "reqHdrKeepAlive",
  "reqHdrAccept",
  "reqHdrAcceptEncoding",
  "reqHdrAcceptLanguage",
  "reqHdrExpect",
  "reqHdrMaxForwards",
  "reqHdrCookie",
  "reqHdrOrigin",
  "reqHdrAccessControlRequestHeaders",
  "reqHdrAccessControlRequestMethod",
  "reqHdrContentType",
  "reqHdrContentLength",
  "reqHdrContentEncoding",
  "reqHdrContentLanguage",
  "reqHdrForwarded",
  "reqHdrVia",
  "reqHdrRange",
  "reqHdrFrom",
  "reqHdrHost",
  "reqHdrReferer",
  "reqHdrReferrerPolicy",
  "reqHdrUserAgent",
  "reqHdrTe",
  "reqHdrTransferEncoding",
  "reqHdrTrailer",
  "reqHdrSecWebsocketKey",
  "reqHdrSecWebsocketVersion",
  "reqHdrSecWebsocketProtocol",
  "reqHdrSecWebsocketExtensions",
  "reqHdrSecFetchSite",
  "reqHdrSecFetchMode",
  "reqHdrSecFetchUser",
  "reqHdrSecFetchDest",
  "reqHdrSecPurpose",
  "reqHdrServiceWorkerNavigationPreload",
  "reqHdrSecChUa",
  "reqHdrSecChUaArch",
  "reqHdrSecChUaBitness",
  "reqHdrSecChUaFullVersion",
  "reqHdrSecChUaFullVersionList",
  "reqHdrSecChUaMobile",
  "reqHdrSecChUaModel",
  "reqHdrSecChUaPlatform",
  "reqHdrSecChUaPlatformVersion",
  "reqHdrSecChUaWow64",
  "reqHdrSecChPrefersColorScheme",
  "reqHdrSecChPrefersReducedMotion",
  "reqHdrSecChPrefersReducedTransparency",
  "reqHdrWantContentDigest",
  "reqHdrWantReprDigest",
  "reqHdrUpgradeInsecureRequests",
  "reqHdrDate",
  "reqHdrLink",
  "reqHdrUpgrade",
  "reqHdrPriority",
  "reqHdrAltUsed",
  "rspBody",
  "rspBodyHash",
  "rspHdrWwwAuthenticate",
  "rspHdrProxyAuthenticate",
  "rspHdrAge",
  "rspHdrCacheControl",
  "rspHdrClearSiteData",
  "rspHdrExpires",
  "rspHdrNoVarySearch",
  "rspHdrLastModified",
  "rspHdrEtag",
  "rspHdrVary",
  "rspHdrConnection",
  "rspHdrKeepAlive",
  "rspHdrAcceptPatch",
  "rspHdrAcceptPost",
  "rspHdrSetCookie",
  "rspHdrAccessControlAllowCredentials",
  "rspHdrAccessControlAllowHeaders",
  "rspHdrAccessControlAllowMethods",
  "rspHdrAccessControlAllowOrigin",
  "rspHdrAccessControlExposeHeaders",
  "rspHdrAccessControlMaxAge",
  "rspHdrTimingAllowOrigin",
  "rspHdrContentDisposition",
  "rspHdrContentDigest",
  "rspHdrReprDigest",
  "rspHdrContentType",
  "rspHdrContentLength",
  "rspHdrContentEncoding",
  "rspHdrContentLanguage",
  "rspHdrContentLocation",
  "rspHdrForwarded",
  "rspHdrVia",
  "rspHdrAcceptRanges",
  "rspHdrContentRange",
  "rspHdrLocation",
  "rspHdrRefresh",
  "rspHdrAllow",
  "rspHdrServer",
  "rspHdrCrossOriginEmbedderPolicy",
  "rspHdrCrossOriginOpenerPolicy",
  "rspHdrCrossOriginResourcePolicy",
  "rspHdrContentSecurityPolicy",
  "rspHdrContentSecurityPolicyReportOnly",
  "rspHdrExpectCt",
  "rspHdrPermissionsPolicy",
  "rspHdrStrictTransportSecurity",
  "rspHdrXContentTypeOptions",
  "rspHdrXFrameOptions",
  "rspHdrXPermittedCrossDomainPolicies",
  "rspHdrXPoweredBy",
  "rspHdrXXssProtection",
  "rspHdrReportingEndpoints",
  "rspHdrReportTo",
  "rspHdrTransferEncoding",
  "rspHdrTrailer",
  "rspHdrSecWebsocketAccept",
  "rspHdrSecWebsocketProtocol",
  "rspHdrSecWebsocketExtensions",
  "rspHdrAcceptCh",
  "rspHdrCriticalCh",
  "rspHdrAttributionReportingEligible",
  "rspHdrAttributionReportingRegisterSource",
  "rspHdrAttributionReportingRegisterTrigger",
  "rspHdrDate",
  "rspHdrLink",
  "rspHdrRetryAfter",
  "rspHdrServerTiming",
  "rspHdrServiceWorker",
  "rspHdrServiceWorkerAllowed",
  "rspHdrSourceMap",
  "rspHdrUpgrade",
  "rspHdrAltSvc",
  "rspHdrPriority",
  "clientConsumed",
  "clientStartPayloadOffset",
  "clientEndPayloadOffset",
  "clientStartTimestamp",
  "clientEndTimestamp",
  "serverConsumed",
  "serverStartPayloadOffset",
  "serverEndPayloadOffset",
  "serverStartTimestamp",
  "serverEndTimestamp",
  "fpTcpClientOsQuality",
  "fpTcpClientSig",
  "fpTcpServerOsQuality",
  "fpTcpServerSig",
  "fpTcpMtu",
  "fpHttpBrowserQuality",
  "fpHttpServerQuality",
  "fpHttpLanguage",
  "tags",
];

// typeKey 路由类型 → 翻译键类型段（ipv4-tcp → ipv4Tcp、http-transport → httpTransport）
const typeKey = computed(() => {
  return type.value.replace(/-(\w)/g, (_, c: string) => c.toUpperCase());
});

// trafficFieldTitle 列标题：按当前流量类型取该类型独立字段翻译（model.<type>.field.<key>）；
// 委托共享工具 modelFieldTitle，行为与统一标签解析保持一致
const trafficFieldTitle = (key: string): string => {
  return modelFieldTitle(typeKey.value, key);
};

// h 便捷构造表头：defaultVisible 控制默认是否展示（非常用字段显式传 false）
const h = (
  key: string,
  type: BackendDataType = BackendDataType.STRING,
  defaultVisible = true,
): TableHeader => {
  return {
    key,
    title: trafficFieldTitle(key),
    type,
    align: "start",
    sortable: false,
    defaultVisible,
  };
};

// 操作列：固定右侧，仅含行操作菜单（查看详情）；fixed 列不参与列设置切换与排序
const actionsHeader: TableHeader = {
  key: "__actions",
  title: i18n.global.t("model.__base.field.__actions"),
  type: BackendDataType.ACTIONS,
  align: "center",
  sortable: false,
  fixed: "end",
};

const headers = computed((): Array<TableHeader> => {
  // 常用审计字段默认展示；其余全量模型字段在列设置菜单中罗列并默认隐藏
  const common: Array<TableHeader> = [
    h("namespace"),
    { ...h("timestamp", BackendDataType.DATETIME), sortable: true },
    h("createdAt", BackendDataType.DATETIME),
    h("sessionId", BackendDataType.UUID),
  ];
  const commonTail: Array<TableHeader> = [h("capture"), h("version")];
  // TCP 常用审计字段：序号、确认号、经典六标志位、窗口大小
  const tcpAudit: Array<TableHeader> = [
    h("seq", BackendDataType.UINT32),
    h("ack", BackendDataType.UINT32),
    h("bitFin", BackendDataType.BOOLEAN),
    h("bitSyn", BackendDataType.BOOLEAN),
    h("bitRst", BackendDataType.BOOLEAN),
    h("bitPsh", BackendDataType.BOOLEAN),
    h("bitAck", BackendDataType.BOOLEAN),
    h("bitUrg", BackendDataType.BOOLEAN),
    h("window", BackendDataType.UINT16),
  ];
  // TCP 被动指纹（p0f 风格）：OS、指纹签名与 MTU
  const tcpFingerprint: Array<TableHeader> = [
    h("fpOs"),
    h("fpSig"),
    h("fpMtu", BackendDataType.UINT16),
  ];
  if (type.value === "http-transport") {
    return [
      ...common,
      h("traceId", BackendDataType.UUID),
      h("method"),
      h("host"),
      h("path"),
      h("clientIp"),
      h("serverIp"),
      h("clientPort", BackendDataType.UINT16),
      h("serverPort", BackendDataType.UINT16),
      h("httpVersion"),
      h("reqBodyLength", BackendDataType.DECIMAL_BYTE),
      h("rspBodyLength", BackendDataType.DECIMAL_BYTE),
      h("fpTcpClientOs"),
      h("fpTcpServerOs"),
      h("fpHttpBrowser"),
      h("fpHttpServer"),
      h("fpDeviceGuess"),
      ...commonTail,
      ...HTTP_EXTRA_KEYS.map((key) => h(key, EXTRA_TYPES[key], false)),
      actionsHeader,
    ];
  }
  const ipv4 = type.value === "ipv4-tcp";
  return [
    ...common,
    h("tcpStreamId", BackendDataType.HEX),
    h(ipv4 ? "ipv4Src" : "ipv6Src"),
    h(ipv4 ? "ipv4Dst" : "ipv6Dst"),
    h("srcPort", BackendDataType.UINT16),
    h("dstPort", BackendDataType.UINT16),
    ...tcpAudit,
    h(ipv4 ? "ipv4Ttl" : "ipv6HopLimit", BackendDataType.UINT8),
    h(ipv4 ? "ipv4IpProtocol" : "ipv6NextHeader", BackendDataType.UINT8),
    h(ipv4 ? "ipv4Length" : "ipv6Length", BackendDataType.UINT16),
    ...tcpFingerprint,
    ...commonTail,
    ...(ipv4 ? IPV4_TCP_EXTRA_KEYS : IPV6_TCP_EXTRA_KEYS).map((key) =>
      h(key, EXTRA_TYPES[key], false),
    ),
    actionsHeader,
  ];
});

// 列显示配置：按流量类型区分持久化到 localStorage，切换类型自动应用各自的保存配置
const { allToggleable, columnOrder, visibleKeys, visibleHeaders } = useColumnVisibility(
  () => `traffic:${type.value}`,
  headers,
);

const currentFrom = (): {
  timestampRange: [DateTime, DateTime];
  createdAtRange: [DateTime, DateTime];
} | null => {
  const tsRange = timestampRange.value;
  const caRange = createdAtRange.value;
  if (
    Array.isArray(tsRange) &&
    tsRange.length === 2 &&
    Array.isArray(caRange) &&
    caRange.length === 2
  ) {
    return { timestampRange: tsRange, createdAtRange: caRange };
  }
  return null;
};

const buildSearchIn = (from: {
  timestampRange: [DateTime, DateTime];
  createdAtRange: [DateTime, DateTime];
}): SearchIn => {
  // 范围精确到分钟（DateRangeField 内部已处理秒级边界），直接按原值发送
  const [tsStart, tsEnd] = from.timestampRange;
  const [caStart, caEnd] = from.createdAtRange;
  const filterBy: Record<string, FilterByItem> = {
    timestampFrom: {
      action: SearchAction.GREATER_EQUAL,
      value: tsStart.toUTC().toISO(),
    },
    timestampTo: {
      action: SearchAction.LESS_EQUAL,
      value: tsEnd.toUTC().toISO(),
    },
    createdAtFrom: {
      action: SearchAction.GREATER_EQUAL,
      value: caStart.toUTC().toISO(),
    },
    createdAtTo: {
      action: SearchAction.LESS_EQUAL,
      value: caEnd.toUTC().toISO(),
    },
    namespace: {
      action: SearchAction.EQUAL,
      value: appStore.namespace,
    },
  };
  // 可选筛选字段：仅注入非空值（精确匹配 eq）；端口等数值字段转 Number 保证后端 uint16 绑定成功
  for (const { key, kind } of OPTIONAL_FILTER_FIELDS[type.value]) {
    const raw = optionalFilters[key];
    if (raw === undefined || raw === null || raw === "") {
      continue;
    }
    filterBy[key] = {
      action: SearchAction.EQUAL,
      value: kind === "number" ? Number(raw) : raw,
    };
  }
  return {
    greedy: false,
    page: page.value,
    pageSize: pageSize.value,
    orderBy: sortBy.value.map((s) => {
      return {
        name: s.key,
        order: s.order === "asc" || s.order === true ? 1 : -1,
      };
    }),
    filterBy,
  };
};

const getListAction = async (from: {
  timestampRange: [DateTime, DateTime];
  createdAtRange: [DateTime, DateTime];
}): Promise<void> => {
  try {
    loading.value = true;
    await getResponse<TrafficListOut>(
      trafficListGet(type.value, buildSearchIn(from)),
      async (rsp) => {
        items.value = rsp.data.items;
        total.value = rsp.data.total;
      },
      async (rsp) => {
        appStore.notify({
          text: rsp.summary,
          timeout: 3000,
          color: "error",
        });
      },
    );
  } finally {
    loading.value = false;
  }
};

const refreshAction = async (): Promise<void> => {
  const from = currentFrom();
  if (from) {
    await getListAction(from);
  }
};

const submitQuery = handleSubmit(async () => {
  page.value = 1;
  const from = currentFrom();
  if (from) {
    await getListAction(from);
  }
});

watch(type, async () => {
  detailDialog.value = false;
  page.value = 1;
  // 高级筛选值按类型独立，切换类型时清空，避免残留脏值参与查询
  for (const key of Object.keys(optionalFilters)) {
    delete optionalFilters[key];
  }
  await refreshAction();
});

// 命名空间由全局按钮统一切换（appStore.namespace）：切换后回到第一页并刷新列表
watch(
  () => appStore.namespace,
  async () => {
    page.value = 1;
    await refreshAction();
  },
);

watch(page, async () => {
  await refreshAction();
});

watch(pageSize, async () => {
  if (page.value !== 1) {
    page.value = 1;
  } else {
    await refreshAction();
  }
});

watch(sortBy, async () => {
  page.value = 1;
  await refreshAction();
});

// openDetail 打开行详情全屏对话框（列表已返回全字段，直接展示行数据）
const openDetail = (item: Record<string, unknown>): void => {
  detailItem.value = item;
  detailDialog.value = true;
};

// favoriteSampleAction 收藏样例：将当前行数据原样收录为标签规则样例。
// 加密字段（base64 密文）与 S3 body 字段（对象键）一并保留，保证样例 JSON 可反序列化回
// 目标模型，规则测试结果不因字段缺失而受影响；名称自动生成避免重复收藏冲突。
const favoriteSampleAction = async (item: Record<string, unknown>): Promise<void> => {
  const modelKey = TrafficModelKeys[type.value];
  const input = { ...item };
  // 列表分页计数（BaseTotalOut）非模型字段，从样例文档中剔除
  delete input.total;
  await getSimpleResponse(
    tagRuleSampleCreate({
      model: modelKey!,
      name: `traffic-${DateTime.now().toFormat("yyyyMMddHHmmssSSS")}`,
      description: "",
      input,
    }),
    async (rsp) => {
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

// 行操作菜单：查看详情 + 收藏样例（仅已注册样例模型的类型显示）；菜单项文案经 action.<name> 翻译
const rowActions = (item: Record<string, unknown>): ItemActions => {
  const actions: ItemActions = {
    viewDetail: () => openDetail(item),
  };
  if (TrafficModelKeys[type.value]) {
    actions.favoriteSample = () => void favoriteSampleAction(item);
  }
  // http-transport 携带 traceId 时提供网络堆栈下钻入口（前端按 payload offset 高亮请求/响应包）
  if (type.value === "http-transport" && item.traceId) {
    actions.viewStack = () => openStack(item);
  }
  return actions;
};

const onClickRow = (
  _event: Event,
  // Table row values are heterogeneous.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  data: { item: Record<string, any> },
): void => {
  openDetail(data.item);
};

const formatValue = (value: unknown): string => {
  if (value === null || value === undefined) {
    return "";
  }
  if (typeof value === "object") {
    return JSON.stringify(value);
  }
  return String(value);
};

// detailFields 行详情字段列表：列表已返回全字段，直接由行数据渲染
// crypto/s3 字段清单复用顶层 cryptoKeys / s3Keys（与列表单元格展示一致）
const detailFields = computed((): DetailField[] => {
  const typeName = type.value;
  const item = detailItem.value;
  if (!typeName || !item) {
    return [];
  }
  const metaKeys = [
    "namespace",
    "timestamp",
    "createdAt",
    "sessionId",
    "tcpStreamId",
    "capture",
    "version",
    "traceId",
  ];
  const entries = Object.entries(item).filter(([, value]) => {
    return value !== null && value !== undefined && value !== "";
  });
  const meta = metaKeys
    .map((key) => {
      return [key, item[key]] as [string, unknown];
    })
    .filter(([, value]) => {
      return value !== null && value !== undefined && value !== "";
    });
  const rest = entries.filter(([key]) => {
    return !metaKeys.includes(key);
  });
  return [...meta, ...rest].map(([key, value]) => {
    // 字段标签统一走 trafficFieldTitle：按类型取模型字段翻译，避免界面露出原始 key
    const label = trafficFieldTitle(key);
    if (cryptoKeys.value.has(key)) {
      return { key, label, display: "", kind: "crypto" as const, length: 0, value };
    }
    if (s3Keys.value.has(key)) {
      return {
        key,
        label,
        display: String(value),
        kind: "s3" as const,
        length: Number(item[`${key}_length`] ?? 0),
        value,
      };
    }
    let display = formatValue(value);
    if (DATETIME_FIELD_KEYS.has(key) && typeof value === "string") {
      const dt = DateTime.fromISO(value);
      if (dt.isValid) {
        display = localeDatetimeFormat(dt);
      }
    }
    return { key, label, display, kind: "plain" as const, length: 0, value };
  });
});

// viewPlaintextCrypto 列表单元格直接解密加密字段：构造 DetailField 复用明文对话框解密流程
const viewPlaintextCrypto = async (item: Record<string, unknown>, key: string): Promise<void> => {
  const ciphertext = item[key];
  if (typeof ciphertext !== "string" || ciphertext === "") {
    return;
  }
  const field: DetailField = {
    key,
    label: trafficFieldTitle(key),
    display: "",
    kind: "crypto",
    length: 0,
    value: ciphertext,
  };
  await viewPlaintextAction(field);
};

// viewPlaintextS3 列表单元格直接解密 S3 body 字段：按对象键从存储读取明文，复用明文对话框流程
const viewPlaintextS3 = async (item: Record<string, unknown>, key: string): Promise<void> => {
  const objectKey = item[key];
  if (typeof objectKey !== "string" || objectKey === "") {
    return;
  }
  const field: DetailField = {
    key,
    label: trafficFieldTitle(key),
    display: objectKey,
    kind: "s3",
    length: Number(item[`${key}Length`] ?? 0),
    value: objectKey,
  };
  await viewPlaintextAction(field);
};

// viewPlaintextAction 统一入口：构造 DetailField 后交共享明文对话框解密展示，
// loading 反馈给调用方（列表单元格/详情行按钮）以标记正在加载的字段
const viewPlaintextAction = async (field: DetailField): Promise<void> => {
  plaintextLoading.value = field.key;
  try {
    await plaintextDialogRef.value?.open(field, type.value, field.label);
  } finally {
    plaintextLoading.value = "";
  }
};

onMounted(refreshAction);
</script>

<style scoped>
/* Collapse the empty message slot of filter inputs so the action buttons
   align with the input control line; validation errors still expand it.
   `.v-input__details` is a Vuetify BEM element class (double underscore),
   exempted from the kebab-case class pattern rule. */
/* stylelint-disable-next-line selector-class-pattern */
.filter-form :deep(.v-input__details:has(.v-messages:empty)) {
  min-height: 0;
  padding-top: 0;
}

.filter-form :deep(.v-messages:empty) {
  min-height: 0;
}
</style>
