# 贡献指南

感谢你愿意为 zhongkui 做贡献。本指南说明开发环境、提交规范与评审要求。

## 1. 行为与沟通

- 参与本项目即表示同意遵守 [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md)。
- 主仓位于 Gitee，GitHub 为只读镜像；Issue 与 Pull Request 请在 Gitee 提交。
- 安全问题不要公开提交，见 [SECURITY.md](SECURITY.md)。

## 2. 开发环境

前置：Go 1.27.3+、Node.js 26+ 与 pnpm、Docker（本地依赖容器）。

```bash
make run-dev-deps   # 启动 PostgreSQL / ClickHouse / Redis / etcd / S3
make dev-tools      # 安装开发辅助工具
make ts-tools       # 安装前端工具链（如需改 web/）
go run cmd/zki/main.go migrate up
```

前端：

```bash
cd web
pnpm install
pnpm dev
```

## 3. 代码规范

- 后端与前端约定统一沉淀在 [AGENTS.md](AGENTS.md)，提交前请先阅读对应章节。
- 注释一律中文；导出声明必须有注释，由 `make lint` 强制检查。
- 错误使用 `internal/pkg/errors` 包裹；面向用户的错误详情走 i18n。
- 前端禁止内联样式，样式分层与设计令牌见 AGENTS.md「Vue Frontend Conventions」。
- 不允许引入第二份设计文档或第二份事实来源。

## 4. 提交与分支

- 使用 Conventional Commits（Angular 规范）：`build`、`ci`、`docs`、`feat`、`fix`、`perf`、`refactor`、`revert`、`style`、`test`、`chore`。
- 一个提交只做一件事；格式化与重命名尽量独立提交。
- 功能开发使用特性分支，合并前保持 `make ci` 通过。
- 不要提交生成的二进制、覆盖率文件或本地配置。

### 分支与提交脚本

`master`、`develop`、`release/*` 受保护，只能通过 PR 合入。`scripts/gitflow.sh` 把常见的分支创建、同步、推送与对比页跳转组合成一条命令：

```bash
scripts/gitflow.sh feature start login-fix   # develop → feature/login-fix
scripts/gitflow.sh feature finish            # 推送并打开 feature → develop 的 PR
scripts/gitflow.sh release start v1.2.3      # develop → release/v1.2.3
scripts/gitflow.sh release finish v1.2.3     # 打开 release → master 的 PR；
                                             # PR 合并后重跑，创建标签并打开回合并 PR
scripts/gitflow.sh hotfix start fix-null     # master → hotfix/fix-null
scripts/gitflow.sh hotfix finish             # 推送并打开 hotfix → master 的 PR
scripts/gitflow.sh sync                      # fetch --prune 并快进 develop / master
scripts/gitflow.sh status                    # 查看保护状态与领先/落后
scripts/gitflow.sh clean                     # 清理已合入目标分支的本地分支
scripts/gitflow.sh commit -t feat -s gitflow -m "新增提交助手" -y
```

- `commit` 按 Angular 规范拼装并校验 message；省略 `-t` 或 `-m` 时进入交互问答。
- `--breaking` 自动生成 `!` 与 `BREAKING CHANGE:` 页脚，`-i 12,#34` 生成 `Closes #12, #34`。
- `--dry-run` 只预览 message；`-a` 暂存已跟踪文件，`-- <文件>` 指定待提交文件。
- `master`、`develop` 上禁止提交，需先切到特性分支。
- PR 描述仍在 Gitee 页面填写，脚本只负责推送分支并打开对比页；`--no-open` 只打印链接。
- `--ci` 在推送前执行 `make ci`，`--force` 使用 `--force-with-lease`。
- 标签仍由 `make release` 创建，`release finish` 在 PR 合入后调用它并只推送 tag，不推送受保护分支。
- `release/*` 同样禁止直接推送，推送被拒绝时脚本会提示由有权限的维护者推送或在 Gitee 页面创建分支。

## 5. 版本与发布

