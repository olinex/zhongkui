<template>
  <main-layout current-nav="traffic">
    <template #body>
      <!-- 复用全项目统一的 SearchTable：显式查询按钮 + 高级条件编辑器常驻；
             抓包/入库时间窗口经 extra-filters 插槽置于筛选区，随查询载荷统一注入（buildFilter） -->
      <search-table
        ref="searchTableRef"
        :title="$t(`model.${typeKey}.name`)"
        :list-request="trafficListRequest"
        :filter-headers="searchHeaders"
        :headers="headers"
        :query="submitQuery"
        :build-filter="buildFilter"
        :default-order-by="defaultOrderBy"
        :row-click="openDetail"
        :project-fields="projectFields"
        :storage-key="`traffic:${type}`"
      >
        <!-- 额外筛选区：抓包时间（timestamp）与入库时间（createdAt）两个必填时间范围，
               不参与高级条件编辑器（由 buildFilter 强制注入） -->
        <template #extra-filters>
          <div class="extra-filters zk-filter-form d-flex flex-wrap align-center ga-2 mt-2">
            <date-time-range-field
              v-model="timestampRange"
              v-bind="timestampRangeProps"
              :label="$t('model.__base.field.timestampRange')"
            />
            <date-time-range-field
              v-model="createdAtRange"
              v-bind="createdAtRangeProps"
              :label="$t('model.__base.field.createdAtRange')"
            />
          </div>
        </template>
        <!-- 通用单元格接管：加密/S3 字段展示占位与明文入口，其余交 AdaptiveCell 按类型渲染 -->
        <template #cell="{ item, header }">
          <encrypted-value-cell
            v-if="cryptoKeys.has(header.key) || s3Keys.has(header.key)"
            :kind="cryptoKeys.has(header.key) ? 'crypto' : 's3'"
            :item="item"
            :field-key="header.key"
            :loading="plaintextLoading === header.key"
            @view-plaintext="onViewPlaintext"
          />
          <adaptive-cell v-else :header="header" :item="item" :actions="rowActions" />
        </template>
      </search-table>

      <!-- 行详情：列表行仅含投影字段，经 detail 接口拉取全字段后展示；
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

      <!-- 网络堆栈对话框：以选中传输的 traceID 展示其所在 TCP 流的网络堆栈，
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
import type { AxiosPromise } from "axios";
import { DateTime } from "luxon";
import { useForm } from "vee-validate";
import type { SortItem } from "vuetify/lib/components/VDataTable/composables/sort.mjs";

import { getResponse, getSimpleResponse } from "@/apis/base";
import { tagRuleSampleCreate } from "@/apis/tagEvent";
import {
  TrafficCryptoFields,
  TrafficExtraFieldTypes,
  TrafficModelKeys,
  TrafficS3Fields,
  trafficDetailGet,
  trafficListGet,
  trafficListOptions,
} from "@/apis/traffic";
import type { TrafficType } from "@/apis/@types/traffic";
import type { DetailField } from "@/components/@types/detail";
import type {
  FilterByItem,
  FilterByOption,
  ItemActions,
  SearchIn,
  TableHeader,
} from "@/components/@types/table";
import { trafficFilterSchema } from "@/schemas/traffic";
import { useAppStore } from "@/stores/app";
import { BackendDataType, SearchAction } from "@/types";
import { vuetifyCfg } from "@/utils/form";
import { isEmptyCiphertext } from "@/utils/encrypted";
import i18n from "@/utils/i18n";
import { modelFieldTitle } from "@/utils/modelField";
import { enumFieldTypeOfMeta, enumOptionsOf, ensureEnum } from "@/utils/enums";
import type { MetaOut } from "@/components/@types/meta";

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

const appStore = useAppStore();
const searchTableRef = useTemplateRef("searchTableRef");

// 默认排序：按抓包时间倒序，最新记录置顶
const defaultOrderBy: SortItem[] = [{ key: "timestamp", order: "desc" }];

