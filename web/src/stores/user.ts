import {
  Enforcer,
  newEnforcer,
  newModelFromString,
  StringAdapter,
} from "casbin";
import consola from "consola";
import { jwtDecode, type JwtPayload } from "jwt-decode";
import { DateTime, Duration } from "luxon";
import { defineStore } from "pinia";

import { getResponse, getSimpleResponse } from "@/apis/base";
import { authorizationInfoGet, jwtRefresh } from "@/apis/user";
import { arrayToCsv } from "@/utils/base";

import { localManager } from "./local";

import type { Emitter } from "mitt";
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
      const dur: Duration = this.expiresAt.diff(
        DateTime.local(),
        "milliseconds"
      );
      return dur.milliseconds * 0.9;
    },
    valid(): boolean {
      return (
        this.exists &&
        this.expiresAt.diff(DateTime.local(), "milliseconds").milliseconds > 0
      );
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
      let jwt = localManager.getJwt();
      if (!jwt) {
        this.clear();
        return;
      }
      let payload = jwtDecode<JwtClaims>(jwt);
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
          let model = newModelFromString(rsp.data.model);
          let adapter = new StringAdapter(arrayToCsv(rsp.data.policies));
          this.enforcer = await newEnforcer(model, adapter);
          consola.info("user loaded");
        },
        async (rsp) => {
          consola.warn(`code: ${rsp.code} - ${rsp.summary}`);
          this.clear();
        }
      );
    },
    async refresh(auto: boolean) {
      const jwt = localManager.getJwt();
      if (!jwt) {
        this.clear();
        return;
      }
      await getSimpleResponse(
        jwtRefresh(),
        async () => {
          this.load();
          if (!this.valid || !auto) {
            return;
          }
          let timeout = this.timeout;
          setTimeout(() => this.refresh(true), timeout);
          consola.info(`refresh after ${timeout} ms`);
        },
        async (rsp, _) => {
          consola.warn(`code: ${rsp.code} - ${rsp.summary}`);
        }
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
      let timeout = this.timeout;
      setTimeout(() => this.refresh(true), timeout);
    },
    refreshEventOn(mitter: Emitter<any>) {
      mitter.on(USER_REFRESH_EVENT, () => this.refresh(false));
      this.init();
    },
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
      const policeNamespaces = (
        await this.enforcer.getFilteredNamedGroupingPolicy("g", 0)
      ).map((item) => item[2]);
      if (policeNamespaces.some((name) => name === "*")) {
        return this.namespaces;
      } else {
        return policeNamespaces;
      }
    },
  },
});
