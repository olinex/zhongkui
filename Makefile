# Detect OS platform: Darwin (macOS), Linux, or Windows (MINGW/MSYS/CYGWIN)
UNAME_S := $(shell uname -s)
GOBIN := $(shell go env GOBIN)
ifeq ($(UNAME_S),Darwin)
LAN_IP := $(shell ipconfig getifaddr en0 2>/dev/null || echo localhost)
CGO_LDFLAGS += -Wl,-no_warn_duplicate_libraries
export CGO_LDFLAGS
else ifeq ($(UNAME_S),Linux)
LAN_IP := $(shell hostname -I 2>/dev/null | awk '{print $$1}' || echo localhost)
else
LAN_IP := localhost
endif

# 需要 sudo 才能抓包的平台（可通过 SUDO= make ... 跳过）
ifeq ($(UNAME_S),Linux)
SUDO := sudo
else
SUDO := sudo
endif

.PHONY:
	run-dev-deps
	stop-dev-deps
	remove-dev-deps
	dev-tools
	protobuf-tools
	ts-tools
	pcap-tools
	build
	clean
	bench-prepare
	bench-smoke
	bench-slow
	bench-fast
	bench-raw

# run-dev-deps - 启动开发依赖容器 (redis, etcd, clickhouse, postgres, rustfs)
# 用法: make run-dev-deps
run-dev-deps:
	@case "$(UNAME_S)" in \
		Darwin) \
			echo "Starting dependencies with Orbstack (docker)..."; \
			docker run -itd --restart unless-stopped --name redis --network host \
				redis:8.2.1; \
			docker run -itd --restart unless-stopped --name etcd --network host \
				quay.io/coreos/etcd:v3.6.5; \
			docker run -itd --restart unless-stopped --name clickhouse-server --network host \
				--ulimit nofile=262144:262144 \
				clickhouse:26.3; \
			docker run -itd --restart unless-stopped --name postgres --network host \
				-e POSTGRES_PASSWORD=root_password \
				-e POSTGRES_USER=postgres \
				-e POSTGRES_DB=default \
				postgres:16-alpine3.22; \
			docker run -itd --restart unless-stopped --name rustfs --network host \
				-e RUSTFS_ADDRESS=localhost:9002 \
				-e RUSTFS_CONSOLE_ENABLE=true \
				-e RUSTFS_ACCESS_KEY=rustfsadmin \
				-e RUSTFS_SECRET_KEY=rustfsadmin \
				rustfs/rustfs:latest ;; \
		Linux) \
			echo "Starting dependencies with nerdctl..."; \
			nerdctl run -td --restart=always --name redis --net=host \
				redis:8.2.1; \
			nerdctl run -td --restart=always --name etcd --net=host \
				quay.io/coreos/etcd:v3.6.5; \
			nerdctl run -td --restart=always --name clickhouse-server --net=host \
				--ulimit nofile=262144:262144 \
				clickhouse:26.3; \
			nerdctl run -td --restart=always --name postgres --net=host \
				-e POSTGRES_PASSWORD=root_password \
				-e POSTGRES_USER=postgres \
				-e POSTGRES_DB=default \
				postgres:16-alpine3.22; \
			nerdctl run -td --restart=always --name rustfs --net=host \
				-e RUSTFS_ADDRESS=localhost:9002 \
				-e RUSTFS_CONSOLE_ENABLE=true \
				-e RUSTFS_ACCESS_KEY=rustfsadmin \
				-e RUSTFS_SECRET_KEY=rustfsadmin \
				rustfs/rustfs:latest ;; \
		MINGW*|MSYS*|CYGWIN*) \
			echo "Starting dependencies with Docker Desktop..."; \
			docker run -itd --restart unless-stopped --name redis --network host \
				redis:8.2.1; \
			docker run -itd --restart unless-stopped --name etcd --network host \
				quay.io/coreos/etcd:v3.6.5; \
			docker run -itd --restart unless-stopped --name clickhouse-server --network host \
				--ulimit nofile=262144:262144 \
				clickhouse:26.3; \
			docker run -itd --restart unless-stopped --name postgres --network host \
				-e POSTGRES_PASSWORD=root_password \
				-e POSTGRES_USER=postgres \
				-e POSTGRES_DB=default \
				postgres:16-alpine3.22; \
			docker run -itd --restart unless-stopped --name rustfs --network host \
				-e RUSTFS_ADDRESS=localhost:9002 \
				-e RUSTFS_CONSOLE_ENABLE=true \
				-e RUSTFS_ACCESS_KEY=rustfsadmin \
				-e RUSTFS_SECRET_KEY=rustfsadmin \
				rustfs/rustfs:latest ;; \
		*) \
			echo "Warning: Unsupported OS '$(UNAME_S)'. Please start dependencies manually."; \
			exit 1 ;; \
	esac