// projectFields 字段投影：列表仅返回可见列 + 行操作定位字段（id/namespace，http-transport 另含 traceID），
// 行详情/收藏样例/堆栈下钻经 detail 接口拉取全字段
const projectFields = (visible: string[]): string[] => {
  return ["id", "namespace", ...(type.value === "http-transport" ? ["traceID"] : []), ...visible];
};

const detailDialog = ref(false);
const detailLoading = ref(false);
const detailItem = ref<Record<string, unknown> | null>(null);

// 网络堆栈对话框：选中 http-transport 行（携带 traceID 与 AppTailer offset）后打开
const stackDialog = ref(false);
const stackTransport = ref<Record<string, unknown> | null>(null);
// openStack 打开网络堆栈对话框：经 detail 接口拉取全字段（含 AppTailer payload offset）后下钻
const openStack = async (item: Record<string, unknown>): Promise<void> => {
  await getSimpleResponse(
    trafficDetailGet(type.value, {
      namespace: String(item.namespace ?? ""),
      id: String(item.id ?? ""),
    }),
    async (rsp) => {
      stackTransport.value = rsp.data ?? null;
      stackDialog.value = true;
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

const plaintextDialogRef = useTemplateRef("plaintextDialogRef");
const plaintextLoading = ref("");

// searchHeaders 高级筛选可选字段元数据（由后端 OPTIONS 返回，已过滤时间窗口与命名空间），
// 经 filter-headers 传入 SearchTable 供高级条件编辑器（ExpressionField）动态渲染
const searchHeaders = ref<Record<string, FilterByOption>>({});

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
  "ipv4ID",
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
  sortable: false,
  fixed: "end",
};

const headers = computed((): Array<TableHeader> => {
  // 常用审计字段默认展示；其余全量模型字段在列设置菜单中罗列并默认隐藏
  const common: Array<TableHeader> = [
    h("namespace"),
    { ...h("timestamp", BackendDataType.DATETIME), sortable: true },
    h("createdAt", BackendDataType.DATETIME),
    h("sessionID", BackendDataType.UUID),
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
      h("traceID", BackendDataType.UUID),
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
      ...HTTP_EXTRA_KEYS.map((key) => h(key, TrafficExtraFieldTypes[key], false)),
      actionsHeader,
    ];
  }
  const ipv4 = type.value === "ipv4-tcp";
  return [
    ...common,
    h("tcpStreamID", BackendDataType.HEX),
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
      h(key, TrafficExtraFieldTypes[key], false),
    ),
    actionsHeader,
  ];
});

// currentFrom 当前两个时间范围二元组；任一时间范围不完整（未选/被清除）时返回 null
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

