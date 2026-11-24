# 第 1 章：接入流量

> 状态：草稿（AI 起草）
> 责任人：@olinex
> 依据：`internal/capture/capture.go`、`internal/manager/views/admin/admin_capturer.go`、`internal/models/core/access_key.go`

本章完成系列的第一个闭环：把真实流量接入平台。系列章节顺序与阅读路线见[第 0 章](00-roadmap.md)。

## 你将完成什么

- 在平台创建一条采集器记录和一个采集访问密钥。
- 在 Linux 采集机上配置 Reporter 地址与认证信息，并启动采集进程。
- 在「流量审计 → HTTP 传输」中看到由该采集器产生的记录。

## 前置条件

- 已按[Linux 开发环境部署](linux-dev.md)或任一部署教程完成平台部署，`web`、`worker`、`grpc` 正常运行。
- 拥有管理员账号，且至少有一个可用命名空间（本文以 `default` 为例）。
- 一台 Linux 采集机，具备抓包权限（通常需要 root 或 `CAP_NET_RAW`），并能访问平台的 Reporter gRPC 端口（默认 `8081`）。
- 采集机上已放置 `zki` 二进制与 `config.toml`。

## 步骤

### 1. 确认命名空间

登录管理端，进入「系统管理 → 命名空间」，确认目标命名空间存在。

验证：列表中出现你要使用的命名空间名称，例如 `default`。

如果还没有命名空间，先按[第 13 章](13-namespace-user-permission.md)创建；采集器、访问密钥与后续规则都按命名空间隔离。

### 2. 创建访问密钥

进入「个人资料 → 访问密钥」，新建一条密钥：

- 填写描述，例如 `capture client`。
- 保持「启用」开启。
- 设置过期时间；生产环境建议按轮换周期设置，不要长期不过期。

保存后平台只展示一次 `AccessKeySecret`，立即记录 `AccessKeyID` 与 `AccessKeySecret`。

验证：访问密钥列表中该条记录为启用状态，且未过期。

### 3. 创建采集器记录

进入「系统管理 → 采集器」，新建采集器：

| 字段 | 说明 |
| --- | --- |
| 命名空间 | 选择第 1 步确认的命名空间 |
| 名称 | 采集器名称，长度 3–32 |
| 版本号 | 采集器版本，长度 3–32，例如 `v1.0` |
| 设备 | 采集机上的网卡名，例如 `eth0` |
| BPF 表达式 | 抓包过滤条件，例如 `tcp port 80 or tcp port 443` |
| 捕获长度 | 单包截取长度（1–65535），常用 `1600` |
| 超时时长 | 单位为秒（1–86400） |
| 混杂模式 | 需要抓取非本机流量时开启 |
| 启用 | 保持开启 |

验证：采集器列表出现该记录，字段与填写一致。采集器的运行状态不在本页展示；启动采集端后到「系统管理 → 系统状态 → 采集器」查看运行中状态与最后上报时间。

> 也可以在平台侧用 CLI 创建：
> `zki create access-key -u <用户名> -d "capture client" --enabled --ttl 8760h`
> `zki create capturer --namespace default --name capture-1 --version v1.0 --device eth0 --bpf-expr 'tcp port 80 or tcp port 443' --snap-len 1600 --read-timeout 5000 --promisc --enabled`

### 4. 配置采集机的 Reporter 连接

在采集机的 `config.toml` 中填写 Reporter 地址、TLS 与访问密钥：

```toml
[grpc.client.reporter]
host = '10.0.0.12'   # Reporter gRPC 所在主机
port = 8081
serviceConfig = {}

[grpc.client.reporter.tls]
enabled = true
domain = 'reporter'                              # 必须与证书 SAN 一致
cert = ''
certFilePath = '/etc/zhongkui/certs/reporter-cert.pem'

[grpc.client.reporter.auth]
enabled = true
authorize = 'accesskeySecret'
accessKeyID = '<第 2 步的 AccessKeyID>'
accessKeySecret = '<第 2 步的 AccessKeySecret>'
```

同时确认 `[capture.resource]` 中的命名空间、名称、版本与第 3 步创建的采集器一致。

验证：`zki describe config` 能打印上述配置；`zki describe deps` 能探测数据库、Redis、etcd、S3 等依赖连通性（不探测 Reporter 端口，采集端连通性以启动日志与系统状态为准）。

### 5. 启动采集端

在前台启动用于排查：

```bash
sudo zki --config /etc/zhongkui/config.toml run capture
```

确认无误后改用 systemd 托管：

```bash
sudo cp deploys/binary/zk-capture.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now zk-capture
sudo journalctl -u zk-capture -f
```

验证：日志中没有 TLS、认证或 Reporter 连接错误，并周期性出现抓包与上报记录；「系统管理 → 系统状态 → 采集器」中该采集器为运行中，最后上报时间持续更新。

### 6. 在流量审计中确认数据

打开「流量审计 → HTTP 传输」（或「IPv4 TCP」）。

验证：

- 能看到与采集器名称、版本对应的记录。
- 记录的时间、源/目的地址与端口符合 BPF 表达式的预期。
- 详情中能看到该条流量的基础字段与标签区域（此时标签可能为空，第 3 章会写入第一条标签）。

## 成功标志

「流量审计」中持续出现由你的采集器产生的记录，且「系统管理 → 系统状态 → 采集器」中状态为运行中、最后上报时间持续更新。

## 下一步

- 继续[第 2 章：Rego 规则讲解](02-rego-basics.md)，理解平台如何对这些流量打标签。
- 配置项细节见[配置参考](../../generated/reference/config.md)；采集端 CLI 见[CLI 参考](../../generated/reference/cli.md)。

## 常见问题

- **采集器一直没有「最后上报」**：先确认采集端已启动；再到「系统管理 → 系统状态 → 采集器」查看。检查采集机到 Reporter 的 `8081` 端口、TLS `domain` 与证书 SAN、访问密钥是否启用且未过期。
- **能启动但抓不到数据**：确认网卡名正确、BPF 表达式没有过滤掉目标流量、混杂模式是否满足场景、进程是否有抓包权限。
- **鉴权失败**：确认 `accessKeyID` / `accessKeySecret` 与平台创建时一致，且密钥未被停用。
