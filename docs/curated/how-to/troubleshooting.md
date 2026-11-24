# 故障排查：启动、采集、标签、通知与检索

> 状态：草稿（AI 起草）
> 责任人：@olinex
> 依据：`internal/cmd`、`internal/manager`、`internal/capture`、`internal/tagengine`、`internal/notify`、`internal/ai`

本指南按「症状 → 检查 → 处理」组织。每一步都给出可执行的验证命令，避免凭猜测改配置。

**适用角色**：运维、安全运营、研发。

> 流量接入、规则编写、事件配置与通知绑定的基础步骤已由教程覆盖，本文不重复；请先按教程对应章节的验证步骤排查，再使用本文的跨组件诊断。

## 1. 先做三件事

无论遇到什么问题，先收集这三项信息：

```bash
go run cmd/zki/main.go version          # 确认版本与构建信息
go run cmd/zki/main.go describe config  # 打印全部配置项及其说明
go run cmd/zki/main.go describe deps    # 探测数据库、Redis、etcd、S3 连通性
```

`describe deps` 有一项失败，就先解决依赖连通性，不要继续排查业务逻辑。

## 2. 服务无法启动

**症状**：`run web` / `run worker` 启动即退出，或端口无响应。

**检查**：

1. 配置是否被正确加载：Viper 搜索 `/etc/zhongkui`、当前目录与上级目录，也可用 `--config` 指定。
2. 依赖是否可达：`describe deps`。
3. 数据库是否已迁移：启动日志中的迁移错误、`migrate status`。
4. 端口是否被占用：`[api]` 配置的地址与端口。

**处理**：

- 未迁移：`go run cmd/zki/main.go migrate up`。
- 配置缺项：对照 `describe config` 的输出补齐，不要凭记忆手写。
- 依赖容器未启动：`scripts/dev-containers.sh start`；不确定哪一项异常时先运行 `scripts/dev-doctor.sh` 定位失败项，再启动服务。
- 依赖镜像拉取失败：用 `DEV_IMAGE_REGISTRY` 统一替换镜像源（镜像源不代理 quay.io 时再单独设置 `ETCD_IMAGE`），详见[Linux 开发环境部署](../tutorials/linux-dev.md)第 3 节。

## 3. 数据库迁移失败

**症状**：`migrate up` 报版本冲突、校验和不一致或 DDL 错误。

**检查**：

- 是否同时迁移了 core 与 raw：默认 `--db all`，也可分别指定。
- 是否跳过或手工改过已执行脚本：goose 会校验版本与校验和。
- 目标库是否已有同名对象：先用 `migrate check` 比对模型与库结构。

**处理**：

- 开发阶段可直接重建库后重跑迁移；已部署环境先备份再处理。
- 新增脚本遵循「向前/向后兼容」约定，见[如何编写一次数据库迁移](../how-to/write-migration.md)。

## 4. 采集没有数据

**症状**：管理端流量列表为空，或采集端进程存在但无上报。

先按[第 1 章：接入流量](../tutorials/01-ingest-traffic.md)核对抓包权限、网卡名、BPF、TLS 域名与访问密钥；这些基础项通过后，再查跨组件链路：

1. Reporter gRPC 是否可达：`[grpc.server.reporter]` 与 `[grpc.client.reporter]` 是否对齐。
2. 原始库是否有写入：直接查询 raw 库的流量表。
3. 对象存储是否可用：大字段写入失败会导致快照不完整。
4. Reporter 与 worker 的日志是否有鉴权、反序列化或落库错误。

**处理**：

- 先用一个明确的测试请求（如访问一个已知 URL）验证端到端链路。
- 若 `describe deps` 中 S3 失败，先修对象存储，再看采集。
- 采集侧原理见[采集架构](../../generated/explanation/capture-architecture.md)。

## 5. 标签不触发

**症状**：规则已启用、流量存在，但没有标签或没有事件。

规则语法、契约、试运行与样例排查见[第 2 章](../tutorials/02-rego-basics.md)与[第 3 章](../tutorials/03-first-tag.md)；事件条件、阈值与状态见[第 5 章](../tutorials/05-first-risk-rule.md)。教程步骤均通过仍不触发时，再查跨组件链路：

1. 规则版本与缓存失效：规则变更后各进程是否已加载新版本。
2. worker 是否正常消费标签识别任务，队列是否堆积。
3. 聚合表中窗口内是否有数据，事件窗口与数据时间范围是否重叠。
4. 规则是否直接引用了敏感字段：敏感字段不会出现在输入文档中，直接引用会得到未定义结果；需要明文时必须调用 `crypto.reveal("字段名")`。

