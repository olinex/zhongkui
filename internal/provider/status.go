package provider

import (
	// 标准库
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"strconv"
	"time"

	// 第三方
	"github.com/minio/minio-go/v7"
	"github.com/samber/do/v2"
	"github.com/spf13/viper"

	// 项目内部

	"zhongkuisec.com/zhongkui/internal/ai"
	aimodel "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/base"
	"zhongkuisec.com/zhongkui/internal/pkg/crypto/key"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/etcd"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
	"zhongkuisec.com/zhongkui/internal/pkg/mail"
	"zhongkuisec.com/zhongkui/internal/pkg/redis"
	"zhongkuisec.com/zhongkui/internal/pkg/s3"
	"zhongkuisec.com/zhongkui/internal/pkg/status"
	"zhongkuisec.com/zhongkui/internal/pkg/validator"
)

// 各探测项依赖类别，作为 Spec.Category 的取值。
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
	reg.Register(status.Spec{Name: "llm-chat", Category: dependencyCategoryLLM, Probe: probeModel(viper, dbLogger, aimodel.SlotStandard)})
	reg.Register(status.Spec{Name: "llm-embedding", Category: dependencyCategoryLLM, Probe: probeModel(viper, dbLogger, aimodel.SlotEmbedding)})
	return reg, nil
}

// probeDatabase 依配置构造 GORM DB，并对底层 *sql.DB 执行 PingContext 探测，
// 结束后关闭连接（避免泄漏）。
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
func probeEtcd(v *viper.Viper, logger *logging.Logger) status.Probe {
	return func(ctx context.Context) status.Status {
		if cfg, err := etcd.ExtractConfig(v, validator.Validate); err != nil {
			return status.Status{Configured: true, Reachable: false, Err: "extract etcd config: " + err.Error()}
		} else if client, err := etcd.NewEtcdClient(ctx, cfg, logger); err != nil {
			return status.Status{Configured: true, Reachable: false, Err: "construct etcd client: " + err.Error()}
		} else {
			defer client.Close()
			// Get 命中不存在的键仍返回成功，仅用于验证连接与鉴权往返。
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

// probeModel 依配置自建 core 库连接，从模型注册表（llm_models + llm_bindings）读取指定
// 档位的模型端点配置并对该端点做一次轻量探活（见 ai.TestModel：chat 走 GET
// /models，embedding 走一次最小向量化请求）。档位无可用绑定（含未启用/类型不符）→
// Configured=false；配置读取或探活失败 → Configured=true、Reachable=false（英文原因）。
// 探测结束后关闭自建连接，避免连接泄漏。
func probeModel(v *viper.Viper, logger *logging.Logger, slot aimodel.Slot) status.Probe {
	return func(ctx context.Context) status.Status {
		if cfg, err := database.ExtractConfig(v, validator.Validate, base.CoreDbName); err != nil {
			return status.Status{Configured: false}
		} else if db, err := database.NewDatabase(cfg, logger); err != nil {
			return status.Status{Configured: true, Reachable: false, Err: "construct database: " + err.Error()}
		} else if sqlDB, err := db.DB(); err != nil {
			return status.Status{Configured: true, Reachable: false, Err: "resolve database connection: " + err.Error()}
		} else {
			defer sqlDB.Close()
			if store, err := ai.NewStore(db); err != nil {
				return status.Status{Configured: true, Reachable: false, Err: "construct config store: " + err.Error()}
			} else if model, err := store.ModelConfig(ctx, slot); errors.Is(err, ai.ErrSlotNotBound) {
				return status.Status{Configured: false}
			} else if err != nil {
				return status.Status{Configured: true, Reachable: false, Err: "load model config: " + err.Error()}
			} else if err := ai.TestModel(ctx, model); err != nil {
				return status.Status{Configured: true, Reachable: false, Err: err.Error()}
			}
			return status.Status{Configured: true, Reachable: true}
		}
	}
}
