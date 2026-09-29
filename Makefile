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

.PHONY:
	run-dev-deps
	stop-dev-deps
	remove-dev-deps
	pull-dev-deps
	dev-tools
	protobuf-tools
	ts-tools
	pcap-tools
	p0f-update
	build
	build-release
	run
	clean
	lint
	bench-prepare
	bench-smoke
	bench-slow
	bench-fast
	bench-raw
	bench-xdp-phys
	bench-xdp-phys-raw
	bench-probe
	uptrace-install
	uptrace-config
	uptrace-init
	uptrace-start
	uptrace-stop
	uptrace-clear
	uptrace-uninstall
	uptrace-cli-help

# ============================================================
# 依赖工具构建
# ============================================================

# create-test-dbs - 在 dev-deps (postgres/clickhouse) 内创建测试库 core_test / raw_test（幂等）
# 用法: make create-test-dbs
create-test-dbs:
	@echo "创建测试数据库 core_test (postgres) 与 raw_test (clickhouse)..."
	@case "$(UNAME_S)" in \
		Darwin|MINGW*|MSYS*|CYGWIN*) \
			cmd=docker; \
			clickhouse_cli="$$cmd exec clickhouse-server clickhouse-client"; \
			psql_cli="PGPASSWORD=root_password $$cmd exec postgres psql -U postgres";; \
		Linux) \
			cmd=nerdctl; \
			clickhouse_cli="$$cmd exec clickhouse-server clickhouse-client"; \
			psql_cli="PGPASSWORD=root_password $$cmd exec postgres psql -U postgres";; \
		*) \
			echo "警告: 不支持的操作系统 '$(UNAME_S)'，请手动执行 psql/clickhouse-client 创建测试库。"; \
			exit 1 ;; \
	esac; \
	$$psql_cli -tAc "SELECT 1 FROM pg_database WHERE datname='core_test'" | grep -q 1 \
		&& echo "  core_test 已存在" \
		|| $$psql_cli -c "CREATE DATABASE core_test"; \
	$$clickhouse_cli -q "CREATE DATABASE IF NOT EXISTS raw_test"; \
	echo "测试库就绪 (core_test@postgres, raw_test@clickhouse)"

# pull-dev-deps - 按本机 CPU 架构重新拉取开发依赖镜像 (解决 platform 不匹配 warning)
# 通过 uname -m 探测宿主架构并显式指定 --platform，避免拉取到架构不匹配的镜像
# 用法: make pull-dev-deps
pull-dev-deps:
	@case "$(UNAME_S)" in \
		Darwin|MINGW*|MSYS*|CYGWIN*) \
			cmd=docker ;; \
		Linux) \
			cmd=nerdctl ;; \
		*) \
			echo "警告: 不支持的操作系统 '$(UNAME_S)'。" ; \
			exit 1 ;; \
	esac; \
	arch=$$(uname -m); \
	case "$$arch" in \
		arm64|aarch64) \
			platform=linux/arm64 ;; \
		x86_64|amd64) \
			platform=linux/amd64 ;; \
		*) \
			echo "警告: 无法识别的架构 '$$arch'，缺省使用 linux/amd64"; \
			platform=linux/amd64 ;; \
	esac; \
	echo "检测到宿主架构: $$arch，拉取平台: $$platform"; \
	for item in \
		"redis redis:8.2.1" \
		"etcd quay.io/coreos/etcd:v3.6.5" \
		"clickhouse clickhouse:25.3.3" \
		"postgres pgvector/pgvector:pg17" \
		"rustfs rustfs/rustfs:latest"; \
	do \
		set -- $$item; \
		name=$$1; img=$$2; \
		echo "=== 拉取 $$name ($$platform) ==="; \
		$$cmd pull --platform "$$platform" "$$img" || echo "  !! 拉取 $$name 失败，镜像可能不提供 $$platform 架构"; \
	done

