import axios, { AxiosError, type AxiosPromise, HttpStatusCode } from "axios";

import { useAppStore } from "@/stores/app";
import { localManager } from "@/stores/local";
import { HttpHeader } from "@/types";
import { mitter } from "@/utils/mitter";

import {
  type BaseHeaders,
  type ErrorResponse,
  Status,
  type SuccessResponse,
  X_JWT_HEADER_NAME,
} from "./@types/base";
import consola from "consola";

export const current_base_url = (): string => {
  const baseURL = process.env.ZHONGKUI_BASE_URL || "http://localhost:8080/api/v1";
  consola.info("current_base_url", baseURL);
  return baseURL;
};

// API_VERSION_PREFIX 是后端原生接口的固定版本前缀：能力规范入参中的 path 总是携带它。
const API_VERSION_PREFIX = "/api/v1";

// apiRequestURL 把后端下发的完整接口路径转换为可执行的绝对地址。
//
// axios 实例的 baseURL 通常已包含 /api/v1（默认 http://localhost:8080/api/v1）；若把
// /api/v1/xxx 直接交给 request.request，axios 会再次拼接 baseURL，得到
// /api/v1/api/v1/xxx 而 404。此处以 baseURL 的 origin 与部署前缀重建绝对地址：
//   - baseURL 以 /api/v1 结尾时，去掉该版本前缀后保留其部署前缀（如 /zhongkui）；
//   - 能力 path 仍使用后端下发的完整 /api/v1 路径，不接受调用方覆盖 host。
export function apiRequestURL(path: string): string {
  const baseURL = request.defaults.baseURL || window.location.origin;
  const base = new URL(baseURL, window.location.origin);
  const basePath = base.pathname.replace(/\/+$/, "");
  const deploymentPrefix = basePath.endsWith(API_VERSION_PREFIX)
    ? basePath.slice(0, -API_VERSION_PREFIX.length)
    : basePath;
  const targetPath = deploymentPrefix + (path.startsWith("/") ? path : `/${path}`);
  return new URL(targetPath, base.origin).toString();
}

// AUTH_FAILED_EVENT 全局鉴权失败事件名：已登录却收到后端 HTTP 401 时由响应拦截器广播，
// 订阅方据此统一复位会话并跳转 401 页面（见 router/index.ts）。
export const AUTH_FAILED_EVENT: string = "base:auth-failed";

export const request = axios.create({
  baseURL: current_base_url(),
  timeout: 1000,
  withCredentials: true,
  headers: {
    [HttpHeader.ACCEPT]: "application/json",
    [HttpHeader.CONTENT_TYPE]: "application/json",
  },
});

request.interceptors.request.use((config) => {
  // 携带文件的 FormData 请求必须走 multipart/form-data：axios 默认序列化在 Content-Type 含
  // application/json 时会把 FormData 转成 JSON（见 axios defaults.transformRequest 的
  // isFormData 分支：hasJSONContentType ? JSON.stringify(formDataToJSON(data)) : data），
  // 导致后端收到 JSON 体而非 multipart，报「请求体格式错误」。故此处清除实例默认的 JSON
  // Content-Type，让 axios 原样透传 FormData，由浏览器补全 multipart 边界。
  if (config.data instanceof FormData) {
    config.headers.delete(HttpHeader.CONTENT_TYPE);
  }
  const store = useAppStore();
  if (!config.headers[HttpHeader.ACCEPT_LANGUAGE]) {
    config.headers[HttpHeader.ACCEPT_LANGUAGE] = store.language;
  }
  if (!config.headers[HttpHeader.CONTENT_LANGUAGE]) {
    config.headers[HttpHeader.CONTENT_LANGUAGE] = store.language;
  }
  // 全局携带当前命名空间：后端 NamespaceMiddleware 据此做多租户隔离。
  // 登录/注册等未挂命名空间中间件的接口会忽略该头，无副作用。
  // 命名空间名称可能包含中文等非 ASCII 字符，而 HTTP 请求头值仅允许
  // ISO-8859-1（Latin-1）字符，直接写入会触发浏览器
  // "String contains non ISO-8859-1 code point" 异常导致请求从未发出；
  // 故此处先做 URL 编码（后端 NamespaceMiddleware 会相应解码）。
  if (!config.headers[HttpHeader.NAMESPACE]) {
    config.headers[HttpHeader.NAMESPACE] = encodeURIComponent(store.namespace);
  }
  return config;
});

// 鉴权失败统一捕获：后端仅在 JWT 无效 / 过期 / 用户不存在时返回 401（见后端
// middlewares/authenticate.go），而各调用点的 failed 回调大多省略（如枚举预取、
// 功能开关加载），401 只会被 getResponse 静默吞掉——用户看到空下拉框却无任何提示。
// 故在唯一的请求出口广播事件，由订阅方按「是否已登录」判定后跳转 401 页面。
// 此处只广播不复位、不跳转：apis 层不反向依赖 router 与 stores/user。
request.interceptors.response.use(
  (rsp) => rsp,
  (err: AxiosError) => {
    if (err.response?.status === HttpStatusCode.Unauthorized) {
      mitter.emit(AUTH_FAILED_EVENT);
    }
    return Promise.reject(err);
  },
);

export function getJwtHeader(): Record<string, string> {
  return {
    [X_JWT_HEADER_NAME]: localManager.getJwt() || "",
  };
}

export async function getResponse<Data>(
  request: AxiosPromise,
  success: (rsp: SuccessResponse<Data>, headers: BaseHeaders) => Promise<void>,
  failed?: (rsp: ErrorResponse, headers: BaseHeaders) => Promise<void>,
  noResponse?: (req: XMLHttpRequest) => Promise<void>,
  noRequest?: (msg: string) => Promise<void>,
  final?: () => Promise<void>,
): Promise<Status> {
  let status = Status.Success;
  try {
    const rawRsp = await request;
    if (rawRsp.status === HttpStatusCode.Ok) {
      success(rawRsp.data as SuccessResponse<Data>, rawRsp.headers);
    } else if (failed) {
      failed(rawRsp.data as ErrorResponse, rawRsp.headers);
      status = Status.Fail;
    }
  } catch (rawErr: unknown) {
    const err = rawErr as AxiosError;
    if (failed && err.response) {
      failed(err.response.data as ErrorResponse, err.response.headers);
      status = Status.Fail;
    } else if (noResponse && err.request) {
      noResponse(err.request as XMLHttpRequest);
      status = Status.NoResponse;
    } else if (noRequest && err.message) {
      noRequest(err.message as string);
      status = Status.NoRequest;
    }
  } finally {
    if (final) final();
    return status;
  }
}

export async function getSimpleResponse(
  request: AxiosPromise,
  success: (rsp: SuccessResponse<undefined>, headers: BaseHeaders) => Promise<void>,
  failed?: (rsp: ErrorResponse, headers: BaseHeaders) => Promise<void>,
  noResponse?: (req: XMLHttpRequest) => Promise<void>,
  noRequest?: (msg: string) => Promise<void>,
  final?: () => Promise<void>,
): Promise<Status> {
  return await getResponse<undefined>(request, success, failed, noResponse, noRequest, final);
}
