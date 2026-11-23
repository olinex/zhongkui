# 文档体系架构与执行规划

> 状态：已校对
> 责任人：@olinex
> 依据：`internal/docs`、`internal/manager/views/catalog.go`
> 范围：仓库文档、文档生成流水线、AI 上下文、开源门面与文档治理
> 结论：现有「注释抽取 + Diátaxis」机制可靠，但缺少信息架构、事实与意图的分工、面向 AI 的结构化上下文。本方案在其上补三层——大纲、事实投影、AI 能力层——并把开源门面与质量门禁纳入同一条流水线。全部文档以 Markdown 为唯一载体，中文为权威语言。
> 迁移说明：本页已迁移至 `docs/curated/explanation/`，由人持有并审校，生成器不会覆盖。
> 进度：P0 已完成；P1 的 VitePress 站点与五类参考投影（接口/数据表/枚举/配置/CLI）已落地，链接与模板守卫待补齐；P2 的能力声明与按需能力检索、`knowledge_search` 分区过滤（接口详情复用 `native_api` RAG）、只读 `api_invoke`、`schema`/`capability` RAG 分区与 `zki sync` 均已落地；P3 待执行。

## 1. 背景与问题

### 1.1 现状

| 能力 | 当前实现 | 评价 |
| --- | --- | --- |
| 注释抽取 | `internal/docs` 用 gomarkdoc 把包注释渲染为叙述页 | 机制可靠，但只有包级视角 |
| 文档注册表 | `internal/docs/registry.go` 唯一登记 20 条条目 | 是平铺清单，不是大纲 |
| 分类 | tutorial / how-to / reference / explanation | 分类正确，但与导航分区混为一谈 |
| 参考文档 | 自动导出 102 篇包符号参考 | 占文档总量 83%，人类不可顺读 |
| 接口目录 | 约 209 处 `RegisterApi/RegisterListApi` 声明，含入参/出参/字段/枚举 | 已是高质量结构化事实源 |
| AI 知识同步 | `zki sync docs`（叙述页）、`zki sync apis`（接口知识） | 只覆盖概念与接口，缺数据结构与场景 |
| 注释门禁 | `scripts/check.sh lint`（commentlint） | 只保证「有注释」，不保证「注释有用」 |
| 文档导出 | `zki create docs` 幂等生成叙述页、参考页与根 `README.md` | 缺 `--check`，CI 无法发现漂移 |
| 手写文档 | 前端设计体系、功能开关、Agent 架构、XDP 等 5 篇 | 质量高，但未成体系 |

### 1.2 根因

| 问题 | 直接现象 | 根因 |
| --- | --- | --- |
| AI 不知道何时调哪个接口、字段什么意思 | 只有 `knowledge_search` 一条语义通道；接口文档只有一行用途 | 缺能力发现索引、精确 schema 查询工具、内部数据模型入 RAG、调用工具 |
| 注释生成文档散乱、没有大纲 | 根 `README.md` 是平铺索引；教程/指南被塞进包注释；页面间几乎无链接 | 注册表只有类别没有分区/顺序/层级；教程与指南的载体错配 |
| 生成文档与手写文档混放、互相覆盖 | `docs/explanation/` 同时容纳生成页与手写页；根 README 被生成器覆盖 | 缺少按来源隔离的目录边界与写入保护 |
| 不符合开源标准、不引导用户、不好维护 | 无 LICENSE/CONTRIBUTING/SECURITY/CHANGELOG/CI；README 不是门面；无站点与搜索 | 把「文档生成」当成了「文档体系」，缺少门面、治理与门禁 |

### 1.3 关键判断

1. 包注释适合写「这个包是什么、边界在哪、有什么不变量」，不适合承载「用户如何完成一个任务」。当前 6 篇教程/指南由包注释生成，任务型知识被锁在实现包里。
2. 接口声明已经是很好的事实源，但人类没有入口、AI 没有地图；约 209 条声明只进了 RAG。
3. `internal/models` 下约 99 个结构体没有表/字段文档，字段语义无法被 AI 稳定获取。
4. 根 `README.md` 被生成器占用，第一次访问仓库的人看不到项目是什么、怎么跑、能做什么。
5. 生成物与人工产物同目录是覆盖事故的根源；仅靠「禁止手改」约定不够，必须用目录边界与写入保护在机制上隔离。

## 2. 目标与原则

### 2.1 目标