# stop-dev-deps - 停止所有开发依赖容器 (不删除容器，保留数据)
# 用法: make stop-dev-deps
stop-dev-deps:
	@echo "Stopping development dependencies..."
	@case "$(UNAME_S)" in \
		Darwin|MINGW*|MSYS*|CYGWIN*) \
			cmd=docker ;; \
		Linux) \
			cmd=nerdctl ;; \
		*) \
			echo "Warning: Unsupported OS '$(UNAME_S)'."; \
			exit 1 ;; \
	esac; \
	for container in redis etcd clickhouse-server postgres rustfs; do \
		if $$cmd ps --format '{{.Names}}' | grep -q "^$$container$$" 2>/dev/null; then \
			echo "  Stopping $$container..."; \
			$$cmd stop $$container; \
		else \
			echo "  $$container not running, skipping"; \
		fi \
	done

# remove-dev-deps - 停止并删除所有开发依赖容器 (数据将丢失)
# 用法: make remove-dev-deps
remove-dev-deps: stop-dev-deps
	@echo "Removing development dependencies..."
	@case "$(UNAME_S)" in \
		Darwin|MINGW*|MSYS*|CYGWIN*) \
			cmd=docker ;; \
		Linux) \
			cmd=nerdctl ;; \
		*) \
			echo "Warning: Unsupported OS '$(UNAME_S)'."; \
			exit 1 ;; \
	esac; \
	for container in redis etcd clickhouse-server postgres rustfs; do \
		if $$cmd ps -a --format '{{.Names}}' | grep -q "^$$container$$" 2>/dev/null; then \
			echo "  Removing $$container..."; \
			$$cmd rm -f $$container; \
		else \
			echo "  $$container not found, skipping"; \
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
			echo "Please install protobuf manually from https://github.com/protocolbuffers/protobuf/releases" ;; \
		*) \
			echo "Warning: Unsupported OS '$(UNAME_S)'. Please install protobuf manually." ;; \
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
				echo "node already installed, skipping"; \
			fi ;; \
		Linux) \
			if ! command -v node >/dev/null 2>&1; then \
				curl -fsSL https://deb.nodesource.com/setup_22.x | bash - && apt install -y nodejs; \
			else \
				echo "node already installed, skipping"; \
			fi ;; \
		MINGW*|MSYS*|CYGWIN*) \
			if ! command -v node >/dev/null 2>&1; then \
				echo "Please install Node.js from https://nodejs.org and re-run this target"; \
				exit 1; \
			fi ;; \
		*) \
			if ! command -v node >/dev/null 2>&1; then \
				echo "Warning: Unsupported OS '$(UNAME_S)'. Please install Node.js manually from https://nodejs.org"; \
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
			echo "libpcap is included with macOS Xcode Command Line Tools" ;; \
		Linux) \
			apt install -y libpcap-dev ;; \
		MINGW*|MSYS*|CYGWIN*) \
			echo "Please install Npcap SDK from https://npcap.com/dist/" ;; \
		*) \
			echo "Warning: Unsupported OS '$(UNAME_S)'. Please install libpcap manually." ;; \
	esac