# run-dev-deps - 启动开发依赖容器 (redis, etcd, clickhouse, postgres, rustfs)
# 用法: make run-dev-deps
run-dev-deps:
	@case "$(UNAME_S)" in \
		Darwin|MINGW*|MSYS*|CYGWIN*) \
			echo "使用 docker 启动依赖容器..."; \
			docker run -itd --restart unless-stopped --name redis --network host \
				redis:8.2.1; \
			docker run -itd --restart unless-stopped --name etcd --network host \
				quay.io/coreos/etcd:v3.6.5; \
			docker run -itd --restart unless-stopped --name clickhouse-server --network host \
				--ulimit nofile=262144:262144 \
				-e CLICKHOUSE_PASSWORD=root_password \
				clickhouse:25.3.3; \
			docker run -itd --restart unless-stopped --name postgres --network host \
				-e POSTGRES_PASSWORD=root_password \
				-e POSTGRES_USER=postgres \
				-e POSTGRES_DB=default \
				pgvector/pgvector:pg17; \
			docker run -itd --restart unless-stopped --name rustfs --network host \
				-e RUSTFS_ADDRESS=localhost:9002 \
				-e RUSTFS_CONSOLE_ENABLE=true \
				-e RUSTFS_ACCESS_KEY=rustfsadmin \
				-e RUSTFS_SECRET_KEY=rustfsadmin \
				rustfs/rustfs:latest ;; \
		Linux) \
			echo "使用 nerdctl 启动依赖容器..."; \
			nerdctl run -td --restart=always --name redis --net=host \
				redis:8.2.1; \
			nerdctl run -td --restart=always --name etcd --net=host \
				quay.io/coreos/etcd:v3.6.5; \
			nerdctl run -td --restart=always --name clickhouse-server --net=host \
				--ulimit nofile=262144:262144 \
				-e CLICKHOUSE_PASSWORD=root_password \
				clickhouse:25.3.3; \
			nerdctl run -td --restart=always --name postgres --net=host \
				-e POSTGRES_PASSWORD=root_password \
				-e POSTGRES_USER=postgres \
				-e POSTGRES_DB=default \
				pgvector/pgvector:pg17; \
			nerdctl run -td --restart=always --name rustfs --net=host \
				-e RUSTFS_ADDRESS=localhost:9002 \
				-e RUSTFS_CONSOLE_ENABLE=true \
				-e RUSTFS_ACCESS_KEY=rustfsadmin \
				-e RUSTFS_SECRET_KEY=rustfsadmin \
				rustfs/rustfs:latest ;; \
		*) \
			echo "警告: 不支持的操作系统 '$(UNAME_S)'，请手动启动依赖服务。"; \
			exit 1 ;; \
	esac

# stop-dev-deps - 停止所有开发依赖容器 (不删除容器，保留数据)
# 用法: make stop-dev-deps
stop-dev-deps:
	@echo "停止开发依赖容器..."
	@case "$(UNAME_S)" in \
		Darwin|MINGW*|MSYS*|CYGWIN*) \
			cmd=docker ;; \
		Linux) \
			cmd=nerdctl ;; \
		*) \
			echo "警告: 不支持的操作系统 '$(UNAME_S)'。"; \
			exit 1 ;; \
	esac; \
	for container in redis etcd clickhouse-server postgres rustfs; do \
		if $$cmd ps --format '{{.Names}}' | grep -q "^$$container$$" 2>/dev/null; then \
			echo "  正在停止 $$container..."; \
			$$cmd stop $$container; \
		else \
			echo "  $$container 未在运行，跳过"; \
		fi \
	done

