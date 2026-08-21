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

// tag rule
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

// tag rule sample
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

// tag event rule
export interface TagEventRuleConditionIn {
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

export interface TagEventRuleCreateIn {
  name: string;
  description: string;
  matchAll: boolean;
  enabled: boolean;
  conditions: TagEventRuleConditionIn[];
}

export interface TagEventRuleUpdateIn {
  description: string;
  matchAll: boolean;
  enabled: boolean;
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
