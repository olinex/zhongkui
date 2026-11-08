# 故障排查：启动、采集、标签、通知与检索

> 状态：草稿（AI 起草）
> 责任人：@olinex
> 依据：`internal/cmd`、`internal/manager`、`internal/capture`、`internal/tagengine`、`internal/notify`、`internal/ai`

本指南按「症状 → 检查 → 处理」组织。每一步都给出可执行的验证命令，避免凭猜测改配置。

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
- 依赖容器未启动：`scripts/dev-deps.sh start`，确认容器状态后再启动服务。

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

**检查**：

1. 采集端是否以足够权限运行（抓包通常需要 root 或相应 capability）。
2. `run capture` 的网卡参数是否指向真实流量接口。
3. Reporter gRPC 是否可达：`[grpc.server.reporter]` 与 `[grpc.client.reporter]` 是否对齐。
4. 原始库是否有写入：直接查询 raw 库的流量表。
5. 对象存储是否可用：大字段写入失败会导致快照不完整。

**处理**：

- 先用一个明确的测试请求（如访问一个已知 URL）验证端到端链路。
- 若 `describe deps` 中 S3 失败，先修对象存储，再看采集。
- 采集侧原理见[采集架构](../../generated/explanation/capture-architecture.md)。

## 5. 标签不触发

**症状**：规则已启用、流量存在，但没有标签或没有事件。

**检查**：

1. 规则是否通过校验：语法、包名、`tags` 规则、跨规则冲突。
2. 目标模型是否匹配：规则绑定的模型类型必须与流量实体一致。
3. 输入文档字段是否符合预期：加密字段在规则中为空串，不能据此匹配。
4. 窗口是否满足：事件规则的时间窗口与阈值。
5. 是否有样例数据：模型缺少运行期样例会导致校验或试跑失败。

**处理**：

- 用最小规则（固定命中）验证链路，再逐步加条件。
- 核对规则版本与事件记录中的版本号，确认新版本已生效。
- 完整流程见[教程：看到你的第一条风险告警](../tutorials/first-alert.md)。

## 6. 通知没有送达

**症状**：产生了标签事件，但没有收到通知。

**检查**：

1. 事件规则是否绑定了通知组；未绑定则使用默认组。
2. 通知组是否有目标；目标是否属于启用的渠道。
3. 渠道凭据是否有效（邮件、Webhook 等）。
4. `notification_records` 是否有对应记录：有记录表示已投递，问题在接收侧；无记录表示未进入发送。
5. 命名空间级模板是否覆盖错误：渲染失败会中止发送。

**处理**：

- 先用一个最小事件验证渠道，再排查模板。
- 新增渠道与模板的步骤见[如何新增通知渠道与模板](../how-to/add-notify-channel.md)。

## 7. AI 检索异常

**症状**：问答答非所问、引用为空、知识文档缺失。

**检查**：

1. AI 功能是否启用；未启用时 `sync` 命令会直接拒绝。
2. embedding 模型与向量列维度是否一致：换模型后需重嵌入。
3. 知识文档是否已同步：`zki sync docs`、`zki sync apis` 是否执行。
4. 命名空间是否正确：检索在调用方命名空间内并入系统命名空间。
5. 文档是否已建索引：文档状态与分块数量。

**处理**：

- 换 embedding 模型：先 `zki sync embedding-dimensions --dimensions <n>`，再 `zki sync embeddings --namespace <ns>`。
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

**原因**：开发容器的 ClickHouse 系统日志（尤其 `trace_log` / `text_log`）长期累积，后台 merge 与系统表缓存占用内存，使普通写入被 OvercommitTracker 选中终止。仓库已在 `config/clickhouse/config.d/99-dev-logs.xml` 中关闭 trace/text 日志并限制其余系统日志的 TTL；该配置通过 `scripts/dev-deps.sh start` 挂载，需重建容器后生效。

**处理**：

```bash
scripts/dev-deps.sh remove
scripts/dev-deps.sh start
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
