// 周期性报告接口类型：与后端 internal/manager/views/ai/report.go 的 In/Out 结构一一对应
// （JSON 键为 camelCase，字段名与后端保持一致）。

import type { SearchIn } from "@/components/@types/table";

// PeriodicReportSkillInputField 报告技能的输入字段描述，供计划表单按类型渲染输入控件。
export interface PeriodicReportSkillInputField {
  name: string;
  // type 取值至少包含 date / number / string / enum / boolean；name=nowTime 的字段类型为 date，
  // 由运行时注入，计划表单不得渲染或提交
  type: string;
  required: boolean;
  description: string;
}

// PeriodicReportSkillOption 可作为报告来源的技能选项。
export interface PeriodicReportSkillOption {
  id: number;
  name: string;
  description: string;
  // fixedFields 技能固定参数，值保留原始 JSON 类型（字符串/数字/布尔/对象/数组）
  fixedFields: Record<string, unknown>;
  inputFields: PeriodicReportSkillInputField[];
}

export interface PeriodicReportSkillOptionListOut {
  items: PeriodicReportSkillOption[];
}

// PeriodicReportPlanItem 报告计划列表项。
export interface PeriodicReportPlanItem {
  id: number;
  name: string;
  description: string;
  skillID: number;
  skillName: string;
  cronSpec: string;
  // inputValues 技能输入与计划偏置的实参，值保留原始 JSON 类型
  inputValues: Record<string, unknown>;
  enabled: boolean;
  retentionDays: number;
  // expiresAt 为空表示长期有效
  expiresAt?: string | null;
  // nextRunAt 为空表示待分发器计算
  nextRunAt?: string | null;
  createdAt: string;
  updatedAt: string;
}

// PeriodicReportPlanListIn 报告计划列表查询入参：通用 SearchIn（page/pageSize/orderBy/filterBy/fields）；
// filterBy 支持 keyword（计划名称模糊，action 取 ctn/ictn）与 enabled（启停状态等值，action 取 eq）。
export type PeriodicReportPlanListIn = SearchIn;

export interface PeriodicReportPlanListOut {
  total: number;
  items: PeriodicReportPlanItem[];
}

// PeriodicReportPlanCreateIn 创建 / 更新报告计划的输入（后端两个接口的字段集合一致）。
export interface PeriodicReportPlanCreateIn {
  name: string;
  description: string;
  skillID: number;
  cronSpec: string;
  // inputValues 输入字段实参与固定字段偏置，值须与技能字段的原始 JSON 类型一致
  inputValues: Record<string, unknown>;
  retentionDays: number;
  expiresAt?: string | null;
}

export type PeriodicReportPlanUpdateIn = PeriodicReportPlanCreateIn;

export interface PeriodicReportPlanToggleIn {
  enabled: boolean;
}

// PeriodicReportRunItem 报告运行记录列表项。
export interface PeriodicReportRunItem {
  id: number;
  planID: number;
  // status 取值来自枚举类 periodicReportRunStatus（running / success / failed）
  status: string;
  title: string;
  objectKey: string;
  sizeBytes: number;
  error: string;
  triggerAt: string;
  // sessionID 本次运行专属的 Agent 会话 ID，0 表示尚未创建
  sessionID: number;
  startedAt?: string | null;
  finishedAt?: string | null;
  createdAt: string;
}

// PeriodicReportRunListIn 报告运行记录列表查询入参：通用 SearchIn；
// filterBy 支持 planID（所属计划等值）与 status（运行状态枚举等值）。
export type PeriodicReportRunListIn = SearchIn;

export interface PeriodicReportRunListOut {
  total: number;
  items: PeriodicReportRunItem[];
}

// PeriodicReportRunDetailOut 运行详情：列表项 + 解密后的正文原文（统一 JSON 信封，
// 展示前须用 periodicReportEnvelopeContent 取出 content 字段的 Markdown）。
export type PeriodicReportRunDetailOut = PeriodicReportRunItem & { content: string };

// PeriodicReportWatchPlan 可关注计划列表项（watched 由后端按当前用户关注关系回填）。
export interface PeriodicReportWatchPlan {
  id: number;
  name: string;
  description: string;
  enabled: boolean;
  watched: boolean;
  createdAt: string;
  updatedAt: string;
}

// PeriodicReportWatchListIn 关注计划候选列表查询入参：通用 SearchIn；
// filterBy 支持 keyword（计划名称模糊）与 enabled（启停状态等值）。
export type PeriodicReportWatchListIn = SearchIn;

export interface PeriodicReportWatchListOut {
  total: number;
  items: PeriodicReportWatchPlan[];
}

export interface PeriodicReportWatchSetIn {
  planID: number;
  watched: boolean;
}

// PeriodicReportFeedItem 报告中心信息流条目（仅关注计划的成功运行记录）。
export interface PeriodicReportFeedItem {
  runID: number;
  planID: number;
  planName: string;
  title: string;
  status: string;
  triggerAt: string;
  createdAt: string;
}

// PeriodicReportFeedListIn 报告中心信息流查询入参：通用 SearchIn（后端固定按关注计划与成功状态过滤）。
export type PeriodicReportFeedListIn = SearchIn;

export interface PeriodicReportFeedListOut {
  total: number;
  items: PeriodicReportFeedItem[];
}
