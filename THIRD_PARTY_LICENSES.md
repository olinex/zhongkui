# 第三方依赖与许可证

> 本文件记录随本项目分发的第三方组件及其许可证，用于满足 Apache-2.0 第 4(d) 条的归属与保留声明要求。
> 生成日期：2026-10-21；依赖版本以本仓库当前 `go.mod`、`go.sum`、`web/pnpm-lock.yaml` 为准。

## 合规结论（摘要）

- 运行时依赖中**不存在** GPL、AGPL、LGPL、SSPL、Commons Clause 或“仅非商业用途”类许可证，用户可以在商业项目中使用本项目，无需开源自己的代码。
- 存在两类**弱 copyleft** 组件：MPL-2.0 的 `go-sql-driver/mysql`、`hashicorp/go-version`，以及 EPL-2.0 的 `elkjs`。它们只约束**这些组件自身**，不改变本项目的 Apache-2.0 许可，也不要求使用者的项目开源。
- 义务集中在“分发方”（发布二进制或前端静态资源的场景）：保留版权与许可证声明；若修改了 MPL/EPL 组件，需按原许可证公开修改部分；提供对应版本的源码获取方式。
- 双许可组件已选择宽松分支：`dompurify` 按 Apache-2.0、`type-fest` 按 MIT 使用；`@bufbuild/protobuf` 同时遵守 Apache-2.0 与 BSD-3-Clause。
- 本项目自身仍为 Apache-2.0；第三方组件的许可不会传染到项目代码或用户代码。
- 仓库已通过 `scripts/check.sh license` 在 CI 中强制校验：出现 GPL/AGPL/LGPL/SSPL/非商业或未知许可证时，构建会失败。

## 1. 范围与口径

| 分发物 | 覆盖范围 | 数量 |
| --- | --- | --- |
| Go 二进制 `zki` | 链接进 `./cmd/zki` 的全部非标准库模块 | 232 |
| Web 管理端 | `pnpm list --prod` 的生产依赖树 | 483 |
| 文档站点 | VitePress 及其依赖（仅构建期使用，不随产品分发） | 不计入 |
| 开发/测试工具 | `cmd/commentlint`、`cmd/template`、`cmd/nic_probe` 等仅开发期模块 | 不计入 |
| Vendored 代码 | 当前仓库不含 `third_party/` 目录 | 0 |

说明：

- Go 侧按「实际链接进 `zki` 二进制」统计，测试与开发工具依赖不进入本清单；它们仍是 `go.mod` 的一部分，来源与版本可追溯。
- Web 侧按 pnpm 生产依赖树统计，包含直接与传递依赖；开发依赖（构建、类型检查、Lint）不进入分发物。
- 许可证识别优先取包内 `LICENSE`/`COPYING`/`NOTICE` 文件，其次取 `package.json` 的 `license` 字段；子目录模块未随包携带 LICENSE 时，按上游仓库根许可证核对（见第 5 节）。
- 本文件只做归属清单；具体条款以各组件随附的许可证原文为准。

## 2. 许可证分布

### Go 运行时依赖

| 许可证 | 模块数 |
| --- | --- |
| MIT | 112 |
| Apache-2.0 | 73 |
| BSD-3-Clause | 37 |
| BSD-2-Clause | 8 |
| MPL-2.0 | 2 |

### Web 生产依赖

| 许可证 | 包数 |
| --- | --- |
| MIT | 380 |
| ISC | 54 |
| Apache-2.0 | 17 |
| BSD-3-Clause | 13 |
| BSD-2-Clause | 9 |
| (MIT OR CC0-1.0) | 3 |
| 0BSD | 2 |
| (Apache-2.0 AND BSD-3-Clause) | 1 |
| (MPL-2.0 OR Apache-2.0) | 1 |
| EPL-2.0 | 1 |
| Python-2.0 | 1 |
| Unlicense | 1 |

## 3. Go 运行时依赖清单

