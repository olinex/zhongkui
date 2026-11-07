# 检测操作系统平台: Darwin (macOS), Linux, 或 Windows (MINGW/MSYS/CYGWIN)
UNAME_S := $(shell uname -s)
GOBIN := $(shell go env GOBIN)
ifeq ($(UNAME_S),Darwin)
# macOS: 从 en0 网卡获取局域网 IP，失败时回退到 localhost
LAN_IP := $(shell ipconfig getifaddr en0 2>/dev/null || echo localhost)
# gopacket/pcap (cgo) 在 macOS 上会重复链接 -lpcap，抑制 ld 警告
CGO_LDFLAGS += -Wl,-no_warn_duplicate_libraries
export CGO_LDFLAGS
else ifeq ($(UNAME_S),Linux)
# Linux: 取主机名解析出的第一个 IP，失败时回退到 localhost
LAN_IP := $(shell hostname -I 2>/dev/null | awk '{print $$1}' || echo localhost)
else
LAN_IP := localhost
endif

# 抓包基准测试需要 root 权限，默认使用 sudo；可通过 SUDO= make bench-slow 显式跳过
SUDO ?= sudo

# CI/CD 临时产物根目录：测试报告、逐项检查日志，整体不入库、不进镜像
REPORT_ROOT ?= .cicd
REPORT_DIR ?= $(REPORT_ROOT)/reports
GOLANG_REPORT_DIR ?= $(REPORT_ROOT)/golang-report
export REPORT_ROOT REPORT_DIR GOLANG_REPORT_DIR

.PHONY: bench-fast
.PHONY: bench-prepare
.PHONY: bench-probe
.PHONY: bench-raw
.PHONY: bench-slow
.PHONY: bench-smoke
.PHONY: bench-xdp-phys
.PHONY: bench-xdp-phys-raw
.PHONY: build
.PHONY: build-check
.PHONY: build-release
.PHONY: ci
.PHONY: ci-report
.PHONY: clean
.PHONY: create-test-dbs
.PHONY: dev-tools
.PHONY: docs
.PHONY: docs-build
.PHONY: docs-check
.PHONY: docs-dev
.PHONY: docs-install
.PHONY: docs-preview
.PHONY: fmt-check
.PHONY: license-check
.PHONY: lint
.PHONY: p0f-update
.PHONY: pcap-tools
.PHONY: release
.PHONY: dist
.PHONY: protobuf-tools
.PHONY: pull-dev-deps
.PHONY: remove-dev-deps
.PHONY: run
.PHONY: run-dev-deps
.PHONY: secret-scan
.PHONY: secret-scan-all
.PHONY: secret-scan-baseline
.PHONY: secret-scan-install
.PHONY: secret-scan-staged
.PHONY: stop-dev-deps
.PHONY: test
.PHONY: test-integration
.PHONY: test-report
.PHONY: ts-tools
.PHONY: uptrace-clear
.PHONY: uptrace-cli-help
.PHONY: uptrace-config
.PHONY: uptrace-init
.PHONY: uptrace-install
.PHONY: uptrace-start
.PHONY: uptrace-stop
.PHONY: uptrace-uninstall
.PHONY: vet

# ============================================================
# 依赖工具构建
# ============================================================

# create-test-dbs - 在 dev-deps (postgres/clickhouse) 内创建测试库 core_test / raw_test（幂等）
# 用法: make create-test-dbs
create-test-dbs:
	@scripts/dev-deps.sh create-dbs

# pull-dev-deps - 按本机 CPU 架构重新拉取开发依赖镜像 (解决 platform 不匹配 warning)
# 用法: make pull-dev-deps
pull-dev-deps:
	@scripts/dev-deps.sh pull

# run-dev-deps - 启动开发依赖容器 (redis, etcd, clickhouse, postgres, rustfs)
# 用法: make run-dev-deps
run-dev-deps:
	@scripts/dev-deps.sh start

