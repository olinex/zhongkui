package provider

import (
	// stdlib
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"strconv"
	"time"

	// third-party
	"github.com/cloudwego/eino/schema"
	"github.com/minio/minio-go/v7"
	"github.com/samber/do/v2"
	"github.com/spf13/viper"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/crypto/key"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/etcd"
	"zhongkui.com/zhongkui/internal/pkg/llm"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/mail"
	"zhongkui.com/zhongkui/internal/pkg/redis"
	"zhongkui.com/zhongkui/internal/pkg/s3"
	"zhongkui.com/zhongkui/internal/pkg/status"
	"zhongkui.com/zhongkui/internal/pkg/validator"
)

// 各探测项依赖类别，作为 Spec.Category 的取值。
// Dependency categories used as the Spec.Category value of each probe.
const (
	dependencyCategoryDatabase = "database"
	dependencyCategoryRedis    = "redis"
	dependencyCategoryEtcd     = "etcd"
	dependencyCategoryS3       = "s3"
	dependencyCategoryMail     = "mail"
	dependencyCategoryLLM      = "llm"
)

// NewDependencyRegistry 构造核心基础设施探测注册表。
// 探测项全部「独立按配置构造客户端并显式探测连通性」，不依赖全局已建连接；
// CLI（旁路轻量注入器）与 Web（完整注入器）共用同一函数与同一批探测。
//
// 处理流程：
//   - 从注入器 Invoke viper / key-derivation / database / etcd logger 等共享依赖；
//   - 逐项注册 status.Spec（Name/Category 在此固定，探测实现只填 Configured/Reachable/Err）；
//   - 返回 *status.Registry，调用方决定何时 Run / Render。
//
// 探测语义约定（实现内不抛错误，一律经 Status 表达）：
//   - 构造失败（配置提取/客户端构建错误）→ Configured=true、Reachable=false、Err=英文原因；
//   - 连通探测成功 → Reachable=true、Err="";
//   - 配置缺失/未启用 → Configured=false。
//
// 探测自建客户端（db/redis/etcd/grpc）用完后显式 Close，避免连接/协程泄漏。
//
// NewDependencyRegistry builds the core-infrastructure probe registry. Every probe
// constructs its own client from config and probes connectivity explicitly, depending on
// no pre-existing global connection; the CLI (lightweight side injector) and the Web (full
// injector) share this same function and probe set.
//
// It Invokes shared deps (viper / key-derivation / database / etcd logger) from the
// injector, registers each status.Spec with a fixed Name/Category (probes only fill
// Configured/Reachable/Err), and returns the *status.Registry for the caller to Run/Render.
// Probe semantics (a probe never returns an error; it always reports via Status):
// construction failure (config extraction / client build) → Configured=true, Reachable=false
// with an English Err; successful reachability → Reachable=true, Err=""; missing or disabled
// config → Configured=false. Self-built clients (db/redis/etcd/grpc) are explicitly closed.
func NewDependencyRegistry(ctx context.Context, i do.Injector) (*status.Registry, error) {
	viper, err := InvokeViper(i)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	kd, err := InvokeKeyDerivation(i)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	dbLogger, err := InvokeDatabaseLogger(i)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	etcdLogger, err := InvokeEtcdLogger(i)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	reg := status.New()
	reg.Register(status.Spec{Name: "core-database", Category: dependencyCategoryDatabase, Probe: probeDatabase(viper, dbLogger, base.CoreDbName)})
	reg.Register(status.Spec{Name: "raw-database", Category: dependencyCategoryDatabase, Probe: probeDatabase(viper, dbLogger, base.RawDbName)})
	reg.Register(status.Spec{Name: "cache-redis", Category: dependencyCategoryRedis, Probe: probeRedis(viper, base.CacheRedisName)})
	reg.Register(status.Spec{Name: "pipeline-redis", Category: dependencyCategoryRedis, Probe: probeRedis(viper, base.PipelineRedisName)})
	reg.Register(status.Spec{Name: "etcd", Category: dependencyCategoryEtcd, Probe: probeEtcd(viper, etcdLogger)})
	reg.Register(status.Spec{Name: "s3", Category: dependencyCategoryS3, Probe: probeS3(viper, kd)})
	reg.Register(status.Spec{Name: "smtp", Category: dependencyCategoryMail, Probe: probeSMTP(viper)})
	reg.Register(status.Spec{Name: "llm-chat", Category: dependencyCategoryLLM, Probe: probeLLMChat(viper)})
	reg.Register(status.Spec{Name: "llm-embedding", Category: dependencyCategoryLLM, Probe: probeLLMEmbedding(viper)})
	return reg, nil
}

