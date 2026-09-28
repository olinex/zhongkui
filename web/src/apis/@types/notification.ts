// 第三方
import type { SearchIn } from "@/components/@types/table";

// ---- 通知渠道类型 ----
// NotificationChannelTypeField 渠道类型配置字段描述（channel/target 通用，对应后端 FieldMeta）
// 字段展示文案由前端依据字段名与渠道类型拼接 i18n key 自行翻译，后端不返回。
export interface NotificationChannelTypeField {
  name: string;
  kind: string;
  type?: string;
  binding?: string;
  required: boolean;
  defaultValue?: unknown;
}

// NotificationChannelTypeConfigSchema 渠道类型配置结构（发送方 channel 与接收方 target 两段）
export interface NotificationChannelTypeConfigSchema {
  channel: NotificationChannelTypeField[];
  target: NotificationChannelTypeField[];
}

// NotificationChannelTypeItem 渠道类型条目
export interface NotificationChannelTypeItem {
  type: string;
  name: string;
  // placeholder 是否为占位符渠道（如 webhook）：渠道级配置无字段，系统固定数据，
  // 前端应隐藏其创建入口且不可删除
  placeholder: boolean;
  config: NotificationChannelTypeConfigSchema;
}

// NotificationChannelTypesOut 渠道类型列表出参
export interface NotificationChannelTypesOut {
  types: NotificationChannelTypeItem[];
}

// ---- 通知渠道 ----
// NotificationChannelItem 通知渠道项
export interface NotificationChannelItem {
  id: number;
  createdAt: string;
  updatedAt: string;
  name: string;
  type: string;
  enabled: boolean;
  config: string;
  description: string;
}

// NotificationChannelListOut 通知渠道列表出参
export interface NotificationChannelListOut {
  items: NotificationChannelItem[];
  total: number;
}

// NotificationChannelCreateIn 创建通知渠道入参
export interface NotificationChannelCreateIn {
  name: string;
  type: string;
  enabled: boolean;
  config: string;
  description?: string;
}

// NotificationChannelUpdateIn 更新通知渠道入参
export interface NotificationChannelUpdateIn {
  enabled: boolean;
  config: string;
  description?: string;
}

// NotificationChannelBatchIn 通知渠道批量操作入参（删除/启停）
export interface NotificationChannelBatchIn {
  ids: number[];
}

// NotificationChannelSearchIn 通知渠道搜索入参
// 继承通用 SearchIn（page/pageSize/orderBy/filterBy 等），附加特有过滤字段
// 过滤字段命名与后端 search.go 的 NotificationChannelListGetFilterBy 一致
export interface NotificationChannelSearchIn extends SearchIn {
  search?: string;
  name?: string;
  type?: string;
  enabled?: boolean;
  createdAt?: string;
  updatedAt?: string;
}

// ---- 通知目标 ----
// NotificationTargetItem 通知目标项
export interface NotificationTargetItem {
  id: number;
  createdAt: string;
  updatedAt: string;
  name: string;
  channelId: number;
  channelName: string;
  channelType: string;
  channelEnabled: boolean;
  type: string;
  enabled: boolean;
  config: string;
  language: string;
  description: string;
}

// NotificationTargetListOut 通知目标列表出参
export interface NotificationTargetListOut {
  items: NotificationTargetItem[];
  total: number;
}

// NotificationTargetCreateIn 创建通知目标入参
export interface NotificationTargetCreateIn {
  name: string;
  channelId: number;
  enabled: boolean;
  config: string;
  language?: string;
  description?: string;
}

// NotificationTargetUpdateIn 更新通知目标入参
export interface NotificationTargetUpdateIn {
  enabled: boolean;
  config: string;
  language?: string;
  description?: string;
}

// NotificationTargetBatchIn 通知目标批量操作入参（删除/启停）
export interface NotificationTargetBatchIn {
  ids: number[];
}

