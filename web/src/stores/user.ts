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
// USER_REFRESH_EVENT 令牌刷新事件的名称，用于跨组件通知重新拉取令牌。
const USER_REFRESH_EVENT: string = "base:jwt-token-refresh";
// ZERO_DATETIME 零值时间，作为未加载令牌时的初始过期时间。
const ZERO_DATETIME: DateTime = DateTime.fromSeconds(0);

// JwtClaims JWT 载荷中本系统额外使用的声明字段。
interface JwtClaims extends JwtPayload {
  unionID: string;
  nickleName: string;
  email: string;
}

// AuthorizationInfo 后端下发的授权信息：RBAC 模型、策略集合与可见命名空间。
interface AuthorizationInfo {
  model: string;
  policies: string[][];
  namespaces: string[];
}

export const useUserStore = defineStore("user", {
  state: () => ({
    // 元数据字段
    initial: false,
    issuer: "",
    subject: "",
    audience: [] as Array<string>,
    expiresAt: ZERO_DATETIME,
    notBefore: ZERO_DATETIME,
    issuedAt: ZERO_DATETIME,
    // 自定义字段
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
    // clear 重置会话状态为未登录的初始值。
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
    // load 从本地令牌解析用户信息，并拉取授权信息构建 RBAC 执行器。
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
    // refresh 刷新 JWT 令牌并重新加载用户信息；auto 为真时按过期时间自动续期。
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
    // signOut 清除本地令牌并重置会话状态。
    signOut() {
      localManager.removeJwt();
      this.clear();
    },
    // init 初始化会话：已加载则直接返回，否则加载并按需安排自动续期。
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
    // 订阅令牌刷新事件并触发会话初始化；mitt 事件在本项目中未做类型定义。
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    refreshEventOn(mitter: Emitter<any>) {
      mitter.on(USER_REFRESH_EVENT, () => this.refresh(false));
      this.init();
    },
    // 取消订阅令牌刷新事件并重置会话状态。
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    refreshEventOff(mitter: Emitter<any>) {
      mitter.off(USER_REFRESH_EVENT);
      this.clear();
    },
    // checkPermission 异步校验一组权限点是否全部通过。
    async checkPermission(...rvals: string[]): Promise<boolean> {
      if (this.enforcer === null) {
        return false;
      } else {
        return await this.enforcer?.enforce(rvals);
      }
    },
    // can 同步校验单个权限点：参数顺序 (sub, obj, act, nsp) 与后端鉴权中间件一致，
    // 命名空间取自本地存储，与 X-Namespace 请求头同源；
    // 授权信息尚未加载时返回 false。
    can(obj: string, act: string): boolean {
      if (this.enforcer === null) {
        return false;
      }
      return this.enforcer.enforceSync(this.unionID, obj, act, localManager.getNamespace());
    },
    // getAvailableNamespaces 返回当前用户可见的命名空间列表。
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