| 目标 | 可验证结果 |
| --- | --- |
| G1 人类可读 | 新访客 10 分钟内完成 Linux 开发环境部署或任一部署教程；任意事实最多 2 次点击可达；每页都有上一级与延伸阅读 |
| G2 AI 可用 | AI 能回答「该调哪个接口、参数是什么、返回哪个实体、字段什么意思」，并能以用户身份执行只读查询 |
| G3 可维护 | 同一事实只有一个来源；生成物漂移在 CI 被拦截；新增页面必须挂到大纲上 |
| G4 开源可用 | 具备 LICENSE、README 门面、贡献指南、安全策略、变更日志、CI 与文档站点 |

### 2.2 原则

1. **事实从声明生成，意图由人策展。** 接口签名、字段、枚举、配置键、表结构、CLI 参数一律生成；教程、指南、架构权衡、术语解释一律策展（AI 起草、人审校）。
2. **按来源隔离目录。** 流水线生成物只写 `docs/generated/`；策展文档只写 `docs/curated/`；生成器在机制上无法写入策展目录。
3. **大纲先行。** 注册表同时是导航树与生成清单；没有挂到大纲上的页面不允许存在。
4. **单一事实来源。** 策展文档不得复制字段表、签名或枚举值，只能链接生成页的稳定锚点。
5. **人机同源。** 人类文档与 AI 上下文从同一批声明派生，只是投影不同；禁止为 AI 另维护一份说明。
6. **Markdown 唯一载体。** 所有文档以 `.md` 提交与评审；站点只做渲染，不引入第二份 HTML/PDF 内容。

### 2.3 非目标

- 不做中英双语：中文为唯一文档语言；不维护英文版本文档。
- 不把 102 篇包符号参考改写为叙事；它们作为「查得到」的底层事实保留。
- 不把逐包参考页整篇灌入 RAG；只把面向 AI 的原子事实入 RAG。
- 本期不开放任意接口调用；`api_invoke` 先只读、白名单、可审计。
- 不让 AI 起草的策展文档跳过人工校对直接发布。

## 3. 已定决策

| 决策 | 结论 | 落地约束 |
| --- | --- | --- |
| 许可证 | Apache-2.0 | 根目录 `LICENSE`；README 标注；第三方依赖与 `third_party/` 补丁副本做许可证合规清单 |
| 托管与 CI | Gitee 为主仓，GitHub 为只读镜像 | 内容不依赖平台；`scripts/ci.sh` 是唯一检查入口；GitHub 工作流只做薄封装；镜像同步由定时任务完成 |
| `api_invoke` 执行侧 | 前端持用户凭据执行 | 后端不代理请求，只做白名单校验、参数校验、审核分级与审计；安全边界仍是接口自身的鉴权中间件 |
| 文档站点 | VitePress | `docs/.vitepress/`；侧边栏由 `nav.json` 生成；本地搜索；编辑链接 |
| 文档形态 | Markdown | 源文件与生成物均为 `.md`；站点只渲染；不产出 HTML/PDF 文档交付物 |
| 文档语言 | 中文唯一权威 | 不维护英文版本文档；生成物、策展文档、提交与注释均为中文 |
| 目录布局 | 按来源隔离 | `docs/generated/` 仅生成器写入；`docs/curated/` 仅人工写入；两条边界由守卫与写入保护双重保证 |
| 策展文档来源 | AI 起草 + 人工审校 | 统一模板与元信息头；AI 必须标注依据；状态从「草稿」变为「已校对」才可发布 |

## 4. 目标架构

### 4.1 双源四层

```text
声明源（代码，唯一事实）                策展源（人，唯一意图）
├── 包注释                             ├── 系统总览 / 领域模型 / 术语表
├── RegisterApi / RegisterListApi      ├── 教程 / 操作指南
├── GORM Model 结构体与 tag            ├── 能力剧本（何时调什么）
├── 枚举注册表                         └── ADR 决策记录
├── 配置结构体 comment tag
└── Cobra 命令树
        │                                      │
        ├──────────────┬───────────────────────┤
        ▼              ▼                       ▼
  参考事实页        AI 上下文              人类文档站
（接口/表/枚举/    （能力地图 +          （Markdown + VitePress）
  配置/CLI/包）      RAG + 工具）
        │
        └──► 社区门面（README/贡献/安全/变更日志）
```

### 4.2 目录布局：来源隔离