# stop-dev-deps - 停止所有开发依赖容器 (不删除容器，保留数据)
# 用法: make stop-dev-deps
stop-dev-deps:
	@scripts/dev-deps.sh stop

# remove-dev-deps - 停止并删除所有开发依赖容器 (数据将丢失)
# 用法: make remove-dev-deps
remove-dev-deps:
	@scripts/dev-deps.sh remove

# dev-tools - 安装 Go 开发辅助工具 (asynq mon, pkgsite, goconvey)
# 用法: make dev-tools
dev-tools:
	go install github.com/hibiken/asynq/tools/asynq@latest
	go install golang.org/x/pkgsite/cmd/pkgsite@latest
	go install github.com/smartystreets/goconvey@v1.8.1

# protobuf-tools - 安装 protobuf 编译器及 Go 插件 (protoc-gen-go, protoc-gen-go-grpc)
# 用法: make protobuf-tools
protobuf-tools:
	@scripts/install-tools.sh protobuf

# ts-tools - 安装 TypeScript/Node.js 开发工具链 (node, npm, pnpm)
# 用法: make ts-tools
ts-tools:
	@scripts/install-tools.sh ts

# pcap-tools - 安装 libpcap 开发库 (抓包必需依赖)
# 用法: make pcap-tools
pcap-tools:
	@scripts/install-tools.sh pcap

# ============================================================
# 构建与版本管理
# ============================================================
#
# 版本号规则:
#   - 正式版本: vX.Y.Z（示例: v1.2.3）
#   - 预发布版本: vX.Y.Z-{alpha|beta|rc}.n（示例: v1.2.3-alpha.1, v1.2.3-rc.2）
#   - 版本号唯一来源是 annotated tag，由 make release 创建
#   - 开发构建不注入 SemVer，版本显示回落为 commit hash，如 "c2d4d21"
#   - Release 构建必须显式指定 VERSION，如 make build-release VERSION=v1.2.3-rc.1
#
# 编译时版本信息通过 -ldflags 注入到二进制，运行时可通过以下命令查看:
#   ./bin/zki version          # 精简输出，如 c2d4d21 或 v1.2.3
#   ./bin/zki version -v       # 详细输出，包含 Commit / BuildTime / Builder
#   go run cmd/zki/main.go version  # 开发时无需预编译，直接查看 commit hash
#
# 版本变量说明:
#   VERSION     - 语义化版本号，仅 release 构建使用
#   COMMIT      - git commit hash (短格式)
#   BUILD_TIME  - 构建时间 (ISO8601 UTC)
#   BUILDER     - 构建者用户名
#

VERSION ?=
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
BUILDER  ?= $(shell whoami)

# 开发构建只注入 commit 等信息，SemVer 留空，避免把 git describe 的非语义化结果当成版本号
DEV_LDFLAGS = -X zhongkuisec.com/zhongkui/internal/pkg/version.Commit=$(COMMIT) \
              -X zhongkuisec.com/zhongkui/internal/pkg/version.BuildTime=$(BUILD_TIME) \
              -X zhongkuisec.com/zhongkui/internal/pkg/version.Builder=$(BUILDER)

# Release 构建额外注入语义化版本号
RELEASE_LDFLAGS = $(DEV_LDFLAGS) \
                  -X zhongkuisec.com/zhongkui/internal/pkg/version.SemVer=$(VERSION)

# p0f 指纹签名库（随 Go 包内嵌）：
# p0f.fp 为上游基础库（可用 make p0f-update 刷新覆盖）；p0f.local.fp 为本地定制签名，
# 不随上游更新，两者拼接后由 internal/network/huginn 内嵌加载（见 database.go 的 embed）
P0F_FP := internal/network/huginn/p0f.fp

# p0f 签名库上游源（社区维护的 p0f3 数据库；官方 p0f 已停更，此为最活跃的社区维护版）
# 可通过 make p0f-update P0F_UPSTREAM_URL=<新地址> 覆盖
P0F_UPSTREAM_URL ?= https://raw.githubusercontent.com/peace-maker/p0f3-database/main/p0f.fp