// probeDatabase 依配置构造 GORM DB，并对底层 *sql.DB 执行 PingContext 探测，
// 结束后关闭连接（避免泄漏）。
// probeDatabase builds a GORM DB from config and pings the underlying *sql.DB via
// PingContext, closing the connection afterwards to avoid leaks.
func probeDatabase(v *viper.Viper, logger *logging.Logger, name string) status.Probe {
	return func(ctx context.Context) status.Status {
		if cfg, err := database.ExtractConfig(v, validator.Validate, name); err != nil {
			return status.Status{Configured: true, Reachable: false, Err: "extract database config: " + err.Error()}
		} else if db, err := database.NewDatabase(cfg, logger); err != nil {
			return status.Status{Configured: true, Reachable: false, Err: "construct database: " + err.Error()}
		} else if sqlDB, err := db.DB(); err != nil {
			return status.Status{Configured: true, Reachable: false, Err: "resolve database connection: " + err.Error()}
		} else {
			defer sqlDB.Close()
			if err := sqlDB.PingContext(ctx); err != nil {
				return status.Status{Configured: true, Reachable: false, Err: "ping database: " + err.Error()}
			}
			return status.Status{Configured: true, Reachable: true}
		}
	}
}

// probeRedis 依配置构造 redis 客户端并 Ping，结束后关闭客户端（避免泄漏）。
// probeRedis builds a redis client from config and pings it, closing the client
// afterwards to avoid leaks.
func probeRedis(v *viper.Viper, name string) status.Probe {
	return func(ctx context.Context) status.Status {
		if cfg, err := redis.ExtractConfig(v, validator.Validate, name); err != nil {
			return status.Status{Configured: true, Reachable: false, Err: "extract redis config: " + err.Error()}
		} else {
			client := redis.NewClient(cfg)
			defer client.Shutdown()
			if err := client.Ping(ctx).Err(); err != nil {
				return status.Status{Configured: true, Reachable: false, Err: "ping redis: " + err.Error()}
			}
			return status.Status{Configured: true, Reachable: true}
		}
	}
}

// probeEtcd 依配置构造 etcd 客户端，做一次短超时读（Get）判定连通，结束后关闭客户端。
// probeEtcd builds an etcd client from config and performs a short-timeout Get to
// determine reachability, closing the client afterwards.
func probeEtcd(v *viper.Viper, logger *logging.Logger) status.Probe {
	return func(ctx context.Context) status.Status {
		if cfg, err := etcd.ExtractConfig(v, validator.Validate); err != nil {
			return status.Status{Configured: true, Reachable: false, Err: "extract etcd config: " + err.Error()}
		} else if client, err := etcd.NewEtcdClient(ctx, cfg, logger); err != nil {
			return status.Status{Configured: true, Reachable: false, Err: "construct etcd client: " + err.Error()}
		} else {
			defer client.Close()
			// Get 命中不存在的键仍返回成功，仅用于验证连接与鉴权往返。
			// A Get on a missing key still succeeds; it only exercises the round trip.
			if _, err := client.Get(ctx, etcd.WithPrefix("status/")); err != nil {
				return status.Status{Configured: true, Reachable: false, Err: "etcd get: " + err.Error()}
			}
			return status.Status{Configured: true, Reachable: true}
		}
	}
}

// probeS3 依配置构造 S3 客户端（注入可关闭的自建 transport），做一次查桶（StatObject）
// 判定连通。S3Client 本身无 Close 方法（minio 走 HTTP），故探测后关闭自建 transport 的
// 空闲连接以释放资源。
// probeS3 builds an S3 client (with its own closable transport) from config and performs a
// bucket StatObject to determine reachability. S3Client has no Close method (minio is HTTP),
// so the probe closes the self-built transport's idle connections afterwards.
func probeS3(v *viper.Viper, kd *key.KeyDerivation) status.Probe {
	return func(ctx context.Context) status.Status {
		if cfg, err := s3.ExtractConfig(v, validator.Validate); err != nil {
			return status.Status{Configured: true, Reachable: false, Err: "extract s3 config: " + err.Error()}
		} else {
			transport := &http.Transport{}
			if client, err := s3.NewS3Client(cfg, kd, s3.WithTransport(transport)); err != nil {
				transport.CloseIdleConnections()
				return status.Status{Configured: true, Reachable: false, Err: "construct s3 client: " + err.Error()}
			} else {
				defer transport.CloseIdleConnections()
				if _, err := client.StatObject(ctx, "zk-status-probe", minio.StatObjectOptions{}); err != nil {
					// 对象不存在/桶不存在等 S3 语义错误同样证明服务已响应，视为可达。
					// S3 semantic errors (NoSuchKey/NoSuchBucket) also prove the service
					// responded, so they count as reachable.
					if s3.IsNotExists(err) {
						return status.Status{Configured: true, Reachable: true}
					}
					return status.Status{Configured: true, Reachable: false, Err: "stat object: " + err.Error()}
				}
				return status.Status{Configured: true, Reachable: true}
			}
		}
	}
}

