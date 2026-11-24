# 如何接入 KMS 获取主密钥

> 状态：草稿（AI 起草）
> 责任人：@olinex
> 依据：`internal/pkg/crypto/key/provider.go`、`internal/pkg/crypto/key/config.go`、`internal/provider/crypto.go`

服务启动时如果 stderr 出现「警告：正在使用配置文件中的父密钥，这不适用于生产环境，请尽快切换到 KMS」，说明 `[crypto]` 的 `source` 仍是 `config`，主密钥以明文形式来自配置文件。把 `source` 改为 `kms` 后，进程会在首次取密钥时执行一条由你配置的外部命令，从 KMS 取回主密钥；结果只在进程内缓存一次，后续派生密钥不再重复执行。

该方式不绑定具体厂商：命令可以是 AWS KMS、阿里云 KMS、HashiCorp Vault 的 CLI，也可以是封装好的取密钥脚本。本指南面向二进制 / systemd、Docker Compose 与源码运行，Helm 的限制见文末。

**适用角色**：运维。本文属于生产密钥接入的运维操作，普通使用者（安全运营）无需阅读。

## 适用场景

- 生产环境已经出现父密钥警告，需要把主密钥来源从配置文件切换到 KMS。
- 希望在不同云厂商或自建密钥服务之间保持同一套接入方式。

不适用：

- 只想更换密码本：用 `zki create password-book` 生成新文件即可，见[CLI 参考](../../generated/reference/cli.md)。
- 本地开发：继续使用 `source = 'config'`，无需接入 KMS。

## 前置检查

1. **主密钥必须一致（最重要）**：KMS 返回的明文必须与当前 `crypto.parent_secret_key` 完全一致。派生密钥由「主密钥 + 密码本」决定；一旦主密钥变化，历史 `crypto` / `hash` / `s3crypto` 字段将无法解密，带密钥摘要也不再匹配。正确做法是把现有主密钥导入 KMS（作为密文或 Secret 托管），或解密既有密文得到原值，而不是新建一个随机密钥。
2. **密码本不能更换**：`crypto.password_book_path` 指向的文件必须保持原样（权限 0600），也不能回退到内置密码本。
3. **凭据已就绪**：部署机上的 KMS CLI 或脚本能以服务运行账号的身份取到密钥。云上通常通过实例角色 / IAM Role、Vault Token 或 AppRole 等方式注入。
4. 备份当前 `config.toml` 与密码本文件。

## 步骤

### 1. 准备取密钥命令

`crypto.kms.command` 是 argv 数组，不经过 shell；命令的标准输出（去掉首尾空白）即主密钥明文。建议使用绝对路径，避免 systemd 环境下 `PATH` 缺失。

可以直接用 argv 表达的形式（以 Vault 为例）：

```toml
[crypto.kms]
command = ['/usr/local/bin/vault', 'kv', 'get', '-field=master_key', 'secret/zhongkui']
```

需要管道、base64 解码或多步处理时，先封装为脚本（权限 0700），再把脚本路径写入 `command`：

```sh
#!/bin/sh
set -eu
# AWS KMS：解密预先上传的密文文件并输出明文
aws kms decrypt --ciphertext-blob fileb:///etc/zhongkui/master-key.bin \
  --output text --query Plaintext | base64 -d
```

阿里云 KMS 同理：用 `aliyun kms Decrypt` 解密密文，再 `base64 -d` 输出明文。脚本中不要把密钥写入磁盘或日志，只写 stdout。

### 2. 比对主密钥（不打印明文）

```bash
# KMS 命令输出的摘要
/etc/zhongkui/get-master-key.sh | sha256sum
# 现有配置中主密钥的摘要（printf 不追加换行）
printf '%s' '<当前 parent_secret_key>' | sha256sum
```

两个摘要一致后再继续。不一致时先修正 KMS 托管的主密钥，不要直接切换 `source`。

### 3. 修改配置

```toml
[crypto]
source = 'kms'
# 切换后该字段不再使用，可置空
parent_secret_key = ''
password_book_path = '/etc/zhongkui/password-book.bin'

[crypto.kms]
command = ['/etc/zhongkui/get-master-key.sh']
timeout = '10s'
```

- `command` 为空时启动直接报错 `crypto.kms.command is required when crypto.source is kms`。
- `timeout` 为非正数时按默认值 `10s` 处理。
- 配置文件属于敏感文件，建议 `chmod 600`；取密钥脚本建议 `chmod 700`，且凭据通过运行账号环境提供，不写入配置。

### 4. 所有角色共用同一份配置

`web`、`worker`、`capture`、`grpc` 与 `migrate` 都会初始化密钥派生器。多实例部署时，每个实例都必须配置同一条命令，并返回同一个主密钥。

## 验证

1. 启动任一角色，stderr 不再出现父密钥警告。
2. 在管理端保存一个加密字段（例如数据源的密码），再查看其明文，确认加密与解密链路正常。
3. 读取一条切换前写入的加密数据（加密字段或 S3 加密正文），确认历史数据仍可解密。
4. 确认命令只执行一次：可在脚本中追加计数，或用 `ps` 观察；进程重启后才会再次执行。

## 回滚与排障

回滚：把 `source` 改回 `'config'`，恢复原 `parent_secret_key`，重启全部角色。只要主密钥与密码本未变，历史数据不受影响。

| 错误信息 | 原因与处理 |
| --- | --- |
| `crypto.kms.command is required when crypto.source is kms` | `[crypto.kms]` 的 `command` 为空 |
| `crypto.kms.command "..." failed: exit status N: ...` | 命令不可执行、凭据缺失或 CLI 不在 `PATH`；错误会附带命令的 stderr |
| `crypto.kms.command "..." timed out after 10s` | KMS 或网络不可达，或 `timeout` 设置过短 |
| `crypto.kms.command returned empty secret key` | 命令成功退出但 stdout 为空 |
| 历史密文解密失败或摘要不匹配 | KMS 返回的主密钥与切换前不一致，或密码本被更换 |

主密钥缓存在进程内存中，只在首次取值时读取一次；轮换主密钥等同于数据重加密，不能只修改 KMS 或配置。

## Kubernetes / Helm 限制

Helm Chart 当前把 `[crypto]` 的 `source` 固定为 `'config'`，并从 Secret 注入 `ZKI_CRYPTO_PARENT_SECRET_KEY`（见 `deploys/helm/zhongkui/templates/_helpers.tpl`），没有提供 `[crypto.kms]` 入口。按本指南切换前，需要先让 Chart 支持 KMS 配置（或在部署后覆盖生成的 ConfigMap），并为容器提供 KMS CLI 与凭据；在此之前，Helm 部署仍会打印父密钥警告。

## 相关参考

- [配置参考](../../generated/reference/config.md)：`crypto.source`、`crypto.parent_secret_key`、`crypto.password_book_path`、`crypto.kms.*`
- [CLI 参考](../../generated/reference/cli.md)：`zki create password-book`
- [故障排查：启动、采集、标签、通知与检索](./troubleshooting.md)
- [Linux 二进制部署](../tutorials/deploy-binary.md)
- [Docker Compose 部署](../tutorials/deploy-docker-compose.md)