internal/pkg/protobuf/reporter/v1/reporter.pb.go: api/reporter.proto
	PATH="$(GOBIN):$$PATH" protoc --go_out=internal/pkg/protobuf/reporter/v1 api/reporter.proto

internal/pkg/protobuf/reporter/v1/reporter_grpc.pb.go: api/reporter.proto
	PATH="$(GOBIN):$$PATH" protoc --go-grpc_out=internal/pkg/protobuf/reporter/v1 api/reporter.proto

internal/manager/views/search.go internal/manager/views/enum_bindings.go: cmd/template/main.go cmd/template/base.tmpl cmd/template/enum_bindings.tmpl internal/manager/views/search.toml
	go run cmd/template/main.go search-generate \
		--packageName="views" \
		--templatePath="cmd/template/base.tmpl" \
		--configPath="internal/manager/views/search.toml" \
		--outputPath="internal/manager/views/search.go" \
		--enumBindingsTemplatePath="cmd/template/enum_bindings.tmpl" \
		--enumBindingsOutputPath="internal/manager/views/enum_bindings.go"
	gofmt -w internal/manager/views/search.go internal/manager/views/enum_bindings.go

# p0f-update - 从社区维护的 p0f3 签名库刷新内嵌的上游 p0f.fp，并运行指纹测试
# 用法: make p0f-update
# 自定义上游源: make p0f-update P0F_UPSTREAM_URL=<新地址>
p0f-update:
	@scripts/p0f-update.sh "$(P0F_UPSTREAM_URL)"

# build - 编译 zki 二进制 (开发版本，版本显示回落为 commit hash)
# 用法: make build
build: internal/pkg/protobuf/reporter/v1/reporter.pb.go internal/pkg/protobuf/reporter/v1/reporter_grpc.pb.go internal/manager/views/search.go internal/manager/views/enum_bindings.go
	go build -ldflags "$(DEV_LDFLAGS)" -o bin/zki cmd/zki/main.go

# build-release - 编译 release 版本 (需手动指定并通过校验的语义化版本号，支持正式版和预发布版)
# 用法: make build-release VERSION=v1.2.3  或  make build-release VERSION=v1.2.3-alpha.1
build-release:
	@scripts/validate-semver.sh "$(VERSION)"
	go build -ldflags "$(RELEASE_LDFLAGS)" -o bin/zki cmd/zki/main.go

# release - 在 master 上创建发布标签 (要求 CHANGELOG 与 web/package.json 已对齐)
# 用法: make release VERSION=v1.2.3  或  make release VERSION=v1.2.3-rc.1
release:
	@scripts/release.sh "$(VERSION)"

# dist - 打包 release 制品与 sha256 校验和 (供 Gitee Release 上传)
# 用法: make dist VERSION=v1.2.3  或  make dist VERSION=v1.2.3-beta.1
dist:
	@scripts/dist.sh "$(VERSION)"

# run - 使用 go run 直接运行 zki
# 用法: make run ARGS="version" 或 make run ARGS="run web --config=config.toml"
run:
	go run cmd/zki/main.go $(ARGS)

# clean - 删除自动生成的 protobuf 代码
# 用法: make clean
clean:
	rm -f internal/pkg/protobuf/reporter/v1/reporter.pb.go
	rm -f internal/pkg/protobuf/reporter/v1/reporter_grpc.pb.go

# ============================================================
# 代码检查
# ============================================================

# lint - 运行自建注释检查 analyzer (cmd/commentlint)，校验包注释与导出声明的注释完备性
# 检查范围、豁免规则与口径见 AGENTS.md「Comments」，实现见 internal/lint/commentlint
# 诊断输出到 stderr，发现缺口时退出码为 3 (make 因此失败)，可直接作为 CI/提交前检查
# 用法: make lint
lint:
	go run cmd/commentlint/main.go ./...

# fmt-check - 检查已跟踪的 Go 文件是否符合 gofmt
# 用法: make fmt-check
fmt-check:
	@scripts/fmt-check.sh

