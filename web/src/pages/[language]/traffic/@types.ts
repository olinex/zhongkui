import type { NormalModelOut } from "@/types";

// ---- 标签识别规则（流量审计命名空间化）----
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
  ruleId: number;
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
  /** duration as duration string (e.g. "5s") */
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
  description: string;
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

export interface TagRuleModelOptionOut {
  value: string;
  label: string;
}

export interface TagRuleModelOptionsOut {
  items: TagRuleModelOptionOut[];
}

export interface TagRuleSampleDetailOut extends NormalModelOut {
  model: string;
  name: string;
  description: string;
  input: Record<string, unknown>;
}

// ---- 标签事件规则（流量审计命名空间化）----
export interface TagEventRuleConditionOut extends NormalModelOut {
  model: string;
  dimension: string;
  item: string;
  name: string;
  /** window as duration string (e.g. "5m") */
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
  enabled: boolean;
}

export interface TagEventRuleDetailOut extends TagEventRuleBaseDetailOut {
  conditions: TagEventRuleConditionOut[];
  version: number;
  versionCount: number;
}

export interface TagEventRuleListOut extends TagEventRuleBaseDetailOut {
  total: number;
}

export interface TagEventRuleVersionOut extends NormalModelOut {
  ruleId: number;
  version: number;
  name: string;
  matchAll: boolean;
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
  /** window as duration string (e.g. "5m") */
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