| 模块 | 版本 | 许可证 | 直接依赖 |
| --- | --- | --- | --- |
| `atomicgo.dev/cursor` | `v0.2.0` | MIT | - |
| `atomicgo.dev/keyboard` | `v0.2.9` | MIT | - |
| `atomicgo.dev/schedule` | `v0.1.0` | MIT | - |
| `filippo.io/edwards25519` | `v1.2.0` | BSD-3-Clause | - |
| `github.com/ClickHouse/ch-go` | `v0.71.0` | Apache-2.0 | - |
| `github.com/ClickHouse/clickhouse-go/v2` | `v2.45.0` | Apache-2.0 | 是 |
| `github.com/agnivade/levenshtein` | `v1.2.1` | MIT | - |
| `github.com/alecthomas/participle/v2` | `v2.1.4` | MIT | 是 |
| `github.com/andybalholm/brotli` | `v1.2.1` | MIT | - |
| `github.com/bahlo/generic-list-go` | `v0.2.0` | BSD-3-Clause | - |
| `github.com/bmatcuk/doublestar/v4` | `v4.10.0` | MIT | - |
| `github.com/buger/jsonparser` | `v1.1.1` | MIT | - |
| `github.com/bytedance/gopkg` | `v0.1.4` | Apache-2.0 | - |
| `github.com/bytedance/sonic` | `v1.15.4` | Apache-2.0 | - |
| `github.com/casbin/casbin/v2` | `v2.103.0` | Apache-2.0 | 是 |
| `github.com/casbin/gorm-adapter/v3` | `v3.32.0` | Apache-2.0 | 是 |
| `github.com/casbin/govaluate` | `v1.3.0` | MIT | 是 |
| `github.com/cenkalti/backoff/v5` | `v5.0.3` | MIT | 是 |
| `github.com/cespare/xxhash/v2` | `v2.3.0` | MIT | - |
| `github.com/clipperhouse/uax29/v2` | `v2.7.0` | MIT | - |
| `github.com/cloudwego/eino` | `v0.9.19` | Apache-2.0 | 是 |
| `github.com/cloudwego/eino-ext/components/embedding/openai` | `v0.0.0-20260909094858-6fe16d48b736` | Apache-2.0 | 是 |
| `github.com/cloudwego/eino-ext/components/model/openai` | `v0.1.13` | Apache-2.0 | 是 |
| `github.com/cloudwego/eino-ext/components/tool/mcp` | `v0.0.9` | Apache-2.0 | 是 |
| `github.com/cloudwego/eino-ext/libs/acl/openai` | `v0.1.17` | Apache-2.0 | - |
| `github.com/containerd/console` | `v1.0.5` | Apache-2.0 | - |
| `github.com/coreos/go-semver` | `v0.3.0` | Apache-2.0 | - |
| `github.com/coreos/go-systemd/v22` | `v22.3.2` | Apache-2.0 | - |
| `github.com/creasty/defaults` | `v1.8.0` | MIT | 是 |
| `github.com/dlclark/regexp2` | `v1.11.4` | MIT | 是 |
| `github.com/dustin/go-humanize` | `v1.0.1` | MIT | 是 |
| `github.com/eino-contrib/jsonschema` | `v1.0.3` | MIT | - |
| `github.com/emirpasic/gods` | `v1.12.0` | BSD-2-Clause | - |
| `github.com/evanphx/json-patch` | `v0.5.2` | BSD-3-Clause | - |
| `github.com/fatih/color` | `v1.18.0` | MIT | 是 |
| `github.com/felixge/httpsnoop` | `v1.0.4` | MIT | - |
| `github.com/fsnotify/fsnotify` | `v1.10.1` | BSD-3-Clause | - |
| `github.com/gabriel-vasile/mimetype` | `v1.4.13` | MIT | - |
| `github.com/gin-contrib/sse` | `v1.1.1` | MIT | - |
| `github.com/gin-gonic/gin` | `v1.12.0` | MIT | 是 |
| `github.com/glebarez/go-sqlite` | `v1.20.3` | BSD-3-Clause | 是 |
| `github.com/glebarez/sqlite` | `v1.7.0` | MIT | 是 |
| `github.com/go-faster/city` | `v1.0.1` | MIT | - |
| `github.com/go-faster/errors` | `v0.7.1` | BSD-3-Clause | - |
| `github.com/go-git/gcfg` | `v1.5.0` | BSD-3-Clause | - |
| `github.com/go-git/go-billy/v5` | `v5.3.1` | Apache-2.0 | - |
| `github.com/go-git/go-git/v5` | `v5.3.0` | Apache-2.0 | - |
| `github.com/go-ini/ini` | `v1.67.0` | Apache-2.0 | - |
| `github.com/go-logr/logr` | `v1.4.4` | Apache-2.0 | - |
| `github.com/go-logr/stdr` | `v1.2.2` | Apache-2.0 | - |
| `github.com/go-playground/locales` | `v0.14.1` | MIT | 是 |
| `github.com/go-playground/universal-translator` | `v0.18.1` | MIT | 是 |
| `github.com/go-playground/validator/v10` | `v10.30.2` | MIT | 是 |
| `github.com/go-sql-driver/mysql` | `v1.9.3` | MPL-2.0 | 是 |
| `github.com/go-viper/mapstructure/v2` | `v2.4.0` | MIT | - |
| `github.com/gobwas/glob` | `v0.2.3` | MIT | - |
| `github.com/goccy/go-json` | `v0.10.6` | MIT | - |
| `github.com/goccy/go-yaml` | `v1.19.2` | MIT | - |
| `github.com/gogo/protobuf` | `v1.3.2` | BSD-3-Clause | - |
| `github.com/golang-jwt/jwt/v5` | `v5.3.1` | MIT | 是 |
| `github.com/golang-sql/civil` | `v0.0.0-20220223132316-b832511892a9` | Apache-2.0 | - |
| `github.com/golang-sql/sqlexp` | `v0.1.0` | BSD-3-Clause | - |
| `github.com/golang/protobuf` | `v1.5.4` | BSD-3-Clause | - |
| `github.com/google/uuid` | `v1.6.0` | BSD-3-Clause | 是 |
| `github.com/gookit/color` | `v1.6.0` | MIT | - |
| `github.com/gopacket/gopacket` | `v1.6.0` | BSD-3-Clause | 是 |
| `github.com/goph/emperror` | `v0.17.2` | MIT | - |
| `github.com/grpc-ecosystem/grpc-gateway/v2` | `v2.29.0` | BSD-3-Clause | - |
| `github.com/hashicorp/go-version` | `v1.8.0` | MPL-2.0 | - |
| `github.com/hibiken/asynq` | `v0.25.1` | MIT | 是 |
| `github.com/imdario/mergo` | `v0.3.12` | BSD-3-Clause | - |
| `github.com/invopop/jsonschema` | `v0.13.0` | MIT | - |
| `github.com/jackc/pgpassfile` | `v1.0.0` | MIT | - |
| `github.com/jackc/pgservicefile` | `v0.0.0-20240606120523-5a60cdf6a761` | MIT | - |
| `github.com/jackc/pgx/v5` | `v5.9.2` | MIT | 是 |
| `github.com/jackc/puddle/v2` | `v2.2.2` | MIT | - |
| `github.com/jbenet/go-context` | `v0.0.0-20150711004518-d14ea06fba99` | MIT | - |
| `github.com/jinzhu/inflection` | `v1.0.0` | MIT | - |
| `github.com/jinzhu/now` | `v1.1.5` | MIT | - |
| `github.com/jolestar/go-commons-pool/v2` | `v2.1.2` | Apache-2.0 | 是 |
| `github.com/json-iterator/go` | `v1.1.12` | MIT | - |
| `github.com/kelseyhightower/envconfig` | `v1.4.0` | MIT | 是 |
| `github.com/kevinburke/ssh_config` | `v1.1.0` | MIT | - |
| `github.com/klauspost/compress` | `v1.19.0` | Apache-2.0 | 是 |
| `github.com/klauspost/cpuid/v2` | `v2.3.0` | MIT | - |
| `github.com/ledongthuc/pdf` | `v0.0.0-20260907135840-6c8c28e0e8a0` | BSD-3-Clause | 是 |
| `github.com/leodido/go-urn` | `v1.4.0` | MIT | - |
| `github.com/lestrrat-go/blackmagic` | `v1.0.4` | MIT | - |
| `github.com/lestrrat-go/dsig` | `v1.2.1` | MIT | - |
| `github.com/lestrrat-go/httpcc` | `v1.0.1` | MIT | - |
| `github.com/lestrrat-go/httprc/v3` | `v3.0.5` | MIT | - |
| `github.com/lestrrat-go/jwx/v3` | `v3.1.1` | MIT | - |
| `github.com/lestrrat-go/option/v2` | `v2.0.0` | MIT | - |
| `github.com/lithammer/fuzzysearch` | `v1.1.8` | MIT | - |
| `github.com/mailru/easyjson` | `v0.9.0` | MIT | - |
| `github.com/mark3labs/mcp-go` | `v0.43.0` | MIT | 是 |
| `github.com/mattn/go-colorable` | `v0.1.14` | MIT | - |
| `github.com/mattn/go-isatty` | `v0.0.22` | MIT | - |
| `github.com/mattn/go-runewidth` | `v0.0.20` | MIT | - |
| `github.com/meguminnnnnnnnn/go-openai` | `v0.1.2` | Apache-2.0 | - |
| `github.com/mfridman/interpolate` | `v0.0.2` | MIT | - |
| `github.com/mgutz/ansi` | `v0.0.0-20200706080929-d51e80ef957d` | MIT | - |
| `github.com/microsoft/go-mssqldb` | `v1.9.8` | BSD-3-Clause | - |
| `github.com/minio/crc64nvme` | `v1.0.2` | Apache-2.0 | - |
| `github.com/minio/md5-simd` | `v1.1.2` | Apache-2.0 | - |
| `github.com/minio/minio-go/v7` | `v7.0.95` | Apache-2.0 | 是 |
| `github.com/mitchellh/go-homedir` | `v1.1.0` | MIT | - |
| `github.com/mitchellh/mapstructure` | `v1.5.0` | MIT | 是 |
| `github.com/modern-go/concurrent` | `v0.0.0-20180306012644-bacd9c7ef1dd` | Apache-2.0 | - |
| `github.com/modern-go/reflect2` | `v1.0.2` | Apache-2.0 | - |
| `github.com/ncruces/go-sqlite3` | `v0.35.4` | MIT | 是 |
| `github.com/ncruces/go-sqlite3-wasm/v5` | `v5.0.35304` | MIT | - |
| `github.com/ncruces/go-strftime` | `v1.0.0` | MIT | - |
| `github.com/ncruces/julianday` | `v1.0.0` | MIT | - |
| `github.com/nicksnyder/go-i18n/v2` | `v2.6.0` | MIT | 是 |
| `github.com/nikolalohinski/gonja` | `v1.5.3` | MIT | - |
| `github.com/open-policy-agent/opa` | `v1.19.0` | Apache-2.0 | 是 |
| `github.com/paulmach/orb` | `v0.13.0` | MIT | - |
| `github.com/pelletier/go-toml/v2` | `v2.3.1` | MIT | 是 |
| `github.com/philhofer/fwd` | `v1.2.0` | MIT | - |
| `github.com/pierrec/lz4/v4` | `v4.1.26` | BSD-3-Clause | - |
| `github.com/pkg/errors` | `v0.9.1` | BSD-2-Clause | 是 |
| `github.com/pressly/goose/v3` | `v3.27.1` | MIT | 是 |
| `github.com/princjef/gomarkdoc` | `v1.1.0` | MIT | 是 |
| `github.com/pterm/pterm` | `v0.12.83` | MIT | 是 |
| `github.com/quic-go/qpack` | `v0.6.0` | MIT | - |
| `github.com/quic-go/quic-go` | `v0.59.1` | MIT | - |
| `github.com/rcrowley/go-metrics` | `v0.0.0-20250401214520-65e299d6c5c9` | BSD-2-Clause | - |
| `github.com/redis/go-redis/extra/rediscmd/v9` | `v9.21.0` | BSD-2-Clause | - |
| `github.com/redis/go-redis/extra/redisotel/v9` | `v9.21.0` | BSD-2-Clause | 是 |
| `github.com/redis/go-redis/v9` | `v9.21.0` | BSD-2-Clause | 是 |
| `github.com/remyoudompheng/bigfft` | `v0.0.0-20230129092748-24d4a6f8daec` | BSD-3-Clause | - |
| `github.com/robfig/cron/v3` | `v3.0.1` | MIT | 是 |
| `github.com/rs/xid` | `v1.6.0` | MIT | - |
| `github.com/russross/blackfriday/v2` | `v2.1.0` | BSD-2-Clause | - |
| `github.com/sagikazarmark/locafero` | `v0.11.0` | MIT | - |
| `github.com/samber/do/v2` | `v2.0.0` | MIT | 是 |
| `github.com/samber/go-type-to-string` | `v1.8.0` | MIT | - |
| `github.com/segmentio/asm` | `v1.2.1` | MIT | - |
| `github.com/sergi/go-diff` | `v1.4.0` | MIT | 是 |
| `github.com/sethvargo/go-retry` | `v0.3.0` | Apache-2.0 | - |
| `github.com/shopspring/decimal` | `v1.4.0` | MIT | - |
| `github.com/sirupsen/logrus` | `v1.9.4` | MIT | - |
| `github.com/slongfield/pyfmt` | `v0.0.0-20220222012616-ea85ff4c361f` | BSD-3-Clause | - |
| `github.com/smallnest/ringbuffer` | `v0.1.1` | MIT | 是 |
| `github.com/sourcegraph/conc` | `v0.3.1-0.20240121214520-5f936abd7ae8` | MIT | - |
| `github.com/spf13/afero` | `v1.15.0` | Apache-2.0 | - |
| `github.com/spf13/cast` | `v1.10.0` | MIT | - |
| `github.com/spf13/cobra` | `v1.10.2` | Apache-2.0 | 是 |
| `github.com/spf13/pflag` | `v1.0.10` | BSD-3-Clause | 是 |
| `github.com/spf13/viper` | `v1.21.0` | MIT | 是 |
| `github.com/subosito/gotenv` | `v1.6.0` | MIT | - |
| `github.com/tchap/go-patricia/v2` | `v2.3.3` | MIT | - |
| `github.com/tinylib/msgp` | `v1.5.0` | MIT | - |
| `github.com/ugorji/go/codec` | `v1.3.1` | MIT | - |
| `github.com/valyala/fastjson` | `v1.6.10` | MIT | - |
| `github.com/vektah/gqlparser/v2` | `v2.5.36` | MIT | - |
| `github.com/wk8/go-ordered-map/v2` | `v2.1.8` | Apache-2.0 | - |
| `github.com/x-cray/logrus-prefixed-formatter` | `v0.5.2` | MIT | - |
| `github.com/xanzy/ssh-agent` | `v0.3.0` | Apache-2.0 | - |
| `github.com/xdg-go/scram` | `v1.2.0` | Apache-2.0 | - |
| `github.com/xdg-go/stringprep` | `v1.0.4` | Apache-2.0 | - |
| `github.com/xeipuuv/gojsonpointer` | `v0.0.0-20190905194746-02993c407bfb` | Apache-2.0 | - |
| `github.com/xeipuuv/gojsonreference` | `v0.0.0-20180127040603-bd5ef7bd5415` | Apache-2.0 | - |
| `github.com/xo/terminfo` | `v0.0.0-20220910002029-abceb7e1c41e` | MIT | - |
| `github.com/yargevad/filepathx` | `v1.0.0` | MIT | - |
| `github.com/yashtewari/glob-intersection` | `v0.2.0` | Apache-2.0 | - |
| `github.com/yosida95/uritemplate/v3` | `v3.0.2` | BSD-3-Clause | - |
| `github.com/youmark/pkcs8` | `v0.0.0-20240726163527-a2c0da244d78` | MIT | - |
| `go.etcd.io/etcd/api/v3` | `v3.5.18` | Apache-2.0 | 是 |
| `go.etcd.io/etcd/client/pkg/v3` | `v3.5.18` | Apache-2.0 | - |
| `go.etcd.io/etcd/client/v3` | `v3.5.18` | Apache-2.0 | 是 |
| `go.mongodb.org/mongo-driver/v2` | `v2.6.0` | Apache-2.0 | 是 |
| `go.opentelemetry.io/auto/sdk` | `v1.2.1` | Apache-2.0 | - |
| `go.opentelemetry.io/contrib/bridges/otelzap` | `v0.19.0` | Apache-2.0 | 是 |
| `go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin` | `v0.69.0` | Apache-2.0 | 是 |
| `go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc` | `v0.69.0` | Apache-2.0 | 是 |
| `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp` | `v0.69.0` | Apache-2.0 | 是 |
| `go.opentelemetry.io/otel` | `v1.44.0` | Apache-2.0 | 是 |
| `go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc` | `v0.20.0` | Apache-2.0 | 是 |
| `go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp` | `v0.20.0` | Apache-2.0 | 是 |
| `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc` | `v1.44.0` | Apache-2.0 | 是 |
| `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp` | `v1.44.0` | Apache-2.0 | 是 |
| `go.opentelemetry.io/otel/exporters/otlp/otlptrace` | `v1.44.0` | Apache-2.0 | - |
| `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc` | `v1.44.0` | Apache-2.0 | 是 |
| `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp` | `v1.44.0` | Apache-2.0 | 是 |
| `go.opentelemetry.io/otel/exporters/stdout/stdoutlog` | `v0.20.0` | Apache-2.0 | 是 |
| `go.opentelemetry.io/otel/exporters/stdout/stdoutmetric` | `v1.44.0` | Apache-2.0 | 是 |
| `go.opentelemetry.io/otel/exporters/stdout/stdouttrace` | `v1.44.0` | Apache-2.0 | 是 |
| `go.opentelemetry.io/otel/log` | `v0.20.0` | Apache-2.0 | 是 |
| `go.opentelemetry.io/otel/metric` | `v1.44.0` | Apache-2.0 | 是 |
| `go.opentelemetry.io/otel/sdk` | `v1.44.0` | Apache-2.0 | 是 |
| `go.opentelemetry.io/otel/sdk/log` | `v0.20.0` | Apache-2.0 | 是 |
| `go.opentelemetry.io/otel/sdk/metric` | `v1.44.0` | Apache-2.0 | 是 |
| `go.opentelemetry.io/otel/trace` | `v1.44.0` | Apache-2.0 | 是 |
| `go.opentelemetry.io/proto/otlp` | `v1.10.0` | Apache-2.0 | - |
| `go.uber.org/atomic` | `v1.11.0` | MIT | - |
| `go.uber.org/multierr` | `v1.11.0` | MIT | - |
| `go.uber.org/zap` | `v1.28.0` | MIT | 是 |
| `go.yaml.in/yaml/v2` | `v2.4.4` | Apache-2.0 | - |
| `go.yaml.in/yaml/v3` | `v3.0.4` | Apache-2.0 | - |
| `golang.org/x/arch` | `v0.27.0` | BSD-3-Clause | - |
| `golang.org/x/crypto` | `v0.57.0` | BSD-3-Clause | 是 |
| `golang.org/x/exp` | `v0.0.0-20260410095643-746e56fc9e2f` | BSD-3-Clause | - |
| `golang.org/x/mod` | `v0.41.0` | BSD-3-Clause | - |
| `golang.org/x/net` | `v0.59.0` | BSD-3-Clause | 是 |
| `golang.org/x/sync` | `v0.23.0` | BSD-3-Clause | 是 |
| `golang.org/x/sys` | `v0.48.0` | BSD-3-Clause | 是 |
| `golang.org/x/term` | `v0.46.0` | BSD-3-Clause | - |
| `golang.org/x/text` | `v0.42.0` | BSD-3-Clause | 是 |
| `golang.org/x/time` | `v0.15.0` | BSD-3-Clause | - |
| `golang.org/x/tools` | `v0.50.0` | BSD-3-Clause | 是 |
| `google.golang.org/genproto/googleapis/api` | `v0.0.0-20260526163538-3dc84a4a5aaa` | Apache-2.0 | - |
| `google.golang.org/genproto/googleapis/rpc` | `v0.0.0-20260526163538-3dc84a4a5aaa` | Apache-2.0 | - |
| `google.golang.org/grpc` | `v1.82.1` | Apache-2.0 | 是 |
| `google.golang.org/protobuf` | `v1.36.11` | BSD-3-Clause | 是 |
| `gopkg.in/warnings.v0` | `v0.1.2` | BSD-2-Clause | - |
| `gopkg.in/yaml.v3` | `v3.0.1` | Apache-2.0 | - |
| `gorm.io/datatypes` | `v1.2.5` | MIT | 是 |
| `gorm.io/driver/clickhouse` | `v0.7.0` | MIT | 是 |
| `gorm.io/driver/mysql` | `v1.5.7` | MIT | - |
| `gorm.io/driver/postgres` | `v1.5.11` | MIT | 是 |
| `gorm.io/driver/sqlserver` | `v1.5.4` | MIT | - |
| `gorm.io/gorm` | `v1.30.1` | MIT | 是 |
| `gorm.io/plugin/dbresolver` | `v1.6.2` | MIT | - |
| `gorm.io/plugin/opentelemetry` | `v0.1.16` | MIT | 是 |
| `modernc.org/libc` | `v1.72.1` | MIT | - |
| `modernc.org/mathutil` | `v1.7.1` | BSD-3-Clause | - |
| `modernc.org/memory` | `v1.11.0` | BSD-3-Clause | - |
| `modernc.org/sqlite` | `v1.49.1` | BSD-3-Clause | - |
| `mvdan.cc/xurls/v2` | `v2.2.0` | BSD-3-Clause | - |
| `sigs.k8s.io/yaml` | `v1.6.0` | Apache-2.0 | - |

## 4. Web 生产依赖清单

