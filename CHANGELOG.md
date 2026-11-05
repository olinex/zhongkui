# 变更日志

本文件记录面向用户的显著变更。格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [0.1.0] - 2026-11-05

首个开发版本。接口、数据模型与配置在 `v1.0.0` 之前仍可能发生破坏性变更。

### 新增

- 流量采集：基于 XDP/wire 抓包，生成结构化流量快照并经 gRPC 上报。
- 协议与指纹：解析网络层与 HTTP 传输模型，提取可用于匹配的指纹。
- 标签规则：以 Rego 规则打标，支持历史版本与回滚。
- 风险事件：按时间窗口聚合标签命中并产生风险事件。
- 通知：邮件、Webhook 等渠道，支持命名空间级模板与发送去重。
- 多租户：命名空间隔离配置与数据，系统命名空间承载公共知识。
- 管理端与 worker：HTTP API、异步任务与配置广播。
- 内置 AI 助手：会话、长期记忆、知识库检索、工具调用与人工审批。
- 文档体系：`docs/generated/` 与 `docs/curated/` 来源隔离，提供 VitePress 站点。
- 版本管理：SemVer + annotated tag，`make release` 统一发布流程。

[0.1.0]: https://gitee.com/olinex/zhongkui/releases/tag/v0.1.0
