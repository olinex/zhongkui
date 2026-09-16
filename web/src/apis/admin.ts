import type { AxiosPromise } from "axios";
import { getJwtHeader, request } from "./base";

import type { SearchIn } from "@/components/@types/table";
import type {
  CapturerConfigIn,
  CapturerCreateIn,
  FlexIDs,
  FlexNames,
  FlexUnionIds,
  GroupCreateIn,
  GroupRbacPolicyIn,
  GroupUpdateIn,
  NamespaceCreateIn,
  NamespaceUpdateIn,
  UserCreateIn,
  UserGroupIn,
  UserPasswordResetIn,
} from "./@types/admin";
import type { profileChangeIn } from "./@types/user";

// user
export function userListGet(data: SearchIn): AxiosPromise {
  return request.post("/admin/user/list", data, { headers: getJwtHeader() });
}

export function userListOptions(): AxiosPromise {
  return request.options("/admin/user/list", { headers: getJwtHeader() });
}

export function userCreate(data: UserCreateIn): AxiosPromise {
  return request.post("/admin/user/create", data, { headers: getJwtHeader() });
}

export function userDetailGet(unionID: string): AxiosPromise {
  return request.post(`/admin/user/${unionID}/get`, {}, { headers: getJwtHeader() });
}

export function userUpdate(unionID: string, data: profileChangeIn): AxiosPromise {
  return request.post(`/admin/user/${unionID}/update`, data, {
    headers: getJwtHeader(),
  });
}

export function userSetActive(unionIds: FlexUnionIds, active: boolean): AxiosPromise {
  return request.post(
    `/admin/user/${active ? "activate" : "deactivate"}`,
    { unionIds },
    {
      headers: getJwtHeader(),
    },
  );
}

export function userPasswordReset(unionID: string, data: UserPasswordResetIn): AxiosPromise {
  return request.post(`/admin/user/${unionID}/password/reset`, data, {
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

export function userFrontendConfigReset(unionID: string): AxiosPromise {
  return request.post(
    `/admin/user/${unionID}/frontend-config/reset`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
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

export function groupDelete(names: FlexNames): AxiosPromise {
  return request.post(
    "/admin/group/delete",
    { names },
    {
      headers: getJwtHeader(),
    },
  );
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

export function namespaceDelete(names: FlexNames): AxiosPromise {
  return request.post(
    "/admin/namespace/delete",
    { names },
    {
      headers: getJwtHeader(),
    },
  );
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

export function capturerDelete(ids: FlexIDs): AxiosPromise {
  return request.post(
    "/admin/capturer/delete",
    { ids },
    {
      headers: getJwtHeader(),
    },
  );
}

export function capturerStatusListGet(): AxiosPromise {
  return request.post("/admin/capturer/status/list", {}, { headers: getJwtHeader() });
}

// 复导出类型，方便消费方按需 import。
export type { CapturerStatusItemOut, CapturerStatusListOut } from "./@types/admin";
