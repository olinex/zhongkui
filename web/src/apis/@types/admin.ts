export interface UserGroupIn {
  namespace: string;
  groupName: string;
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