```text
docs/
├── index.md                 # 生成：总目录与阅读路径（DO NOT EDIT）
├── nav.json                 # 生成：站点侧边栏与链接检查的机器可读大纲
├── generated/               # 生成器独占，人工禁止修改
│   ├── explanation/         # 包注释派生的概念页
│   └── reference/
│       ├── api/             # 接口参考（按业务域）
│       ├── schema/          # 数据表参考（按业务域）
│       ├── enums.md
│       ├── config.md
│       ├── cli.md
│       └── internal/        # gomarkdoc 包符号参考
├── curated/                 # 人工独占，生成器禁止写入
│   ├── tutorials/
│   ├── how-to/
│   ├── explanation/         # 总览、领域模型、术语、架构深入
│   ├── capabilities/        # 能力剧本
│   ├── adr/
│   └── kernel/              # 内核侧手写设计
├── templates/               # 页面模板与 AI 起草须知
├── static/                  # 图片等静态资源
├── package.json             # 文档站点依赖与脚本
└── .vitepress/              # VitePress 配置与主题（非文档内容）
```

| 目录/文件 | 写入方 | 内容 | 保护机制 |
| --- | --- | --- | --- |
| 根 `README.md` | 人（AI 起草 + 审校） | 产品门面 | 生成器根目录白名单不含它 |
| `docs/index.md`、`docs/nav.json` | 生成器 | 总目录、导航大纲 | `DO NOT EDIT`；参与 `--check` |
| `docs/generated/**` | 生成器 | 概念页与全部参考事实页 | 生成器独占；每页带 `DO NOT EDIT` |
| `docs/curated/**` | 人 | 教程、指南、解释、能力、ADR | 生成器写入前校验根目录；守卫测试 |
| `docs/templates/**` | 人 | 模板与起草须知 | 不进入站点导航 |
| `docs/static/**` | 人 | 图片与静态资源 | 只被引用，不被生成 |
| `docs/.vitepress/**` | 人 | VitePress 配置与主题 | 不参与文档生成 |

规则：**先来源、后分类**。Diátaxis 分类只作为 `docs/generated/` 与 `docs/curated/` 下的二级目录，导航分区则完全独立于文件系统。

**AI 起草的文档归 `curated/`。** 是否「AI 生成」不看起草者，而看所有权：由流水线从声明派生、可随时重算的页面归 `generated/`；由人持有、经人审校、需要人负责的页面归 `curated/`，即使初稿由 AI 完成。

### 4.3 文档类型与投射

| 类型 | Diátaxis | 来源 | 目标位置 | AI 摄入 |
| --- | --- | --- | --- | --- |
| 产品门面 | overview | 策展 | 根 `README.md` | 否 |
| 系统总览/领域模型/术语 | explanation | 策展 | `docs/curated/explanation/` | `curated` |
| 概念深入 | explanation | 包注释生成 | `docs/generated/explanation/` | `generated` |
| 教程 | tutorial | 策展 | `docs/curated/tutorials/` | `curated` |
| 操作指南 | how-to | 策展 | `docs/curated/how-to/` | `curated` |
| 接口参考 | reference | 投影 | `docs/generated/reference/api/` | `native_api` |
| 数据表参考 | reference | 投影 | `docs/generated/reference/schema/` | `schema` |
| 枚举/配置/CLI 参考 | reference | 投影 | `docs/generated/reference/` | `schema` / `curated` |
| 包符号参考 | reference | gomarkdoc | `docs/generated/reference/internal/` | 否 |
| 能力剧本 | explanation | 策展 | `docs/curated/capabilities/` | `capability` |
| ADR | explanation | 策展 | `docs/curated/adr/` | 否 |
| 变更日志 | release | 生成 | `CHANGELOG.md` | 否 |

### 4.4 信息架构

```text
首页（README）
├── 开始使用
│   ├── Linux 开发环境部署
│   ├── Linux 二进制部署
│   ├── Docker Compose 部署
│   ├── Kubernetes Helm 部署
│   └── 教程：看到你的第一条风险告警
├── 核心概念
│   ├── 系统总览：组件、数据流、部署拓扑
│   ├── 领域模型：实体、关系、生命周期、多租户
│   ├── 术语表
│   ├── 采集与检测
│   ├── 标签与告警
│   ├── 数据源与 ETL
│   └── AI Agent
├── 操作指南
│   ├── 部署与升级
│   ├── 数据源 / 通知 / 迁移 / 枚举
│   ├── 前端页面扩展
│   └── 故障排查
├── 参考
│   ├── 接口索引（按业务域）
│   ├── 数据表参考（按域/表）
│   ├── 枚举参考
│   ├── 配置参考
│   ├── CLI 参考
│   └── Go 包参考（折叠）
├── AI 能力
│   ├── 能力地图（生成）
│   ├── 能力剧本：查流量 / 查告警 / 查标签 / 查数据源
│   └── 接口调用契约与审核
└── 贡献与治理
    ├── 贡献指南
    ├── 安全策略
    ├── 发布与版本
    └── ADR
```

