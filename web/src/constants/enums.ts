// 前端静态枚举值集合的唯一字面量来源（value 字面量集中收纳处）。
//
// 说明：
// - 后端驱动的枚举值集合（如 status / channelType / eventType / taskType / queue 等）
//   不在前端重复声明；其唯一来源是后端 Search.toml 配置与语言键 model.<x>.option.<value>，
//   前端仅经 enumValueTitle(modelName, value) 显示。
// - 本文件仅收纳前端静态、需自行构造选项的枚举值集合，例如 redis 数据结构下拉、policy effect
//   下拉等。新增此类枚举时，值集合统一登记于此，页面经 enumOptions 构造选项、经
//   enumValueTitle 展示，避免字面量散落。
import { enumOptions } from "@/utils/options";

// redis 数据结构枚举值集合（datasource 页 structField 下拉）。
export const DATASOURCE_REDIS_STRUCT = ["string", "hash", "list", "set", "zset"] as const;

// policy 权限 effect 枚举值集合（admin/group 页 effect 下拉）。
export const POLICY_EFFECT = ["allow", "deny"] as const;

// datasource 数据结构下拉选项（title=枚举值，渲染期经 enumValueTitle("datasource", ...) 翻译）
export const datasourceStructOptions = enumOptions(DATASOURCE_REDIS_STRUCT);

// policy effect 下拉选项（title=枚举值，渲染期经 enumValueTitle("policy", ...) 翻译）
export const policyEffectOptions = enumOptions(POLICY_EFFECT);