# remove-dev-deps - 停止并删除所有开发依赖容器 (数据将丢失)
# 用法: make remove-dev-deps
remove-dev-deps: stop-dev-deps
	@echo "删除开发依赖容器..."
	@case "$(UNAME_S)" in \
		Darwin|MINGW*|MSYS*|CYGWIN*) \
			cmd=docker ;; \
		Linux) \
			cmd=nerdctl ;; \
		*) \
			echo "警告: 不支持的操作系统 '$(UNAME_S)'。"; \
			exit 1 ;; \
	esac; \
	for container in redis etcd clickhouse-server postgres rustfs; do \
		if $$cmd ps -a --format '{{.Names}}' | grep -q "^$$container$$" 2>/dev/null; then \
			echo "  正在删除 $$container..."; \
			$$cmd rm -f $$container; \
		else \
			echo "  $$container 不存在，跳过"; \
		fi \
	done

# dev-tools - 安装 Go 开发辅助工具 (asynq mon, pkgsite, goconvey)
# 用法: make dev-tools
dev-tools:
	go install github.com/hibiken/asynq/tools/asynq@latest
	go install golang.org/x/pkgsite/cmd/pkgsite@latest
	go install github.com/smartystreets/goconvey@v1.8.1

# protobuf-tools - 安装 protobuf 编译器及 Go 插件 (protoc-gen-go, protoc-gen-go-grpc)
# 用法: make protobuf-tools
protobuf-tools:
	@case "$(UNAME_S)" in \
		Darwin) \
			HOMEBREW_NO_AUTO_UPDATE=1 brew install protobuf ;; \
		Linux) \
			apt install -y protobuf-compiler ;; \
		MINGW*|MSYS*|CYGWIN*) \
			echo "请从 https://github.com/protocolbuffers/protobuf/releases 手动安装 protobuf" ;; \
		*) \
			echo "警告: 不支持的操作系统 '$(UNAME_S)'，请手动安装 protobuf。" ;; \
	esac
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	@case "$(UNAME_S)" in \
		Darwin) \
			shell_profile=~/.zshrc; \
			if [ -f ~/.bash_profile ]; then shell_profile=~/.bash_profile; fi; \
			echo 'export PATH=$$PATH:~/go/bin' >> $$shell_profile ;; \
		*) \
			echo 'export PATH=$$PATH:~/go/bin' >> ~/.bashrc ;; \
	esac

# ts-tools - 安装 TypeScript/Node.js 开发工具链 (node, npm, pnpm)
# 用法: make ts-tools
ts-tools:
	@case "$(UNAME_S)" in \
		Darwin) \
			if ! command -v node >/dev/null 2>&1; then \
				HOMEBREW_NO_AUTO_UPDATE=1 brew install node; \
			else \
				echo "node 已安装，跳过"; \
			fi ;; \
		Linux) \
			if ! command -v node >/dev/null 2>&1; then \
				curl -fsSL https://deb.nodesource.com/setup_22.x | bash - && apt install -y nodejs; \
			else \
				echo "node 已安装，跳过"; \
			fi ;; \
		MINGW*|MSYS*|CYGWIN*) \
			if ! command -v node >/dev/null 2>&1; then \
				echo "请从 https://nodejs.org 安装 Node.js 后重新执行该目标"; \
				exit 1; \
			fi ;; \
		*) \
			if ! command -v node >/dev/null 2>&1; then \
				echo "警告: 不支持的操作系统 '$(UNAME_S)'，请从 https://nodejs.org 手动安装 Node.js"; \
				exit 1; \
			fi ;; \
	esac
	npm config set registry=https://registry.npmjs.org/
	npm install -g npm@10
	npm install -g pnpm@latest-10
	pnpm self-update

# pcap-tools - 安装 libpcap 开发库 (抓包必需依赖)
# 用法: make pcap-tools
pcap-tools:
	@case "$(UNAME_S)" in \
		Darwin) \
			xcode-select -p >/dev/null 2>&1 || xcode-select --install; \
			echo "libpcap 已包含在 macOS Xcode Command Line Tools 中" ;; \
		Linux) \
			apt install -y libpcap-dev ;; \
		MINGW*|MSYS*|CYGWIN*) \
			echo "请从 https://npcap.com/dist/ 安装 Npcap SDK" ;; \
		*) \
			echo "警告: 不支持的操作系统 '$(UNAME_S)'，请手动安装 libpcap。" ;; \
	esac