导航分区由注册表 `Section` 决定，与目录无关；同一分区的页面可以来自 `generated/` 与 `curated/` 两个来源，但不同来源的文件永不共用路径。

### 4.5 页面模板

| 模板 | 固定小节 |
| --- | --- |
| 概念页 | 它解决什么问题 → 心智模型 → 组件与数据流 → 边界与不变量 → 常见误解 → 延伸阅读 |
| 教程 | 你将完成什么 → 前置条件 → 步骤（每步含验证） → 成功标志 → 下一步 |
| 操作指南 | 适用场景 → 前置检查 → 步骤 → 验证 → 回滚/排障 → 相关参考 |
| 接口页 | 用途 → 何时使用/不使用 → 请求 → 请求示例 → 入参 → 出参 → 字段语义 → 错误码 → 相关能力 |
| 数据表页 | 表用途 → 归属域 → 字段表（列/类型/可空/默认/含义） → 索引/外键 → 读写方 → 关联能力 |
| 能力剧本 | 用户意图 → 触发语 → 前置条件 → 调用序列 → 参数要点 → 返回实体与字段 → 权限/副作用/审核级别 → 失败处理 |
| ADR | 背景 → 决策 → 备选方案 → 后果 → 失效条件 |

模板文件存放于 `docs/templates/`，每份包含两段：

1. **起草须知**（AI 与人都读）：本节写清该类型必须回答的问题、必须引用的声明来源、禁止出现的复制内容。定稿前必须删除。
2. **页面骨架**：与上表一致的固定小节，直接填写。

## 5. 关键机制

### 5.1 注册表即大纲

`internal/docs/registry.go` 的 `Entry` 扩展：

```go
type Entry struct {
    Category Category // 保留：Diátaxis 语义
    Section  Section  // 新增：导航分区
    Order    int      // 新增：分区内顺序
    Owner    string   // 新增：文档责任人
    Source   Source   // 新增：comment | curated | projection
    Root     Root     // 新增：generated | curated | root（由 Source 唯一确定）
    AiIngest string   // 新增：curated | native_api | schema | capability | 空
    // 现有：Title/Summary/Roles/Scene/Package/Output/NarrativeOnly/Overview
}
```

守卫扩展：

- 每条条目有合法 `Section`、唯一 `(Section, Order)`、非空 `Owner`。
- `Source` 与 `Root` 必须一致：`comment`/`projection` 只能落 `generated/`，`curated` 只能落 `curated/` 或根 README。
- `docs/curated/**` 中每个 `.md` 都必须存在注册条目；`docs/generated/**` 只能由注册条目或包发现产生（孤儿页按根分别检查）。
- 生成条目必须声明 `AiIngest` 分区；策展条目面向 AI 时同样必须声明。
- 生成标题仍唯一，作为 RAG 稳定键；`docs/nav.json` 逐字节幂等。

### 5.2 生成边界与覆盖保护

这部分是「不互相覆盖」的机制保证，不依赖约定：

1. **写入根白名单**：生成器只能写 `docs/generated/`、`docs/index.md`、`docs/nav.json`、`CHANGELOG.md`。路径归一化后若逃逸出白名单，直接返回错误，不落盘。
2. **覆盖前身份校验**：目标文件已存在且首行不带 `DO NOT EDIT` 时拒绝覆盖；即使注册表路径配置错误，也不会冲掉策展文档。
3. **策展目录零写入断言**：CI 在生成后执行 `git diff --exit-code -- docs/curated`，任一改动即失败。
4. **`--check` 只比对生成域**：`zki create docs --check` 导出到临时目录，只比较白名单内路径，避免把策展文档纳入漂移判定。
5. **单向引用**：生成页可以链接策展页；策展页可以链接生成页；两类页面不共享文件名与锚点命名空间。

### 5.3 事实投影

