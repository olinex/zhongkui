// 动态枚举注册表：集中管理“后端注册表驱动 + i18n option 键决定展示名”的动态枚举元信息，
// 与静态枚举（constants/enums.ts）相对。
//
// 背景：
// - 一类动态枚举（通知渠道类型 / 数据源类型 / 模板事件 / 模板来源）的“值集合”由后端注册表
//   （/types、/events）动态返回，前端不在 constants/enums.ts 重复声明字面量；
// - 其“展示名”统一收敛到 i18n 命名空间 model.<modelName>.option.<value>，未配置翻译时由
//   enumValueTitle 回退为枚举值本身。
// 本文件以单一注册表（DynamicEnums）声明这些枚举的 i18n 归属（modelName 与嵌套 optionPath），
// 页面据此经 dynamicEnumTitle / dynamicEnumOptions 构造展示名与下拉选项，避免散落硬编码
// model.*.option.* 键与嵌套命名空间前缀。

import { enumValueTitle } from "@/utils/options";

// DynamicEnumSpec 动态枚举条目元信息：
// - modelName 定位 i18n 命名空间 model.<modelName>.option.*；
// - optionPath 为嵌套 option 命名空间前缀（如 "event" → model.<m>.option.event.<value>，
//   当同一模型存在多个枚举字段、option 键需要再加一层字段区分时使用）。
export interface DynamicEnumSpec {
  modelName: string;
  optionPath?: string;
}

// optionValueOf 依据条目构造展示名所需的完整 value 片段：
// 无嵌套命名空间 = value 本身；有嵌套（如 event）= "event.<value>"。
const optionValueOf = (spec: DynamicEnumSpec, value: string | number): string =>
  spec.optionPath ? `${spec.optionPath}.${value}` : String(value);

// dynamicEnumTitle 动态枚举展示名：经统一入口 enumValueTitle 翻译
// （定位 model.<modelName>.option.<value>，未配置翻译时回退为枚举值本身）。
export const dynamicEnumTitle = (spec: DynamicEnumSpec, value: string | number): string =>
  enumValueTitle(spec.modelName, optionValueOf(spec, value));

// dynamicEnumOptionKey 动态枚举展示名对应的原始 i18n 键字符串（model.<modelName>.option.<value>）。
// 供 SearchTable 筛选 OPTIONS（list/options 接口）等需要在渲染期由 $t 统一翻译的消费方使用，
// 与 dynamicEnumTitle 返回已翻译文本不同，本函数返回键本身。
export const dynamicEnumOptionKey = (spec: DynamicEnumSpec, value: string | number): string =>
  `model.${spec.modelName}.option.${optionValueOf(spec, value)}`;

// dynamicEnumOptions 由后端动态枚举条目数组构造下拉选项：
// - value 取 item[valueKey]（后端枚举值本身，供 v-select 提交与筛选匹配，统一为字符串）；
// - title 经 dynamicEnumTitle 翻译（依赖 i18n 渲染；未配置翻译时回退为枚举值本身）。
// 后端动态枚举值在本项目内均为字符串键（如渠道类型 / 数据源类型 / 模板事件），故 value 直接定为 string。
export const dynamicEnumOptions = <I extends Record<string, unknown>>(
  spec: DynamicEnumSpec,
  items: I[],
  valueKey: keyof I & string,
): Array<{ title: string; value: string }> =>
  items.map((item) => ({
    title: dynamicEnumTitle(spec, item[valueKey] as string | number),
    value: String(item[valueKey]),
  }));

// DynamicEnums 全项目动态枚举注册表：新增动态枚举时在此登记其 modelName / optionPath，
// 页面经 dynamicEnumTitle / dynamicEnumOptions 消费，不得在页面内硬编码 option 键。
export const DynamicEnums = {
  // 通知渠道类型：model.notificationChannel.option.<type>
  channelType: { modelName: "notificationChannel" } as DynamicEnumSpec,
  // 数据源类型：model.datasource.option.<type>
  datasourceType: { modelName: "datasource" } as DynamicEnumSpec,
  // 通知模板事件：model.notificationTemplate.option.event.<event>（嵌套命名空间）
  notificationEvent: { modelName: "notificationTemplate", optionPath: "event" } as DynamicEnumSpec,
  // 通知模板来源：model.notificationTemplate.option.source.<source>（嵌套命名空间）
  notificationSource: {
    modelName: "notificationTemplate",
    optionPath: "source",
  } as DynamicEnumSpec,
} as const;