| 包 | 版本 | 许可证 |
| --- | --- | --- |
| `@antfu/install-pkg` | `2.1.0` | MIT |
| `@babel/helper-string-parser` | `7.27.1` | MIT |
| `@babel/helper-validator-identifier` | `7.28.5` | MIT |
| `@babel/parser` | `7.28.5` | MIT |
| `@babel/types` | `7.28.5` | MIT |
| `@braintree/sanitize-url` | `7.1.2` | MIT |
| `@bufbuild/protobuf` | `2.10.0` | (Apache-2.0 AND BSD-3-Clause) |
| `@casbin/expression-eval` | `5.3.0` | MIT |
| `@chevrotain/cst-dts-gen` | `13.2.0` | Apache-2.0 |
| `@chevrotain/gast` | `13.2.0` | Apache-2.0 |
| `@chevrotain/regexp-to-ast` | `13.2.0` | Apache-2.0 |
| `@chevrotain/types` | `13.2.0` | Apache-2.0 |
| `@chevrotain/utils` | `13.2.0` | Apache-2.0 |
| `@codemirror/autocomplete` | `6.20.3` | MIT |
| `@codemirror/commands` | `6.10.4` | MIT |
| `@codemirror/lang-angular` | `0.1.4` | MIT |
| `@codemirror/lang-cpp` | `6.0.3` | MIT |
| `@codemirror/lang-css` | `6.3.1` | MIT |
| `@codemirror/lang-go` | `6.0.1` | MIT |
| `@codemirror/lang-html` | `6.4.12` | MIT |
| `@codemirror/lang-java` | `6.0.2` | MIT |
| `@codemirror/lang-javascript` | `6.2.5` | MIT |
| `@codemirror/lang-jinja` | `6.0.1` | MIT |
| `@codemirror/lang-json` | `6.0.2` | MIT |
| `@codemirror/lang-less` | `6.0.2` | MIT |
| `@codemirror/lang-liquid` | `6.3.3` | MIT |
| `@codemirror/lang-markdown` | `6.5.2` | MIT |
| `@codemirror/lang-php` | `6.0.2` | MIT |
| `@codemirror/lang-python` | `6.2.1` | MIT |
| `@codemirror/lang-rust` | `6.0.2` | MIT |
| `@codemirror/lang-sass` | `6.0.2` | MIT |
| `@codemirror/lang-sql` | `6.10.0` | MIT |
| `@codemirror/lang-vue` | `0.1.3` | MIT |
| `@codemirror/lang-wast` | `6.0.2` | MIT |
| `@codemirror/lang-xml` | `6.1.0` | MIT |
| `@codemirror/lang-yaml` | `6.1.3` | MIT |
| `@codemirror/language` | `6.12.4` | MIT |
| `@codemirror/language-data` | `6.5.2` | MIT |
| `@codemirror/legacy-modes` | `6.5.5` | MIT |
| `@codemirror/lint` | `6.9.7` | MIT |
| `@codemirror/search` | `6.7.2` | MIT |
| `@codemirror/state` | `6.7.1` | MIT |
| `@codemirror/streamparser` | `6.0.0` | MIT |
| `@codemirror/theme-one-dark` | `6.1.3` | MIT |
| `@codemirror/view` | `6.43.8` | MIT |
| `@date-io/core` | `3.2.0` | MIT |
| `@date-io/dayjs` | `3.2.0` | MIT |
| `@esbuild/darwin-arm64` | `0.21.5` | MIT |
| `@eslint-community/eslint-utils` | `4.9.0` | MIT |
| `@eslint-community/regexpp` | `4.12.2` | MIT |
| `@eslint/eslintrc` | `2.1.4` | MIT |
| `@eslint/js` | `8.57.1` | MIT |
| `@formatjs/intl-localematcher` | `0.6.2` | MIT |
| `@humanwhocodes/config-array` | `0.13.0` | Apache-2.0 |
| `@humanwhocodes/module-importer` | `1.0.1` | Apache-2.0 |
| `@humanwhocodes/object-schema` | `2.0.3` | BSD-3-Clause |
| `@iconify/types` | `2.0.0` | MIT |
| `@iconify/utils` | `3.1.7` | MIT |
| `@intlify/core-base` | `11.1.5` | MIT |
| `@intlify/message-compiler` | `11.1.5` | MIT |
| `@intlify/shared` | `11.1.5` | MIT |
| `@jest/schemas` | `29.6.3` | MIT |
| `@jridgewell/sourcemap-codec` | `1.5.5` | MIT |
| `@lezer/common` | `1.5.2` | MIT |
| `@lezer/cpp` | `1.1.6` | MIT |
| `@lezer/css` | `1.3.9` | MIT |
| `@lezer/go` | `1.0.1` | MIT |
| `@lezer/highlight` | `1.2.3` | MIT |
| `@lezer/html` | `1.3.13` | MIT |
| `@lezer/java` | `1.1.5` | MIT |
| `@lezer/javascript` | `1.5.6` | MIT |
| `@lezer/json` | `1.0.3` | MIT |
| `@lezer/lr` | `1.4.10` | MIT |
| `@lezer/markdown` | `1.8.0` | MIT |
| `@lezer/php` | `1.0.6` | MIT |
| `@lezer/python` | `1.1.19` | MIT |
| `@lezer/rust` | `1.0.3` | MIT |
| `@lezer/sass` | `1.1.0` | MIT |
| `@lezer/xml` | `1.0.6` | MIT |
| `@lezer/yaml` | `1.0.4` | MIT |
| `@lucide/vue` | `1.53.0` | ISC |
| `@marijn/find-cluster-break` | `1.0.3` | MIT |
| `@mdi/font` | `7.4.47` | Apache-2.0 |
| `@mermaid-js/parser` | `2.0.1` | MIT |
| `@nodelib/fs.scandir` | `2.1.5` | MIT |
| `@nodelib/fs.stat` | `2.0.5` | MIT |
| `@nodelib/fs.walk` | `1.2.8` | MIT |
| `@rollup/rollup-darwin-arm64` | `4.52.5` | MIT |
| `@sinclair/typebox` | `0.27.8` | MIT |
| `@types/d3` | `7.4.3` | MIT |
| `@types/d3-array` | `3.2.2` | MIT |
| `@types/d3-axis` | `3.0.6` | MIT |
| `@types/d3-brush` | `3.0.6` | MIT |
| `@types/d3-chord` | `3.0.6` | MIT |
| `@types/d3-color` | `3.1.3` | MIT |
| `@types/d3-contour` | `3.0.6` | MIT |
| `@types/d3-delaunay` | `6.0.4` | MIT |
| `@types/d3-dispatch` | `3.0.7` | MIT |
| `@types/d3-drag` | `3.0.7` | MIT |
| `@types/d3-dsv` | `3.0.7` | MIT |
| `@types/d3-ease` | `3.0.2` | MIT |
| `@types/d3-fetch` | `3.0.7` | MIT |
| `@types/d3-force` | `3.0.10` | MIT |
| `@types/d3-format` | `3.0.4` | MIT |
| `@types/d3-geo` | `3.1.1` | MIT |
| `@types/d3-hierarchy` | `3.1.7` | MIT |
| `@types/d3-interpolate` | `3.0.4` | MIT |
| `@types/d3-path` | `3.1.1` | MIT |
| `@types/d3-polygon` | `3.0.2` | MIT |
| `@types/d3-quadtree` | `3.0.6` | MIT |
| `@types/d3-random` | `3.0.4` | MIT |
| `@types/d3-scale` | `4.0.9` | MIT |
| `@types/d3-scale-chromatic` | `3.1.0` | MIT |
| `@types/d3-selection` | `3.0.12` | MIT |
| `@types/d3-shape` | `3.2.0` | MIT |
| `@types/d3-time` | `3.0.4` | MIT |
| `@types/d3-time-format` | `4.0.3` | MIT |
| `@types/d3-timer` | `3.0.2` | MIT |
| `@types/d3-transition` | `3.0.9` | MIT |
| `@types/d3-zoom` | `3.0.9` | MIT |
| `@types/estree` | `1.0.8` | MIT |
| `@types/geojson` | `7946.0.16` | MIT |
| `@types/linkify-it` | `5.0.0` | MIT |
| `@types/markdown-it` | `14.2.0` | MIT |
| `@types/mdurl` | `2.0.0` | MIT |
| `@types/node` | `22.18.12` | MIT |
| `@types/numeral` | `2.0.5` | MIT |
| `@types/trusted-types` | `2.0.7` | MIT |
| `@typescript-eslint/parser` | `6.21.0` | BSD-2-Clause |
| `@typescript-eslint/scope-manager` | `6.21.0` | MIT |
| `@typescript-eslint/types` | `6.21.0` | MIT |
| `@typescript-eslint/typescript-estree` | `6.21.0` | BSD-2-Clause |
| `@typescript-eslint/visitor-keys` | `6.21.0` | MIT |
| `@ungap/structured-clone` | `1.3.0` | ISC |
| `@upsetjs/venn.js` | `2.0.0` | MIT |
| `@vavt/copy2clipboard` | `1.0.3` | MIT |
| `@vavt/markdown-theme` | `6.0.1` | MIT |
| `@vavt/util` | `2.1.2` | MIT |
| `@vee-validate/i18n` | `4.15.1` | MIT |
| `@vue/compiler-core` | `3.5.22` | MIT |
| `@vue/compiler-dom` | `3.5.22` | MIT |
| `@vue/compiler-sfc` | `3.5.22` | MIT |
| `@vue/compiler-ssr` | `3.5.22` | MIT |
| `@vue/devtools-api` | `6.6.4` | MIT |
| `@vue/devtools-api` | `7.7.7` | MIT |
| `@vue/devtools-kit` | `7.7.7` | MIT |
| `@vue/devtools-shared` | `7.7.7` | MIT |
| `@vue/reactivity` | `3.5.22` | MIT |
| `@vue/runtime-core` | `3.5.22` | MIT |
| `@vue/runtime-dom` | `3.5.22` | MIT |
| `@vue/server-renderer` | `3.5.22` | MIT |
| `@vue/shared` | `3.5.22` | MIT |
| `@vuetify/loader-shared` | `2.1.2` | MIT |
| `@vuetify/v0` | `1.2.3` | MIT |
| `acorn` | `8.15.0` | MIT |
| `acorn-jsx` | `5.3.2` | MIT |
| `ajv` | `6.12.6` | MIT |
| `ansi-regex` | `2.1.1` | MIT |
| `ansi-regex` | `5.0.1` | MIT |
| `ansi-styles` | `2.2.1` | MIT |
| `ansi-styles` | `4.3.0` | MIT |
| `ansi-styles` | `5.2.0` | MIT |
| `anymatch` | `3.1.3` | ISC |
| `argparse` | `2.0.1` | Python-2.0 |
| `array-union` | `2.1.0` | MIT |
| `asynckit` | `0.4.0` | MIT |
| `await-lock` | `2.2.2` | MIT |
| `axios` | `1.12.2` | MIT |
| `balanced-match` | `1.0.2` | MIT |
| `base64-js` | `1.5.1` | MIT |
| `binary-extensions` | `2.3.0` | MIT |
| `birpc` | `2.6.1` | MIT |
| `brace-expansion` | `1.1.12` | MIT |
| `brace-expansion` | `2.0.2` | MIT |
| `braces` | `3.0.3` | MIT |
| `buffer` | `6.0.3` | MIT |
| `buffer-builder` | `0.2.0` | MIT |
| `buffer-equal-constant-time` | `1.0.1` | BSD-3-Clause |
| `call-bind-apply-helpers` | `1.0.2` | MIT |
| `callsites` | `3.1.0` | MIT |
| `casbin` | `5.40.0` | Apache-2.0 |
| `chalk` | `1.1.3` | MIT |
| `chalk` | `4.1.2` | MIT |
| `chevrotain` | `13.2.0` | Apache-2.0 |
| `chokidar` | `3.6.0` | MIT |
| `codemirror` | `6.0.2` | MIT |
| `codemirror-lang-rego` | `0.1.0` | MIT |
| `color-convert` | `2.0.1` | MIT |
| `color-name` | `1.1.4` | MIT |
| `colorjs.io` | `0.5.2` | MIT |
| `combined-stream` | `1.0.8` | MIT |
| `commander` | `2.20.3` | MIT |
| `commander` | `7.2.0` | MIT |
| `commander` | `8.3.0` | MIT |
| `common-tags` | `1.8.2` | MIT |
| `concat-map` | `0.0.1` | MIT |
| `consola` | `3.4.2` | MIT |
| `copy-anything` | `4.0.5` | MIT |
| `core-js` | `3.46.0` | MIT |
| `cose-base` | `1.0.3` | MIT |
| `cose-base` | `2.2.0` | MIT |
| `crelt` | `1.0.7` | MIT |
| `cross-spawn` | `7.0.6` | MIT |
| `cssfilter` | `0.0.10` | MIT |
| `csstype` | `3.1.3` | MIT |
| `csv-parse` | `5.6.0` | MIT |
| `cytoscape` | `3.34.3` | MIT |
| `cytoscape-cose-bilkent` | `4.1.0` | MIT |
| `cytoscape-fcose` | `2.2.0` | MIT |
| `d3` | `7.9.0` | ISC |
| `d3-array` | `2.12.1` | BSD-3-Clause |
| `d3-array` | `3.2.4` | ISC |
| `d3-axis` | `3.0.0` | ISC |
| `d3-brush` | `3.0.0` | ISC |
| `d3-chord` | `3.0.1` | ISC |
| `d3-color` | `3.1.0` | ISC |
| `d3-contour` | `4.0.2` | ISC |
| `d3-delaunay` | `6.0.4` | ISC |
| `d3-dispatch` | `3.0.1` | ISC |
| `d3-drag` | `3.0.0` | ISC |
| `d3-dsv` | `3.0.1` | ISC |
| `d3-ease` | `3.0.1` | BSD-3-Clause |
| `d3-fetch` | `3.0.1` | ISC |
| `d3-force` | `3.0.0` | ISC |
| `d3-format` | `3.1.2` | ISC |
| `d3-geo` | `3.1.1` | ISC |
| `d3-hierarchy` | `3.1.2` | ISC |
| `d3-interpolate` | `3.0.1` | ISC |
| `d3-path` | `1.0.9` | BSD-3-Clause |
| `d3-path` | `3.1.0` | ISC |
| `d3-polygon` | `3.0.1` | ISC |
| `d3-quadtree` | `3.0.1` | ISC |
| `d3-random` | `3.0.1` | ISC |
| `d3-sankey` | `0.12.3` | BSD-3-Clause |
| `d3-scale` | `4.0.2` | ISC |
| `d3-scale-chromatic` | `3.1.0` | ISC |
| `d3-selection` | `3.0.0` | ISC |
| `d3-shape` | `1.3.7` | BSD-3-Clause |
| `d3-shape` | `3.2.0` | ISC |
| `d3-time` | `3.1.0` | ISC |
| `d3-time-format` | `4.1.0` | ISC |
| `d3-timer` | `3.0.1` | ISC |
| `d3-transition` | `3.0.1` | ISC |
| `d3-zoom` | `3.0.0` | ISC |
| `dagre-d3-es` | `7.0.14` | MIT |
| `dayjs` | `1.11.23` | MIT |
| `debug` | `4.4.3` | MIT |
| `deep-is` | `0.1.4` | MIT |
| `delaunator` | `5.1.0` | ISC |
| `delayed-stream` | `1.0.0` | MIT |
| `dir-glob` | `3.0.1` | MIT |
| `dlv` | `1.1.3` | MIT |
| `doctrine` | `3.0.0` | Apache-2.0 |
| `dompurify` | `3.4.16` | (MPL-2.0 OR Apache-2.0) |
| `dunder-proto` | `1.0.1` | MIT |
| `ecdsa-sig-formatter` | `1.0.11` | Apache-2.0 |
| `echarts` | `6.0.0` | Apache-2.0 |
| `elkjs` | `0.9.3` | EPL-2.0 |
| `entities` | `4.5.0` | BSD-2-Clause |
| `es-define-property` | `1.0.1` | MIT |
| `es-errors` | `1.3.0` | MIT |
| `es-object-atoms` | `1.1.1` | MIT |
| `es-set-tostringtag` | `2.1.0` | MIT |
| `es-toolkit` | `1.52.0` | MIT |
| `esbuild` | `0.21.5` | MIT |
| `escape-string-regexp` | `1.0.5` | MIT |
| `escape-string-regexp` | `4.0.0` | MIT |
| `eslint` | `8.57.1` | MIT |
| `eslint-scope` | `7.2.2` | BSD-2-Clause |
| `eslint-visitor-keys` | `3.4.3` | Apache-2.0 |
| `espree` | `9.6.1` | BSD-2-Clause |
| `esquery` | `1.6.0` | BSD-3-Clause |
| `esrecurse` | `4.3.0` | BSD-2-Clause |
| `estraverse` | `5.3.0` | BSD-2-Clause |
| `estree-walker` | `2.0.2` | MIT |
| `esutils` | `2.0.3` | BSD-2-Clause |
| `fast-deep-equal` | `3.1.3` | MIT |
| `fast-glob` | `3.3.3` | MIT |
| `fast-json-stable-stringify` | `2.1.0` | MIT |
| `fast-levenshtein` | `2.0.6` | MIT |
| `fastq` | `1.19.1` | ISC |
| `file-entry-cache` | `6.0.1` | MIT |
| `fill-range` | `7.1.1` | MIT |
| `find-up` | `5.0.0` | MIT |
| `flat-cache` | `3.2.0` | MIT |
| `flatted` | `3.3.3` | ISC |
| `follow-redirects` | `1.15.11` | MIT |
| `form-data` | `4.0.4` | MIT |
| `fs.realpath` | `1.0.0` | ISC |
| `fsevents` | `2.3.3` | MIT |
| `function-bind` | `1.1.2` | MIT |
| `get-intrinsic` | `1.3.0` | MIT |
| `get-proto` | `1.0.1` | MIT |
| `glob` | `7.2.3` | ISC |
| `glob-parent` | `5.1.2` | ISC |
| `glob-parent` | `6.0.2` | ISC |
| `globals` | `13.24.0` | MIT |
| `globby` | `11.1.0` | MIT |
| `gopd` | `1.2.0` | MIT |
| `graphemer` | `1.4.0` | MIT |
| `hachure-fill` | `0.5.2` | MIT |
| `has-ansi` | `2.0.0` | MIT |
| `has-flag` | `4.0.0` | MIT |
| `has-symbols` | `1.1.0` | MIT |
| `has-tostringtag` | `1.0.2` | MIT |
| `hasown` | `2.0.2` | MIT |
| `hookable` | `5.5.3` | MIT |
| `iconv-lite` | `0.6.3` | MIT |
| `ieee754` | `1.2.1` | BSD-3-Clause |
| `ignore` | `5.3.2` | MIT |
| `immutable` | `4.3.7` | MIT |
| `immutable` | `5.1.4` | MIT |
| `import-fresh` | `3.3.1` | MIT |
| `import-meta-resolve` | `4.2.0` | MIT |
| `imurmurhash` | `0.1.4` | MIT |
| `indent-string` | `4.0.0` | MIT |
| `inflight` | `1.0.6` | ISC |
| `inherits` | `2.0.4` | ISC |
| `internmap` | `1.0.1` | ISC |
| `internmap` | `2.0.3` | ISC |
| `is-binary-path` | `2.1.0` | MIT |
| `is-extglob` | `2.1.1` | MIT |
| `is-glob` | `4.0.3` | MIT |
| `is-number` | `7.0.0` | MIT |
| `is-path-inside` | `3.0.3` | MIT |
| `is-what` | `5.5.0` | MIT |
| `isexe` | `2.0.0` | ISC |
| `js-cookie` | `3.0.5` | MIT |
| `js-yaml` | `4.1.0` | MIT |
| `jsep` | `0.3.5` | MIT |
| `json-buffer` | `3.0.1` | MIT |
| `json-schema-traverse` | `0.4.1` | MIT |
| `json-stable-stringify-without-jsonify` | `1.0.1` | MIT |
| `json5` | `2.2.3` | MIT |
| `jsonwebtoken` | `9.0.2` | MIT |
| `jwa` | `1.4.2` | MIT |
| `jws` | `3.2.2` | MIT |
| `jwt-decode` | `4.0.0` | MIT |
| `katex` | `0.16.47` | MIT |
| `keyv` | `4.5.4` | MIT |
| `khroma` | `2.1.0` | MIT |
| `layout-base` | `1.0.2` | MIT |
| `layout-base` | `2.0.1` | MIT |
| `levn` | `0.4.1` | MIT |
| `linkify-it` | `5.0.2` | MIT |
| `locate-path` | `6.0.0` | MIT |
| `lodash` | `4.17.21` | MIT |
| `lodash-es` | `4.18.1` | MIT |
| `lodash.includes` | `4.3.0` | MIT |
| `lodash.isboolean` | `3.0.3` | MIT |
| `lodash.isinteger` | `4.0.4` | MIT |
| `lodash.isnumber` | `3.0.3` | MIT |
| `lodash.isplainobject` | `4.0.6` | MIT |
| `lodash.isstring` | `4.0.1` | MIT |
| `lodash.merge` | `4.6.2` | MIT |
| `lodash.once` | `4.1.1` | MIT |
| `loglevel` | `1.9.2` | MIT |
| `loglevel-colored-level-prefix` | `1.0.0` | MIT |
| `luxon` | `3.7.2` | MIT |
| `magic-string` | `0.30.19` | MIT |
| `markdown-it` | `14.3.2` | MIT |
| `markdown-it-image-figures` | `2.1.1` | MIT |
| `markdown-it-sub` | `2.0.0` | MIT |
| `markdown-it-sup` | `2.0.0` | MIT |
| `marked` | `16.4.2` | MIT |
| `math-intrinsics` | `1.1.0` | MIT |
| `md-editor-v3` | `7.1.0` | MIT |
| `mdurl` | `2.1.0` | MIT |
| `medium-zoom` | `1.1.0` | MIT |
| `merge2` | `1.4.1` | MIT |
| `mermaid` | `12.1.0` | MIT |
| `micromatch` | `4.0.8` | MIT |
| `mime-db` | `1.52.0` | MIT |
| `mime-types` | `2.1.35` | MIT |
| `minimatch` | `3.1.2` | ISC |
| `minimatch` | `7.4.6` | ISC |
| `minimatch` | `9.0.3` | ISC |
| `mitt` | `3.0.1` | MIT |
| `ms` | `2.1.3` | MIT |
| `nanoid` | `3.3.11` | MIT |
| `natural-compare` | `1.4.0` | MIT |
| `normalize-path` | `3.0.0` | MIT |
| `numeral` | `2.0.6` | MIT |
| `once` | `1.4.0` | ISC |
| `optionator` | `0.9.4` | MIT |
| `p-limit` | `3.1.0` | MIT |
| `p-locate` | `5.0.0` | MIT |
| `package-manager-detector` | `1.9.0` | MIT |
| `parent-module` | `1.0.1` | MIT |
| `parse-duration` | `2.1.8` | MIT |
| `path-data-parser` | `0.1.0` | MIT |
| `path-exists` | `4.0.0` | MIT |
| `path-is-absolute` | `1.0.1` | MIT |
| `path-key` | `3.1.1` | MIT |
| `path-type` | `4.0.0` | MIT |
| `perfect-debounce` | `1.0.0` | MIT |
| `picocolors` | `1.1.1` | ISC |
| `picomatch` | `2.3.1` | MIT |
| `points-on-curve` | `0.2.0` | MIT |
| `points-on-path` | `0.2.1` | MIT |
| `postcss` | `8.5.6` | MIT |
| `prelude-ls` | `1.2.1` | MIT |
| `prettier` | `3.9.6` | MIT |
| `prettier-eslint` | `16.4.2` | MIT |
| `pretty-format` | `29.7.0` | MIT |
| `property-expr` | `2.0.6` | MIT |
| `proxy-from-env` | `1.1.0` | MIT |
| `punycode` | `2.3.1` | MIT |
| `punycode.js` | `2.3.1` | MIT |
| `queue-microtask` | `1.2.3` | MIT |
| `react-is` | `18.3.1` | MIT |
| `readdirp` | `3.6.0` | MIT |
| `require-relative` | `0.8.7` | MIT |
| `resolve-from` | `4.0.0` | MIT |
| `reusify` | `1.1.0` | MIT |
| `rfdc` | `1.4.1` | MIT |
| `rimraf` | `3.0.2` | ISC |
| `roboto-fontface` | `0.10.0` | Apache-2.0 |
| `robust-predicates` | `3.0.3` | Unlicense |
| `rollup` | `4.52.5` | MIT |
| `roughjs` | `4.6.6` | MIT |
| `run-parallel` | `1.2.0` | MIT |
| `rw` | `1.3.3` | BSD-3-Clause |
| `rxjs` | `7.8.2` | Apache-2.0 |
| `safe-buffer` | `5.2.1` | MIT |
| `safer-buffer` | `2.1.2` | MIT |
| `sass` | `1.77.8` | MIT |
| `sass-embedded` | `1.93.2` | MIT |
| `sass-embedded-darwin-arm64` | `1.93.2` | MIT |
| `semver` | `7.7.3` | ISC |
| `shebang-command` | `2.0.0` | MIT |
| `shebang-regex` | `3.0.0` | MIT |
| `slash` | `3.0.0` | MIT |
| `source-map-js` | `1.2.1` | BSD-3-Clause |
| `speakingurl` | `14.0.1` | BSD-3-Clause |
| `strip-ansi` | `3.0.1` | MIT |
| `strip-ansi` | `6.0.1` | MIT |
| `strip-json-comments` | `3.1.1` | MIT |
| `style-mod` | `4.1.3` | MIT |
| `stylis` | `4.4.0` | MIT |
| `superjson` | `2.2.3` | MIT |
| `supports-color` | `2.0.0` | MIT |
| `supports-color` | `7.2.0` | MIT |
| `supports-color` | `8.1.1` | MIT |
| `sync-child-process` | `1.0.2` | MIT |
| `sync-message-port` | `1.1.3` | MIT |
| `text-table` | `0.2.0` | MIT |
| `tiny-case` | `1.0.3` | MIT |
| `tinyexec` | `1.3.1` | MIT |
| `to-regex-range` | `5.0.1` | MIT |
| `toposort` | `2.0.2` | MIT |
| `ts-api-utils` | `1.4.3` | MIT |
| `ts-dedent` | `2.3.0` | MIT |
| `tslib` | `2.3.0` | 0BSD |
| `tslib` | `2.8.1` | 0BSD |
| `type-check` | `0.4.0` | MIT |
| `type-fest` | `0.20.2` | (MIT OR CC0-1.0) |
| `type-fest` | `2.19.0` | (MIT OR CC0-1.0) |
| `type-fest` | `4.41.0` | (MIT OR CC0-1.0) |
| `typescript` | `5.7.3` | Apache-2.0 |
| `uc.micro` | `2.1.0` | MIT |
| `undici-types` | `6.21.0` | MIT |
| `upath` | `2.0.1` | MIT |
| `uri-js` | `4.4.1` | BSD-2-Clause |
| `uuid` | `11.1.0` | MIT |
| `varint` | `6.0.0` | MIT |
| `vee-validate` | `4.15.1` | MIT |
| `vite` | `5.4.21` | MIT |
| `vite-plugin-vuetify` | `2.1.3` | MIT |
| `vue` | `3.5.22` | MIT |
| `vue-echarts` | `8.0.1` | MIT |
| `vue-eslint-parser` | `9.4.3` | MIT |
| `vue-i18n` | `11.1.5` | MIT |
| `vue-json-pretty` | `2.6.0` | MIT |
| `vuetify` | `4.2.4` | MIT |
| `w3c-keyname` | `2.2.8` | MIT |
| `which` | `2.0.2` | ISC |
| `word-wrap` | `1.2.5` | MIT |
| `wrappy` | `1.0.2` | ISC |
| `xss` | `1.0.15` | MIT |
| `yocto-queue` | `0.1.0` | MIT |
| `yup` | `1.7.1` | MIT |
| `zrender` | `6.0.0` | BSD-3-Clause |