# vet - 运行 go vet
# 用法: make vet
vet:
	go vet ./...

# test - 运行全部单元测试（终端友好，失败即停）
# 默认不依赖外部服务：PostgreSQL / ClickHouse 相关用例在未设置 ZKI_TEST_INTEGRATION=1 时自动 SKIP
# 用法: make test
test:
	go test ./...

# test-report - 以 Gitee Go 可直接解析的格式运行单元测试，并生成覆盖率报告
# 产物均位于 .cicd/golang-report/：report.jsonl 测试报告、index.html 覆盖率页面、coverage.txt 汇总
# 用法: make test-report
test-report:
	@scripts/test-report.sh

# test-integration - 运行依赖外部服务的集成测试（PostgreSQL / ClickHouse）
# 前置条件：make run-dev-deps && make create-test-dbs，或提供 ZKI_DATABASE_CORE_TEST_DSN / ZKI_DATABASE_RAW_TEST_DSN
# 用法: make test-integration
test-integration:
	ZKI_TEST_INTEGRATION=1 go test ./...

# secret-scan - 本地密钥扫描：只扫「将要提交」的文件，与 CI 的 Betterleaks 门禁一致
# 只扫描待提交文件，避免构建产物与既有测试夹具产生大量噪音；报告落在 .cicd/secret-scan/
# 用法: make secret-scan
secret-scan:
	@scripts/secret-scan.sh --changed

# secret-scan-staged - 仅扫描暂存区，适合接到 pre-commit 钩子
# 用法: make secret-scan-staged
secret-scan-staged:
	@scripts/secret-scan.sh --staged

# secret-scan-all - 扫描全部受版本控制的文件（与 CI 全量扫描口径一致，会包含测试夹具）
# 用法: make secret-scan-all
secret-scan-all:
	@scripts/secret-scan.sh --all

# secret-scan-baseline - 把当前扫描结果记为基线，后续只对新增发现失败
# 基线文件位于 .cicd/secret-scan/baseline.tsv（不纳入版本控制）
# 用法: make secret-scan-baseline
secret-scan-baseline:
	@scripts/secret-scan.sh --all --baseline

# secret-scan-install - 安装 Betterleaks（macOS / Linux / Windows Git Bash，x64 与 arm64）
# 默认从 GitHub Release 下载预编译二进制到 ~/.local/bin；偏好 Homebrew 可加 --brew
# 用法: make secret-scan-install
#       make secret-scan-install INSTALL_ARGS="--brew"
secret-scan-install:
	@scripts/install-betterleaks.sh $(INSTALL_ARGS)

# build-check - 编译全部包但不产出二进制，作为 CI 的构建门禁
# 用法: make build-check
build-check:
	go build ./...

# docs - 生成文档：叙述页、参考页、总目录与导航文件
# 用法: make docs
docs:
	go run cmd/zki/main.go create docs

# docs-check - 校验生成文档与代码声明一致，不写盘；CI 用它拦截文档漂移
# 用法: make docs-check
docs-check:
	go run cmd/zki/main.go create docs --check

# docs-install - 安装文档站点依赖（VitePress）
# 用法: make docs-install
docs-install:
	cd docs && pnpm install

# docs-dev - 本地启动文档站点（默认 http://localhost:5173）
# 用法: make docs-dev
docs-dev:
	cd docs && pnpm dev

# docs-build - 构建静态文档站点到 docs/.vitepress/dist
# 部署到子路径时用 DOCS_BASE 覆盖，例如 DOCS_BASE=/zhongkui/ make docs-build
# 用法: make docs-build
docs-build:
	cd docs && pnpm build

# docs-preview - 预览已构建的文档站点
# 用法: make docs-preview
docs-preview:
	cd docs && pnpm preview

# license-check - 第三方依赖许可证门禁：拒绝 GPL/AGPL/LGPL/SSPL/非商业与未知许可证
# Go 侧检查链接进 cmd/zki 的模块；Web 侧检查 pnpm 生产依赖（需要 pnpm）
# CI 设置 LICENSE_CHECK_REQUIRE_WEB=1 强制要求 Web 检查
# 用法: make license-check
license-check:
	go run cmd/licensecheck/main.go --root .