# ============================================================
# 构建与版本管理
# ============================================================
#
# 版本号规则:
#   - 开发构建: 版本号 = git describe --tags --always --dirty，如 "c2d4d21-dirty"
#   - Release 构建: 版本号 = 语义化版本，如 "v1.2.3"，通过 make build-release VERSION=v1.2.3 设置
#
# 编译时版本信息通过 -ldflags 注入到二进制，运行时可通过以下命令查看:
#   ./bin/zki version          # 精简输出，如 c2d4d21 或 v1.2.3
#   ./bin/zki version -v       # 详细输出，包含 Commit / BuildTime / Builder
#   go run cmd/zki/main.go version  # 开发时无需预编译，直接查看 commit hash
#
# 版本变量说明:
#   VERSION     - 语义化版本号 / git describe 结果，优先显示
#   COMMIT      - git commit hash (短格式)
#   BUILD_TIME  - 构建时间 (ISO8601 UTC)
#   BUILDER     - 构建者用户名
#

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "unknown")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
BUILDER  ?= $(shell whoami)

LDFLAGS = -X zhongkui.com/zhongkui/internal/pkg/version.SemVer=$(VERSION) \
          -X zhongkui.com/zhongkui/internal/pkg/version.Commit=$(COMMIT) \
          -X zhongkui.com/zhongkui/internal/pkg/version.BuildTime=$(BUILD_TIME) \
          -X zhongkui.com/zhongkui/internal/pkg/version.Builder=$(BUILDER)

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
# 本地定制签名保存在 p0f.local.fp，不会被上游覆盖；刷新后若指纹基线变化，
# 需人工确认 TestFingerprintFromPcap 的结果并更新 internal/network/detectors/fingerprint_pcap_test.go
# 用法: make p0f-update
# 自定义上游源: make p0f-update P0F_UPSTREAM_URL=<新地址>
p0f-update:
	@echo "=== 下载上游 p0f 签名库: $(P0F_UPSTREAM_URL) ==="
	@curl -fsSL "$(P0F_UPSTREAM_URL)" -o "$(P0F_FP).tmp" || { echo "下载失败，请检查网络或 P0F_UPSTREAM_URL"; exit 1; }
	@echo "=== 下载完成，校验格式（行数/首个 section）==="
	@wc -l "$(P0F_FP).tmp"
	@grep -m1 '^\[tcp:request\]' "$(P0F_FP).tmp" >/dev/null || { echo "校验失败：下载内容不含 [tcp:request] section，中止"; rm -f "$(P0F_FP).tmp"; exit 1; }
	@mv "$(P0F_FP).tmp" "$(P0F_FP)"
	@echo "=== 运行指纹测试验证 ==="
	go test ./internal/network/detectors -run '^TestFingerprintFromPcap$$' -v

# build - 编译 zki 二进制 (开发版本，版本号自动取 git commit)
# 用法: make build
build: internal/pkg/protobuf/reporter/v1/reporter.pb.go internal/pkg/protobuf/reporter/v1/reporter_grpc.pb.go internal/manager/views/search.go internal/manager/views/enum_bindings.go
	go build -ldflags "$(LDFLAGS)" -o bin/zki cmd/zki/main.go

# build-release - 编译 release 版本 (需手动指定语义化版本号)
# 用法: make build-release VERSION=v1.2.3
build-release:
	@if [ -z "$(VERSION)" ] || [ "$(VERSION)" = "unknown" ]; then \
		echo "ERROR: 请设置 VERSION 为语义化版本号，例如: make build-release VERSION=v1.2.3"; \
		exit 1; \
	fi
	go build -ldflags "$(LDFLAGS)" -o bin/zki cmd/zki/main.go

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