## 5. 已核对：未随包携带 LICENSE 的模块

Go 模块打包只包含模块目录内的文件，因此子目录模块不会携带仓库根部的 `LICENSE`。以下两个模块已核对为 **Apache-2.0**，无需人工处理：

| 模块 | 版本 | 核对结论 | 上游证据 |
| --- | --- | --- | --- |
| `github.com/cloudwego/eino-ext/components/model/openai` | `v0.1.13` | Apache-2.0 | 属于 `github.com/cloudwego/eino-ext` 仓库；仓库根 `LICENSE` 为 Apache-2.0 全文 |
| `github.com/cloudwego/eino-ext/libs/acl/openai` | `v0.1.17` | Apache-2.0 | 同上；该模块 `README.md` 明确写明 “licensed under the Apache License 2.0” |

核对记录：

- 两个模块目录均不含 `LICENSE`/`COPYING`/`NOTICE` 文件。
- 上游仓库根 `LICENSE`（`https://github.com/cloudwego/eino-ext/blob/main/LICENSE`）为 Apache-2.0 全文。
- 仓库根 `README.md` 标注 `Apache-2.0 License`；`libs/acl/openai/README.md` 的 License 小节同样标注 Apache-2.0。
- 子目录 `components/model/openai/LICENSE` 与 `libs/acl/openai/LICENSE` 在上游均返回 404，说明这两个子目录没有单独声明其他许可证。

