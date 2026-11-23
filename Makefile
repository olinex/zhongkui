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

.PHONY: build
.PHONY: build-release
.PHONY: clean
.PHONY: docs
.PHONY: p0f-update
.PHONY: rag-snapshot
.PHONY: version-env

# ============================================================
# 构建与版本管理
# ============================================================
#
# 版本号规则:
#   - 正式版本: vX.Y.Z（示例: v1.2.3）
#   - 预发布版本: vX.Y.Z-{alpha|beta|rc}.n（示例: v1.2.3-alpha.1, v1.2.3-rc.2）
#   - 版本号唯一来源是 annotated tag，由 scripts/gitflow.sh release finish 创建
#   - 开发构建不注入 SemVer，版本显示回落为 commit hash，如 "c2d4d21"
#   - Release 构建必须显式指定 VERSION，如 make build-release VERSION=v1.2.3-rc.1
#
# 编译时版本信息通过 -ldflags 注入到二进制，运行时可通过以下命令查看:
#   ./bin/zki version          # 详细输出，包含 Version / Commit / BuildTime / Builder
#   go run cmd/zki/main.go version  # 开发时无需预编译，直接查看
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
# 产物是提交进 git 的源码文件，属于构建输入维护
# 用法: make p0f-update
# 自定义上游源: make p0f-update P0F_UPSTREAM_URL=<新地址>
p0f-update:
	@scripts/dependences-update.sh p0f "$(P0F_UPSTREAM_URL)"

# RAG 知识快照（编译期嵌入二进制）：
# 发布二进制不含源码树与 Go 工具链，zki sync docs / sync schema 在运行期回退到该快照；
# 任一 Go 源文件、迁移 SQL 或 go.mod 变化都会触发重新烘焙，漂移校验见 scripts/check.sh docs。
# 生成代码（protobuf / views）带独立生成规则且不影响快照内容，排除在快照依赖外，
# 避免在未安装 protoc 的镜像构建环境中被间接触发。
RAG_SNAPSHOT := internal/docs/ragdata/snapshot.json
GENERATED_SRCS := internal/pkg/protobuf/reporter/v1/reporter.pb.go \
                  internal/pkg/protobuf/reporter/v1/reporter_grpc.pb.go \
                  internal/manager/views/search.go \
                  internal/manager/views/enum_bindings.go
RAG_SNAPSHOT_DEPS := $(filter-out $(GENERATED_SRCS), $(shell find cmd internal -type f \( -name '*.go' -o -name '*.sql' \) 2>/dev/null)) go.mod

$(RAG_SNAPSHOT): $(RAG_SNAPSHOT_DEPS)
	@echo ">>> 烘焙 RAG 知识快照 $@"
	go run cmd/docsnap/main.go -root .

# rag-snapshot - 强制重新烘焙 RAG 知识快照（普通构建会按需自动触发）
# 用法: make rag-snapshot
rag-snapshot: $(RAG_SNAPSHOT)

# 构建产物依赖：生成代码与 RAG 快照必须在编译前与源码对齐
# protoc 只存在于开发机；镜像构建环境没有 protoc，此时只依赖 RAG 快照，
# 避免间接触发代码生成规则导致构建失败（生成码由仓库提交、CI build-check 把关）
HAVE_PROTOC := $(shell command -v protoc >/dev/null 2>&1 && echo yes || { [ -x "$(GOBIN)/protoc" ] && echo yes; })
ifeq ($(HAVE_PROTOC),yes)
BUILD_DEPS := $(GENERATED_SRCS) $(RAG_SNAPSHOT)
else
BUILD_DEPS := $(RAG_SNAPSHOT)
endif

# 编译参数全局统一：所有二进制产物都使用 -trimpath + 剥离符号，
# 入口之间只允许版本信息（DEV / RELEASE）与输出路径不同。
#   OUT      产物路径（镜像构建由 deploys/containers/normal.Dockerfile 的 build 阶段指定 /out/zki）
#   LDFLAGS  链接参数（版本信息注入；-s -w 属于链接参数）
OUT ?= bin/zki
LDFLAGS ?= -s -w $(DEV_LDFLAGS)
define GO_BUILD
mkdir -p "$(dir $(OUT))" && go build -trimpath -ldflags "$(LDFLAGS)" -o $(OUT) cmd/zki/main.go
endef

# build - 编译 zki 二进制 (开发版本，版本显示回落为 commit hash)
# 用法: make build
build: OUT := bin/zki
build: LDFLAGS := -s -w $(DEV_LDFLAGS)
build: $(BUILD_DEPS)
	@$(GO_BUILD)

# build-release - 编译 release 版本 (需手动指定语义化版本号，正式版和预发布版均支持)
# 与 make build 的区别：
#   - RELEASE_LDFLAGS 注入 SemVer（DEV_LDFLAGS 只注入 commit）
#   - 不触发 $(BUILD_DEPS) 的 RAG 快照烘焙
#
#   快照烘焙由外层（调用方）自行决定：
#     本地 release 打包: scripts/dist.sh → 调用前确保快照已烘焙（make rag-snapshot 或 git 提交）
#     Dockerfile / CI : 快照应已由仓库提交；CI docs-check 在合入前把关一致性
#   快照不存在时 go build 会因 embed 报错，外层脚本会给出友好提示。
#
# 用法: make build-release VERSION=v1.2.3  或  make build-release VERSION=v1.2.3-alpha.1
#       make build-release VERSION=v1.2.3 OUT=/custom/path/zki
build-release: OUT := bin/zki
build-release: LDFLAGS := -s -w $(RELEASE_LDFLAGS)
build-release:
	@if [ "$(VERSION)" != "unknown" ]; then scripts/validate-semver.sh "$(VERSION)"; fi
	@$(GO_BUILD)

# build-image 已删除：原来的职责（跳过 BUILD_DEPS，不烘焙快照）现在由 build-release
# 统一承担——build-release 不再依赖 $(BUILD_DEPS)，外层脚本自行保证快照就绪。

# version-env - 只生成 dist/build.env（VERSION/COMMIT/BUILD_TIME/BUILDER），不编译任何产物
# scripts/dist.sh 与 Dockerfile 构建阶段都直接调用本目标，避免各自重复实现版本元数据生成
# 用法: make version-env VERSION=v1.2.3
version-env:
	@test -n "$(VERSION)" || { echo "ERROR: 请设置版本号，例如: make version-env VERSION=v1.2.3" >&2; exit 1; }
	@scripts/validate-semver.sh "$(VERSION)"
	@mkdir -p dist
	@printf 'VERSION=%s\nCOMMIT=%s\nBUILD_TIME=%s\nBUILDER=%s\n' \
	  "$(VERSION)" "$(COMMIT)" "$(BUILD_TIME)" "$(BUILDER)" > dist/build.env
	@echo "已生成 dist/build.env"

# clean - 删除自动生成的 protobuf 代码
# 用法: make clean
clean:
	rm -f internal/pkg/protobuf/reporter/v1/reporter.pb.go
	rm -f internal/pkg/protobuf/reporter/v1/reporter_grpc.pb.go

# ============================================================
# 文档生成
# ============================================================

# docs - 生成文档：叙述页、参考页、总目录与导航文件，并烘焙 RAG 知识快照
# 产物提交进 git，属于构建输入维护
# 用法: make docs
docs:
	go run cmd/zki/main.go create docs
	go run cmd/docsnap/main.go -root .