# ============================================================
# Wire 抓包实现基线测试
# ============================================================

# bench-prepare - 下载 tcpreplay 采样流量并生成 fallback pcap
# 用法: make bench-prepare
bench-prepare:
	@echo "=== 下载 tcpreplay 采样流量 ==="
	@mkdir -p internal/benchmark/capture/testdata
	@curl -sL -o internal/benchmark/capture/testdata/test.pcap \
		https://s3.amazonaws.com/tcpreplay-pcap-files/test.pcap && \
		echo "  test.pcap (141 包)     \xe2\x9c\x93" || \
		echo "  test.pcap              \xe2\x9c\x97"
	@curl -sL -o internal/benchmark/capture/testdata/smallFlows.pcap \
		https://s3.amazonaws.com/tcpreplay-pcap-files/smallFlows.pcap && \
		echo "  smallFlows.pcap (14k 包) \xe2\x9c\x93" || \
		echo "  smallFlows.pcap          \xe2\x9c\x97"
	@curl -sL -o internal/benchmark/capture/testdata/bigFlows.pcap \
		https://s3.amazonaws.com/tcpreplay-pcap-files/bigFlows.pcap && \
		echo "  bigFlows.pcap (790k 包)   \xe2\x9c\x93" || \
		echo "  bigFlows.pcap             \xe2\x9c\x97"
	@echo "=== 生成 fallback pcap (10,000 包) ==="
	@go run cmd/template/main.go gen-fallback-pcap \
		--output internal/benchmark/capture/testdata/fallback.pcap \
		--numPackets 10000
	@echo "=== 完成 ==="
	@ls -lh internal/benchmark/capture/testdata/

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
UPTRACE_BIN := $(GOBIN)/uptrace
UPTRACE_CONFIG := config/uptrace.yml
UPTRACE_PID := /tmp/uptrace.pid
ifeq ($(UNAME_S),Linux)
UPTRACE_DOCKER_CMD := nerdctl
else
UPTRACE_DOCKER_CMD := docker
endif

# sed 原地编辑参数（macOS 的 BSD sed 需要 -i '' 写法，Linux GNU sed 为 -i）
ifeq ($(UNAME_S),Darwin)
SED_I := sed -i ''
else
SED_I := sed -i
endif

# uptrace-install - 下载并安装 uptrace 二进制（已安装则跳过）
# 用法: make uptrace-install
uptrace-install:
	@if [ -f $(UPTRACE_BIN) ] && [ -x $(UPTRACE_BIN) ]; then \
		echo "uptrace 已安装: $(UPTRACE_BIN)"; \
		$(UPTRACE_BIN) version || true; \
		exit 0; \
	fi
	@echo "=== 安装 uptrace v$(UPTRACE_VERSION) ==="
	@mkdir -p $(GOBIN)
	@case "$(UNAME_S)" in \
		Darwin) \
			arch=$$(uname -m); \
			if [ "$$arch" = "arm64" ]; then \
				url="https://github.com/uptrace/uptrace/releases/download/v$(UPTRACE_VERSION)/uptrace_darwin_arm64"; \
			else \
				url="https://github.com/uptrace/uptrace/releases/download/v$(UPTRACE_VERSION)/uptrace_darwin_amd64"; \
			fi ;; \
		Linux) \
			arch=$$(uname -m); \
			if [ "$$arch" = "aarch64" ]; then \
				url="https://github.com/uptrace/uptrace/releases/download/v$(UPTRACE_VERSION)/uptrace_linux_arm64"; \
			else \
				url="https://github.com/uptrace/uptrace/releases/download/v$(UPTRACE_VERSION)/uptrace_linux_amd64"; \
			fi ;; \
		*) \
			echo "不支持的平台，请手动下载: https://github.com/uptrace/uptrace/releases"; \
			exit 1 ;; \
	esac; \
	echo "  下载: $$url"; \
	curl -sL -o $(UPTRACE_BIN) "$$url" && chmod +x $(UPTRACE_BIN)
	@echo "  已安装到 $(UPTRACE_BIN)"
	@$(UPTRACE_BIN) version || true