| 投影 | 来源 | 输出 | 说明 |
| --- | --- | --- | --- |
| 接口索引 | `views.ApiCatalog()` | `docs/generated/reference/api/` | 按路径前缀归域；人类可读；与 RAG 同一渲染器 |
| 数据表 | `internal/models/**` AST + GORM NamingStrategy | `docs/generated/reference/schema/` | 表用途取结构体注释；字段取字段注释与 gorm tag；不臆造关系 |
| 枚举 | `internal/pkg/enum` 注册表 | `docs/generated/reference/enums.md` | 类名 + 值 + 中英 label |
| 配置 | 配置结构体 `comment` tag / `describe config` | `docs/generated/reference/config.md` | 键、类型、默认值、含义、敏感标记 |
| CLI | Cobra 命令树 | `docs/generated/reference/cli.md` | 命令、参数、示例、退出码 |
| 包符号 | 现有 gomarkdoc | `docs/generated/reference/internal/` | 保持现状；导航折叠 |

硬约束：生成器只输出可验证事实，不写建议；同一事实不在两张投影重复定义；所有投影逐字节幂等并参与 `--check`。

### 5.4 AI 上下文五级

| 层级 | 内容 | 生成方式 | 进入模型 | 预算 |
| --- | --- | --- | --- | --- |
| C0 能力地图 | 域 → 意图 → 接口 → 读写属性 → 关联实体 | 视图层能力声明 → `capability` 分区 | 按需检索 | 单次 |
| C1 接口精确描述 | 单接口完整入参/出参/字段/错误 | `knowledge_search` + `format=native_api` | 按需 | 单次 |
| C2 接口执行 | 实际发起请求 | `api_invoke` 客户端工具 | 按需 | 受结果上限约束 |
| C3 语义检索 | 概念、表、能力剧本 | `zki sync docs/apis/schema/capabilities` | 按需 | 现有 TopK |
| C4 评测 | 查询 → 期望能力/接口/字段 | 待实现（按能力回归基线） | CI/夜间 | 不进入模型 |

**C0 能力地图**：能力经 `views.WithCapability` 随接口在视图层声明，由 `views.CapabilityItems()` 派生，覆盖全部只读接口；敏感、诊断或编辑器辅助接口标为 `Invokable=false`（仅参考，`api_invoke` 拒绝执行）。地图写入 RAG 的 `capability` 分区，模型判断平台能力时用 `knowledge_search` + `format=capability` 按需检索，系统提示只保留一句固定指引，不再把整张地图常驻上下文。

**C1 接口精确描述**：不另建接口快照或专用工具。`zki sync apis` 已把注册期接口目录的完整用途与入参/出参字段表写入 `native_api` 分区；`knowledge_search` 增加可选 `format` 参数，模型需要精确字段时传 `format=native_api`，在既有混合检索管线上按 `knowledge_documents.format` 过滤即可，避免维护第二份事实来源。

**C2 `api_invoke`**：

- 客户端工具，由前端在用户自身凭据下执行；后端只做白名单、参数、审核与审计，不代理请求。
- 首期只读（GET 与只读 POST 列表/搜索）；写操作必须显式登记并走审核。
- 复用现有 `auditLevel × reviewValve` 机制；审计记录用户、会话、能力、方法、路径、参数摘要、状态、耗时、结果大小。
- 只允许 `/api/v1/` 路径；禁止任意 URL 与越权命名空间；响应受 `agent.maxToolResultBytes` 与超时约束。

**C3 RAG 分区**：分区名与目录来源一一对应，便于核对：

| 分区 | 内容来源 | 目录 | 同步命令 |
| --- | --- | --- | --- |
| `generated` | 包注释派生的概念页 | `docs/generated/explanation/` | `zki sync docs` |
| `curated` | 策展教程/指南/总览/术语 | `docs/curated/` | `zki sync docs --curated` |
| `native_api` | 单接口知识 | 接口声明 | `zki sync apis` |
| `schema` | 单表知识 | 模型声明 | `zki sync schema` |
| `capability` | 单能力剧本 | `docs/curated/capabilities/` | `zki sync capabilities` |

所有分区仍写入 `base.SystemNamespace`，经 `NamespaceScope` 对所有租户可见。稳定标题：概念=条目标题；接口=`原生接口 METHOD /path`；表=`数据表 表名`；能力=`能力 域/意图`。逐包参考页继续不入 RAG。

