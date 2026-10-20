import { useEnumsStore } from "@/stores/enums";
import i18n from "@/utils/i18n";

// 类型
import type { EnumEntryOut } from "@/apis/enums";
import type { ApiMeta } from "@/components/@types/meta";

// EnumClass 枚举类名常量（与后端 enum 注册表类名一一对应，消费点统一引用避免手写字面量）
export const EnumClass = {
  channelType: "channelType",
  notificationEvent: "notificationEvent",
  notificationRecordStatus: "notificationRecordStatus",
  tagAlertStatus: "tagAlertStatus",
  datasourceType: "datasourceType",
  datasourceStruct: "datasourceStruct",
  tagRuleModel: "tagRuleModel",
  tagEventDimension: "tagEventDimension",
  policyEffect: "policyEffect",
  notificationSource: "notificationSource",
  language: "language",
  httpMethod: "httpMethod",
  httpVersion: "httpVersion",
  llmModelKind: "llmModelKind",
  llmSlot: "llmSlot",
  mcpTransport: "mcpTransport",
  llmToolSource: "llmToolSource",
  evalStage: "evalStage",
  evalStageStatus: "evalStageStatus",
  diagnosisCategory: "diagnosisCategory",
  chunkUnitKind: "chunkUnitKind",
  // 知识库文档来源格式（归属 internal/pkg/docconv）与处理状态（归属 internal/models/ai）
  documentFormat: "documentFormat",
  knowledgeDocStatus: "knowledgeDocStatus",
  agentKind: "agentKind",
  memoryKind: "memoryKind",
  memoryStatus: "memoryStatus",
  // 客户端工具调用的审核重要级（归属 internal/ai）与会话级审核阀门（归属 internal/models/ai）
  auditLevel: "auditLevel",
  reviewValve: "reviewValve",
  // 会话交互主状态 / 子状态（归属 internal/models/ai）：状态唯一权威来源，label 由后端枚举注册表下发
  agentInteractionState: "agentInteractionState",
  agentInteractionSubstate: "agentInteractionSubstate",
} as const;

// currentLang 当前语言（i18n locale value，如 zh-Hans / en）
const currentLang = (): string => String(i18n.global.locale.value);

// enumFieldTypeOfMeta 从列表接口自省响应（MetaOut）的 bodyOutput 中取字段的枚举类名（enumType）；
// 未注册枚举或字段不存在时返回 undefined，前端据此回退为普通文本筛选项。
export const enumFieldTypeOfMeta = (meta: ApiMeta | undefined, field: string): string | undefined =>
  meta?.POST?.bodyOutput?.fields.find((f) => f.name === field)?.enumType;

// ensureEnum 确保某枚举类已按当前语言缓存（页面挂载/选项构造前调用；store 内部在途去重）
export const ensureEnum = (type: string): Promise<void> =>
  useEnumsStore().ensure(type, currentLang());

// enumEntriesOf 取某枚举类当前语言条目；未缓存返回 undefined
export const enumEntriesOf = (type: string): EnumEntryOut[] | undefined =>
  useEnumsStore().entries(type, currentLang());

// enumLabelOf 按 value 或 key 解析 label（value 优先匹配，再 key）；未命中回退原始值字符串。
// 响应式：读取的是 store 响应式缓存，缓存到达后引用处自动刷新。
export const enumLabelOf = (type: string, value: string | number | null | undefined): string =>
  useEnumsStore().labelOf(type, value, currentLang());

// enumOptionsOf 构造 v-select 下拉选项（value=枚举存储值，title=label）
export const enumOptionsOf = (type: string): Array<{ title: string; value: string }> =>
  (useEnumsStore().entries(type, currentLang()) ?? []).map((e) => ({
    title: e.label,
    value: e.value,
  }));