# uptrace-config - 使用 uptrace 官方命令生成配置文件，并修改为本机连接信息
# 用法: make uptrace-config
# 依赖: make uptrace-install
uptrace-config:
	@if [ ! -f $(UPTRACE_BIN) ]; then \
		echo "uptrace 未安装，请先运行 make uptrace-install"; \
		exit 1; \
	fi
	@if [ -f $(UPTRACE_CONFIG) ]; then \
		echo "配置文件已存在: $(UPTRACE_CONFIG)，跳过生成"; \
	else \
		echo "=== 生成配置文件 $(UPTRACE_CONFIG) ==="; \
		mkdir -p $$(dirname $(UPTRACE_CONFIG)); \
		$(UPTRACE_BIN) --config=$(UPTRACE_CONFIG) config create; \
		$(SED_I) '/^pg:/,/^[a-z]/{s/^  user: .*/  user: postgres/;s/^  password: .*/  password: root_password/;}' $(UPTRACE_CONFIG); \
		$(SED_I) '/^ch_cluster:/,/^[a-z]/{s/^          user: .*/          user: default/;s/^          password: .*/          password: ""/;}' $(UPTRACE_CONFIG); \
		$(SED_I) 's|^    addr: :80|    addr: :14318|' $(UPTRACE_CONFIG); \
		$(SED_I) 's|^  url: http://uptrace.local|  url: http://localhost:14318|' $(UPTRACE_CONFIG); \
		$(SED_I) 's|^  dsn: http://project1_secret@uptrace.local?grpc=4317|  dsn: http://project1_secret@localhost:14318?grpc=4317|' $(UPTRACE_CONFIG); \
		$(SED_I) '/^redis_cache:/,/^[a-z]/{s/^  password: .*/  password: ""/;}' $(UPTRACE_CONFIG); \
		echo "  配置文件已生成 ($(UPTRACE_CONFIG))"; \
	fi

# uptrace-init - 初始化数据库（按官方文档顺序执行迁移和种子数据）
# 用法: make uptrace-init
# 前置条件: make run-dev-deps, make uptrace-install, make uptrace-config
uptrace-init:
	@if [ ! -f $(UPTRACE_BIN) ]; then \
		echo "uptrace 未安装，请先运行 make uptrace-install"; \
		exit 1; \
	fi
	@if [ ! -f $(UPTRACE_CONFIG) ]; then \
		echo "配置文件 $(UPTRACE_CONFIG) 不存在，请先运行 make uptrace-config"; \
		exit 1; \
	fi
	@echo "=== 创建 ClickHouse 数据库 ==="
	@$(UPTRACE_DOCKER_CMD) exec clickhouse-server clickhouse-client -q "CREATE DATABASE IF NOT EXISTS uptrace" 2>/dev/null; \
	if [ $$? -ne 0 ]; then \
		echo "  ClickHouse 不可用，请先运行 make run-dev-deps"; \
		exit 1; \
	fi
	@echo "  数据库已就绪"
	@echo ""
	@echo "=== 创建 PostgreSQL 数据库 ==="
	@$(UPTRACE_DOCKER_CMD) exec postgres psql -U postgres -c "CREATE DATABASE uptrace" 2>/dev/null || true
	@echo "  数据库已就绪"
	@echo ""
	@echo "=== 初始化 PostgreSQL ==="
	@$(UPTRACE_BIN) --config=$(UPTRACE_CONFIG) pg init
	@echo ""
	@echo "=== 运行 PostgreSQL 迁移 ==="
	@$(UPTRACE_BIN) --config=$(UPTRACE_CONFIG) pg migrate
	@echo ""
	@echo "=== 初始化 ClickHouse ==="
	@$(UPTRACE_BIN) --config=$(UPTRACE_CONFIG) ch init
	@echo ""
	@echo "=== 运行 ClickHouse 迁移 ==="
	@$(UPTRACE_BIN) --config=$(UPTRACE_CONFIG) ch migrate
	@echo ""
	@echo "=== 创建 ClickHouse 分布式表 ==="
	@$(UPTRACE_BIN) --config=$(UPTRACE_CONFIG) ch dist
	@echo ""
	@echo "=== 写入种子数据 ==="
	@$(UPTRACE_BIN) --config=$(UPTRACE_CONFIG) db seed 2>/dev/null || echo "  (种子数据将在 serve 首次启动时加载)"
	@echo ""
	@echo "数据库初始化完成"