# clean - 删除自动生成的 protobuf 代码
# 用法: make clean
clean:
	rm -rf internal/pkg/protobuf/reporter/v1/reporter.pb.go
	rm -rf internal/pkg/protobuf/reporter/v1/reporter_grpc.pb.go

internal/pkg/protobuf/reporter/v1/reporter.pb.go: api/reporter.proto
	PATH="$(GOBIN):$$PATH" protoc --go_out=internal/pkg/protobuf/reporter/v1 api/reporter.proto

internal/pkg/protobuf/reporter/v1/reporter_grpc.pb.go: api/reporter.proto
	PATH="$(GOBIN):$$PATH" protoc --go-grpc_out=internal/pkg/protobuf/reporter/v1 api/reporter.proto

internal/manager/routes/search.go: cmd/template/main.go cmd/template/base.tmpl internal/manager/routes/search.toml
	go run cmd/template/main.go search-generate \
		--packageName="routes" \
		--templatePath="cmd/template/base.tmpl" \
		--configPath="internal/manager/routes/search.toml" \
		--outputPath="internal/manager/routes/search.go"

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

# build - 编译 zki 二进制 (开发版本，版本号自动取 git commit)
# 用法: make build
build: internal/pkg/protobuf/reporter/v1/reporter.pb.go internal/pkg/protobuf/reporter/v1/reporter_grpc.pb.go internal/manager/routes/search.go
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
# OpenTelemetry 前端工具 (mesaglio/otel-front)
# ============================================================
# otel-front 是一个轻量级单二进制 OTEL 可视化工具，无需 Docker/数据库。
#   - Web UI: http://localhost:8000
#   - OTLP gRPC: localhost:4317
#   - OTLP HTTP: http://localhost:4318
# 项目地址: https://github.com/mesaglio/otel-front

OTEL_FRONT_VERSION := v0.0.4
OTEL_FRONT_BIN := $(GOBIN)/otel-front
OTEL_FRONT_PID := /tmp/otel-front.pid

# 根据 OS/ARCH 确定下载的 tarball 文件名
OTEL_FRONT_OS :=
OTEL_FRONT_ARCH :=
ifeq ($(UNAME_S),Darwin)
OTEL_FRONT_OS := Darwin
else ifneq (,$(findstring Linux,$(UNAME_S)))
OTEL_FRONT_OS := Linux
else
OTEL_FRONT_OS := Linux
endif
ifeq ($(shell uname -m),arm64)
OTEL_FRONT_ARCH := arm64
else ifeq ($(shell uname -m),aarch64)
OTEL_FRONT_ARCH := arm64
else
OTEL_FRONT_ARCH := x86_64
endif

OTEL_FRONT_TARBALL := otel-front_$(OTEL_FRONT_VERSION)_$(OTEL_FRONT_OS)_$(OTEL_FRONT_ARCH).tar.gz
OTEL_FRONT_URL := https://github.com/mesaglio/otel-front/releases/download/$(OTEL_FRONT_VERSION)/$(OTEL_FRONT_TARBALL)

# otel-front-install - 从 GitHub Releases 下载预编译的 otel-front 二进制
# 用法: make otel-front-install
otel-front-install:
	@echo "=== 安装 otel-front $(OTEL_FRONT_VERSION) ==="
	@echo "下载 $(OTEL_FRONT_URL)"
	@mkdir -p /tmp/otel-front-install
	@curl -sL -o /tmp/otel-front-install/$(OTEL_FRONT_TARBALL) "$(OTEL_FRONT_URL)"
	@cd /tmp/otel-front-install && tar -xzf $(OTEL_FRONT_TARBALL)
	@BIN=$$(find /tmp/otel-front-install -maxdepth 1 -type f -name 'otel-front*' ! -name '*.tar.gz' ! -name '*.md' | head -1); \
		if [ -z "$$BIN" ] || [ ! -f "$$BIN" ]; then \
			echo "安装失败，未找到 otel-front 二进制"; \
			echo "tarball 内容:"; \
			tar -tzf /tmp/otel-front-install/$(OTEL_FRONT_TARBALL) | head -10; \
			rm -rf /tmp/otel-front-install; \
			exit 1; \
		fi; \
		mv "$$BIN" $(OTEL_FRONT_BIN)
	@chmod +x $(OTEL_FRONT_BIN)
	@rm -rf /tmp/otel-front-install
	@echo "已安装到 $(OTEL_FRONT_BIN)"
	@$(OTEL_FRONT_BIN) --version || true