// probeSMTP 依 [smtp] 配置做 host:port 的 TCP/TLS(SMTPS) 连接握手。仅当 config.enabled
// 为 true 才视为已配置；465 端口（隐式 TLS）用 tls.Dial，其余（587/25 STARTTLS）用
// 普通 TCP 拨号。
// probeSMTP performs a TCP/TLS(SMTPS) handshake to host:port per the [smtp] config. It is
// "configured" only when config.enabled is true; port 465 (implicit TLS) uses tls.Dial,
// while other ports (587/25 STARTTLS) use a plain TCP dial.
func probeSMTP(v *viper.Viper) status.Probe {
	return func(ctx context.Context) status.Status {
		if cfg, err := mail.ExtractConfig(v, validator.Validate); err != nil {
			return status.Status{Configured: false}
		} else if !cfg.Enabled {
			return status.Status{Configured: false}
		} else if cfg.Host == "" {
			return status.Status{Configured: true, Reachable: false, Err: "smtp: host not configured"}
		} else {
			addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
			if cfg.Port == 465 {
				dialer := &net.Dialer{Timeout: 5 * time.Second}
				if conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
					return status.Status{Configured: true, Reachable: false, Err: "smtp tls dial: " + err.Error()}
				} else {
					conn.Close()
					return status.Status{Configured: true, Reachable: true}
				}
			}
			dialer := net.Dialer{Timeout: 5 * time.Second}
			if conn, err := dialer.DialContext(ctx, "tcp", addr); err != nil {
				return status.Status{Configured: true, Reachable: false, Err: "smtp dial: " + err.Error()}
			} else {
				conn.Close()
				return status.Status{Configured: true, Reachable: true}
			}
		}
	}
}

// probeLLMChat 对 chat 端点做一次极小真实请求判定可达。仅当 chat 配置（baseURL/apiKey/
// model）完全填写才视为已配置，且不依赖 [llm].enabled 开关（与 llm.NewChatClient 口径一致）。
// probeLLMChat performs a tiny real request to the chat endpoint to determine reachability.
// It is "configured" only when the chat config (baseURL/apiKey/model) is fully filled and,
// following llm.NewChatClient, does not depend on the [llm].enabled switch.
func probeLLMChat(v *viper.Viper) status.Probe {
	return func(ctx context.Context) status.Status {
		if cfg, err := llm.ExtractConfigOrEmpty(v, validator.Validate); err != nil {
			return status.Status{Configured: false}
		} else if cfg.Chat.BaseURL == "" || cfg.Chat.APIKey == "" || cfg.Chat.Model == "" {
			return status.Status{Configured: false}
		} else if chat, err := llm.NewChatClient(ctx, cfg.Chat); err != nil {
			return status.Status{Configured: true, Reachable: false, Err: "construct chat client: " + err.Error()}
		} else if _, err := chat.Generate(ctx, []*schema.Message{{
			// 客户端构造时固定注入 response_format=json_object，DashScope 要求
			// 消息中出现 "json" 字样，否则返回 400；探活消息需包含该词。
			Role: schema.User, Content: "ping: reply with a valid JSON object.",
		}}); err != nil {
			return status.Status{Configured: true, Reachable: false, Err: "chat request: " + err.Error()}
		}
		return status.Status{Configured: true, Reachable: true}
	}
}

// probeLLMEmbedding 对 embedding 端点做一次极小真实请求判定可达。仅当 embedding 配置
// （baseURL/apiKey/model）完全填写才视为已配置，且不依赖 [llm].enabled 开关（与
// llm.NewEmbedClient 口径一致）。
// probeLLMEmbedding performs a tiny real request to the embedding endpoint to determine
// reachability. It is "configured" only when the embedding config (baseURL/apiKey/model) is
// fully filled and, following llm.NewEmbedClient, does not depend on the [llm].enabled switch.
func probeLLMEmbedding(v *viper.Viper) status.Probe {
	return func(ctx context.Context) status.Status {
		if cfg, err := llm.ExtractConfigOrEmpty(v, validator.Validate); err != nil {
			return status.Status{Configured: false}
		} else if cfg.Embedding.BaseURL == "" || cfg.Embedding.APIKey == "" || cfg.Embedding.Model == "" {
			return status.Status{Configured: false}
		} else if emb, err := llm.NewEmbedClient(ctx, cfg.Embedding); err != nil {
			return status.Status{Configured: true, Reachable: false, Err: "construct embedding client: " + err.Error()}
		} else if _, err := emb.EmbedStrings(ctx, []string{"ping"}); err != nil {
			return status.Status{Configured: true, Reachable: false, Err: "embedding request: " + err.Error()}
		}
		return status.Status{Configured: true, Reachable: true}
	}
}
