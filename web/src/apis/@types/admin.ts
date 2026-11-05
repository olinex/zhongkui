import type { SearchIn } from "@/components/@types/table";

export interface UserGroupIn {
  namespace: string;
  groupName: string;
}

export interface UserCreateIn {
  username: string;
  password: string;
  nickleName: string;
  email: string;
}

export interface UserPasswordResetIn {
  password?: string;
}

export interface UserPasswordResetOut {
  password: string;
}

export interface GroupRbacPolicyIn {
  target: string;
  action: string;
  effect: string;
}

export interface GroupUpdateIn {
  description: string;
}

export interface GroupCreateIn extends GroupUpdateIn {
  name: string;
}

export interface NamespaceUpdateIn {
  description: string;
}

export interface NamespaceCreateIn extends NamespaceUpdateIn {
  name: string;
}

export interface CapturerConfigIn {
  device: string;
  snapLen: number;
  promisc: boolean;
  timeout: number;
  bpfExpr: string;
  enabled: boolean;
}

export interface CapturerCreateIn {
  namespace: string;
  name: string;
  version: string;
  config: CapturerConfigIn;
}

// CapturerStatusItemOut 单个采集器资源运行状态项。
export interface CapturerStatusItemOut {
  namespace: string;
  name: string;
  version: string;
  sessionID: string;
  // 运行状态短键（running / stopped），经 model.capturer.option.status.* 渲染标签。
  status: string;
  configVersion: number;
  globalVersion: number;
  lastSeen: string;
}

// CapturerStatusListOut 采集器运行状态列表出参。
export interface CapturerStatusListOut {
  items: CapturerStatusItemOut[];
}

// 标签规则
export interface TagRuleCreateIn {
  name: string;
  description: string;
  model: string;
  rego: string;
  enabled: boolean;
}

export interface TagRuleUpdateIn {
  description: string;
  rego: string;
  enabled: boolean;
}

export interface TagRuleValidateIn {
  rego: string;
  model?: string;
}

export interface TagRuleModelIn {
  model: string;
}

export interface TagRuleDryRunIn {
  /** 候选规则名称，草稿阶段可为空（后端以占位名兜底） */
  name?: string;
  model: string;
  rego: string;
  input: Record<string, unknown>;
}

// 标签规则样本
export interface TagRuleSampleCreateIn {
  model: string;
  name: string;
  description?: string;
  input: Record<string, unknown>;
}

export interface TagRuleSampleUpdateIn {
  name: string;
  description: string;
}

// 标签事件规则
export interface TagEventRuleConditionIn {
  model: string;
  dimension: string;
  item: string;
  name: string;
  /** 窗口，形如时长字符串（如 "5m"） */
  window: string;
  threshold: number;
  expression: string;
  enabled: boolean;
  description: string;
}

export interface TagEventRuleCreateIn {
  name: string;
  description: string;
  matchAll: boolean;
  /** suppressWindow 以纳秒为单位（前端按分钟换算） */
  suppressWindow: number;
  enabled: boolean;
  /** notificationGroupID 规则绑定的通知组 ID，缺省表示未绑定（使用命名空间默认组） */
  notificationGroupID?: number;
  /** notificationTargetID 规则直绑的通知目标 ID，缺省表示未直绑目标；与通知组互斥 */
  notificationTargetID?: number;
  conditions: TagEventRuleConditionIn[];
}

export interface TagEventRuleUpdateIn {
  description: string;
  matchAll: boolean;
  /** suppressWindow 以纳秒为单位（前端按分钟换算） */
  suppressWindow: number;
  enabled: boolean;
  /** notificationGroupID 规则绑定的通知组 ID，缺省表示未绑定（使用命名空间默认组） */
  notificationGroupID?: number;
  /** notificationTargetID 规则直绑的通知目标 ID，缺省表示未直绑目标；与通知组互斥 */
  notificationTargetID?: number;
  conditions: TagEventRuleConditionIn[];
}

export interface TagEventRuleValidateIn {
  expression: string;
}

export interface TagEventRuleDryRunIn {
  name: string;
  matchAll: boolean;
  enabled: boolean;
  conditions: TagEventRuleConditionIn[];
}

// 批量操作
export type FlexIDs = number | number[];
export type FlexUnionIds = string | string[];
export type FlexNames = string | string[];

export interface BatchFailureOut {
  id: string;
  reason: string;
}

export interface BatchResultOut {
  succeeded: number;
  failed: BatchFailureOut[];
}

// ---- 标签事件记录 ----
// TagEventRecordItem 标签事件记录项
export interface TagEventRecordItem {
  id: number;
  createdAt: string;
  updatedAt: string;
  ruleID: number;
  ruleName: string;
  matchAll: boolean;
  status: string;
  detectedAt: string;
}

// TagEventRecordListOut 标签事件记录列表出参
export interface TagEventRecordListOut {
  items: TagEventRecordItem[];
  total: number;
}

// TagEventRecordHitOut 标签事件命中明细项
export interface TagEventRecordHitOut {
  id: number;
  conditionName: string;
  model: string;
  dimension: string;
  item: string;
  count: number;
  threshold: number;
  window: string;
  ids: string[];
}

// TagEventRecordHitRecordsIn 标签事件命中条件原始数据反查入参
// model/ids 来自详情命中明细；createdAtFrom/createdAtTo 为原始记录入库时间预筛范围
// （前端由事件检测时间 detectedAt 与条件窗口 window 推导），
// 用于后端 ClickHouse created_at 分区裁剪（http_transports 按小时分区）
export interface TagEventRecordHitRecordsIn {
  model: string;
  ids: string[];
  createdAtFrom: string;
  createdAtTo: string;
}

// TagEventRecordHitRecordsOut 标签事件命中条件原始数据反查出参
// items 为命中条件对应的原始数据明细行（http_transports 全字段，键与流量列表一致）
export interface TagEventRecordHitRecordsOut {
  items: Record<string, unknown>[];
}

// TagEventRecordSendOut 标签事件关联的通知发送记录项
export interface TagEventRecordSendOut {
  id: number;
  targetID: number;
  targetName: string;
  channelID: number;
  channelName: string;
  channelType: string;
  status: string;
  messageID: string;
  error: string;
  payload: string;
  notifiedAt: string | null;
}

// TagEventRecordDetailOut 标签事件记录详情出参
export interface TagEventRecordDetailOut {
  id: number;
  createdAt: string;
  updatedAt: string;
  ruleID: number;
  ruleName: string;
  matchAll: boolean;
  status: string;
  dedupKey: string;
  detectedAt: string;
  hits: TagEventRecordHitOut[];
  sends: TagEventRecordSendOut[];
}

// TagEventRecordSearchIn 标签事件记录搜索入参
// 过滤字段命名与后端 search.go 的 TagEventRecordListGetFilterBy 一致
export interface TagEventRecordSearchIn extends SearchIn {
  search?: string;
  status?: string;
  matchAll?: boolean;
  ruleName?: string;
  createdAt?: string;
  updatedAt?: string;
  detectedAt?: string;
}
