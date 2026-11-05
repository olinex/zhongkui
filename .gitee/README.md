# Gitee CI 约定

本仓库的主仓位于 Gitee。CI 不在仓库内维护第二套检查逻辑：Gitee Go 流水线只需在构建步骤执行：

```bash
make ci
```

该命令依次执行格式检查、构建、vet、单元测试、注释检查、许可证门禁与文档漂移检查。

许可证门禁会检查 Go 运行时依赖与 Web 生产依赖；Web 侧需要 `pnpm`（只读 `pnpm-lock.yaml`，无需安装 node_modules）。Gitee 流水线请设置：

```bash
export LICENSE_CHECK_REQUIRE_WEB=1
make ci
```

若流水线需要系统依赖，请在构建镜像或前置步骤中安装 `libpcap-dev`（采集侧 cgo 链接需要）。

Gitee Go 的具体流水线文件在 Gitee 控制台启用，本目录只保留 Issue / PR 模板与 CODEOWNERS。

## 发布流水线

tag `vX.Y.Z` 触发的发布流水线依次执行：

```bash
export TAG=v0.1.0            # 替换为本次发布 tag
export LICENSE_CHECK_REQUIRE_WEB=1
make ci
make build-release VERSION="$TAG"
make dist VERSION="$TAG"
```

产物为 `dist/zki_<version>_<os>_<arch>.tar.gz` 及对应 `.sha256`，上传到 Gitee Release。GitHub 为只读镜像，不承担发布。