该核对基于 2026-10-21 的上游 `main` 分支与模块版本 `v0.1.13`、`v0.1.17`。若后续升级版本，应重新确认。

### 双许可组件选择

| 组件 | 版本 | 声明 | 本项目选择 |
| --- | --- | --- | --- |
| `dompurify` | `3.4.16` | `MPL-2.0 OR Apache-2.0` | Apache-2.0（包内同时提供 `LICENSE` 与 `LICENSE-MPL`） |
| `type-fest` | `0.20.2` / `2.19.0` / `4.41.0` | `MIT OR CC0-1.0` | MIT |
| `@bufbuild/protobuf` | `2.10.0` | `Apache-2.0 AND BSD-3-Clause` | 同时遵守两者（均为宽松许可） |

选择宽松分支后，上述组件不会触发 MPL 的源码公开义务；`@bufbuild/protobuf` 为“同时适用”，需保留两份声明。

### 弱 copyleft 组件的源码获取方式

分发二进制或前端静态资源时，需让接收者能够获得以下组件的对应源码（未修改时提供上游链接即可）：

| 组件 | 版本 | 许可证 | 源码 |
| --- | --- | --- | --- |
| `github.com/go-sql-driver/mysql` | `v1.9.3` | MPL-2.0 | <https://github.com/go-sql-driver/mysql> |
| `github.com/hashicorp/go-version` | `v1.8.0` | MPL-2.0 | <https://github.com/hashicorp/go-version> |
| `elkjs` | `0.9.3` | EPL-2.0 | <https://github.com/kieler/elkjs> |

若修改了上述组件，必须按各自许可证公开修改部分；本项目当前未对其做本地修改。

## 6. 许可证全文

以下文本用于满足许可证的保留声明要求。MIT/BSD/ISC 等许可证的示例文本保留了代表性组件的版权行；各组件实际的版权归属与完整声明以其依赖包内 `LICENSE` 文件及上游仓库为准。

### Apache-2.0

```text
Apache License
                           Version 2.0, January 2004
                        http://www.apache.org/licenses/

   TERMS AND CONDITIONS FOR USE, REPRODUCTION, AND DISTRIBUTION

   1. Definitions.

      "License" shall mean the terms and conditions for use, reproduction,
      and distribution as defined by Sections 1 through 9 of this document.

      "Licensor" shall mean the copyright owner or entity authorized by
      the copyright owner that is granting the License.

      "Legal Entity" shall mean the union of the acting entity and all
      other entities that control, are controlled by, or are under common
      control with that entity. For the purposes of this definition,
      "control" means (i) the power, direct or indirect, to cause the
      direction or management of such entity, whether by contract or
      otherwise, or (ii) ownership of fifty percent (50%) or more of the
      outstanding shares, or (iii) beneficial ownership of such entity.

      "You" (or "Your") shall mean an individual or Legal Entity
      exercising permissions granted by this License.

      "Source" form shall mean the preferred form for making modifications,
      including but not limited to software source code, documentation
      source, and configuration files.

      "Object" form shall mean any form resulting from mechanical
      transformation or translation of a Source form, including but
      not limited to compiled object code, generated documentation,
      and conversions to other media types.

      "Work" shall mean the work of authorship, whether in Source or
      Object form, made available under the License, as indicated by a
      copyright notice that is included in or attached to the work
      (an example is provided in the Appendix below).

      "Derivative Works" shall mean any work, whether in Source or Object
      form, that is based on (or derived from) the Work and for which the
      editorial revisions, annotations, elaborations, or other modifications
      represent, as a whole, an original work of authorship. For the purposes
      of this License, Derivative Works shall not include works that remain
      separable from, or merely link (or bind by name) to the interfaces of,
      the Work and Derivative Works thereof.

      "Contribution" shall mean any work of authorship, including
      the original version of the Work and any modifications or additions
      to that Work or Derivative Works thereof, that is intentionally
      submitted to Licensor for inclusion in the Work by the copyright owner
      or by an individual or Legal Entity authorized to submit on behalf of
      the copyright owner. For the purposes of this definition, "submitted"
      means any form of electronic, verbal, or written communication sent
      to the Licensor or its representatives, including but not limited to
      communication on electronic mailing lists, source code control systems,
      and issue tracking systems that are managed by, or on behalf of, the
      Licensor for the purpose of discussing and improving the Work, but
      excluding communication that is conspicuously marked or otherwise
      designated in writing by the copyright owner as "Not a Contribution."

      "Contributor" shall mean Licensor and any individual or Legal Entity
      on behalf of whom a Contribution has been received by Licensor and
      subsequently incorporated within the Work.

   2. Grant of Copyright License. Subject to the terms and conditions of
      this License, each Contributor hereby grants to You a perpetual,
      worldwide, non-exclusive, no-charge, royalty-free, irrevocable
      copyright license to reproduce, prepare Derivative Works of,
      publicly display, publicly perform, sublicense, and distribute the
      Work and such Derivative Works in Source or Object form.

   3. Grant of Patent License. Subject to the terms and conditions of
      this License, each Contributor hereby grants to You a perpetual,
      worldwide, non-exclusive, no-charge, royalty-free, irrevocable
      (except as stated in this section) patent license to make, have made,
      use, offer to sell, sell, import, and otherwise transfer the Work,
      where such license applies only to those patent claims licensable
      by such Contributor that are necessarily infringed by their
      Contribution(s) alone or by combination of their Contribution(s)
      with the Work to which such Contribution(s) was submitted. If You
      institute patent litigation against any entity (including a
      cross-claim or counterclaim in a lawsuit) alleging that the Work
      or a Contribution incorporated within the Work constitutes direct
      or contributory patent infringement, then any patent licenses
      granted to You under this License for that Work shall terminate
      as of the date such litigation is filed.

   4. Redistribution. You may reproduce and distribute copies of the
      Work or Derivative Works thereof in any medium, with or without
      modifications, and in Source or Object form, provided that You
      meet the following conditions:

      (a) You must give any other recipients of the Work or
          Derivative Works a copy of this License; and

      (b) You must cause any modified files to carry prominent notices
          stating that You changed the files; and

      (c) You must retain, in the Source form of any Derivative Works
          that You distribute, all copyright, patent, trademark, and
          attribution notices from the Source form of the Work,
          excluding those notices that do not pertain to any part of
          the Derivative Works; and

      (d) If the Work includes a "NOTICE" text file as part of its
          distribution, then any Derivative Works that You distribute must
          include a readable copy of the attribution notices contained
          within such NOTICE file, excluding those notices that do not
          pertain to any part of the Derivative Works, in at least one
          of the following places: within a NOTICE text file distributed
          as part of the Derivative Works; within the Source form or
          documentation, if provided along with the Derivative Works; or,
          within a display generated by the Derivative Works, if and
          wherever such third-party notices normally appear. The contents
          of the NOTICE file are for informational purposes only and
          do not modify the License. You may add Your own attribution
          notices within Derivative Works that You distribute, alongside
          or as an addendum to the NOTICE text from the Work, provided
          that such additional attribution notices cannot be construed
          as modifying the License.

      You may add Your own copyright statement to Your modifications and
      may provide additional or different license terms and conditions
      for use, reproduction, or distribution of Your modifications, or
      for any such Derivative Works as a whole, provided Your use,
      reproduction, and distribution of the Work otherwise complies with
      the conditions stated in this License.

   5. Submission of Contributions. Unless You explicitly state otherwise,
      any Contribution intentionally submitted for inclusion in the Work
      by You to the Licensor shall be under the terms and conditions of
      this License, without any additional terms or conditions.
      Notwithstanding the above, nothing herein shall supersede or modify
      the terms of any separate license agreement you may have executed
      with Licensor regarding such Contributions.

   6. Trademarks. This License does not grant permission to use the trade
      names, trademarks, service marks, or product names of the Licensor,
      except as required for reasonable and customary use in describing the
      origin of the Work and reproducing the content of the NOTICE file.

   7. Disclaimer of Warranty. Unless required by applicable law or
      agreed to in writing, Licensor provides the Work (and each
      Contributor provides its Contributions) on an "AS IS" BASIS,
      WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
      implied, including, without limitation, any warranties or conditions
      of TITLE, NON-INFRINGEMENT, MERCHANTABILITY, or FITNESS FOR A
      PARTICULAR PURPOSE. You are solely responsible for determining the
      appropriateness of using or redistributing the Work and assume any
      risks associated with Your exercise of permissions under this License.

   8. Limitation of Liability. In no event and under no legal theory,
      whether in tort (including negligence), contract, or otherwise,
      unless required by applicable law (such as deliberate and grossly
      negligent acts) or agreed to in writing, shall any Contributor be
      liable to You for damages, including any direct, indirect, special,
      incidental, or consequential damages of any character arising as a
      result of this License or out of the use or inability to use the
      Work (including but not limited to damages for loss of goodwill,
      work stoppage, computer failure or malfunction, or any and all
      other commercial damages or losses), even if such Contributor
      has been advised of the possibility of such damages.

   9. Accepting Warranty or Additional Liability. While redistributing
      the Work or Derivative Works thereof, You may choose to offer,
      and charge a fee for, acceptance of support, warranty, indemnity,
      or other liability obligations and/or rights consistent with this
      License. However, in accepting such obligations, You may act only
      on Your own behalf and on Your sole responsibility, not on behalf
      of any other Contributor, and only if You agree to indemnify,
      defend, and hold each Contributor harmless for any liability
      incurred by, or claims asserted against, such Contributor by reason
      of your accepting any such warranty or additional liability.

   END OF TERMS AND CONDITIONS

   APPENDIX: How to apply the Apache License to your work.

      To apply the Apache License to your work, attach the following
      boilerplate notice, with the fields enclosed by brackets "[]"
      replaced with your own identifying information. (Don't include
      the brackets!)  The text should be enclosed in the appropriate
      comment syntax for the file format. We also recommend that a
      file or class name and description of purpose be included on the
      same "printed page" as the copyright notice for easier
      identification within third-party archives.

   Copyright [yyyy] [name of copyright owner]

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
```

### MIT

```text
MIT License

Copyright (c) 2020 Marvin Wendt (MarvinJWendt)

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

### BSD-2-Clause

```text
Copyright (c) 2015, Emir Pasic
All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

* Redistributions of source code must retain the above copyright notice, this
  list of conditions and the following disclaimer.

* Redistributions in binary form must reproduce the above copyright notice,
  this list of conditions and the following disclaimer in the documentation
  and/or other materials provided with the distribution.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

-------------------------------------------------------------------------------

AVL Tree:

Copyright (c) 2017 Benjamin Scher Purcell <benjapurcell@gmail.com>

Permission to use, copy, modify, and distribute this software for any
purpose with or without fee is hereby granted, provided that the above
copyright notice and this permission notice appear in all copies.

THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
```

### BSD-3-Clause

```text
Copyright (c) 2009 The Go Authors. All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:

   * Redistributions of source code must retain the above copyright
notice, this list of conditions and the following disclaimer.
   * Redistributions in binary form must reproduce the above
copyright notice, this list of conditions and the following disclaimer
in the documentation and/or other materials provided with the
distribution.
   * Neither the name of Google Inc. nor the names of its
contributors may be used to endorse or promote products derived from
this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
```

### ISC

```text
ISC License

Copyright (c) 2026 Lucide Icons and Contributors

Permission to use, copy, modify, and/or distribute this software for any
purpose with or without fee is hereby granted, provided that the above
copyright notice and this permission notice appear in all copies.

THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.

---

The following Lucide icons are derived from the Feather project:

airplay, alert-circle, alert-octagon, alert-triangle, aperture, arrow-down-circle, arrow-down-left, arrow-down-right, arrow-down, arrow-left-circle, arrow-left, arrow-right-circle, arrow-right, arrow-up-circle, arrow-up-left, arrow-up-right, arrow-up, at-sign, calendar, cast, check, chevron-down, chevron-left, chevron-right, chevron-up, chevrons-down, chevrons-left, chevrons-right, chevrons-up, circle, clipboard, clock, code, columns, command, compass, corner-down-left, corner-down-right, corner-left-down, corner-left-up, corner-right-down, corner-right-up, corner-up-left, corner-up-right, crosshair, database, divide-circle, divide-square, dollar-sign, download, external-link, feather, frown, hash, headphones, help-circle, info, italic, key, layout, life-buoy, link-2, link, loader, lock, log-in, log-out, maximize, meh, minimize, minimize-2, minus-circle, minus-square, minus, monitor, moon, more-horizontal, more-vertical, move, music, navigation-2, navigation, octagon, pause-circle, percent, plus-circle, plus-square, plus, power, radio, rss, search, server, share, shopping-bag, sidebar, smartphone, smile, square, table-2, tablet, target, terminal, trash-2, trash, triangle, tv, type, upload, x-circle, x-octagon, x-square, x, zoom-in, zoom-out

The MIT License (MIT) (for the icons listed above)

Copyright (c) 2013-present Cole Bemis

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

### MPL-2.0

```text
Mozilla Public License Version 2.0
==================================

1. Definitions
--------------

1.1. "Contributor"
    means each individual or legal entity that creates, contributes to
    the creation of, or owns Covered Software.

1.2. "Contributor Version"
    means the combination of the Contributions of others (if any) used
    by a Contributor and that particular Contributor's Contribution.

1.3. "Contribution"
    means Covered Software of a particular Contributor.

1.4. "Covered Software"
    means Source Code Form to which the initial Contributor has attached
    the notice in Exhibit A, the Executable Form of such Source Code
    Form, and Modifications of such Source Code Form, in each case
    including portions thereof.

1.5. "Incompatible With Secondary Licenses"
    means

    (a) that the initial Contributor has attached the notice described
        in Exhibit B to the Covered Software; or

    (b) that the Covered Software was made available under the terms of
        version 1.1 or earlier of the License, but not also under the
        terms of a Secondary License.

1.6. "Executable Form"
    means any form of the work other than Source Code Form.

1.7. "Larger Work"
    means a work that combines Covered Software with other material, in
    a separate file or files, that is not Covered Software.

1.8. "License"
    means this document.

1.9. "Licensable"
    means having the right to grant, to the maximum extent possible,
    whether at the time of the initial grant or subsequently, any and
    all of the rights conveyed by this License.

1.10. "Modifications"
    means any of the following:

    (a) any file in Source Code Form that results from an addition to,
        deletion from, or modification of the contents of Covered
        Software; or

    (b) any new file in Source Code Form that contains any Covered
        Software.

1.11. "Patent Claims" of a Contributor
    means any patent claim(s), including without limitation, method,
    process, and apparatus claims, in any patent Licensable by such
    Contributor that would be infringed, but for the grant of the
    License, by the making, using, selling, offering for sale, having
    made, import, or transfer of either its Contributions or its
    Contributor Version.

1.12. "Secondary License"
    means either the GNU General Public License, Version 2.0, the GNU
    Lesser General Public License, Version 2.1, the GNU Affero General
    Public License, Version 3.0, or any later versions of those
    licenses.

1.13. "Source Code Form"
    means the form of the work preferred for making modifications.

1.14. "You" (or "Your")
    means an individual or a legal entity exercising rights under this
    License. For legal entities, "You" includes any entity that
    controls, is controlled by, or is under common control with You. For
    purposes of this definition, "control" means (a) the power, direct
    or indirect, to cause the direction or management of such entity,
    whether by contract or otherwise, or (b) ownership of more than
    fifty percent (50%) of the outstanding shares or beneficial
    ownership of such entity.

2. License Grants and Conditions
--------------------------------

2.1. Grants

Each Contributor hereby grants You a world-wide, royalty-free,
non-exclusive license:

(a) under intellectual property rights (other than patent or trademark)
    Licensable by such Contributor to use, reproduce, make available,
    modify, display, perform, distribute, and otherwise exploit its
    Contributions, either on an unmodified basis, with Modifications, or
    as part of a Larger Work; and

(b) under Patent Claims of such Contributor to make, use, sell, offer
    for sale, have made, import, and otherwise transfer either its
    Contributions or its Contributor Version.

2.2. Effective Date

The licenses granted in Section 2.1 with respect to any Contribution
become effective for each Contribution on the date the Contributor first
distributes such Contribution.

2.3. Limitations on Grant Scope

The licenses granted in this Section 2 are the only rights granted under
this License. No additional rights or licenses will be implied from the
distribution or licensing of Covered Software under this License.
Notwithstanding Section 2.1(b) above, no patent license is granted by a
Contributor:

(a) for any code that a Contributor has removed from Covered Software;
    or

(b) for infringements caused by: (i) Your and any other third party's
    modifications of Covered Software, or (ii) the combination of its
    Contributions with other software (except as part of its Contributor
    Version); or

(c) under Patent Claims infringed by Covered Software in the absence of
    its Contributions.

This License does not grant any rights in the trademarks, service marks,
or logos of any Contributor (except as may be necessary to comply with
the notice requirements in Section 3.4).

2.4. Subsequent Licenses

No Contributor makes additional grants as a result of Your choice to
distribute the Covered Software under a subsequent version of this
License (see Section 10.2) or under the terms of a Secondary License (if
permitted under the terms of Section 3.3).

2.5. Representation

Each Contributor represents that the Contributor believes its
Contributions are its original creation(s) or it has sufficient rights
to grant the rights to its Contributions conveyed by this License.

2.6. Fair Use

This License is not intended to limit any rights You have under
applicable copyright doctrines of fair use, fair dealing, or other
equivalents.

2.7. Conditions

Sections 3.1, 3.2, 3.3, and 3.4 are conditions of the licenses granted
in Section 2.1.

3. Responsibilities
-------------------

3.1. Distribution of Source Form

All distribution of Covered Software in Source Code Form, including any
Modifications that You create or to which You contribute, must be under
the terms of this License. You must inform recipients that the Source
Code Form of the Covered Software is governed by the terms of this
License, and how they can obtain a copy of this License. You may not
attempt to alter or restrict the recipients' rights in the Source Code
Form.

3.2. Distribution of Executable Form

If You distribute Covered Software in Executable Form then:

(a) such Covered Software must also be made available in Source Code
    Form, as described in Section 3.1, and You must inform recipients of
    the Executable Form how they can obtain a copy of such Source Code
    Form by reasonable means in a timely manner, at a charge no more
    than the cost of distribution to the recipient; and

(b) You may distribute such Executable Form under the terms of this
    License, or sublicense it under different terms, provided that the
    license for the Executable Form does not attempt to limit or alter
    the recipients' rights in the Source Code Form under this License.

3.3. Distribution of a Larger Work

You may create and distribute a Larger Work under terms of Your choice,
provided that You also comply with the requirements of this License for
the Covered Software. If the Larger Work is a combination of Covered
Software with a work governed by one or more Secondary Licenses, and the
Covered Software is not Incompatible With Secondary Licenses, this
License permits You to additionally distribute such Covered Software
under the terms of such Secondary License(s), so that the recipient of
the Larger Work may, at their option, further distribute the Covered
Software under the terms of either this License or such Secondary
License(s).

3.4. Notices

You may not remove or alter the substance of any license notices
(including copyright notices, patent notices, disclaimers of warranty,
or limitations of liability) contained within the Source Code Form of
the Covered Software, except that You may alter any license notices to
the extent required to remedy known factual inaccuracies.

3.5. Application of Additional Terms

You may choose to offer, and to charge a fee for, warranty, support,
indemnity or liability obligations to one or more recipients of Covered
Software. However, You may do so only on Your own behalf, and not on
behalf of any Contributor. You must make it absolutely clear that any
such warranty, support, indemnity, or liability obligation is offered by
You alone, and You hereby agree to indemnify every Contributor for any
liability incurred by such Contributor as a result of warranty, support,
indemnity or liability terms You offer. You may include additional
disclaimers of warranty and limitations of liability specific to any
jurisdiction.

4. Inability to Comply Due to Statute or Regulation
---------------------------------------------------

If it is impossible for You to comply with any of the terms of this
License with respect to some or all of the Covered Software due to
statute, judicial order, or regulation then You must: (a) comply with
the terms of this License to the maximum extent possible; and (b)
describe the limitations and the code they affect. Such description must
be placed in a text file included with all distributions of the Covered
Software under this License. Except to the extent prohibited by statute
or regulation, such description must be sufficiently detailed for a
recipient of ordinary skill to be able to understand it.

5. Termination
--------------

5.1. The rights granted under this License will terminate automatically
if You fail to comply with any of its terms. However, if You become
compliant, then the rights granted under this License from a particular
Contributor are reinstated (a) provisionally, unless and until such
Contributor explicitly and finally terminates Your grants, and (b) on an
ongoing basis, if such Contributor fails to notify You of the
non-compliance by some reasonable means prior to 60 days after You have
come back into compliance. Moreover, Your grants from a particular
Contributor are reinstated on an ongoing basis if such Contributor
notifies You of the non-compliance by some reasonable means, this is the
first time You have received notice of non-compliance with this License
from such Contributor, and You become compliant prior to 30 days after
Your receipt of the notice.

5.2. If You initiate litigation against any entity by asserting a patent
infringement claim (excluding declaratory judgment actions,
counter-claims, and cross-claims) alleging that a Contributor Version
directly or indirectly infringes any patent, then the rights granted to
You by any and all Contributors for the Covered Software under Section
2.1 of this License shall terminate.

5.3. In the event of termination under Sections 5.1 or 5.2 above, all
end user license agreements (excluding distributors and resellers) which
have been validly granted by You or Your distributors under this License
prior to termination shall survive termination.

************************************************************************
*                                                                      *
*  6. Disclaimer of Warranty                                           *
*  -------------------------                                           *
*                                                                      *
*  Covered Software is provided under this License on an "as is"       *
*  basis, without warranty of any kind, either expressed, implied, or  *
*  statutory, including, without limitation, warranties that the       *
*  Covered Software is free of defects, merchantable, fit for a        *
*  particular purpose or non-infringing. The entire risk as to the     *
*  quality and performance of the Covered Software is with You.        *
*  Should any Covered Software prove defective in any respect, You     *
*  (not any Contributor) assume the cost of any necessary servicing,   *
*  repair, or correction. This disclaimer of warranty constitutes an   *
*  essential part of this License. No use of any Covered Software is   *
*  authorized under this License except under this disclaimer.         *
*                                                                      *
************************************************************************

************************************************************************
*                                                                      *
*  7. Limitation of Liability                                          *
*  --------------------------                                          *
*                                                                      *
*  Under no circumstances and under no legal theory, whether tort      *
*  (including negligence), contract, or otherwise, shall any           *
*  Contributor, or anyone who distributes Covered Software as          *
*  permitted above, be liable to You for any direct, indirect,         *
*  special, incidental, or consequential damages of any character      *
*  including, without limitation, damages for lost profits, loss of    *
*  goodwill, work stoppage, computer failure or malfunction, or any    *
*  and all other commercial damages or losses, even if such party      *
*  shall have been informed of the possibility of such damages. This   *
*  limitation of liability shall not apply to liability for death or   *
*  personal injury resulting from such party's negligence to the       *
*  extent applicable law prohibits such limitation. Some               *
*  jurisdictions do not allow the exclusion or limitation of           *
*  incidental or consequential damages, so this exclusion and          *
*  limitation may not apply to You.                                    *
*                                                                      *
************************************************************************

8. Litigation
-------------

Any litigation relating to this License may be brought only in the
courts of a jurisdiction where the defendant maintains its principal
place of business and such litigation shall be governed by laws of that
jurisdiction, without reference to its conflict-of-law provisions.
Nothing in this Section shall prevent a party's ability to bring
cross-claims or counter-claims.

9. Miscellaneous
----------------

This License represents the complete agreement concerning the subject
matter hereof. If any provision of this License is held to be
unenforceable, such provision shall be reformed only to the extent
necessary to make it enforceable. Any law or regulation which provides
that the language of a contract shall be construed against the drafter
shall not be used to construe this License against a Contributor.

10. Versions of the License
---------------------------

10.1. New Versions

Mozilla Foundation is the license steward. Except as provided in Section
10.3, no one other than the license steward has the right to modify or
publish new versions of this License. Each version will be given a
distinguishing version number.

10.2. Effect of New Versions

You may distribute the Covered Software under the terms of the version
of the License under which You originally received the Covered Software,
or under the terms of any subsequent version published by the license
steward.

10.3. Modified Versions

If you create software not governed by this License, and you want to
create a new license for such software, you may create and use a
modified version of this License if you rename the license and remove
any references to the name of the license steward (except to note that
such modified license differs from this License).

10.4. Distributing Source Code Form that is Incompatible With Secondary
Licenses

If You choose to distribute Source Code Form that is Incompatible With
Secondary Licenses under the terms of this version of the License, the
notice described in Exhibit B of this License must be attached.

Exhibit A - Source Code Form License Notice
-------------------------------------------

  This Source Code Form is subject to the terms of the Mozilla Public
  License, v. 2.0. If a copy of the MPL was not distributed with this
  file, You can obtain one at http://mozilla.org/MPL/2.0/.

If it is not possible or desirable to put the notice in a particular
file, then You may include the notice in a location (such as a LICENSE
file in a relevant directory) where a recipient would be likely to look
for such a notice.

You may add additional accurate notices of copyright ownership.

Exhibit B - "Incompatible With Secondary Licenses" Notice
---------------------------------------------------------

  This Source Code Form is "Incompatible With Secondary Licenses", as
  defined by the Mozilla Public License, v. 2.0.
```