# uptrace-start - 后台启动 uptrace serve
# 用法: make uptrace-start
# 前置条件: make uptrace-init
uptrace-start:
	@if [ ! -f $(UPTRACE_BIN) ]; then \
		echo "uptrace 未安装，请先运行 make uptrace-install"; \
		exit 1; \
	elif [ -f $(UPTRACE_PID) ] && kill -0 $$(cat $(UPTRACE_PID)) 2>/dev/null; then \
		echo "uptrace 已在运行 (PID $$(cat $(UPTRACE_PID)))"; \
		exit 0; \
	else \
		echo "=== 后台启动 uptrace serve ==="; \
		nohup $(UPTRACE_BIN) --config=$(UPTRACE_CONFIG) serve > /tmp/uptrace.log 2>&1 & echo $$! > $(UPTRACE_PID); \
		sleep 3; \
		if kill -0 $$(cat $(UPTRACE_PID)) 2>/dev/null; then \
			echo "uptrace 已启动 (PID $$(cat $(UPTRACE_PID)))"; \
		else \
			echo "uptrace 启动失败，查看日志:"; \
			tail -20 /tmp/uptrace.log; \
			rm -f $(UPTRACE_PID); \
			exit 1; \
		fi; \
	fi
	@echo ""
	@echo "┌──────────────────────────────────────────────────────────┐"
	@echo "│  uptrace Web UI (traces / metrics / logs 可视化)         │"
	@echo "│    http://localhost:14318                                  │"
	@echo "├──────────────────────────────────────────────────────────┤"
	@echo "│  OTLP gRPC 端点 (应用上报地址)                              │"
	@echo "│    localhost:4317                                          │"
	@echo "├──────────────────────────────────────────────────────────┤"
	@echo "│  OTLP HTTP 端点                                            │"
	@echo "│    http://localhost:4318                                   │"
	@echo "├──────────────────────────────────────────────────────────┤"
	@echo "│  首次使用请在 Web UI 中创建项目，复制项目的 DSN            │"
	@echo "│  (Data Source Name)，格式如下:                             │"
	@echo "│    https://<token>@localhost:14318?grpc=4317             │"
	@echo "└──────────────────────────────────────────────────────────┘"
	@echo ""
	@echo "在运行应用之前，设置以下环境变量以启用 OpenTelemetry 上报:"
	@echo ""
	@echo "  export OTEL_SERVICE_NAME=zki"
	@echo "  export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317"
	@echo '  export OTEL_EXPORTER_OTLP_HEADERS="uptrace-dsn=http://project1_secret@localhost:14318?grpc=4317"'
	@echo "  export OTEL_EXPORTER_OTLP_COMPRESSION=gzip"
	@echo "  export OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE=DELTA"
	@echo "  export OTEL_EXPORTER_OTLP_METRICS_DEFAULT_HISTOGRAM_AGGREGATION=BASE2_EXPONENTIAL_BUCKET_HISTOGRAM"
	@echo ""
	@echo "  # 然后正常运行应用:"
	@echo "  go run cmd/zki/main.go run"
	@echo ""
	@echo "日志文件: tail -f /tmp/uptrace.log"
	@echo "CLI 帮助: make uptrace-cli-help"
	@echo "停止服务: make uptrace-stop"

