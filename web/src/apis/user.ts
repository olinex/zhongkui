import type { AxiosPromise } from "axios";
import { getJwtHeader, request } from "./base";

import type {
  accessKeyCreateIn,
  FrontendConfigSyncIn,
  PasswordForgotIn,
  PasswordLoginIn,
  PasswordResetByTokenIn,
  PasswordResetIn,
  profileChangeIn,
} from "./@types/user";

export function passwordLogin(data: PasswordLoginIn): AxiosPromise {
  return request.post("/user/password/login", data, {
    headers: getJwtHeader(),
  });
}

export function passwordForgot(data: PasswordForgotIn): AxiosPromise {
  return request.post("/user/password/forgot", data);
}

export function passwordResetByToken(data: PasswordResetByTokenIn): AxiosPromise {
  return request.post("/user/password/reset", data);
}

export function passwordReset(data: PasswordResetIn): AxiosPromise {
  return request.post("/user/self/password/reset", data, {
    headers: getJwtHeader(),
  });
}

export function jwtRefresh(): AxiosPromise {
  return request.post("/user/jwt/refresh", {}, { headers: getJwtHeader() });
}

export function profileChange(data: profileChangeIn): AxiosPromise {
  return request.post("/user/self/profile/change", data, {
    headers: getJwtHeader(),
  });
}

export function authorizationInfoGet(): AxiosPromise {
  return request.post("/user/self/authorization-info/get", null, {
    headers: getJwtHeader(),
  });
}

export function accessKeyListGet(): AxiosPromise {
  return request.post("/user/self/access-key/list", null, {
    headers: getJwtHeader(),
  });
}

export function accessKeyCreate(data: accessKeyCreateIn): AxiosPromise {
  return request.post("/user/self/access-key/create", data, {
    headers: getJwtHeader(),
  });
}

export function accessKeyRemove(id: number): AxiosPromise {
  return request.post(`/user/self/access-key/${id}/remove`, null, {
    headers: getJwtHeader(),
  });
}

export function accessKeyEnabled(id: number, enabled: boolean): AxiosPromise {
  return request.post(`/user/self/access-key/${id}/${enabled ? "activate" : "deactivate"}`, null, {
    headers: getJwtHeader(),
  });
}

export function frontendConfigGet(): AxiosPromise {
  return request.post("/user/self/frontend-config/get", {}, { headers: getJwtHeader() });
}

export function frontendConfigSync(data: FrontendConfigSyncIn): AxiosPromise {
  return request.post("/user/self/frontend-config/sync", data, { headers: getJwtHeader() });
}

export function frontendConfigSyncEnabledSet(enabled: boolean): AxiosPromise {
  return request.post(
    "/user/self/frontend-config/sync-enabled/set",
    { enabled },
    { headers: getJwtHeader() },
  );
}
