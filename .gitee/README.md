# Gitee CI 约定

本仓库的主仓位于 Gitee。CI 不在仓库内维护第二套检查逻辑：Gitee Go 流水线只需在构建步骤执行：

```bash
make ci-report
```

`make ci-report` 与本地 `make ci` 跑同一组检查（`fmt-check build-check vet test lint license-check docs-check`），区别是：

- 不在第一个失败处中断，跑完全部检查后统一判失败，一次暴露所有问题；
- 每项输出写入 `.cicd/reports/<检查名>.log`；
- 单元测试改用 `test-report`，产出 Gitee Go 可直接解析的报告目录 `.cicd/golang-report/`（`report.jsonl` 测试报告、`index.html` 覆盖率页面、`coverage.txt` 汇总）。

流水线需把 `.cicd/golang-report` 与 `.cicd/reports` 声明为构建产物；测试报告页填 `.cicd/golang-report/report.jsonl`，覆盖率页面可从产物中打开。`.cicd/` 已被 `.gitignore` 与 `.dockerignore` 屏蔽，不会入库也不会进入镜像。本地提交前仍建议用 `make ci`（失败即停、输出直连终端）。

许可证门禁会检查 Go 运行时依赖与 Web 生产依赖；Web 侧需要 `pnpm`（只读 `pnpm-lock.yaml`，无需安装 node_modules）。Gitee 流水线请设置：

```bash
export LICENSE_CHECK_REQUIRE_WEB=1
make ci-report
```

若流水线需要系统依赖，请在构建镜像或前置步骤中安装 `libpcap-dev`（采集侧 cgo 链接需要）。

Gitee Go 的具体流水线文件在 Gitee 控制台启用，本目录只保留 Issue / PR 模板与 CODEOWNERS。

## 发布流水线

tag `vX.Y.Z` 触发的发布流水线依次执行：

```bash
export TAG=v0.1.0            # 替换为本次发布 tag
export LICENSE_CHECK_REQUIRE_WEB=1
make ci-report
scripts/dist.sh "$TAG"
```

产物为 `dist/zki_<version>_<os>_<arch>.tar.gz` 及对应 `.sha256`，上传到 Gitee Release。GitHub 为只读镜像，不承担发布。

镜像构建由 tag 触发的构建发布流水线（`.workflow/ci-build.yml`）完成：直接把 `Dockerfile` 作为构建定义，版本信息经构建参数传入（`VERSION=${GITEE_BRANCH}`、`COMMIT=${GITEE_SHORT_COMMIT}`、`BUILDER=触发用户@流水线:构建号`），容器内执行 `make version-env` 生成 `dist/build.env`；默认构建 normal 变体（debian 基底，便于排障），需要 scratch 最小镜像时把 `IMAGE_VARIANT` 改为 `slim`。本地手工构建用 `scripts/docker-image.sh vX.Y.Z`（默认 normal，`VARIANT=slim` 构建 slim 变体）。