**C4 评测**：当前未实现。后续按能力分别建立回归评测基线，覆盖查流量、查告警、查标签规则、查数据源、查用户权限与解释字段；指标为能力 Top-1、Top-3、字段正确率与无依据拒答率。

### 5.5 策展文档的 AI 起草流程

目标：让 AI 承担资料搜集、结构组织与初稿撰写，人只做判断与校对。

#### 起草流程

1. **人定意图**：确定页面类型、目标读者、要回答的问题、所属 `Section` 与 `Owner`，选择 `docs/templates/` 中的模板。
2. **AI 取证**：读取相关声明源（包注释、`ApiCatalog()`、模型结构体、枚举、配置、CLI 树）与既有生成页，先列「依据清单」，再动笔。
3. **AI 起草**：按模板骨架输出到 `docs/curated/<分类>/`，并填写元信息头；不确定处写 `<!-- TODO(owner): 待确认 -->`，不得臆造。
4. **人审校**：核对事实、删除 TODO、补充判断与权衡，把状态改为「已校对」，填写 `Owner`。
5. **登记与校验**：注册条目（`Source: curated`），运行 `scripts/check.sh docs` 与链接检查。
6. **评审**：PR 检查清单确认「事实均为链接、无复制表格、依据可解析」。

**元信息头（所有 `docs/curated/**` 页面必须携带）**

```text
> 状态：草稿（AI 起草） | 已校对
> 责任人：@owner
> 依据：`internal/xxx.go`、`docs/generated/reference/api/xxx.md`
```

#### 守卫

- 三行元信息齐全；`状态` 只能取两个值之一。
- `状态：已校对` 时 `责任人` 必须非空，且属于 CODEOWNERS 中的账号。
- `依据` 至少一项，且路径必须真实存在；发布站点时按相对路径解析。
- 模板的「起草须知」小节必须已删除；残留即失败。
- 策展页不得包含字段表、枚举值表、命令参数表的复制内容；出现即提示改为链接生成页。

**为什么 AI 起草仍归 `curated/`**：所有权与可重算性决定目录，而不是初稿作者。AI 起草的页面包含人的判断，不能被流水线覆盖，因此必须落在 `curated/` 并由人负责。

### 5.6 开源门面与治理

| 产物 | 内容要求 |
| --- | --- |
| `README.md` | 一句话定位、核心能力、架构图、3 条命令快速开始、文档入口、构建/测试、兼容性、贡献/安全/许可证 |
| `LICENSE` | Apache-2.0 全文 |
| `NOTICE` | 版权声明与第三方归属（视依赖许可而定） |
| `THIRD_PARTY_LICENSES.md` | 随分发物携带的第三方组件、版本、许可证与全文清单 |
| `CONTRIBUTING.md` | 开发环境、分支/提交规范、PR 检查清单、文档更新要求、代码风格入口（`AGENTS.md`） |
| `SECURITY.md` | 支持版本、漏洞上报渠道、响应时限、披露策略 |
| `CODE_OF_CONDUCT.md` | Contributor Covenant 或等效 |
| `CHANGELOG.md` | Keep a Changelog + SemVer；由 Conventional Commits 生成 |
| `SUPPORT.md` | 提问渠道、Issue 分类、支持边界 |
| `.gitee/` 与 `.github/` | Issue/PR 模板、CODEOWNERS；`.github/` 另有 CI 薄封装 |
| `docs/.vitepress/` | VitePress：侧边栏由 `nav.json` 生成、本地搜索、编辑链接、版本快照 |

**许可证合规**：Apache-2.0 不覆盖第三方代码。`THIRD_PARTY_LICENSES.md` 按 Go 运行时依赖与 Web 生产依赖逐组件登记版本、许可证与全文；`scripts/check.sh license` 在 CI 中拒绝 GPL/AGPL/LGPL/SSPL/非商业与未知许可证，例外必须以「组件@版本 → 已核对 SPDX」显式登记。依赖变更后必须重新核对，新增许可证类型需补充对应文本。

**双平台策略**：Gitee 为主仓，GitHub 为只读镜像（关闭 Issue/PR，导航到 Gitee）。仓库内只保留 `scripts/ci.sh`，GitHub 工作流仅转调它，避免两套逻辑漂移。

**语言与版本**：中文为唯一文档语言，不维护英文版本。SemVer；文档随 minor 版本打快照。

## 6. 执行规划

### P0 目录隔离、信息架构与门禁

