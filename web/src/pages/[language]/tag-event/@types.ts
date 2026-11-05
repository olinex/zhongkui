import type { NormalModelOut } from "@/types";

// ---- 标签识别规则（标签事件命名空间化）----
export interface TagRuleBaseDetailOut extends NormalModelOut {
  name: string;
  description: string;
  model: string;
  enabled: boolean;
}

export interface TagRuleDetailOut extends TagRuleBaseDetailOut {
  rego: string;
  version: number;
  versionCount: number;
}

export interface TagRuleListOut extends TagRuleBaseDetailOut {
  total: number;
}

export interface TagRuleVersionOut extends NormalModelOut {
  ruleID: number;
  version: number;
  name: string;
  description: string;
  model: string;
  rego: string;
}

export interface TagRuleValidateErrorOut {
  line: number;
  column: number;
  message: string;
  type?: string;
  hint?: string;
}

export interface TagRuleValidateOut {
  valid: boolean;
  errors: TagRuleValidateErrorOut[];
  warnings: TagRuleValidateErrorOut[];
}

export interface TagRuleDryRunRuleOut {
  tags: string[];
  /** duration 为时长字符串（如 "5s"） */
  duration: string;
  error: string;
}

export interface TagRuleDryRunOut {
  tags: string[];
  error: string;
  rules: Record<string, TagRuleDryRunRuleOut>;
}

export interface InputItemMetaOut {
  type: string;
  goType: string;
}

export interface InputFieldMetaOut {
  field: string;
  type: string;
  goType: string;
  description?: string;
  sensitive?: boolean;
  items?: InputItemMetaOut;
}

export interface TagRuleModelDimensionOut {
  value: string;
}

export interface TagRuleModelMetadataOut {
  model: string;
  fields: InputFieldMetaOut[];
  dimensions: TagRuleModelDimensionOut[];
}

export interface TagRuleSampleOut {
  name: string;
  description?: string;
  input: Record<string, unknown>;
}

export interface TagRuleModelSamplesOut {
  model: string;
  items: TagRuleSampleOut[];
}

export interface TagRuleSampleDetailOut extends NormalModelOut {
  model: string;
  name: string;
  description: string;
  input: Record<string, unknown>;
}

// ---- 标签规则自定义 OPA 内置函数 / sqlite vtable 元信息（供 Rego 编辑器补全与提示）----
export interface TagRuleFunctionOut {
  name: string;
  description: string;
  signature: string;
  example?: string;
}

export interface TagRuleSqliteColumnOut {
  name: string;
  type: string;
}

export interface TagRuleSqliteDatasourceOut {
  name: string;
  columns: TagRuleSqliteColumnOut[];
}

export interface TagRuleSqliteMetaOut {
  datasources: TagRuleSqliteDatasourceOut[];
}

// ---- 标签事件规则（标签事件命名空间化）----
export interface TagEventRuleConditionOut extends NormalModelOut {
  model: string;
  dimension: string;
  item: string;
  name: string;
  /** window 为时长字符串（如 "5m"） */
  window: string;
  threshold: number;
  expression: string;
  enabled: boolean;
  description: string;
}

export interface TagEventRuleBaseDetailOut extends NormalModelOut {
  name: string;
  description: string;
  matchAll: boolean;
  /** suppressWindow 为时长字符串（如 "1h0m0s"），0 表示不抑制 */
  suppressWindow: string;
  enabled: boolean;
}

export interface TagEventRuleDetailOut extends TagEventRuleBaseDetailOut {
  /** notificationGroupID 规则绑定的通知组 ID，null 表示未绑定（使用命名空间默认组）。仅详情返回 */
  notificationGroupID: number | null;
  /** notificationTargetID 规则直绑的通知目标 ID，null 表示未直绑目标。仅详情返回 */
  notificationTargetID: number | null;
  conditions: TagEventRuleConditionOut[];
  version: number;
  versionCount: number;
}

export interface TagEventRuleListOut extends TagEventRuleBaseDetailOut {
  total: number;
}

export interface TagEventRuleVersionOut extends NormalModelOut {
  ruleID: number;
  version: number;
  name: string;
  matchAll: boolean;
  /** suppressWindow 为时长字符串（如 "1h0m0s"），0 表示不抑制 */
  suppressWindow: string;
  description: string;
  conditions: TagEventRuleConditionOut[];
}

export interface TagEventRuleValidateOut {
  valid: boolean;
  refined: string;
  error: string;
}

export interface TagEventHitOut {
  conditionID: number;
  conditionName: string;
  model: string;
  dimension: string;
  item: string;
  count: number;
  threshold: number;
  /** window 为时长字符串（如 "5m"） */
  window: string;
}

export interface TagEventOut {
  namespace: string;
  ruleID: number;
  ruleName: string;
  matchAll: boolean;
  detectedAt: string;
  hits: TagEventHitOut[];
}

export interface TagEventRuleDryRunOut {
  ruleName: string;
  matchAll: boolean;
  detectedAt: string;
  events: TagEventOut[];
}
