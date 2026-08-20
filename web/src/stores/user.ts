import { Enforcer, newEnforcer, newModelFromString, StringAdapter } from "casbin";
import consola from "consola";
import { jwtDecode, type JwtPayload } from "jwt-decode";
import { DateTime, Duration } from "luxon";
import { defineStore } from "pinia";

import { getResponse } from "@/apis/base";
import { authorizationInfoGet, jwtRefresh } from "@/apis/user";
import { arrayToCsv } from "@/utils/base";

import { localManager } from "./local";

import type { Emitter } from "mitt";
import type { UserTokenOut } from "@/apis/@types/user";
const USER_REFRESH_EVENT: string = "base:jwt-token-refresh";
const ZERO_DATETIME: DateTime = DateTime.fromSeconds(0);

interface JwtClaims extends JwtPayload {
  unionID: string;
  nickleName: string;
  email: string;
}

interface AuthorizationInfo {
  model: string;
  policies: string[][];
  namespaces: string[];
}

export const useUserStore = defineStore("user", {
  state: () => ({
    // meta fields
    initial: false,
    issuer: "",
    subject: "",
    audience: [] as Array<string>,
    expiresAt: ZERO_DATETIME,
    notBefore: ZERO_DATETIME,
    issuedAt: ZERO_DATETIME,
    // custom fields
    exists: false,
    unionID: "",
    nickleName: "",
    email: "",
    namespaces: [] as string[],
    enforcer: null as Enforcer | null,
  }),
  getters: {
    timeout(): number {
      const dur: Duration = this.expiresAt.diff(DateTime.local(), "milliseconds");
      return dur.milliseconds * 0.9;
    },
    valid(): boolean {
      return this.exists && this.expiresAt.diff(DateTime.local(), "milliseconds").milliseconds > 0;
    },
  },
  actions: {
    clear() {
      this.initial = false;
      this.issuer = "";
      this.subject = "";
      this.audience = [];
      this.expiresAt = ZERO_DATETIME;
      this.notBefore = ZERO_DATETIME;
      this.issuedAt = ZERO_DATETIME;
      this.exists = false;
      this.unionID = "";
      this.nickleName = "";
      this.email = "";
      this.enforcer = null;
    },
    async load() {
      const jwt = localManager.getJwt();
      if (!jwt) {
        this.clear();
        return;
      }
      const payload = jwtDecode<JwtClaims>(jwt);
      if (!payload) {
        this.clear();
        return;
      }
      await getResponse<AuthorizationInfo>(
        authorizationInfoGet(),
        async (rsp) => {
          this.issuer = payload.iss || "";
          this.subject = payload.sub || "";
          this.audience = (payload.aud as Array<string>) || [];
          this.expiresAt = DateTime.fromSeconds(payload.exp || 0);
          this.notBefore = DateTime.fromSeconds(payload.nbf || 0);
          this.issuedAt = DateTime.fromSeconds(payload.iat || 0);
          this.exists = true;
          this.unionID = payload.unionID;
          this.nickleName = payload.nickleName;
          this.email = payload.email;
          this.initial = true;
          this.namespaces = rsp.data.namespaces;
          const model = newModelFromString(rsp.data.model);
          const adapter = new StringAdapter(arrayToCsv(rsp.data.policies));
          this.enforcer = await newEnforcer(model, adapter);
          consola.info("user loaded");
        },
        async (rsp) => {
          consola.warn(`code: ${rsp.code} - ${rsp.summary}`);
          this.clear();
        },
      );
    },
    async refresh(auto: boolean) {
      const jwt = localManager.getJwt();
      if (!jwt) {
        this.clear();
        return;
      }
      await getResponse<UserTokenOut>(
        jwtRefresh(),
        async (rsp) => {
          // 持久化刷新后的令牌，避免依赖受主机作用域限制的 Cookie
          localManager.setJwt(rsp.data.token);
          await this.load();
          if (!this.valid || !auto) {
            return;
          }
          const timeout = this.timeout;
          setTimeout(() => this.refresh(true), timeout);
          consola.info(`refresh after ${timeout} ms`);
        },
        async (rsp) => {
          consola.warn(`code: ${rsp.code} - ${rsp.summary}`);
        },
      );
    },
    // ensureValid 确保当前会话有效，供路由守卫在切换页面时调用。
    // 令牌已过期但本地仍持有会话时，先尝试刷新恢复（滑动过期），
    // 刷新失败仅返回 false，不主动清除本地令牌与初始化状态，
    // 避免瞬时网络异常导致用户被强制登出。
    async ensureValid(): Promise<boolean> {
      if (this.valid) {
        return true;
      }
      if (!localManager.getJwt()) {
        return false;
      }
      await this.refresh(false);
      return this.valid;
    },
    // 重新拉取授权信息并重建 RBAC 执行器，
    // 用于命名空间/用户组等授权相关数据变更后刷新前端权限快照；
    // 失败时仅告警不清空会话，避免瞬时接口异常导致用户掉线
    async refreshAuthorization() {
      await getResponse<AuthorizationInfo>(
        authorizationInfoGet(),
        async (rsp) => {
          this.namespaces = rsp.data.namespaces;
          const model = newModelFromString(rsp.data.model);
          const adapter = new StringAdapter(arrayToCsv(rsp.data.policies));
          this.enforcer = await newEnforcer(model, adapter);
          consola.info("authorization refreshed");
        },
        async (rsp) => {
          consola.warn(`code: ${rsp.code} - ${rsp.summary}`);
        },
      );
    },
    signOut() {
      localManager.removeJwt();
      this.clear();
    },
    async init() {
      if (this.exists) {
        return;
      }
      await this.load();
      if (!this.exists) {
        return;
      }
      const timeout = this.timeout;
      setTimeout(() => this.refresh(true), timeout);
    },
    // mitt events are untyped in this project.
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    refreshEventOn(mitter: Emitter<any>) {
      mitter.on(USER_REFRESH_EVENT, () => this.refresh(false));
      this.init();
    },
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    refreshEventOff(mitter: Emitter<any>) {
      mitter.off(USER_REFRESH_EVENT);
      this.clear();
    },
    async checkPermission(...rvals: string[]): Promise<boolean> {
      if (this.enforcer === null) {
        return false;
      } else {
        return await this.enforcer?.enforce(rvals);
      }
    },
    async getAvailableNamespaces(): Promise<string[]> {
      if (this.enforcer === null) {
        return [];
      }
      const policeNamespaces = (await this.enforcer.getFilteredNamedGroupingPolicy("g", 0)).map(
        (item) => item[2],
      );
      if (policeNamespaces.some((name) => name === "*")) {
        return this.namespaces;
      } else {
        return policeNamespaces;
      }
    },
  },
});