// NotificationTargetTestSendChannelOut 通知目标 test-send 返回的渠道参数
export interface NotificationTargetTestSendChannelOut {
  name: string;
  type: string;
  enabled: boolean;
  // config 为 base64 密文，可经 plaintext 接口解密核对（所见即所发）
  config: string;
}

// NotificationTargetTestSendTargetOut 通知目标 test-send 返回的目标参数
export interface NotificationTargetTestSendTargetOut {
  name: string;
  language: string;
  enabled: boolean;
  // config 为 base64 密文，可经 plaintext 接口解密核对（所见即所发）
  config: string;
}

// NotificationTargetTestSendOut 通知目标测试发送出参
export interface NotificationTargetTestSendOut {
  messageId: string;
  title: string;
  content: string;
  language: string;
  channel: NotificationTargetTestSendChannelOut;
  target: NotificationTargetTestSendTargetOut;
}

// NotificationTargetSearchIn 通知目标搜索入参
// 过滤字段命名与后端 search.go 的 NotificationTargetListGetFilterBy 一致
export interface NotificationTargetSearchIn extends SearchIn {
  search?: string;
  name?: string;
  channelId?: number;
  type?: string;
  enabled?: boolean;
  language?: string;
  createdAt?: string;
  updatedAt?: string;
}

// ---- 通知分组 ----
// NotificationGroupItem 通知分组项
export interface NotificationGroupItem {
  id: number;
  createdAt: string;
  updatedAt: string;
  name: string;
  isDefault: boolean;
  description: string;
  targetCount: number;
}

// NotificationGroupListOut 通知分组列表出参
export interface NotificationGroupListOut {
  items: NotificationGroupItem[];
  total: number;
}

// NotificationGroupCreateIn 创建通知分组入参
export interface NotificationGroupCreateIn {
  name: string;
  isDefault: boolean;
  description?: string;
  // targetIds 创建时随组绑定的通知目标 ID 列表（可选）
  targetIds?: number[];
}

// NotificationGroupUpdateIn 更新通知分组入参
export interface NotificationGroupUpdateIn {
  description?: string;
  // targetIds 目标 ID 列表（可选）：提供时整集替换绑定目标，空数组解绑全部；缺省保持既有绑定不变
  targetIds?: number[];
}

// NotificationGroupBatchIn 通知分组批量操作入参（删除）
export interface NotificationGroupBatchIn {
  ids: number[];
}

// NotificationGroupTargetItem 通知分组关联的目标项
export interface NotificationGroupTargetItem {
  id: number;
  name: string;
  type: string;
  channelId: number;
  channelName: string;
  channelType: string;
  enabled: boolean;
  language: string;
}

// NotificationGroupDetailOut 通知分组详情出参
export interface NotificationGroupDetailOut {
  id: number;
  createdAt: string;
  updatedAt: string;
  name: string;
  isDefault: boolean;
  description: string;
  targets: NotificationGroupTargetItem[];
}

// NotificationGroupSearchIn 通知分组搜索入参
// 过滤字段命名与后端 search.go 的 NotificationGroupListGetFilterBy 一致
export interface NotificationGroupSearchIn extends SearchIn {
  search?: string;
  name?: string;
  isDefault?: boolean;
  createdAt?: string;
  updatedAt?: string;
}

// ---- 通知记录 ----
// NotificationRecordItem 通知记录项
export interface NotificationRecordItem {
  id: number;
  createdAt: string;
  updatedAt: string;
  eventType: string;
  eventId: number;
  targetId: number;
  targetName: string;
  channelType: string;
  status: string;
  messageId: string;
  notifiedAt: string | null;
}

// NotificationRecordListOut 通知记录列表出参
export interface NotificationRecordListOut {
  items: NotificationRecordItem[];
  total: number;
}