**处理**：

- 用最小规则（固定命中）验证链路，再逐步加条件。
- 核对规则版本与事件记录中的版本号，确认新版本已生效。

## 6. 通知没有送达

**症状**：产生了标签事件，但没有收到通知。

渠道、目标、通知组与规则绑定的配置步骤见[第 6 章：接入告警渠道](../tutorials/06-alert-channel.md)。配置检查通过仍未送达时，再查跨组件链路：

1. `notification_records` 是否有对应记录：有记录表示已投递，问题在接收侧；无记录表示未进入发送。
2. 事件是否被抑制：抑制窗口内的记录为 `suppressed`，不会发送。
3. 命名空间级模板是否覆盖错误：渲染失败会中止发送。
4. worker 的通知发送任务是否堆积或持续重试。

**处理**：

- 先用一个最小事件验证渠道，再排查模板。
- 新增渠道与模板的步骤见[如何新增通知渠道与模板](add-notify-channel.md)。

## 7. AI 检索异常

**症状**：问答答非所问、引用为空、知识文档缺失。

模型与档位配置见[第 8 章：如何接入 AI](../tutorials/08-ai-setup.md)，助手使用与知识库操作见[第 9 章：用 AI 助手分析数据](../tutorials/09-ai-analyze-data.md)。以下是运维侧的索引与维度恢复动作：

**检查**：

1. embedding 模型与向量列维度是否一致：换模型后需重嵌入。
2. 知识文档是否已同步：`zki sync docs`、`zki sync apis` 是否执行。
3. 命名空间是否正确：检索在调用方命名空间内并入系统命名空间。
4. 文档是否已建索引：文档状态与分块数量。

**处理**：

- 换 embedding 模型：先在「LLM 配置 → 档位绑定」解绑 `embedding` 档位，再执行 `zki sync embedding-dimensions --dimensions <n>` 重建向量列，最后执行 `zki sync embeddings --namespace <ns>` 重嵌入并重新绑定。
- 仅内容变更：重跑对应 `zki sync` 子命令，正文未变的条目零写入。
- 运行时链路见[LLM Agent + RAG 设计](../../generated/explanation/agent-rag-design.md)。

## 8. 前端能力不可用

**症状**：侧边栏某项功能存在，但页面提示不可用。

**检查**：

- 功能开关是否开启：调用 `features/list` 查看当前部署的开关状态。
- 是否属于「支持但未开通」：这类功能仍应展示入口，并在页面内引导到开通路径。
- 是否缺少后端依赖（如 AI 未启用、模型未绑定）。

**处理**：按功能开关与绑定状态逐项恢复；不要通过隐藏入口来规避。

## 9. 单元测试偶发失败（ClickHouse 内存）

**症状**：`go test ./...` 偶发失败，错误含 `code: 241`、`memory limit exceeded`、`OvercommitTracker`，且失败点常在一条很小的 `INSERT` 上。

**原因**：开发容器的 ClickHouse 系统日志（尤其 `trace_log` / `text_log`）长期累积，后台 merge 与系统表缓存占用内存，使普通写入被 OvercommitTracker 选中终止。仓库已在 `config/clickhouse/config.d/99-dev-logs.xml` 中关闭 trace/text 日志并限制其余系统日志的 TTL；该配置通过 `scripts/dev-containers.sh start` 挂载，需重建容器后生效。

**处理**：

```bash
scripts/dev-containers.sh remove
scripts/dev-containers.sh start
```

已有容器中体积过大的系统表需要先强制清理（超过 `max_table_size_to_drop` 时须放开限制）：

```bash
docker exec clickhouse-server clickhouse-client \
  --query "SET max_table_size_to_drop=0; TRUNCATE TABLE system.trace_log"
docker exec clickhouse-server clickhouse-client --query "SYSTEM DROP MARK CACHE"
```

执行后用 `SELECT metric, value FROM system.metrics WHERE metric LIKE '%Memory%'` 确认内存回落到容器上限的 50% 以下，再重跑测试。

## 10. 收集诊断信息

提交问题时附上：

```bash
go run cmd/zki/main.go version
go run cmd/zki/main.go describe config
go run cmd/zki/main.go describe deps
```

以及：复现步骤、时间点、相关命名空间、错误码与完整错误文本。错误码的中英文文案位于 `internal/pkg/translate/locales/`，可据此定位业务含义。