# uptrace-stop - 停止后台运行的 uptrace serve
# 用法: make uptrace-stop
uptrace-stop:
	@if [ -f $(UPTRACE_PID) ] && kill -0 $$(cat $(UPTRACE_PID)) 2>/dev/null; then \
		pid=$$(cat $(UPTRACE_PID)); \
		echo "=== 停止 uptrace (PID $$pid) ==="; \
		kill $$pid 2>/dev/null || true; \
		sleep 1; \
		if kill -0 $$pid 2>/dev/null; then \
			echo "强制终止..."; \
			kill -9 $$pid 2>/dev/null || true; \
		fi; \
		rm -f $(UPTRACE_PID); \
		echo "已停止"; \
	else \
		echo "uptrace 未在运行"; \
		rm -f $(UPTRACE_PID); \
	fi

# uptrace-clear - 清除 uptrace-init 创建的数据库及数据
# 用法: make uptrace-clear
# 注意: 此操作不可逆，将重置 ClickHouse 和 PostgreSQL 中的 uptrace schema
uptrace-clear:
	@if [ -f $(UPTRACE_PID) ] && kill -0 $$(cat $(UPTRACE_PID)) 2>/dev/null; then \
		echo "uptrace 正在运行，请先执行 make uptrace-stop"; \
		exit 1; \
	fi
	@if [ ! -f $(UPTRACE_BIN) ]; then \
		echo "uptrace 未安装，请先运行 make uptrace-install"; \
		exit 1; \
	fi
	@echo "=== 清除 uptrace 数据 ==="
	@echo ""
	@echo "--- 重置 ClickHouse schema ---"
	@$(UPTRACE_BIN) --config=$(UPTRACE_CONFIG) ch reset
	@echo ""
	@echo "--- 重置 PostgreSQL schema ---"
	@$(UPTRACE_BIN) --config=$(UPTRACE_CONFIG) pg reset
	@echo ""
	@echo "清除完成，可通过 make uptrace-init 重新初始化"

# uptrace-uninstall - 卸载 uptrace 二进制和配置文件
# 用法: make uptrace-uninstall
uptrace-uninstall:
	@if [ -f $(UPTRACE_PID) ] && kill -0 $$(cat $(UPTRACE_PID)) 2>/dev/null; then \
		echo "uptrace 正在运行，请先执行 make uptrace-stop"; \
		exit 1; \
	fi
	@echo "=== 卸载 uptrace ==="
	@rm -f $(UPTRACE_BIN) && echo "  已删除 $(UPTRACE_BIN)" || true
	@rm -f $(UPTRACE_CONFIG) && echo "  已删除 $(UPTRACE_CONFIG)" || true
	@rm -f $(UPTRACE_PID)
	@echo "卸载完成"

# uptrace-cli-help - 打印 uptrace CLI 常用命令参考
# 用法: make uptrace-cli-help
uptrace-cli-help:
	@echo "uptrace CLI 常用命令:"
	@echo ""
	@echo "  uptrace version         - 查看版本"
	@echo "  uptrace serve           - 前台启动服务 (调试用)"
	@echo "  uptrace config create   - 生成配置文件模板"
	@echo "  uptrace config validate - 验证配置文件"
	@echo ""
	@echo "  数据库管理 (按官方文档顺序):"
	@echo "  uptrace pg init         - 初始化 PostgreSQL (仅首次)"
	@echo "  uptrace pg migrate      - 运行 PostgreSQL 迁移"
	@echo "  uptrace ch init         - 初始化 ClickHouse (仅首次)"
	@echo "  uptrace ch migrate      - 运行 ClickHouse 迁移"
	@echo "  uptrace ch dist         - 创建 ClickHouse 分布式表"
	@echo "  uptrace db seed         - 写入项目/用户种子数据"
	@echo ""
	@echo "  在浏览器打开 http://localhost:14318 查看 Web UI"

