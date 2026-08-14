import { AxiosHeaders, type RawAxiosRequestHeaders } from "axios";

export const X_JWT_HEADER_NAME: string = "X-Jwtoken";

export interface BaseResponse {
  code: number;
  summary: string;
}

export type BaseHeaders = RawAxiosRequestHeaders | AxiosHeaders;

export interface SuccessResponse<Data> extends BaseResponse {
  data: Data;
}

export interface ErrorResponse extends BaseResponse {
  details?: string | Record<string, string>;
}

export enum Status {
  Success,
  Fail,
  NoResponse,
  NoRequest,
}

export enum CustomCode {
  CodeUnknown = 1000,
  // basic error code
  CodeUserNotExist = 1001,
  CodeBadBody = 1002,
  CodeBadQuery = 1003,
  CodeBadUrlParam = 1004,
  CodeBadJwtToken = 1005,
  CodeTimeoutJwtToken = 1006,
  CodeAuthorizeBlankFullPath = 1007,
  CodeAuthorizeFailed = 1008,
  CodeNamespaceNotExist = 1009,
  // business error code
  CodeUsernamePasswordCheckFailed = 1100,
  CodeOldPasswordCheckFailed = 1101,
  CodeUsernameOrEmailAlreadyExist = 1102,
}
