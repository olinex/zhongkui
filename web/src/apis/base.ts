import axios, { AxiosError, type AxiosPromise, HttpStatusCode } from "axios";

import { useAppStore } from "@/stores/app";
import { localManager } from "@/stores/local";
import { HttpHeader } from "@/types";

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
