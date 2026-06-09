WITH
    current_end_time AS (SELECT now() - INTERVAL 1 SECOND),
    rule_list AS (
        SELECT
            r.rule_id,
            r.rule_name,
            r.window_sec,
            c.cond_id,
            c.bmp_condition,
            c.threshold
        FROM risk_rules r
        INNER JOIN risk_rule_conditions c ON r.rule_id = c.rule_id
        WHERE r.status = 1 AND c.status = 1
    ),
    agg_data AS (
        SELECT
            api,
            time_minute,
            bmp,
            cnt
        FROM traffic_bmp_agg
        CROSS JOIN current_end_time
        WHERE time_minute >= (current_end_time - INTERVAL rule_list.window_sec SECOND)
    )
SELECT
    rl.rule_id,
    rl.rule_name,
    ad.api,
    rl.cond_id,
    SUM(ad.cnt) AS hit_req_count,
    rl.threshold
FROM rule_list rl
INNER JOIN agg_data ad
WHERE arrayExists(
    x -> x = 1,
    [{{rl.bmp_condition}}]
)
GROUP BY rl.rule_id,rl.rule_name,ad.api,rl.cond_id,rl.threshold
HAVING hit_req_count >= rl.threshold
QUALIFY ROW_NUMBER() OVER (PARTITION BY rl.rule_id, ad.api ORDER BY hit_req_count DESC) = 1;

# Zhongkui (钟馗)

一个高性能的网络数据包捕获与数据分析平台。

## 项目简介

Zhongkui (钟馗) 是一个企业级网络抓包和数据分析系统，支持实时网络流量捕获、pcap 文件解析、ETL 数据处理流水线，并提供完整的 RESTful API 和 gRPC 接口。

## 技术栈

### 后端

- **语言**: Go 1.25+
- **Web 框架**: Gin
- **RPC**: gRPC + Protobuf
- **数据库**: GORM (支持 MySQL, PostgreSQL, SQLite, ClickHouse)
- **缓存/消息队列**: Redis, Asynq
- **服务发现**: etcd
- **对象存储**: S3 (MinIO)
- **权限控制**: Casbin (RBAC)
- **依赖注入**: Uber FX

### 前端

- **框架**: Vue 3
- **UI 库**: Vuetify 3
- **状态管理**: Pinia
- **构建工具**: Vite
- **语言**: TypeScript

## 项目架构

```
zhongkui/
├── api/                    # Protobuf 定义
│   └── reporter.proto      # gRPC 服务定义
├── cmd/                    # 命令行入口
│   ├── zki/               # 主程序入口
│   └── template/          # 代码生成工具
├── internal/              # 内部包
│   ├── capture/           # 网络抓包模块
│   ├── cmd/               # CLI 命令
│   ├── depend/            # 依赖注入配置
│   ├── fixtures/          # 测试数据
│   ├── manager/           # HTTP/gRPC 服务管理
│   │   └── routes/        # HTTP 路由
│   ├── models/            # 数据模型
│   │   ├── api/           # API 模型
│   │   ├── core/          # 核心业务模型
│   │   └── network/       # 网络协议模型
│   ├── pkg/               # 工具包
│   │   ├── apitree/       # API 树
│   │   ├── auth/          # 认证
│   │   ├── database/      # 数据库
│   │   ├── grpc/          # gRPC
│   │   ├── pipeline/      # ETL 流水线
│   │   ├── redis/         # Redis 客户端
│   │   └── s3/            # S3 存储
│   ├── reporter/          # gRPC 报告服务
│   └── tasks/             # 数据处理任务
└── web/                   # Vue.js 前端
```

## 功能特性

### 网络捕获

- 实时网络数据包捕获
- PCAP 文件导入与解析
- 支持多种网络协议: Ethernet, IPv4, IPv6, TCP, UDP, ICMP, DNS, DHCP, ARP, HTTP, TLS
- 数据压缩与 S3 上传

### API 服务

- RESTful API (Gin)
- gRPC API
- JWT 认证
- Access Key 认证
- 多租户 Namespace 支持

### 权限管理

- 基于 Casbin 的 RBAC 权限模型
- 用户分组管理
- 策略模板

### ETL 流水线

- 异步任务处理 (Asynq)
- 任务调度器
- 批量任务处理
- 任务状态监控

### 数据模型

- 用户管理 (User)
- 用户组 (Group)
- 命名空间 (Namespace)
- 抓包器 (Capturer)
- 访问密钥 (AccessKey)
- RBAC 策略

## 快速开始

### 前置要求

- Go 1.25+
- Node.js 22+
- pnpm
- MySQL / PostgreSQL / SQLite
- Redis
- etcd
- libpcap-dev

### 安装依赖

```bash
# 安装 Go 工具
make dev-tools

# 安装 Node.js 工具
make ts-tools

# 安装 pcap 开发库
make pcap-tools
```

### 构建项目

```bash
# 生成 Protobuf 代码
make all
```

### 运行服务

#### HTTP Web API

```bash
go run cmd/zki/main.go run web
```

#### gRPC API

```bash
go run cmd/zki/main.go run grpc
```

#### Worker (ETL 任务处理)

```bash
go run cmd/zki/main.go run worker
```

#### Capture (抓包客户端)

```bash
go run cmd/zki/main.go run capture
```

### 数据库迁移

```bash
# 运行迁移
go run cmd/zki/main.go migrate --timeout 1m
```

### 查看配置说明

```bash
go run cmd/zki/main.go describe-config
```

### 前端开发

```bash
cd web
pnpm install
pnpm dev
```

## CLI 命令

| 命令 | 描述 |
|------|------|
| `run web` | 运行 HTTP Web API 服务 |
| `run grpc` | 运行 gRPC API 服务 |
| `run worker` | 运行 ETL 任务工作器 |
| `run capture` | 运行抓包客户端 |
| `migrate` | 执行数据库迁移 |
| `clear` | 清理数据 |
| `data` | 数据操作 |
| `fixture` | 加载测试数据 |
| `create-user` | 创建用户 |
| `generate-tls` | 生成 TLS 证书 |
| `describe-config` | 显示配置说明 |

## 配置

配置文件默认为 `config.yaml`，支持以下配置项：

- 数据库配置
- Redis 配置
- etcd 配置
- S3 配置
- gRPC 配置
- Pipeline 配置
- 日志配置
- 国际化配置

## 许可证

MIT