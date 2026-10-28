// Package config 承载 AI 各能力包共用的行为参数与工具元信息注册表：Params 以「参数键 → 值」
// 读取 agent_params 的运行时视图，ToolSpecs 登记内置工具的展示元信息与来源。本包只依赖
// internal/pkg，不引用任何业务能力包，故可被上层单向复用；参数键与出厂值由迁移 fixture 注入，
// 本包不硬编码提示词正文。
package config
