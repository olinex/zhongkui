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

dev-tools:
	go install github.com/hibiken/asynq/tools/asynq@latest
	go install golang.org/x/pkgsite/cmd/pkgsite@latest
	go install github.com/smartystreets/goconvey@v1.8.1

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

build: internal/pkg/protobuf/reporter/v1/reporter.pb.go internal/pkg/protobuf/reporter/v1/reporter_grpc.pb.go internal/manager/routes/search.go

run:
	go run cmd/zki/main.go $(ARGS)

# ============================================================
# Wire 抓包实现基线测试
# ============================================================

# bench-prepare - 下载 tcpreplay 采样流量并生成 fallback pcap
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

# bench-smoke - 快速冒烟测试（无需 sudo，仅用内嵌 test.pcap）
bench-smoke:
	@echo "=== Wire 抓包实现冒烟测试 (-short) ==="
	@echo "注意：完整抓包测试需要 sudo 权限，当前仅验证编译与跳过逻辑"
	@echo ""
	go test -v -bench=. -short -benchtime=1x ./internal/benchmark/capture/

# bench-slow - 慢速重放测试（1ms 间隔，验证低速下丢包率）
bench-slow:
	@echo "=== Wire 抓包实现慢速重放测试 (1ms 间隔) ==="
	@if [ ! -f internal/benchmark/capture/testdata/smallFlows.pcap ]; then \
		echo "错误：smallFlows.pcap 不存在，请先运行 make bench-prepare"; \
		exit 1; \
	fi
	@echo "使用 $(SUDO) 运行慢速重放 Benchmark..."
	@echo ""
	$(SUDO) go test -v -bench=Slow -benchtime=1x ./internal/benchmark/capture/

# bench-fast - 全速基线测试（排除 Slow，需 $(SUDO) + 已下载 smallFlows.pcap）
bench-fast:
	@echo "=== Wire 抓包实现全速基线测试 ==="
	@if [ ! -f internal/benchmark/capture/testdata/smallFlows.pcap ]; then \
		echo "错误：smallFlows.pcap 不存在，请先运行 make bench-prepare"; \
		exit 1; \
	fi
	@echo "使用 $(SUDO) 运行全速 Benchmark..."
	@echo ""
	$(SUDO) go test -v -bench='^(BenchmarkPlatform|BenchmarkLibpcap|BenchmarkXdp|BenchmarkAfpacket)(Smoke|SmallFlows|BigFlows)$$' -benchtime=1x ./internal/benchmark/capture/

# bench-raw - 纯计数基线测试（skip copy，测量裸 capture 吞吐上限）
bench-raw:
	@echo "=== 纯计数捕获上限测试 (skip copy) ==="
	@if [ ! -f internal/benchmark/capture/testdata/smallFlows.pcap ]; then \
		echo "错误：smallFlows.pcap 不存在，请先运行 make bench-prepare"; \
		exit 1; \
	fi
	@echo "使用 $(SUDO) 运行 Raw Benchmark..."
	@echo ""
	$(SUDO) go test -v -bench='^BenchmarkRaw' -benchtime=1x ./internal/benchmark/capture/

# bench-xdp-phys - 物理网卡 XDP 测试（双网卡互联环境）
# 需要两台支持 AF_XDP 的物理有线网卡互联 (直连网线或交换机)
# 单网卡 / 不支持 AF_XDP 的网卡会自动 skip
bench-xdp-phys:
	@echo "=== XDP 物理网卡测试 (需 2 张 AF_XDP 兼容网卡互联) ==="
	@if [ ! -f internal/benchmark/capture/testdata/smallFlows.pcap ]; then \
		echo "错误：smallFlows.pcap 不存在，请先运行 make bench-prepare"; \
		exit 1; \
	fi
	@echo "使用 $(SUDO) 运行物理网卡 XDP Benchmark..."
	@echo ""
	$(SUDO) go test -v -bench='^BenchmarkXdpNative' -benchtime=1x ./internal/benchmark/capture/

# bench-xdp-phys-raw - 物理网卡 XDP raw capture（skip copy）测试
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
bench-probe:
	@echo "=== 网卡 XDP 支持探测 ==="
	go build -o /tmp/nic_probe ./cmd/nic_probe/
	$(SUDO) /tmp/nic_probe
	@rm -f /tmp/nic_probe

