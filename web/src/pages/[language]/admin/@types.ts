import type { NormalModelOut } from "@/types";

export interface GroupRbacPolicyDetailOut extends NormalModelOut {
  target: string;
  action: string;
  effect: string;
}

export interface UserRbacPolicyDetailOut extends NormalModelOut {
  namespace: string;
  name: string;
}

export interface UserBaseDetailOut extends NormalModelOut {
  unionID: string;
  nickleName: string;
  username: string;
  email: string;
  active: boolean;
}

export interface UserDetailOut extends UserBaseDetailOut {
  groups: UserRbacPolicyDetailOut[];
}

export interface GroupBaseDetailOut extends NormalModelOut {
  name: string;
  description: string;
}

export interface GroupDetailOut extends GroupBaseDetailOut {
  policies: GroupRbacPolicyDetailOut[];
}

export interface NamespaceDetailOut extends NormalModelOut {
  name: string;
  description: string;
}

export interface CapturerConfigOut {
  device: string;
  snapLen: number;
  promisc: boolean;
  timeout: number;
  bpfExpr: string;
  enabled: boolean;
}

export interface CapturerDetailOut extends NormalModelOut {
  namespace: string;
  name: string;
  version: string;
  config: CapturerConfigOut;
}

// tag rule
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
}

export interface TagRuleValidateOut {
  valid: boolean;
  errors: TagRuleValidateErrorOut[];
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

// tag event rule
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
