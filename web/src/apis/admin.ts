import type { AxiosPromise } from "axios";
import { getJwtHeader, request } from "./base";

import type { SearchIn } from "@/components/@types/table";
import type {
  CapturerConfigIn,
  CapturerCreateIn,
  GroupCreateIn,
  GroupRbacPolicyIn,
  GroupUpdateIn,
  NamespaceCreateIn,
  NamespaceUpdateIn,
  TagEventRuleCreateIn,
  TagEventRuleDryRunIn,
  TagEventRuleUpdateIn,
  TagEventRuleValidateIn,
  TagRuleCreateIn,
  TagRuleDryRunIn,
  TagRuleModelIn,
  TagRuleUpdateIn,
  TagRuleValidateIn,
  UserGroupIn,
} from "./@types/admin";
import type { profileChangeIn } from "./@types/user";

// user
export function userListGet(data: SearchIn): AxiosPromise {
  return request.post("/admin/user/list", data, { headers: getJwtHeader() });
}

export function userListOptions(): AxiosPromise {
  return request.options("/admin/user/list", { headers: getJwtHeader() });
}

export function userDetailGet(unionID: string): AxiosPromise {
  return request.post(`/admin/user/${unionID}/get`, {}, { headers: getJwtHeader() });
}

export function userUpdate(unionID: string, data: profileChangeIn): AxiosPromise {
  return request.post(`/admin/user/${unionID}/update`, data, {
    headers: getJwtHeader(),
  });
}

export function userSetActive(unionID: string, active: boolean): AxiosPromise {
  return request.post(`/admin/user/${unionID}/${active ? "activate" : "deactivate"}`, null, {
    headers: getJwtHeader(),
  });
}

export function userGroupJoin(unionID: string, data: UserGroupIn): AxiosPromise {
  return request.post(`/admin/user/${unionID}/join-group`, data, {
    headers: getJwtHeader(),
  });
}

export function userGroupLeave(unionID: string, data: UserGroupIn): AxiosPromise {
  return request.post(`/admin/user/${unionID}/leave-group`, data, {
    headers: getJwtHeader(),
  });
}

// group
export function groupListGet(data: SearchIn): AxiosPromise {
  return request.post("/admin/group/list", data, { headers: getJwtHeader() });
}

export function groupListOptions(): AxiosPromise {
  return request.options("/admin/group/list", { headers: getJwtHeader() });
}

export function groupDetailGet(name: string): AxiosPromise {
  return request.post(`/admin/group/${name}/get`, {}, { headers: getJwtHeader() });
}

export function groupUpdate(name: string, data: GroupUpdateIn): AxiosPromise {
  return request.post(`/admin/group/${name}/update`, data, {
    headers: getJwtHeader(),
  });
}

export function groupCreate(data: GroupCreateIn): AxiosPromise {
  return request.post(`/admin/group/create`, data, {
    headers: getJwtHeader(),
  });
}

export function groupDelete(name: string): AxiosPromise {
  return request.post(`/admin/group/${name}/delete`, null, {
    headers: getJwtHeader(),
  });
}

export function groupRbacPolicyAdd(groupName: string, data: GroupRbacPolicyIn): AxiosPromise {
  return request.post(`/admin/group/${groupName}/add-rbac-policy`, data, {
    headers: getJwtHeader(),
  });
}

export function groupRbacPolicyRemove(groupName: string, data: GroupRbacPolicyIn): AxiosPromise {
  return request.post(`/admin/group/${groupName}/remove-rbac-policy`, data, {
    headers: getJwtHeader(),
  });
}

// namespace
export function namespaceListGet(data: SearchIn): AxiosPromise {
  return request.post("/admin/namespace/list", data, {
    headers: getJwtHeader(),
  });
}

export function namespaceListOptions(): AxiosPromise {
  return request.options("/admin/namespace/list", { headers: getJwtHeader() });
}

export function namespaceDetailGet(name: string): AxiosPromise {
  return request.post(`/admin/namespace/${name}/get`, null, {
    headers: getJwtHeader(),
  });
}

export function namespaceDelete(name: string): AxiosPromise {
  return request.post(`/admin/namespace/${name}/create`, {
    headers: getJwtHeader(),
  });
}

export function namespaceCreate(data: NamespaceCreateIn): AxiosPromise {
  return request.post("/admin/namespace/create", data, {
    headers: getJwtHeader(),
  });
}

export function namespaceUpdate(name: string, data: NamespaceUpdateIn): AxiosPromise {
  return request.post(`/admin/namespace/${name}/update`, data, {
    headers: getJwtHeader(),
  });
}

// rbac policy template
export function rbacPolicyTemplateListGet(data: SearchIn): AxiosPromise {
  return request.post("/admin/rbac-policy-template/list", data, {
    headers: getJwtHeader(),
  });
}

export function rbacPolicyTemplateListOptions(): AxiosPromise {
  return request.options("/admin/rbac-policy-template/list", {
    headers: getJwtHeader(),
  });
}

// capturer
export function capturerListGet(data: SearchIn): AxiosPromise {
  return request.post("/admin/capturer/list", data, {
    headers: getJwtHeader(),
  });
}

export function capturerListOptions(): AxiosPromise {
  return request.options("/admin/capturer/list", {
    headers: getJwtHeader(),
  });
}