版本号遵循 [语义化版本 2.0.0](https://semver.org/lang/zh-CN/)，首个发布版本为 `v0.1.0`。

### 版本号格式

**正式版本**：`vX.Y.Z`

- `X`（Major）：破坏性变更升级
- `Y`（Minor）：向后兼容的功能新增，`v1.0.0` 前也承载破坏性调整
- `Z`（Patch）：向后兼容的 bug 修复

**预发布版本**：`vX.Y.Z-{label}.{n}`

| Label | 阶段 | 含义 | 示例 |
| --- | --- | --- | --- |
| `alpha` | 内部测试 | 功能仍在迭代，API 可能随时变更 | `v1.2.3-alpha.1` |
| `beta` | 公开测试 | 功能冻结，邀请外部用户试用，专注修复 bug | `v1.2.3-beta.1` |
| `rc` | 候选发布 | 若测试通过即可作为正式版发布 | `v1.2.3-rc.1` |

- 预发布序列号 `.n` 从 1 开始递增，同一 X.Y.Z 内修复 bug 只递增 `.n`，不升 Y/Z
- 功能变更或范围调整时，升 minor 或 patch 后从 `alpha.1` 重新开始
- 预发布阶段可跳过，直接从 alpha → rc 或 beta → rc
- 正式发布时创建不带预发布后缀的 tag，与该 X.Y.Z 的预发布 tag 并存

**版本号演进示例**：

```
v1.2.3-alpha.1 → v1.2.3-alpha.2 → v1.2.3-beta.1 → v1.2.3-rc.1 → v1.2.3
```

### 发布流程

- 版本号唯一来源是 annotated tag，禁止在代码或配置中硬编码。
- 分支流转：`feature/*` → `develop`；发布时从 `develop` 切出 `release/vX.Y.Z`，验证后合入 `master` 并打 tag，最后回合并 `develop`。
- 正式发布前必须更新 `CHANGELOG.md` 的 `## [X.Y.Z] - YYYY-MM-DD` 段，并将 `web/package.json` 的 `version` 对齐到同一版本号（预发布版本的 CHANGELOG 可选更新，正式发布前必须补齐）。
- 开发构建不注入 SemVer，`zki version` 回落显示 commit hash；只有 release 构建注入语义化版本。

### 发布命令（Gitee 为主仓）

```bash
# 编译 release 二进制（支持正式版和预发布版）
make build-release VERSION=v1.2.3            # 正式版
make build-release VERSION=v1.2.3-alpha.1    # 预发布版

# 创建 annotated tag（会校验版本号格式、分支、工作区、CHANGELOG 等）
make release VERSION=v1.2.3
make release VERSION=v1.2.3-rc.1

# 打包制品
make dist VERSION=v1.2.3

# 推送 tag 到 Gitee
git push origin master --follow-tags
```

推送 tag 后在 Gitee 创建 Release，并上传 `dist/` 下的制品与校验和。GitHub 为只读镜像，不发布 Release；Gitee Go 的发布流水线在 Gitee 控制台配置，命令与上面一致（见 `.gitee/README.md`）。

## 6. 测试与检查

提交前必须执行：

```bash
make ci
```

等价于：

| 目标 | 内容 |
| --- | --- |
| `make fmt-check` | gofmt 格式检查 |
| `make build-check` | `go build ./...` |
| `make vet` | `go vet ./...` |
| `make test` | `go test ./...` |
| `make lint` | 注释完备性检查 |
| `make license-check` | 第三方依赖许可证门禁（Go + Web 生产依赖） |
| `make docs-check` | 文档生成物与代码声明一致性 |

### 测试分层

- `make test`、`make test-report`、`make ci` 默认不需要 PostgreSQL、ClickHouse、Redis、S3；依赖外部服务的用例会自动跳过（SKIP）。
- 运行集成测试：先执行 `make run-dev-deps && make create-test-dbs`，再执行 `make test-integration`。
- 也可自行提供 DSN：设置 `ZKI_TEST_INTEGRATION=1`，并配置 `ZKI_DATABASE_CORE_TEST_DSN`（PostgreSQL）/ `ZKI_DATABASE_RAW_TEST_DSN`（ClickHouse）；DSN 也可写在 `config.toml` 的 `[database.core_test]` / `[database.raw_test]` 中。
- 开关取值为 `1` 才启用；未设置或为 `0` 时跳过。开启后若 DSN 缺失或连接失败，测试会直接失败而不是跳过。

前端改动还需在 `web/` 下执行 `pnpm lint` 与 `pnpm type-check`（若脚本存在）。

许可证门禁会拒绝 GPL/AGPL/LGPL/SSPL/非商业与未知许可证。新增依赖若使用未收录的许可证，需先人工核对，再按以下方式登记：

- 新增允许的许可证类型：加入 `internal/lint/licensecheck` 的 `AllowedLicenses`；
- 上游未携带 LICENSE 但已核对的组件：加入 `knownMissingLicense`（键为 `module@version`）；
- npm 元数据缺失但已核对的包：加入 `knownUnknown`（键为 `name@version`）；
- 同步更新 `THIRD_PARTY_LICENSES.md` 中的组件、版本与许可证全文。

`make license-check` 的 Web 侧需要 `pnpm`；只读 `web/pnpm-lock.yaml`，无需安装 `node_modules`。CI 通过 `LICENSE_CHECK_REQUIRE_WEB=1` 强制要求 Web 检查。

## 7. 文档要求

文档按来源分两个目录，边界由生成器与守卫强制：

| 目录 | 谁写 | 规则 |
| --- | --- | --- |
| `docs/generated/` | 生成器 | 带 `DO NOT EDIT`，禁止手改；由 `make docs` 生成 |
| `docs/curated/` | 人（AI 可起草） | 必须登记在注册表，必须带元信息头，生成器禁止写入 |

新增或修改文档时：

1. 事实类内容（接口、字段、枚举、配置、CLI）只改声明源，然后运行 `make docs`。
2. 意图类内容（教程、指南、概念、能力剧本）放在 `docs/curated/`，使用 `docs/templates/` 中的模板。
3. 在 `internal/docs/registry.go` 登记条目（分区、顺序、责任人、来源、AI 摄入）。
4. 运行 `make docs-check`，确保没有孤儿页、死链或元信息缺失。
5. 策展文档只有在「状态：已校对」且责任人明确后才可合并。

文档站点（VitePress，源码即 `docs/` 下的 Markdown）：

```bash
make docs-install   # 首次安装站点依赖
make docs-dev       # 本地预览（默认 http://localhost:5173）
make docs-build     # 构建静态站点到 docs/.vitepress/dist
```

站点配置在 `docs/.vitepress/config.mts`，侧边栏由 `docs/nav.json` 自动生成。不要手工编辑 `nav.json` 或 `index.md`，它们是 `make docs` 的产物。

站点启用 `cleanUrls`：`index.md` 的规范路由是所在目录（如 `/generated/reference/`）。请通过 `make docs-dev` 或 `make docs-preview` 访问，不要用 `file://` 直接打开 `dist/index.html`。部署到 Gitee/GitHub 项目页（子路径）时指定 base：

```bash
DOCS_BASE=/zhongkui/ make docs-build
```

## 8. Pull Request 检查清单

- [ ] `make ci` 通过
- [ ] 前端改动已通过 `pnpm lint`
- [ ] 新增/修改接口已在 `RegisterApi` / `RegisterListApi` 声明用途与入参出参
- [ ] 新增模型字段已带中文注释与 gorm tag
- [ ] 新增枚举已在归属包注册并补齐中英 label
- [ ] 新增配置项已在配置结构体与 `describe config` 中可见
- [ ] 文档已按来源放入正确目录，且已登记注册表
- [ ] 没有复制字段表、枚举值或接口签名到策展文档
- [ ] 没有提交密钥、凭据或本地配置

## 9. 许可证

贡献即表示你同意以 Apache-2.0 许可发布你的贡献。第三方代码必须兼容该许可证，并在 [NOTICE](NOTICE) 与 [THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md) 中登记来源、版本与许可证。