### 0BSD

```text
Copyright (c) Microsoft Corporation.

Permission to use, copy, modify, and/or distribute this software for any
purpose with or without fee is hereby granted.

THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES WITH
REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF MERCHANTABILITY
AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR ANY SPECIAL, DIRECT,
INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES WHATSOEVER RESULTING FROM
LOSS OF USE, DATA OR PROFITS, WHETHER IN AN ACTION OF CONTRACT, NEGLIGENCE OR
OTHER TORTIOUS ACTION, ARISING OUT OF OR IN CONNECTION WITH THE USE OR
PERFORMANCE OF THIS SOFTWARE.
```

### EPL-2.0

```text
# Eclipse Public License - v 2.0

THE ACCOMPANYING PROGRAM IS PROVIDED UNDER THE TERMS OF THIS ECLIPSE
PUBLIC LICENSE (“AGREEMENT”). ANY USE, REPRODUCTION OR DISTRIBUTION OF
THE PROGRAM CONSTITUTES RECIPIENT'S ACCEPTANCE OF THIS AGREEMENT.

## 1. DEFINITIONS

“Contribution” means:

-   a\) in the case of the initial Contributor, the initial content
    Distributed under this Agreement, and
-   b\) in the case of each subsequent Contributor:
    -   i\) changes to the Program, and
    -   ii\) additions to the Program;

    where such changes and/or additions to the Program originate from
    and are Distributed by that particular Contributor. A Contribution
    “originates” from a Contributor if it was added to the Program by
    such Contributor itself or anyone acting on such Contributor's
    behalf. Contributions do not include changes or additions to the
    Program that are not Modified Works.

“Contributor” means any person or entity that Distributes the Program.

“Licensed Patents” mean patent claims licensable by a Contributor which
are necessarily infringed by the use or sale of its Contribution alone
or when combined with the Program.

“Program” means the Contributions Distributed in accordance with this
Agreement.

“Recipient” means anyone who receives the Program under this Agreement
or any Secondary License (as applicable), including Contributors.

“Derivative Works” shall mean any work, whether in Source Code or other
form, that is based on (or derived from) the Program and for which the
editorial revisions, annotations, elaborations, or other modifications
represent, as a whole, an original work of authorship.

“Modified Works” shall mean any work in Source Code or other form that
results from an addition to, deletion from, or modification of the
contents of the Program, including, for purposes of clarity any new file
in Source Code form that contains any contents of the Program. Modified
Works shall not include works that contain only declarations,
interfaces, types, classes, structures, or files of the Program solely
in each case in order to link to, bind by name, or subclass the Program
or Modified Works thereof.

“Distribute” means the acts of a) distributing or b) making available in
any manner that enables the transfer of a copy.

“Source Code” means the form of a Program preferred for making
modifications, including but not limited to software source code,
documentation source, and configuration files.

“Secondary License” means either the GNU General Public License, Version
2.0, or any later versions of that license, including any exceptions or
additional permissions as identified by the initial Contributor.

## 2. GRANT OF RIGHTS

-   a\) Subject to the terms of this Agreement, each Contributor hereby
    grants Recipient a non-exclusive, worldwide, royalty-free copyright
    license to reproduce, prepare Derivative Works of, publicly display,
    publicly perform, Distribute and sublicense the Contribution of such
    Contributor, if any, and such Derivative Works.
-   b\) Subject to the terms of this Agreement, each Contributor hereby
    grants Recipient a non-exclusive, worldwide, royalty-free patent license
    under Licensed Patents to make, use, sell, offer to sell, import and
    otherwise transfer the Contribution of such Contributor, if any, in
    Source Code or other form. This patent license shall apply to the
    combination of the Contribution and the Program if, at the time the
    Contribution is added by the Contributor, such addition of the
    Contribution causes such combination to be covered by the
    Licensed Patents. The patent license shall not apply to any other
    combinations which include the Contribution. No hardware per se is
    licensed hereunder.
-   c\) Recipient understands that although each Contributor grants the
    licenses to its Contributions set forth herein, no assurances are
    provided by any Contributor that the Program does not infringe the
    patent or other intellectual property rights of any other entity. Each
    Contributor disclaims any liability to Recipient for claims brought by
    any other entity based on infringement of intellectual property rights
    or otherwise. As a condition to exercising the rights and licenses
    granted hereunder, each Recipient hereby assumes sole responsibility to
    secure any other intellectual property rights needed, if any. For
    example, if a third party patent license is required to allow Recipient
    to Distribute the Program, it is Recipient's responsibility to acquire
    that license before distributing the Program.
-   d\) Each Contributor represents that to its knowledge it has sufficient
    copyright rights in its Contribution, if any, to grant the copyright
    license set forth in this Agreement.
-   e\) Notwithstanding the terms of any Secondary License, no Contributor
    makes additional grants to any Recipient (other than those set forth in
    this Agreement) as a result of such Recipient's receipt of the Program
    under the terms of a Secondary License (if permitted under the terms of
    Section 3).

## 3. REQUIREMENTS

3.1 If a Contributor Distributes the Program in any form, then:

-   a\) the Program must also be made available as Source Code, in accordance
    with section 3.2, and the Contributor must accompany the Program with a
    statement that the Source Code for the Program is available under this
    Agreement, and informs Recipients how to obtain it in a reasonable
    manner on or through a medium customarily used for software exchange;
    and
-   b\) the Contributor may Distribute the Program under a license different
    than this Agreement, provided that such license:
    -   i\) effectively disclaims on behalf of all other Contributors all
        warranties and conditions, express and implied, including warranties or
        conditions of title and non-infringement, and implied warranties or
        conditions of merchantability and fitness for a particular purpose;
    -   ii\) effectively excludes on behalf of all other Contributors all
        liability for damages, including direct, indirect, special, incidental
        and consequential damages, such as lost profits;
    -   iii\) does not attempt to limit or alter the recipients' rights in the
        Source Code under section 3.2; and
    -   iv\) requires any subsequent distribution of the Program by any party to
        be under a license that satisfies the requirements of this section 3.

3.2 When the Program is Distributed as Source Code:

-   a\) it must be made available under this Agreement, or if the Program (i)
    is combined with other material in a separate file or files made
    available under a Secondary License, and (ii) the initial Contributor
    attached to the Source Code the notice described in Exhibit A of this
    Agreement, then the Program may be made available under the terms of
    such Secondary Licenses, and
-   b\) a copy of this Agreement must be included with each copy of
    the Program.

3.3 Contributors may not remove or alter any copyright, patent,
trademark, attribution notices, disclaimers of warranty, or limitations
of liability (‘notices’) contained within the Program from any copy of
the Program which they Distribute, provided that Contributors may add
their own appropriate notices.

## 4. COMMERCIAL DISTRIBUTION

Commercial distributors of software may accept certain responsibilities
with respect to end users, business partners and the like. While this
license is intended to facilitate the commercial use of the Program, the
Contributor who includes the Program in a commercial product offering
should do so in a manner which does not create potential liability for
other Contributors. Therefore, if a Contributor includes the Program in
a commercial product offering, such Contributor (“Commercial
Contributor”) hereby agrees to defend and indemnify every other
Contributor (“Indemnified Contributor”) against any losses, damages and
costs (collectively “Losses”) arising from claims, lawsuits and other
legal actions brought by a third party against the Indemnified
Contributor to the extent caused by the acts or omissions of such
Commercial Contributor in connection with its distribution of the
Program in a commercial product offering. The obligations in this
section do not apply to any claims or Losses relating to any actual or
alleged intellectual property infringement. In order to qualify, an
Indemnified Contributor must: a) promptly notify the Commercial
Contributor in writing of such claim, and b) allow the Commercial
Contributor to control, and cooperate with the Commercial Contributor
in, the defense and any related settlement negotiations. The Indemnified
Contributor may participate in any such claim at its own expense.

For example, a Contributor might include the Program in a commercial
product offering, Product X. That Contributor is then a Commercial
Contributor. If that Commercial Contributor then makes performance
claims, or offers warranties related to Product X, those performance
claims and warranties are such Commercial Contributor's responsibility
alone. Under this section, the Commercial Contributor would have to
defend claims against the other Contributors related to those
performance claims and warranties, and if a court requires any other
Contributor to pay any damages as a result, the Commercial Contributor
must pay those damages.

## 5. NO WARRANTY {#warranty}

EXCEPT AS EXPRESSLY SET FORTH IN THIS AGREEMENT, AND TO THE EXTENT
PERMITTED BY APPLICABLE LAW, THE PROGRAM IS PROVIDED ON AN “AS IS”
BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, EITHER EXPRESS OR
IMPLIED INCLUDING, WITHOUT LIMITATION, ANY WARRANTIES OR CONDITIONS OF
TITLE, NON-INFRINGEMENT, MERCHANTABILITY OR FITNESS FOR A PARTICULAR
PURPOSE. Each Recipient is solely responsible for determining the
appropriateness of using and distributing the Program and assumes all
risks associated with its exercise of rights under this Agreement,
including but not limited to the risks and costs of program errors,
compliance with applicable laws, damage to or loss of data, programs or
equipment, and unavailability or interruption of operations.

## 6. DISCLAIMER OF LIABILITY {#disclaimer}

EXCEPT AS EXPRESSLY SET FORTH IN THIS AGREEMENT, AND TO THE EXTENT
PERMITTED BY APPLICABLE LAW, NEITHER RECIPIENT NOR ANY CONTRIBUTORS
SHALL HAVE ANY LIABILITY FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL,
EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING WITHOUT LIMITATION LOST
PROFITS), HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN
CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
ARISING IN ANY WAY OUT OF THE USE OR DISTRIBUTION OF THE PROGRAM OR THE
EXERCISE OF ANY RIGHTS GRANTED HEREUNDER, EVEN IF ADVISED OF THE
POSSIBILITY OF SUCH DAMAGES.

## 7. GENERAL

If any provision of this Agreement is invalid or unenforceable under
applicable law, it shall not affect the validity or enforceability of
the remainder of the terms of this Agreement, and without further action
by the parties hereto, such provision shall be reformed to the minimum
extent necessary to make such provision valid and enforceable.

If Recipient institutes patent litigation against any entity (including
a cross-claim or counterclaim in a lawsuit) alleging that the Program
itself (excluding combinations of the Program with other software or
hardware) infringes such Recipient's patent(s), then such Recipient's
rights granted under Section 2(b) shall terminate as of the date such
litigation is filed.

All Recipient's rights under this Agreement shall terminate if it fails
to comply with any of the material terms or conditions of this Agreement
and does not cure such failure in a reasonable period of time after
becoming aware of such noncompliance. If all Recipient's rights under
this Agreement terminate, Recipient agrees to cease use and distribution
of the Program as soon as reasonably practicable. However, Recipient's
obligations under this Agreement and any licenses granted by Recipient
relating to the Program shall continue and survive.

Everyone is permitted to copy and distribute copies of this Agreement,
but in order to avoid inconsistency the Agreement is copyrighted and may
only be modified in the following manner. The Agreement Steward reserves
the right to publish new versions (including revisions) of this
Agreement from time to time. No one other than the Agreement Steward has
the right to modify this Agreement. The Eclipse Foundation is the
initial Agreement Steward. The Eclipse Foundation may assign the
responsibility to serve as the Agreement Steward to a suitable separate
entity. Each new version of the Agreement will be given a distinguishing
version number. The Program (including Contributions) may always be
Distributed subject to the version of the Agreement under which it was
received. In addition, after a new version of the Agreement is
published, Contributor may elect to Distribute the Program (including
its Contributions) under the new version.

Except as expressly stated in Sections 2(a) and 2(b) above, Recipient
receives no rights or licenses to the intellectual property of any
Contributor under this Agreement, whether expressly, by implication,
estoppel or otherwise. All rights in the Program not expressly granted
under this Agreement are reserved. Nothing in this Agreement is intended
to be enforceable by any entity that is not a Contributor or Recipient.
No third-party beneficiary rights are created under this Agreement.

## Exhibit A – Form of Secondary Licenses Notice {#exhibit-a}

“This Source Code may also be made available under the following
Secondary Licenses when the conditions for such availability set forth
in the Eclipse Public License, v. 2.0 are satisfied: {name license(s),
version(s), and exceptions or additional permissions here}.”

> Simply including a copy of this Agreement, including this Exhibit A is
> not sufficient to license the Source Code under Secondary Licenses.
>
> If it is not possible or desirable to put the notice in a particular
> file, then You may include the notice in a location (such as a LICENSE
> file in a relevant directory) where a recipient would be likely to
> look for such a notice.
>
> You may add additional accurate notices of copyright ownership.
```

### Python-2.0