// buildFilter 查询载荷过滤器：在高级条件基础上强制注入时间窗口与命名空间；
// 任一时间范围缺失时返回 null，SearchTable 跳过本次查询（如翻页/切换类型时兜底）
const buildFilter = (base: Record<string, FilterByItem>): Record<string, FilterByItem> | null => {
  const from = currentFrom();
  if (!from) {
    return null;
  }
  // 范围精确到分钟（DateTimeRangeField 内部已处理秒级边界），直接按原值发送
  const [tsStart, tsEnd] = from.timestampRange;
  const [caStart, caEnd] = from.createdAtRange;
  return {
    ...base,
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
};

// trafficListRequest 列表请求：当前类型 + SearchTable 统一构建的查询载荷
const trafficListRequest = (req: SearchIn): AxiosPromise => trafficListGet(type.value, req);

// submitQuery 显式查询：经 vee-validate 校验两个必填时间范围后，交由 SearchTable 重置页码并刷新
const submitQuery = async (): Promise<void> => {
  await handleSubmit(async () => {
    await searchTableRef.value?.query();
  })();
};

// 排除字段：时间窗口与命名空间由顶部必填时间范围与强制注入处理，不暴露在条件编辑器中
const EXCLUDED_FILTER_FIELDS = new Set([
  "timestampFrom",
  "timestampTo",
  "createdAtFrom",
  "createdAtTo",
  "namespace",
]);

// normalizeFilterType 后端 Go 类型名 → 前端 BackendDataType；
// 无对应枚举的 serializer 类型（UUID / IP 地址）以字符串形式渲染输入
const normalizeFilterType = (goType: string): BackendDataType => {
  switch (goType) {
    case "serializer.HexUUID":
    case "serializer.IPv4Addr":
    case "serializer.IPv6Addr":
      return BackendDataType.STRING;
    default:
      return goType as BackendDataType;
  }
};

// loadSearchOptions 拉取当前类型可筛选字段元数据，构造高级筛选条件编辑器的字段/动作选项
const loadSearchOptions = async (): Promise<void> => {
  await getResponse<MetaOut>(
    trafficListOptions(type.value),
    async (rsp) => {
      // 统一枚举接口按类名获取：先遍历 filterBy 收集命中枚举类的字段（enumFieldTypeOfMeta
      // 自省 bodyOutput 的 enumType），全部 ensureEnum 确保选项按当前语言缓存，
      // enumOptionsOf 才能立即返回完整下拉选项（不 await 会导致首屏选项为空）
      const enumTypes = new Set<string>();
      for (const item of rsp.data.filterBy) {
        if (EXCLUDED_FILTER_FIELDS.has(item.name)) {
          continue;
        }
        const enumType = enumFieldTypeOfMeta(rsp.data.meta, item.name);
        if (enumType) {
          enumTypes.add(enumType);
        }
      }
      await Promise.all([...enumTypes].map((t) => ensureEnum(t)));
      searchHeaders.value = rsp.data.filterBy
        .filter((item) => !EXCLUDED_FILTER_FIELDS.has(item.name))
        .reduce(
          (prev, cur) => {
            // 枚举筛选项经统一枚举注册表按类名获取：命中则为枚举下拉，未命中
            // （非枚举字段）保持后端字段类型（如字符串文本框）
            const enumType = enumFieldTypeOfMeta(rsp.data.meta, cur.name);
            prev[cur.name] = {
              name: cur.name,
              title: trafficFieldTitle(cur.name),
              actions: cur.actions,
              type: enumType ? BackendDataType.ENUM : normalizeFilterType(cur.type),
              options: enumType ? enumOptionsOf(enumType) : cur.options,
              enumType,
            };
            return prev;
          },
          {} as Record<string, FilterByOption>,
        );
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

watch(type, async () => {
  detailDialog.value = false;
  stackDialog.value = false;
  // 时间筛选默认值随类型切换刷新：两处必填时间范围重置为"近一天"计算出的最新区间，
  // 避免沿用上一模型固定的起止时刻（数据源不同，历史默认区间无意义）
  timestampRange.value = initialRange();
  createdAtRange.value = initialRange();
  // 高级筛选条件按类型独立：切换类型时清空 SearchTable 内部条件并重载字段元数据，避免残留脏值参与查询
  if (searchTableRef.value) {
    searchTableRef.value.filterBy = [];
  }
  // 立即清空旧数据并进入加载态：表头已随类型即时更新，旧类型数据驻留会错位/空值展示，
  // 加载期间仅显示骨架屏，待字段元数据与真实数据就绪后一次刷新到位
  searchTableRef.value?.startLoading();
  await loadSearchOptions();
  await searchTableRef.value?.query();
});

// 命名空间由全局按钮统一切换（appStore.namespace）：切换后回到第一页并刷新列表
watch(
  () => appStore.namespace,
  async () => {
    await searchTableRef.value?.query();
  },
);

// openDetail 打开行详情全屏对话框：列表行仅含投影字段，经 detail 接口拉取全字段
const openDetail = async (item: Record<string, unknown>): Promise<void> => {
  if (detailLoading.value) {
    return;
  }
  detailLoading.value = true;
  try {
    await getResponse<Record<string, unknown>>(
      trafficDetailGet(type.value, {
        namespace: String(item.namespace ?? ""),
        id: String(item.id ?? ""),
      }),
      async (rsp) => {
        detailItem.value = rsp.data;
        detailDialog.value = true;
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
    detailLoading.value = false;
  }
};

// favoriteSampleAction 收藏样例：列表行仅含投影字段，先经 detail 接口拉取全字段再收录为样例。
// 加密字段（base64 密文）与 S3 body 字段（对象键）一并保留，保证样例 JSON 可反序列化回
// 目标模型，规则测试结果不因字段缺失而受影响；名称自动生成避免重复收藏冲突。
const favoriteSampleAction = async (item: Record<string, unknown>): Promise<void> => {
  const modelKey = TrafficModelKeys[type.value];
  if (!modelKey) {
    return;
  }
  await getResponse<Record<string, unknown>>(
    trafficDetailGet(type.value, {
      namespace: String(item.namespace ?? ""),
      id: String(item.id ?? ""),
    }),
    async (rsp) => {
      const input = { ...rsp.data };
      // 行定位字段（id）与分页计数（total）非模型字段，从样例文档中剔除
      delete input.id;
      delete input.total;
      // 样例归属当前请求命名空间：将输入文档的 namespace 归一为当前命名空间，
      // 保证样例与命名空间自洽（流量记录经命名空间过滤查询，通常一致，此处兜底）
      input.namespace = appStore.namespace;
      await getSimpleResponse(
        tagRuleSampleCreate({
          model: modelKey,
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
  // http-transport 携带 traceID 时提供网络堆栈下钻入口（前端按 payload offset 高亮请求/响应包）
  if (type.value === "http-transport" && item.traceID) {
    actions.viewStack = () => openStack(item);
  }
  return actions;
};

// detailFieldTypes 详情字段渲染类型：由 headers 表头定义推导（含通用/特有/非常用字段），
// 未知字段回退 STRING；plain 字段经 AdaptiveCell 按列表页方式渲染
const detailFieldTypes = computed(() => {
  const map: Record<string, BackendDataType> = {};
  for (const hdr of headers.value) {
    map[hdr.key] = hdr.type;
  }
  return map;
});

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
    "sessionID",
    "tcpStreamID",
    "capture",
    "version",
    "traceID",
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
    const fieldType = detailFieldTypes.value[key] ?? BackendDataType.STRING;
    if (cryptoKeys.value.has(key)) {
      return {
        key,
        label,
        display: "",
        kind: "crypto" as const,
        length: 0,
        value,
        type: fieldType,
      };
    }
    if (s3Keys.value.has(key)) {
      return {
        key,
        label,
        display: String(value),
        kind: "s3" as const,
        length: Number(item[`${key}_length`] ?? 0),
        value,
        type: fieldType,
      };
    }
    return { key, label, display: "", kind: "plain" as const, length: 0, value, type: fieldType };
  });
});

// viewPlaintextCrypto 列表单元格直接解密加密字段：构造 DetailField 复用明文对话框解密流程
const viewPlaintextCrypto = async (item: Record<string, unknown>, key: string): Promise<void> => {
  const ciphertext = item[key];
  if (isEmptyCiphertext(ciphertext)) {
    return;
  }
  const field: DetailField = {
    key,
    label: trafficFieldTitle(key),
    display: "",
    kind: "crypto",
    length: 0,
    value: ciphertext,
    type: BackendDataType.STRING,
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
    type: BackendDataType.STRING,
  };
  await viewPlaintextAction(field);
};

// onViewPlaintext EncryptedValueCell 统一派发入口：按 key 所属集合分发到 crypto/s3 查看流程
const onViewPlaintext = (item: Record<string, unknown>, key: string): void => {
  if (cryptoKeys.value.has(key)) {
    void viewPlaintextCrypto(item, key);
  } else if (s3Keys.value.has(key)) {
    void viewPlaintextS3(item, key);
  }
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

onMounted(async () => {
  // 拉取当前类型可筛选字段元数据；列表初查由 SearchTable onMounted 完成
  await loadSearchOptions();
});
</script>
