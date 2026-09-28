import { AxiosHeaders, type RawAxiosRequestHeaders } from "axios";

// X_JWT_HEADER_NAME 携带 JWT 的请求头名称。
export const X_JWT_HEADER_NAME: string = "X-Jwtoken";

// BaseResponse 接口响应的公共基础结构（统一状态码与摘要）。
export interface BaseResponse {
  code: number;
  summary: string;
}

// BaseHeaders 请求头类型（axios 原始头或 AxiosHeaders 实例）。
export type BaseHeaders = RawAxiosRequestHeaders | AxiosHeaders;

// SuccessResponse 成功响应：在基础结构上附加业务数据。
export interface SuccessResponse<Data> extends BaseResponse {
  data: Data;
}

// ErrorResponse 失败响应：在基础结构上附加可选的错误详情。
export interface ErrorResponse extends BaseResponse {
  details?: string | Record<string, string>;
}

// Status 前端请求状态枚举。
export enum Status {
  Success,
  Fail,
  NoResponse,
  NoRequest,
}

// CustomCode 业务自定义错误码，区分基础错误码与业务错误码两段。
export enum CustomCode {
  CodeUnknown = 1000,
  // 基础错误码
  CodeUserNotExist = 1001,
  CodeBadBody = 1002,
  CodeBadQuery = 1003,
  CodeBadUrlParam = 1004,
  CodeBadJwtToken = 1005,
  CodeTimeoutJwtToken = 1006,
  CodeAuthorizeBlankFullPath = 1007,
  CodeAuthorizeFailed = 1008,
  CodeNamespaceNotExist = 1009,
  // 业务错误码
  CodeUsernamePasswordCheckFailed = 1100,
  CodeOldPasswordCheckFailed = 1101,
  CodeUsernameOrEmailAlreadyExist = 1102,
  CodeFrontendConfigConflict = 1108,
  CodeFrontendConfigSyncDisabled = 1109,
}