```text
A. HISTORY OF THE SOFTWARE
==========================

Python was created in the early 1990s by Guido van Rossum at Stichting
Mathematisch Centrum (CWI, see http://www.cwi.nl) in the Netherlands
as a successor of a language called ABC.  Guido remains Python's
principal author, although it includes many contributions from others.

In 1995, Guido continued his work on Python at the Corporation for
National Research Initiatives (CNRI, see http://www.cnri.reston.va.us)
in Reston, Virginia where he released several versions of the
software.

In May 2000, Guido and the Python core development team moved to
BeOpen.com to form the BeOpen PythonLabs team.  In October of the same
year, the PythonLabs team moved to Digital Creations, which became
Zope Corporation.  In 2001, the Python Software Foundation (PSF, see
https://www.python.org/psf/) was formed, a non-profit organization
created specifically to own Python-related Intellectual Property.
Zope Corporation was a sponsoring member of the PSF.

All Python releases are Open Source (see http://www.opensource.org for
the Open Source Definition).  Historically, most, but not all, Python
releases have also been GPL-compatible; the table below summarizes
the various releases.

    Release         Derived     Year        Owner       GPL-
                    from                                compatible? (1)

    0.9.0 thru 1.2              1991-1995   CWI         yes
    1.3 thru 1.5.2  1.2         1995-1999   CNRI        yes
    1.6             1.5.2       2000        CNRI        no
    2.0             1.6         2000        BeOpen.com  no
    1.6.1           1.6         2001        CNRI        yes (2)
    2.1             2.0+1.6.1   2001        PSF         no
    2.0.1           2.0+1.6.1   2001        PSF         yes
    2.1.1           2.1+2.0.1   2001        PSF         yes
    2.1.2           2.1.1       2002        PSF         yes
    2.1.3           2.1.2       2002        PSF         yes
    2.2 and above   2.1.1       2001-now    PSF         yes

Footnotes:

(1) GPL-compatible doesn't mean that we're distributing Python under
    the GPL.  All Python licenses, unlike the GPL, let you distribute
    a modified version without making your changes open source.  The
    GPL-compatible licenses make it possible to combine Python with
    other software that is released under the GPL; the others don't.

(2) According to Richard Stallman, 1.6.1 is not GPL-compatible,
    because its license has a choice of law clause.  According to
    CNRI, however, Stallman's lawyer has told CNRI's lawyer that 1.6.1
    is "not incompatible" with the GPL.

Thanks to the many outside volunteers who have worked under Guido's
direction to make these releases possible.


B. TERMS AND CONDITIONS FOR ACCESSING OR OTHERWISE USING PYTHON
===============================================================

PYTHON SOFTWARE FOUNDATION LICENSE VERSION 2
--------------------------------------------

1. This LICENSE AGREEMENT is between the Python Software Foundation
("PSF"), and the Individual or Organization ("Licensee") accessing and
otherwise using this software ("Python") in source or binary form and
its associated documentation.

2. Subject to the terms and conditions of this License Agreement, PSF hereby
grants Licensee a nonexclusive, royalty-free, world-wide license to reproduce,
analyze, test, perform and/or display publicly, prepare derivative works,
distribute, and otherwise use Python alone or in any derivative version,
provided, however, that PSF's License Agreement and PSF's notice of copyright,
i.e., "Copyright (c) 2001, 2002, 2003, 2004, 2005, 2006, 2007, 2008, 2009, 2010,
2011, 2012, 2013, 2014, 2015, 2016, 2017, 2018, 2019, 2020 Python Software Foundation;
All Rights Reserved" are retained in Python alone or in any derivative version
prepared by Licensee.

3. In the event Licensee prepares a derivative work that is based on
or incorporates Python or any part thereof, and wants to make
the derivative work available to others as provided herein, then
Licensee hereby agrees to include in any such work a brief summary of
the changes made to Python.

4. PSF is making Python available to Licensee on an "AS IS"
basis.  PSF MAKES NO REPRESENTATIONS OR WARRANTIES, EXPRESS OR
IMPLIED.  BY WAY OF EXAMPLE, BUT NOT LIMITATION, PSF MAKES NO AND
DISCLAIMS ANY REPRESENTATION OR WARRANTY OF MERCHANTABILITY OR FITNESS
FOR ANY PARTICULAR PURPOSE OR THAT THE USE OF PYTHON WILL NOT
INFRINGE ANY THIRD PARTY RIGHTS.

5. PSF SHALL NOT BE LIABLE TO LICENSEE OR ANY OTHER USERS OF PYTHON
FOR ANY INCIDENTAL, SPECIAL, OR CONSEQUENTIAL DAMAGES OR LOSS AS
A RESULT OF MODIFYING, DISTRIBUTING, OR OTHERWISE USING PYTHON,
OR ANY DERIVATIVE THEREOF, EVEN IF ADVISED OF THE POSSIBILITY THEREOF.

6. This License Agreement will automatically terminate upon a material
breach of its terms and conditions.

7. Nothing in this License Agreement shall be deemed to create any
relationship of agency, partnership, or joint venture between PSF and
Licensee.  This License Agreement does not grant permission to use PSF
trademarks or trade name in a trademark sense to endorse or promote
products or services of Licensee, or any third party.

8. By copying, installing or otherwise using Python, Licensee
agrees to be bound by the terms and conditions of this License
Agreement.


BEOPEN.COM LICENSE AGREEMENT FOR PYTHON 2.0
-------------------------------------------

BEOPEN PYTHON OPEN SOURCE LICENSE AGREEMENT VERSION 1

1. This LICENSE AGREEMENT is between BeOpen.com ("BeOpen"), having an
office at 160 Saratoga Avenue, Santa Clara, CA 95051, and the
Individual or Organization ("Licensee") accessing and otherwise using
this software in source or binary form and its associated
documentation ("the Software").

2. Subject to the terms and conditions of this BeOpen Python License
Agreement, BeOpen hereby grants Licensee a non-exclusive,
royalty-free, world-wide license to reproduce, analyze, test, perform
and/or display publicly, prepare derivative works, distribute, and
otherwise use the Software alone or in any derivative version,
provided, however, that the BeOpen Python License is retained in the
Software, alone or in any derivative version prepared by Licensee.

3. BeOpen is making the Software available to Licensee on an "AS IS"
basis.  BEOPEN MAKES NO REPRESENTATIONS OR WARRANTIES, EXPRESS OR
IMPLIED.  BY WAY OF EXAMPLE, BUT NOT LIMITATION, BEOPEN MAKES NO AND
DISCLAIMS ANY REPRESENTATION OR WARRANTY OF MERCHANTABILITY OR FITNESS
FOR ANY PARTICULAR PURPOSE OR THAT THE USE OF THE SOFTWARE WILL NOT
INFRINGE ANY THIRD PARTY RIGHTS.

4. BEOPEN SHALL NOT BE LIABLE TO LICENSEE OR ANY OTHER USERS OF THE
SOFTWARE FOR ANY INCIDENTAL, SPECIAL, OR CONSEQUENTIAL DAMAGES OR LOSS
AS A RESULT OF USING, MODIFYING OR DISTRIBUTING THE SOFTWARE, OR ANY
DERIVATIVE THEREOF, EVEN IF ADVISED OF THE POSSIBILITY THEREOF.

5. This License Agreement will automatically terminate upon a material
breach of its terms and conditions.

6. This License Agreement shall be governed by and interpreted in all
respects by the law of the State of California, excluding conflict of
law provisions.  Nothing in this License Agreement shall be deemed to
create any relationship of agency, partnership, or joint venture
between BeOpen and Licensee.  This License Agreement does not grant
permission to use BeOpen trademarks or trade names in a trademark
sense to endorse or promote products or services of Licensee, or any
third party.  As an exception, the "BeOpen Python" logos available at
http://www.pythonlabs.com/logos.html may be used according to the
permissions granted on that web page.

7. By copying, installing or otherwise using the software, Licensee
agrees to be bound by the terms and conditions of this License
Agreement.


CNRI LICENSE AGREEMENT FOR PYTHON 1.6.1
---------------------------------------

1. This LICENSE AGREEMENT is between the Corporation for National
Research Initiatives, having an office at 1895 Preston White Drive,
Reston, VA 20191 ("CNRI"), and the Individual or Organization
("Licensee") accessing and otherwise using Python 1.6.1 software in
source or binary form and its associated documentation.

2. Subject to the terms and conditions of this License Agreement, CNRI
hereby grants Licensee a nonexclusive, royalty-free, world-wide
license to reproduce, analyze, test, perform and/or display publicly,
prepare derivative works, distribute, and otherwise use Python 1.6.1
alone or in any derivative version, provided, however, that CNRI's
License Agreement and CNRI's notice of copyright, i.e., "Copyright (c)
1995-2001 Corporation for National Research Initiatives; All Rights
Reserved" are retained in Python 1.6.1 alone or in any derivative
version prepared by Licensee.  Alternately, in lieu of CNRI's License
Agreement, Licensee may substitute the following text (omitting the
quotes): "Python 1.6.1 is made available subject to the terms and
conditions in CNRI's License Agreement.  This Agreement together with
Python 1.6.1 may be located on the Internet using the following
unique, persistent identifier (known as a handle): 1895.22/1013.  This
Agreement may also be obtained from a proxy server on the Internet
using the following URL: http://hdl.handle.net/1895.22/1013".

3. In the event Licensee prepares a derivative work that is based on
or incorporates Python 1.6.1 or any part thereof, and wants to make
the derivative work available to others as provided herein, then
Licensee hereby agrees to include in any such work a brief summary of
the changes made to Python 1.6.1.

4. CNRI is making Python 1.6.1 available to Licensee on an "AS IS"
basis.  CNRI MAKES NO REPRESENTATIONS OR WARRANTIES, EXPRESS OR
IMPLIED.  BY WAY OF EXAMPLE, BUT NOT LIMITATION, CNRI MAKES NO AND
DISCLAIMS ANY REPRESENTATION OR WARRANTY OF MERCHANTABILITY OR FITNESS
FOR ANY PARTICULAR PURPOSE OR THAT THE USE OF PYTHON 1.6.1 WILL NOT
INFRINGE ANY THIRD PARTY RIGHTS.

5. CNRI SHALL NOT BE LIABLE TO LICENSEE OR ANY OTHER USERS OF PYTHON
1.6.1 FOR ANY INCIDENTAL, SPECIAL, OR CONSEQUENTIAL DAMAGES OR LOSS AS
A RESULT OF MODIFYING, DISTRIBUTING, OR OTHERWISE USING PYTHON 1.6.1,
OR ANY DERIVATIVE THEREOF, EVEN IF ADVISED OF THE POSSIBILITY THEREOF.

6. This License Agreement will automatically terminate upon a material
breach of its terms and conditions.

7. This License Agreement shall be governed by the federal
intellectual property law of the United States, including without
limitation the federal copyright law, and, to the extent such
U.S. federal law does not apply, by the law of the Commonwealth of
Virginia, excluding Virginia's conflict of law provisions.
Notwithstanding the foregoing, with regard to derivative works based
on Python 1.6.1 that incorporate non-separable material that was
previously distributed under the GNU General Public License (GPL), the
law of the Commonwealth of Virginia shall govern this License
Agreement only as to issues arising under or with respect to
Paragraphs 4, 5, and 7 of this License Agreement.  Nothing in this
License Agreement shall be deemed to create any relationship of
agency, partnership, or joint venture between CNRI and Licensee.  This
License Agreement does not grant permission to use CNRI trademarks or
trade name in a trademark sense to endorse or promote products or
services of Licensee, or any third party.

8. By clicking on the "ACCEPT" button where indicated, or by copying,
installing or otherwise using Python 1.6.1, Licensee agrees to be
bound by the terms and conditions of this License Agreement.

        ACCEPT


CWI LICENSE AGREEMENT FOR PYTHON 0.9.0 THROUGH 1.2
--------------------------------------------------

Copyright (c) 1991 - 1995, Stichting Mathematisch Centrum Amsterdam,
The Netherlands.  All rights reserved.

Permission to use, copy, modify, and distribute this software and its
documentation for any purpose and without fee is hereby granted,
provided that the above copyright notice appear in all copies and that
both that copyright notice and this permission notice appear in
supporting documentation, and that the name of Stichting Mathematisch
Centrum or CWI not be used in advertising or publicity pertaining to
distribution of the software without specific, written prior
permission.

STICHTING MATHEMATISCH CENTRUM DISCLAIMS ALL WARRANTIES WITH REGARD TO
THIS SOFTWARE, INCLUDING ALL IMPLIED WARRANTIES OF MERCHANTABILITY AND
FITNESS, IN NO EVENT SHALL STICHTING MATHEMATISCH CENTRUM BE LIABLE
FOR ANY SPECIAL, INDIRECT OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT
OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
```

### Unlicense

```text
This is free and unencumbered software released into the public domain.

Anyone is free to copy, modify, publish, use, compile, sell, or
distribute this software, either in source code form or as a compiled
binary, for any purpose, commercial or non-commercial, and by any
means.

In jurisdictions that recognize copyright laws, the author or authors
of this software dedicate any and all copyright interest in the
software to the public domain. We make this dedication for the benefit
of the public at large and to the detriment of our heirs and
successors. We intend this dedication to be an overt act of
relinquishment in perpetuity of all present and future rights to this
software under copyright law.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
IN NO EVENT SHALL THE AUTHORS BE LIABLE FOR ANY CLAIM, DAMAGES OR
OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
OTHER DEALINGS IN THE SOFTWARE.

For more information, please refer to <http://unlicense.org>
```

## 7. 维护方式

依赖变更后按以下口径重新核对本文件：

```bash
# 自动门禁：Go 运行时依赖 + Web 生产依赖
scripts/check.sh license

# 手动核对辅助：列出链接进 zki 的模块
go list -deps -f '{{with .Module}}{{.Path}} {{.Version}}{{end}}' ./cmd/zki | sort -u

# 手动核对辅助：列出 Web 生产依赖与其许可证
cd web && pnpm licenses list --prod --json
```

门禁拒绝的许可证不会自动放行：必须人工核对后，在 `internal/lint/licensecheck` 中显式登记允许项或已核对例外，并同步本文件。