# ci - 提交与 CI 的统一入口：格式、构建、vet、测试、注释检查、许可证与文档漂移检查
# 失败即停，适合本地提交前快速自查
# 用法: make ci
ci: fmt-check build-check vet test lint license-check docs-check
	@echo "CI 检查全部通过"

# ci-report - 汇总式 CI：逐项执行检查并留存日志，跑完全部检查后再统一判失败
# 与 ci 的区别：
#   1. 不在第一个失败处中断，一次暴露所有问题；
#   2. 每项输出写入 $(REPORT_DIR)/<检查名>.log；
#   3. 测试改用 test-report，额外产出 Gitee Go 可解析的 .cicd/golang-report/。
# 用法: make ci-report
ci-report:
	@scripts/ci-report.sh

# ============================================================
# Wire 抓包实现基线测试
# ============================================================

# bench-prepare - 下载 tcpreplay 采样流量并生成 fallback pcap
# 用法: make bench-prepare
bench-prepare:
	@scripts/bench-prepare.sh

# bench-smoke - 快速冒烟测试 (无需 sudo，仅用内嵌 test.pcap)
# 用法: make bench-smoke
bench-smoke:
	@echo "=== Wire 抓包实现冒烟测试 (-short) ==="
	@echo "注意：完整抓包测试需要 sudo 权限，当前仅验证编译与跳过逻辑"
	@echo ""
	go test -v -bench=. -short -benchtime=1x ./internal/benchmark/capture/

# bench-slow - 慢速重放测试 (1ms 间隔，验证低速下丢包率)
# 用法: make bench-slow
bench-slow:
	@echo "=== Wire 抓包实现慢速重放测试 (1ms 间隔) ==="
	@if [ ! -f internal/benchmark/capture/testdata/smallFlows.pcap ]; then \
		echo "错误：smallFlows.pcap 不存在，请先运行 make bench-prepare"; \
		exit 1; \
	fi
	@echo "使用 $(SUDO) 运行慢速重放 Benchmark..."
	@echo ""
	$(SUDO) go test -v -bench=Slow -benchtime=1x ./internal/benchmark/capture/

# bench-fast - 全速基线测试 (排除 Slow，需 sudo + 已下载 smallFlows.pcap)
# 用法: make bench-fast
bench-fast:
	@echo "=== Wire 抓包实现全速基线测试 ==="
	@if [ ! -f internal/benchmark/capture/testdata/smallFlows.pcap ]; then \
		echo "错误：smallFlows.pcap 不存在，请先运行 make bench-prepare"; \
		exit 1; \
	fi
	@echo "使用 $(SUDO) 运行全速 Benchmark..."
	@echo ""
	$(SUDO) go test -v -bench='^(BenchmarkPlatform|BenchmarkLibpcap|BenchmarkXdp|BenchmarkAfpacket)(Smoke|SmallFlows|BigFlows)$$' -benchtime=1x ./internal/benchmark/capture/

# bench-raw - 纯计数基线测试 (skip copy，测量裸 capture 吞吐上限)
# 用法: make bench-raw
bench-raw:
	@echo "=== 纯计数捕获上限测试 (skip copy) ==="
	@if [ ! -f internal/benchmark/capture/testdata/smallFlows.pcap ]; then \
		echo "错误：smallFlows.pcap 不存在，请先运行 make bench-prepare"; \
		exit 1; \
	fi
	@echo "使用 $(SUDO) 运行 Raw Benchmark..."
	@echo ""
	$(SUDO) go test -v -bench='^BenchmarkRaw' -benchtime=1x ./internal/benchmark/capture/