// NotificationRecordDetailOut 通知记录详情出参
export interface NotificationRecordDetailOut {
  id: number;
  createdAt: string;
  updatedAt: string;
  eventType: string;
  eventId: number;
  groupId: number;
  targetId: number;
  targetName: string;
  channelId: number;
  channelName: string;
  channelType: string;
  status: string;
  error: string;
  messageId: string;
  payload: string;
  notifiedAt: string | null;
}

// NotificationRecordSearchIn 通知记录搜索入参
// 过滤字段命名与后端 search.go 的 NotificationRecordListGetFilterBy 一致
export interface NotificationRecordSearchIn extends SearchIn {
  search?: string;
  status?: string;
  eventType?: string;
  channelType?: string;
  createdAt?: string;
  updatedAt?: string;
}

// ---- 通知模板 ----
// NotificationTemplateEventMetaItemOut 单条事件元数据（events/meta 接口出参项）：
// 仅编辑器语言提示与变量补全所需，不承载枚举值集合（事件枚举值经统一枚举接口 notificationEvent 类获取）
export interface NotificationTemplateEventMetaItemOut {
  event: string;
  languages: string[];
  fields: string[];
}

// NotificationTemplateEventMetaOut 事件元数据出参
export interface NotificationTemplateEventMetaOut {
  events: NotificationTemplateEventMetaItemOut[];
}

// NotificationTemplateItem 通知模板列表项
export interface NotificationTemplateItem {
  id: number;
  namespaceName: string;
  event: string;
  channelType: string;
  language: string;
  title: string;
  content: string;
  // source 模板来源：default | override
  source: string;
  description: string;
}

// NotificationTemplateListOut 通知模板列表出参
export interface NotificationTemplateListOut {
  total: number;
  items: NotificationTemplateItem[];
}

// NotificationTemplateCreateIn 创建通知模板入参（创建用户覆盖模板，source=override）
export interface NotificationTemplateCreateIn {
  event: string;
  channelType: string;
  language: string;
  title: string;
  content: string;
  description?: string;
}

// NotificationTemplateUpdateIn 更新通知模板入参（仅标题/正文/描述可改）
export interface NotificationTemplateUpdateIn {
  id: number;
  title: string;
  content: string;
  description?: string;
}

// NotificationTemplateRestoreDefaultIn 恢复事件模板为内置默认的入参
export interface NotificationTemplateRestoreDefaultIn {
  event: string;
  language: string;
}

// NotificationTemplateValidateIn 模板语法校验入参（校验编辑器内尚未保存的标题/正文）
export interface NotificationTemplateValidateIn {
  title: string;
  content: string;
}

// NotificationTemplateValidateOut 模板语法校验出参
// 标题/正文各自返回是否合法与错误信息，校验失败不视为 HTTP 错误
export interface NotificationTemplateValidateOut {
  titleValid: boolean;
  titleError: string;
  contentValid: boolean;
  contentError: string;
}

// NotificationTemplateDryRunIn 模板试运行入参
// 使用事件的内置样例参数渲染编辑器内尚未保存的标题/正文
export interface NotificationTemplateDryRunIn {
  event: string;
  channelType?: string;
  language?: string;
  title: string;
  content: string;
}

// NotificationTemplateDryRunOut 模板试运行出参
export interface NotificationTemplateDryRunOut {
  event: string;
  channelType: string;
  language: string;
  title: string;
  content: string;
}

// NotificationTemplateRenderTestIn 模板渲染预览入参
// params 为供 Go text/template 渲染的参数字典（map[string]any）
export interface NotificationTemplateRenderTestIn {
  event: string;
  channelType?: string;
  language?: string;
  params?: Record<string, unknown>;
}

// NotificationTemplateRenderTestOut 模板渲染预览出参
export interface NotificationTemplateRenderTestOut {
  event: string;
  channelType: string;
  language: string;
  title: string;
  content: string;
}

// BatchResultOut 批量操作结果（复用 admin 中的既有定义）
export type { BatchResultOut } from "./admin";