# otel-front-start - 后台启动 otel-front 并打印环境变量配置
# 用法: make otel-front-start
otel-front-start:
	@if [ ! -f "$(OTEL_FRONT_BIN)" ]; then \
		echo "otel-front 未安装，请先运行 make otel-front-install"; \
		exit 1; \
	fi
	@if [ -f "$(OTEL_FRONT_PID)" ] && kill -0 $$(cat "$(OTEL_FRONT_PID)") 2>/dev/null; then \
		echo "otel-front 已在运行 (PID: $$(cat $(OTEL_FRONT_PID)))"; \
		exit 0; \
	fi
	@echo "=== 启动 otel-front ==="
	@nohup $(OTEL_FRONT_BIN) --no-browser > /tmp/otel-front.log 2>&1 & \
		echo $$! > $(OTEL_FRONT_PID) && \
		sleep 1 && \
		echo "otel-front 已启动 (PID: $$(cat $(OTEL_FRONT_PID)))"
	@echo ""
	@echo "┌──────────────────────────────────────────────────────────┐"
	@echo "│  otel-front Web UI (traces / metrics / logs 可视化)      │"
	@echo "│    http://localhost:8000                                   │"
	@echo "├──────────────────────────────────────────────────────────┤"
	@echo "│  OTLP gRPC 端点 (应用上报地址)                              │"
	@echo "│    localhost:4317                                          │"
	@echo "├──────────────────────────────────────────────────────────┤"
	@echo "│  OTLP HTTP 端点                                            │"
	@echo "│    http://localhost:4318                                   │"
	@echo "└──────────────────────────────────────────────────────────┘"
	@echo ""
	@echo "在运行应用之前，设置以下环境变量以启用 OpenTelemetry 上报:"
	@echo ""
	@echo "  # 启用 OTEL 可观测功能"
	@echo "  export ZKI_OTEL=true"
	@echo ""
	@echo "  # 使用 gRPC 上报 (默认)"
	@echo "  export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317"
	@echo "  export OTEL_EXPORTER_OTLP_PROTOCOL=grpc"
	@echo ""
	@echo "  # 或使用 HTTP 上报 (otel-front 推荐)"
	@echo "  export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318"
	@echo "  export OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf"
	@echo ""
	@echo "  # 通用设置"
	@echo "  export OTEL_TRACES_EXPORTER=otlp"
	@echo "  export OTEL_METRICS_EXPORTER=otlp"
	@echo "  export OTEL_LOGS_EXPORTER=otlp"
	@echo "  export OTEL_EXPORTER_OTLP_INSECURE=true"
	@echo "  export OTEL_SERVICE_NAME=zhongkui-dev"
	@echo ""
	@echo "  # 然后正常运行应用:"
	@echo "  go run cmd/zki/main.go run"
	@echo ""
	@echo "日志文件: /tmp/otel-front.log"
	@echo "停止服务: make otel-front-stop"

# otel-front-stop - 停止后台运行的 otel-front
# 用法: make otel-front-stop
otel-front-stop:
	@if [ -f "$(OTEL_FRONT_PID)" ] && kill -0 $$(cat "$(OTEL_FRONT_PID)") 2>/dev/null; then \
		echo "=== 停止 otel-front (PID: $$(cat $(OTEL_FRONT_PID))) ==="; \
		kill $$(cat $(OTEL_FRONT_PID)) && rm -f $(OTEL_FRONT_PID); \
		echo "已停止"; \
	else \
		echo "otel-front 未在运行"; \
		rm -f $(OTEL_FRONT_PID); \
	fi

