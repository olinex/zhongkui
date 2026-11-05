// DetailField 详情字段渲染单元：key/label 来自模型字段元信息，
// kind 决定值渲染分支（plain 普通文本 / crypto 加密 / s3 对象存储），length 为 s3 长度，
// type 为字段渲染类型（BackendDataType），plain 分支经 AdaptiveCell 按列表页方式渲染。
import type { BackendDataType } from "@/types";

export interface DetailField {
  key: string;
  label: string;
  display: string;
  kind: "plain" | "crypto" | "s3";
  length: number;
  value: unknown;
  type: BackendDataType;
}