1. **目录迁移**：建立 `docs/generated/`、`docs/curated/`、`docs/templates/`；按来源搬迁现有文件（生成页进 `generated/`，手写页进 `curated/`）；更新注册表 `Output`、README 链接与全部相对链接。
2. **生成边界**：生成器写入根白名单、覆盖前身份校验、`docs/curated` 零写入断言、`--check` 只比对生成域。
3. **注册表大纲**：增加 `Section/Order/Owner/Source/Root/AiIngest`，补齐全部条目并扩展守卫（分区、顺序唯一、Owner、来源与根一致、孤儿页）。
4. **入口拆分**：`zki create docs` 改为生成 `docs/index.md` 与 `docs/nav.json`；根 `README.md` 改为策展门面。
5. **脊柱文档**：新增 `curated/explanation/system-overview.md`、`domain-model.md`、`glossary.md`，与现有概念页双向链接。
6. **教程/指南迁移**：6 篇由包注释改为策展页（AI 起草 + 审校）；对应包注释改写为包职责/边界/不变量；新增 `troubleshooting.md`。
7. **AI 起草基建**：落地 `docs/templates/` 七类模板（含起草须知）、元信息头守卫、`AGENTS.md` 增补「策展文档起草与审校」规则。
8. **社区门面**：`LICENSE`（Apache-2.0）、`NOTICE`、`CONTRIBUTING`、`SECURITY`、`CODE_OF_CONDUCT`、`SUPPORT`、`CHANGELOG`、Issue/PR 模板、CODEOWNERS。
9. **统一 CI**：`scripts/ci.sh` 串联 `scripts/check.sh` 的 gofmt、`go vet`、`go build`、`go test`、注释、许可证、文档检查与配置漂移检查；GitHub 工作流只做薄封装。

完成标志：`docs-check` 通过、`docs/curated` 零写入、无孤儿页、新访客按 README 可跑通。

### P1 参考事实与文档站点

1. 接口参考投影：`docs/generated/reference/api/`，按域分组，固定页面骨架，与表页交叉链接。
2. 数据表参考投影：抽取 `internal/models/**`，生成 `docs/generated/reference/schema/`，字段表含列/类型/可空/默认/含义/枚举。
3. 枚举、配置、CLI 三类参考生成并纳入 `--check`。
4. `docs/.vitepress/` 搭建 VitePress：侧边栏由 `nav.json` 生成、本地搜索、编辑链接、参考区折叠；只渲染中文文档。
5. 页面模板、链接与锚点守卫、策展页元信息与「禁止复制事实」检查接入 `docs-check`；CI 构建站点，部署到 Gitee/GitHub 项目页（子路径需指定 base）。

完成标志：任意事实 2 次点击可达、0 死链、站点可构建、生成器不触碰策展目录。

### P2 AI 能力层

1. 接口知识以 `native_api` 分区为唯一事实来源，不另建接口快照；检索经 `knowledge_search` 的 `format` 过滤复用。
2. 在视图层通过 `views.WithCapability` 随接口声明能力：域、意图、关联实体、读写属性、可调用标记；覆盖全部只读接口，敏感/诊断类标为不可调用。
3. 生成能力地图并写入 `capability` 分区，由模型按需检索；守卫校验接口存在与声明一致性。
4. RAG 增加 `schema`、`capability` 分区与对应 `zki sync schema/capabilities` 子命令，保持分区隔离与幂等。
5. 通过 `knowledge_search` 的 `format` 分区过滤复用接口知识，实现只读 `api_invoke`（client）；接入审核、审计、结果上限与拒绝策略。

完成标志：AI 能稳定选择接口并完成只读查询。

### P3 开源与可持续

1. Conventional Commits + SemVer；`git-cliff` 或等效工具生成 `CHANGELOG.md` 与 release notes。
2. release 流程：打标签、重新生成文档、构建站点快照、发布说明。
3. CODEOWNERS 按文档分区指定责任人；PR 模板增加「文档影响」必填项。
4. 记录孤儿页数、死链数、生成漂移数、AI 评测分数四项基线并季度复核。

完成标志：外部贡献者可仅凭公开文档完成一次 PR。

### 阶段依赖

- P0 先做目录迁移与生成边界：没有目录隔离，后续所有生成行为都可能覆盖策展文档。
- P0 再做注册表大纲：没有大纲，生成器只会生产更多散装页面。
- P1 先于 P2：AI 需要的接口/数据表事实投影与人类参考页是同一份生成物，避免为 AI 另写一套。
- P2 的 `api_invoke` 最后做：依赖能力白名单、接口描述工具与审核机制，先只读再写。
- P3 可与 P1/P2 并行，但应在 P0 后尽快补齐门面。