# bench-xdp-phys - 物理网卡 XDP 测试 (需 2 张 AF_XDP 兼容网卡互联)
# 用法: make bench-xdp-phys
bench-xdp-phys:
	@echo "=== XDP 物理网卡测试 (需 2 张 AF_XDP 兼容网卡互联) ==="
	@if [ ! -f internal/benchmark/capture/testdata/smallFlows.pcap ]; then \
		echo "错误：smallFlows.pcap 不存在，请先运行 make bench-prepare"; \
		exit 1; \
	fi
	@echo "使用 $(SUDO) 运行物理网卡 XDP Benchmark..."
	@echo ""
	$(SUDO) go test -v -bench='^BenchmarkXdpNative' -benchtime=1x ./internal/benchmark/capture/

# bench-xdp-phys-raw - 物理网卡 XDP raw capture (skip copy) 测试
# 用法: make bench-xdp-phys-raw
bench-xdp-phys-raw:
	@echo "=== XDP 物理网卡 Raw Capture 测试 (需 2 张 AF_XDP 兼容网卡互联) ==="
	@if [ ! -f internal/benchmark/capture/testdata/smallFlows.pcap ]; then \
		echo "错误：smallFlows.pcap 不存在，请先运行 make bench-prepare"; \
		exit 1; \
	fi
	@echo "使用 $(SUDO) 运行物理网卡 XDP Raw Benchmark..."
	@echo ""
	$(SUDO) go test -v -bench='^BenchmarkRawXdpNative' -benchtime=1x ./internal/benchmark/capture/

# bench-probe - 探测本机网卡 XDP 支持情况
# 用法: make bench-probe
bench-probe:
	@echo "=== 网卡 XDP 支持探测 ==="
	go build -o /tmp/nic_probe ./cmd/nic_probe/
	$(SUDO) /tmp/nic_probe
	@rm -f /tmp/nic_probe

# ============================================================
# OpenTelemetry 后端工具 (uptrace/uptrace)
# ============================================================
# uptrace 是一个开源 APM 平台，支持 traces / metrics / logs 全链路监控。
# 依赖 ClickHouse + PostgreSQL + Redis（通过 make run-dev-deps 启动）。
#   - Web UI:      http://localhost:14318
#   - OTLP gRPC:   localhost:4317
#   - OTLP HTTP:   http://localhost:4318
# 安装流程参考: https://uptrace.dev/get/hosted/install
# 项目地址: https://github.com/uptrace/uptrace

UPTRACE_VERSION ?= 2.0.3
export UPTRACE_VERSION

# uptrace-install - 下载并安装 uptrace 二进制（已安装则跳过）
# 用法: make uptrace-install
uptrace-install:
	@scripts/uptrace.sh install

# uptrace-config - 使用 uptrace 官方命令生成配置文件，并修改为本机连接信息
# 用法: make uptrace-config
uptrace-config:
	@scripts/uptrace.sh config

# uptrace-init - 初始化数据库（按官方文档顺序执行迁移和种子数据）
# 用法: make uptrace-init
# 前置条件: make run-dev-deps, make uptrace-install, make uptrace-config
uptrace-init:
	@scripts/uptrace.sh init

# uptrace-start - 后台启动 uptrace serve
# 用法: make uptrace-start
# 前置条件: make uptrace-init
uptrace-start:
	@scripts/uptrace.sh start

# uptrace-stop - 停止后台运行的 uptrace serve
# 用法: make uptrace-stop
uptrace-stop:
	@scripts/uptrace.sh stop

# uptrace-clear - 清除 uptrace-init 创建的数据库及数据
# 用法: make uptrace-clear
# 注意: 此操作不可逆，将重置 ClickHouse 和 PostgreSQL 中的 uptrace schema
uptrace-clear:
	@scripts/uptrace.sh clear

# uptrace-uninstall - 卸载 uptrace 二进制和配置文件
# 用法: make uptrace-uninstall
uptrace-uninstall:
	@scripts/uptrace.sh uninstall

# uptrace-cli-help - 打印 uptrace CLI 常用命令参考
# 用法: make uptrace-cli-help
uptrace-cli-help:
	@scripts/uptrace.sh cli-help
