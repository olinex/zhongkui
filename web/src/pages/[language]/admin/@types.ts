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
  frontendConfigSyncEnabled: boolean;
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