## 7. 验收标准

| 阶段 | 验收 |
| --- | --- |
| P0 | 目录按来源隔离；生成器根白名单生效；`docs/curated` 生成后零 diff；`docs/index.md`、`nav.json` 幂等；`--check` 可拦截漂移；三篇脊柱文档与 6 篇策展教程/指南到位；模板与元信息头守卫生效；社区文件齐备；`scripts/ci.sh` 通过 |
| P1 | 接口/表/枚举/配置/CLI 参考全覆盖且参与 `--check`；站点可构建、可搜索；0 死链；参考区折叠；策展页无复制事实 |
| P2 | 视图层能力声明与派生地图通过守卫；接口详情经 `knowledge_search` + `format=native_api` 与接口知识同源；`api_invoke` 白名单/只读/审计/超限全部生效；各分区可独立幂等同步 |
| P3 | 发布流程可重复；CODEOWNERS 与 PR 模板生效；四项指标有基线 |

## 8. 风险与对策

| 风险 | 对策 |
| --- | --- |
| 迁移期间链接大面积失效 | 迁移与链接守卫同批完成；一次性搬迁后运行全量链接检查 |
| 生成器路径配置错误导致覆盖策展页 | 写入根白名单 + 覆盖前身份校验 + CI 零 diff 断言，三重保护 |
| AI 起草产生事实幻觉 | 强制「依据清单」、元信息头、事实只链接不复制、人工「已校对」后才能发布 |
| 策展文档与包注释重复 | 包注释改写为职责/边界/不变量；守卫检查教程不再由包注释生成 |
| 两份事实源重新出现 | 策展页只写意图，字段/签名/枚举一律链接生成页；PR 检查清单增加「是否复制事实」 |
| AI 上下文膨胀 | 能力地图不常驻，统一走 `knowledge_search` 按需检索；完整 schema 同样按需获取 |
| `api_invoke` 越权或滥用 | 白名单 + 只读优先 + 后端鉴权 + 审核分级 + 审计 + 响应上限 + 禁止任意 URL |
| 生成物噪音（102 篇包参考） | 导航折叠；按域索引；不灌入 RAG |
| 站点增加维护成本 | Markdown 为唯一源；`nav.json` 自动生成；站点只做构建 |
| `--check` 误报 | 生成必须确定性排序、无时间戳、无随机 map 遍历 |
| 双平台 CI 漂移 | `scripts/ci.sh` 为唯一入口；工作流只转调，不写检查逻辑 |

## 9. 迁移映射

| 现有条目 | 现位置 | 目标位置 |
| --- | --- | --- |
| `tutorials/linux-dev`（原 `internal/cmd` 包注释） | `docs/tutorials/` | `docs/curated/tutorials/`（策展重写） |
| `tutorials/first-alert`（`internal/tagengine` 包注释） | `docs/tutorials/` | `docs/curated/tutorials/`（策展重写） |
| `how-to/add-datasource-plugin` | `docs/how-to/` | `docs/curated/how-to/` |
| `how-to/add-notify-channel` | `docs/how-to/` | `docs/curated/how-to/` |
| `how-to/write-migration` | `docs/how-to/` | `docs/curated/how-to/` |
| `how-to/add-enum-class` | `docs/how-to/` | `docs/curated/how-to/` |
| `how-to/frontend-add-page` | `docs/how-to/` | `docs/curated/how-to/` |
| `explanation/capture-architecture` 等 9 篇生成页 | `docs/explanation/` | `docs/generated/explanation/` |
| `explanation/frontend-design-system`、`frontend-feature-flags`、`agent-architecture`、本页 | `docs/explanation/` | `docs/curated/explanation/` |
| `kernel/xdp_bpf_design` | `docs/kernel/` | `docs/curated/kernel/` |
| 102 篇包符号参考 | `docs/reference/internal/` | `docs/generated/reference/internal/` |

新增页面：`curated/explanation/system-overview`、`curated/explanation/domain-model`、`curated/explanation/glossary`、`curated/how-to/troubleshooting`、`curated/capabilities/*`、`generated/reference/api/*`、`generated/reference/schema/*`、`generated/reference/enums`、`generated/reference/config`、`generated/reference/cli`、`curated/adr/*`。
