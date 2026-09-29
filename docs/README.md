# zhongkui 文档

本项目文档按 [Diátaxis](https://diataxis.fr/) 框架组织为四类：教程（学习导向）、操作指南（任务导向）、参考（信息导向）与解释（理解导向）。

## 教程

- [快速开始：从零运行 zhongkui](tutorials/quick-start.md) — 从安装、配置到启动服务，完成一次最小可运行的部署。（角色：运维；来源 `zhongkui.com/zhongkui/internal/cmd`）
- [教程：看到你的第一条风险告警](tutorials/first-alert.md) — 编写一条 rego 规则、触发标签窗口统计，收到第一封风险告警。（角色：安全运营；来源 `zhongkui.com/zhongkui/internal/tagengine`）

## 操作指南

- [如何接入新的数据源插件](how-to/add-datasource-plugin.md) — 实现数据源接口并注册一个新的数据源插件。（角色：运维/研发；来源 `zhongkui.com/zhongkui/internal/datasource`）
- [如何新增通知渠道与模板](how-to/add-notify-channel.md) — 扩展通知渠道类型并维护对应的多语言模板。（角色：运维/研发；来源 `zhongkui.com/zhongkui/internal/notify`）
- [如何编写一次数据库迁移](how-to/write-migration.md) — 新增一次向前/向后兼容的数据库迁移脚本。（角色：运维/研发；来源 `zhongkui.com/zhongkui/internal/pkg/migrate`）
- [如何新增枚举类并生成绑定](how-to/add-enum-class.md) — 在枚举注册表中新增枚举类并生成前后端绑定。（角色：研发；来源 `zhongkui.com/zhongkui/internal/pkg/enum`）
- [如何新增一个管理端页面](how-to/frontend-add-page.md) — 说明新增管理端页面时的路由、视图与 API 接入步骤。（角色：研发；手写）

## 参考

后端全部包自动导出参考文档，共 101 篇：[reference/](reference/)

## 解释

- [采集架构：wire/XDP、快照与上报](explanation/capture-architecture.md) — 说明采集侧的 wire/XDP 抓包、快照生成与上报链路。（角色：运维/研发；来源 `zhongkui.com/zhongkui/internal/capture`）
- [网络指纹与 TCP 检测原理](explanation/network-detection.md) — 解释网络指纹的计算方式与 TCP 流检测原理。（角色：安全运营/研发；来源 `zhongkui.com/zhongkui/internal/network/detectors`）
- [LLM Agent + RAG 设计](explanation/agent-rag-design.md) — 说明 LLM Agent 编排与 RAG 检索的整体设计。（角色：研发；来源 `zhongkui.com/zhongkui/internal/ai`）
- [Agent 工具契约](explanation/agent-tools.md) — 约定 Agent 可调用工具的入参、出参与错误契约。（角色：研发；来源 `zhongkui.com/zhongkui/internal/ai/tools`）
- [数据源插件体系与 sqlite vtab](explanation/datasource-plugin.md) — 说明数据源插件体系与 sqlite 虚拟表机制的配合方式。（角色：研发；来源 `zhongkui.com/zhongkui/internal/pkg/sqlitevtab/vtab`）
- [异步任务与队列](explanation/async-worker.md) — 说明基于队列的异步任务调度与执行模型。（角色：运维/研发；来源 `zhongkui.com/zhongkui/internal/worker`）
- [流水线与调度](explanation/pipeline.md) — 说明流水线的阶段划分与调度协作方式。（角色：研发；来源 `zhongkui.com/zhongkui/internal/pkg/pipeline`）
- [管理端分层与视图 In/Out 解耦](explanation/manager-layering.md) — 说明管理端分层结构以及视图层 In/Out 解耦约束。（角色：研发；来源 `zhongkui.com/zhongkui/internal/manager`）
- [依赖注入与生命周期](explanation/dependency-injection.md) — 说明 samber/do 依赖注入三层模式与资源生命周期管理。（角色：研发；来源 `zhongkui.com/zhongkui/internal/provider`）
- [前端设计体系分层](explanation/frontend-design-system.md) — 说明前端样式体系的分层与设计令牌组织方式。（角色：研发；手写）
- [前端功能开关与不可用态](explanation/frontend-feature-flags.md) — 说明前端功能开关的接入方式与不可用态呈现约定。（角色：管理员/研发；手写）
- [XDP/BPF 设计说明](kernel/xdp_bpf_design.md) — 说明内核侧 XDP/BPF 程序的设计与数据交互方式。（角色：研发；手写）

## 按角色找文档

### 管理员

- [前端功能开关与不可用态](explanation/frontend-feature-flags.md) — 说明前端功能开关的接入方式与不可用态呈现约定。

### 安全运营

- [教程：看到你的第一条风险告警](tutorials/first-alert.md) — 编写一条 rego 规则、触发标签窗口统计，收到第一封风险告警。
- [网络指纹与 TCP 检测原理](explanation/network-detection.md) — 解释网络指纹的计算方式与 TCP 流检测原理。

### 运维

- [快速开始：从零运行 zhongkui](tutorials/quick-start.md) — 从安装、配置到启动服务，完成一次最小可运行的部署。
- [如何接入新的数据源插件](how-to/add-datasource-plugin.md) — 实现数据源接口并注册一个新的数据源插件。
- [如何新增通知渠道与模板](how-to/add-notify-channel.md) — 扩展通知渠道类型并维护对应的多语言模板。
- [如何编写一次数据库迁移](how-to/write-migration.md) — 新增一次向前/向后兼容的数据库迁移脚本。
- [采集架构：wire/XDP、快照与上报](explanation/capture-architecture.md) — 说明采集侧的 wire/XDP 抓包、快照生成与上报链路。
- [异步任务与队列](explanation/async-worker.md) — 说明基于队列的异步任务调度与执行模型。

### 研发

- [如何接入新的数据源插件](how-to/add-datasource-plugin.md) — 实现数据源接口并注册一个新的数据源插件。
- [如何新增通知渠道与模板](how-to/add-notify-channel.md) — 扩展通知渠道类型并维护对应的多语言模板。
- [如何编写一次数据库迁移](how-to/write-migration.md) — 新增一次向前/向后兼容的数据库迁移脚本。
- [如何新增枚举类并生成绑定](how-to/add-enum-class.md) — 在枚举注册表中新增枚举类并生成前后端绑定。
- [采集架构：wire/XDP、快照与上报](explanation/capture-architecture.md) — 说明采集侧的 wire/XDP 抓包、快照生成与上报链路。
- [网络指纹与 TCP 检测原理](explanation/network-detection.md) — 解释网络指纹的计算方式与 TCP 流检测原理。
- [LLM Agent + RAG 设计](explanation/agent-rag-design.md) — 说明 LLM Agent 编排与 RAG 检索的整体设计。
- [Agent 工具契约](explanation/agent-tools.md) — 约定 Agent 可调用工具的入参、出参与错误契约。
- [数据源插件体系与 sqlite vtab](explanation/datasource-plugin.md) — 说明数据源插件体系与 sqlite 虚拟表机制的配合方式。
- [异步任务与队列](explanation/async-worker.md) — 说明基于队列的异步任务调度与执行模型。
- [流水线与调度](explanation/pipeline.md) — 说明流水线的阶段划分与调度协作方式。
- [管理端分层与视图 In/Out 解耦](explanation/manager-layering.md) — 说明管理端分层结构以及视图层 In/Out 解耦约束。
- [依赖注入与生命周期](explanation/dependency-injection.md) — 说明 samber/do 依赖注入三层模式与资源生命周期管理。
- [前端设计体系分层](explanation/frontend-design-system.md) — 说明前端样式体系的分层与设计令牌组织方式。
- [前端功能开关与不可用态](explanation/frontend-feature-flags.md) — 说明前端功能开关的接入方式与不可用态呈现约定。
- [如何新增一个管理端页面](how-to/frontend-add-page.md) — 说明新增管理端页面时的路由、视图与 API 接入步骤。
- [XDP/BPF 设计说明](kernel/xdp_bpf_design.md) — 说明内核侧 XDP/BPF 程序的设计与数据交互方式。
