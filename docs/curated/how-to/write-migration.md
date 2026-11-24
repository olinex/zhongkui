# 如何编写一次数据库迁移

> 状态：已校对
> 责任人：@olinex
> 依据：`internal/pkg/migrate/migrate.go`

新增一次向前/向后兼容的数据库迁移脚本。

**适用角色**：研发、开源贡献者。普通使用者（安全运营/运维）无需阅读；只有修改代码或数据库结构时才需要编写迁移。

包内文件分工：migrate.go（Migrator 与 goose 执行器封装、版本升降级与状态表格）、 config.go（Config 及其链式配置）、check.go（模型声明 vs 数据库结构校验）、diff.go （两套结构差异比对）、dryrun.go（迁移演练）、tempdb.go（临时库生命周期）。CLI 入口在 internal/cmd/migrate.go。

## 一、机制概览

迁移文件是 goose 格式的 SQL，带 `-- +goose Up` / `-- +goose Down` 两段；文件名形如 `NNN_描述.sql`（三位零填充递增版本号 + 下划线 + 描述）。goose 以 `goose_db_version` 表记录 每个版本是否已应用：`up` 按版本升序执行未应用的 Up 段，`down` 只回滚最近一个已应用版本。 文件本身经 embed 打进二进制，版本号因此是唯一且不可变的发布标识。

两个库各持一套迁移：core 库存业务表与出厂数据（`001_init.sql` 建表、`002_fixtures.sql` 写入 参数与 RBAC 权限点等 fixture），raw 库存抓包原始数据表。两库分别经 migrations.MigrationFor （internal/migrations/collect.go）按库名分发嵌入式文件系统。

## 二、分步清单：新增一次迁移

1. 生成骨架（自动取下一个版本号并创建文件）：

  go run cmd/zki/main.go migrate create {描述}

  它在 internal/migrations/core（以及 raw）下建出 `NNN_{描述}.sql`，含 Up/Down 两段空模板。 普通建表/补列优先新增独立文件承载，不要原位改写已发布的既有迁移。

2. 编写 Up 段：一律用幂等写法（CREATE TABLE IF NOT EXISTS、ADD COLUMN IF NOT EXISTS、 CREATE INDEX IF NOT EXISTS 等），使脚本可在结构已知/未知的库上重复安全执行；建表后 用 COMMENT ON COLUMN 补列注释（与 `001_init.sql` 保持一致）。

3. 编写 Down 段：与 Up 对称地撤销变更（DROP ... IF EXISTS）。首版本封闭开发阶段 schema 可自由重构（见 AGENTS.md「开发阶段」），但仍应保持 Down 可用，以便 diff / dry-run。

4. 递增版本号：版本号必须严格递增，且一旦文件被任何库应用就不得再原位改写其内容——既有库 的 `goose_db_version` 已记为已应用，改写后不会重跑，导致结构漂移；承载变更须新增一个 更高版本号的文件。

5. 若改动涉及模型字段，同步更新 GORM 模型与 provider.CoreModels() 等登记清单（见 check）。

6. 用下节命令验证。

## 三、验证命令（CLI）

均支持 --db=core|raw|all（默认 all）与 --verbose / --timeout：

- go run cmd/zki/main.go migrate status 查看各版本是否已应用及其应用时间；
- go run cmd/zki/main.go migrate dry-run 在临时库上跑一遍全部 Up，验证脚本可执行；
- go run cmd/zki/main.go migrate up 对目标库执行全部未应用迁移；
- go run cmd/zki/main.go migrate down 回滚最近一次已应用的迁移；
- go run cmd/zki/main.go migrate up-to N 迁移至版本 N；down-to N 回滚至版本 N；
- go run cmd/zki/main.go migrate diff 比对「当前库结构」与「空库跑完全部迁移后的结构」， 列出多出/缺失的表与列；
- go run cmd/zki/main.go migrate check 见下节。

## 四、migrate check 的意义

check 先建临时库并跑完全部迁移，再用 GORM 解析模型的期望结构，逐表逐列比对类型不匹配、缺失 表/列与多余表/列（postgres 与 clickhouse 各有一套类型别名归一，见 check.go）。它保证「代码里 的模型声明」与「迁移脚本产出的真实结构」一致——两侧任一改动而另一侧未跟上都会被它拦下；因此 新增模型后必须同步登记到 provider.CoreModels() / provider.Models() 等清单，否则该表会被判为 多余表。--keep-temp 在失败时保留临时库供人工排查。

## 五、注意事项

- 迁移只改结构，不承载业务逻辑；数据回填也应写成幂等 SQL（如 INSERT ... ON CONFLICT DO NOTHING）。
- 临时库在 postgres 下需具备建库权限（tempdb.go 用管理连接 CREATE DATABASE），演练结束自动 清理（--keep-temp 可保留）。
- clickhouse 的 `goose_db_version` 由迁移器在 init 时显式建表（兼容性 workaround）。