export function capturerCreate(data: CapturerCreateIn): AxiosPromise {
  return request.post("/admin/capturer/create", data, {
    headers: getJwtHeader(),
  });
}

export function capturerDetailGet(id: number): AxiosPromise {
  return request.post(`/admin/capturer/${id}/get`, null, {
    headers: getJwtHeader(),
  });
}

export function capturerUpdate(id: number, data: CapturerConfigIn): AxiosPromise {
  return request.post(`/admin/capturer/${id}/update`, data, {
    headers: getJwtHeader(),
  });
}

export function capturerDelete(id: number): AxiosPromise {
  return request.post(`/admin/capturer/${id}/delete`, null, {
    headers: getJwtHeader(),
  });
}

// tag rule
export function tagRuleListGet(data: SearchIn): AxiosPromise {
  return request.post("/admin/tag-rule/list", data, { headers: getJwtHeader() });
}

export function tagRuleListOptions(): AxiosPromise {
  return request.options("/admin/tag-rule/list", { headers: getJwtHeader() });
}

export function tagRuleDetailGet(id: number): AxiosPromise {
  return request.post(
    `/admin/tag-rule/${id}/get`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

export function tagRuleCreate(data: TagRuleCreateIn): AxiosPromise {
  return request.post("/admin/tag-rule/create", data, {
    headers: getJwtHeader(),
  });
}

export function tagRuleUpdate(id: number, data: TagRuleUpdateIn): AxiosPromise {
  return request.post(`/admin/tag-rule/${id}/update`, data, {
    headers: getJwtHeader(),
  });
}

export function tagRuleDelete(id: number): AxiosPromise {
  return request.post(`/admin/tag-rule/${id}/delete`, null, {
    headers: getJwtHeader(),
  });
}

export function tagRuleSetEnabled(id: number, enabled: boolean): AxiosPromise {
  return request.post(`/admin/tag-rule/${id}/${enabled ? "enable" : "disable"}`, null, {
    headers: getJwtHeader(),
  });
}

export function tagRuleValidate(data: TagRuleValidateIn): AxiosPromise {
  return request.post("/admin/tag-rule/validate", data, {
    headers: getJwtHeader(),
  });
}

export function tagRuleModelOptions(): AxiosPromise {
  return request.post(
    "/admin/tag-rule/model/options",
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

export function tagRuleModelMetadata(data: TagRuleModelIn): AxiosPromise {
  return request.post("/admin/tag-rule/model/metadata", data, {
    headers: getJwtHeader(),
  });
}

export function tagRuleModelSamples(data: TagRuleModelIn): AxiosPromise {
  return request.post("/admin/tag-rule/model/samples", data, {
    headers: getJwtHeader(),
  });
}

export function tagRuleDryRun(data: TagRuleDryRunIn): AxiosPromise {
  return request.post("/admin/tag-rule/single/dry-run", data, {
    headers: getJwtHeader(),
  });
}

export function tagRuleVersionList(id: number): AxiosPromise {
  return request.post(
    `/admin/tag-rule/${id}/version/list`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

export function tagRuleVersionRollback(id: number, version: number): AxiosPromise {
  return request.post(`/admin/tag-rule/${id}/version/${version}/rollback`, null, {
    headers: getJwtHeader(),
  });
}

// tag event rule
export function tagEventRuleListGet(data: SearchIn): AxiosPromise {
  return request.post("/admin/tag-event-rule/list", data, {
    headers: getJwtHeader(),
  });
}

export function tagEventRuleListOptions(): AxiosPromise {
  return request.options("/admin/tag-event-rule/list", {
    headers: getJwtHeader(),
  });
}

export function tagEventRuleDetailGet(id: number): AxiosPromise {
  return request.post(
    `/admin/tag-event-rule/${id}/get`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

export function tagEventRuleCreate(data: TagEventRuleCreateIn): AxiosPromise {
  return request.post("/admin/tag-event-rule/create", data, {
    headers: getJwtHeader(),
  });
}

export function tagEventRuleUpdate(id: number, data: TagEventRuleUpdateIn): AxiosPromise {
  return request.post(`/admin/tag-event-rule/${id}/update`, data, {
    headers: getJwtHeader(),
  });
}

export function tagEventRuleDelete(id: number): AxiosPromise {
  return request.post(`/admin/tag-event-rule/${id}/delete`, null, {
    headers: getJwtHeader(),
  });
}

export function tagEventRuleSetEnabled(id: number, enabled: boolean): AxiosPromise {
  return request.post(`/admin/tag-event-rule/${id}/${enabled ? "enable" : "disable"}`, null, {
    headers: getJwtHeader(),
  });
}

export function tagEventRuleValidate(data: TagEventRuleValidateIn): AxiosPromise {
  return request.post("/admin/tag-event-rule/validate", data, {
    headers: getJwtHeader(),
  });
}

export function tagEventRuleDryRun(data: TagEventRuleDryRunIn): AxiosPromise {
  return request.post("/admin/tag-event-rule/dry-run", data, {
    headers: getJwtHeader(),
  });
}

export function tagEventRuleVersionList(id: number): AxiosPromise {
  return request.post(
    `/admin/tag-event-rule/${id}/version/list`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

export function tagEventRuleVersionRollback(id: number, version: number): AxiosPromise {
  return request.post(`/admin/tag-event-rule/${id}/version/${version}/rollback`, null, {
    headers: getJwtHeader(),
  });
}
