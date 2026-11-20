package cmd

import (
	"os"
	"path/filepath"
	"time"

	"github.com/creasty/defaults"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"

	"zhongkuisec.com/zhongkui/internal/capture"
	managerconfig "zhongkuisec.com/zhongkui/internal/manager/config"
	pkgconfig "zhongkuisec.com/zhongkui/internal/pkg/config"
	cryptokey "zhongkuisec.com/zhongkui/internal/pkg/crypto/key"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/etcd"
	"zhongkuisec.com/zhongkui/internal/pkg/grpc"
	"zhongkuisec.com/zhongkui/internal/pkg/health"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
	"zhongkuisec.com/zhongkui/internal/pkg/mail"
	"zhongkuisec.com/zhongkui/internal/pkg/pipeline"
	reporter_v1 "zhongkuisec.com/zhongkui/internal/pkg/protobuf/reporter/v1"
	"zhongkuisec.com/zhongkui/internal/pkg/redis"
	"zhongkuisec.com/zhongkui/internal/pkg/s3"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
	"zhongkuisec.com/zhongkui/internal/pkg/translate"
	"zhongkuisec.com/zhongkui/internal/tagengine"
)

// defaultConfigHeader 是生成配置文件顶部的说明注释。
const defaultConfigHeader = `# zhongkui 默认配置文件
# 本文件由 ` + "`zki create default-config`" + ` 依据配置结构体自动生成，字段与注释随代码结构同步，
# 标记为 <...> 的占位值请替换为实际值后再启动服务。
`

// defaultConfig 是生成默认配置文件所用的根结构体。
// 各字段类型直接引用运行时真实配置结构体，确保生成的配置项、层级与注释不会随代码演进漂移。
type defaultConfig struct {
	Crypto    cryptokey.Config     `mapstructure:"crypto" comment:"密钥派生与密码本配置"`
	API       managerconfig.Config `mapstructure:"api" comment:"API 管理层配置"`
	Capture   capture.Config       `mapstructure:"capture" comment:"抓包配置"`
	Pipeline  pipeline.Config      `mapstructure:"pipeline" comment:"任务队列与流水线配置"`
	Zap       namedLoggingConfig   `mapstructure:"zap" comment:"日志配置"`
	Database  namedDatabaseConfig  `mapstructure:"database" comment:"数据库配置"`
	Redis     namedRedisConfig     `mapstructure:"redis" comment:"Redis 配置"`
	Etcd      etcd.Config          `mapstructure:"etcd" comment:"etcd 配置"`
	Grpc      namedGrpcConfig      `mapstructure:"grpc" comment:"gRPC 服务端与客户端配置"`
	S3        s3.Config            `mapstructure:"s3" comment:"对象存储配置"`
	SMTP      mail.Config          `mapstructure:"smtp" comment:"系统层邮件直发配置"`
	Health    health.Config        `mapstructure:"health" comment:"健康探针配置"`
	TagEngine tagengine.Config     `mapstructure:"tagengine" comment:"标签识别引擎（Rego）配置"`
}

// namedLoggingConfig 对应按名称区分的多套日志配置（zap.<name>）。
type namedLoggingConfig struct {
	Default  logging.Config `mapstructure:"default"`
	Capture  logging.Config `mapstructure:"capture"`
	API      logging.Config `mapstructure:"api"`
	Database logging.Config `mapstructure:"database"`
	Etcd     logging.Config `mapstructure:"etcd"`
	Grpc     logging.Config `mapstructure:"grpc"`
	Pipeline logging.Config `mapstructure:"pipeline"`
}

// namedDatabaseConfig 对应按用途区分的数据库配置（database.<name>）。
type namedDatabaseConfig struct {
	Core database.Config `mapstructure:"core"`
	Raw  database.Config `mapstructure:"raw"`
}

// namedRedisConfig 对应按用途区分的 Redis 配置（redis.<name>）。
type namedRedisConfig struct {
	Cache    redis.Config `mapstructure:"cache"`
	Pipeline redis.Config `mapstructure:"pipeline"`
}

// namedGrpcConfig 对应 gRPC 服务端与客户端配置（grpc.server/client）。
type namedGrpcConfig struct {
	Server namedGrpcServerConfig `mapstructure:"server"`
	Client namedGrpcClientConfig `mapstructure:"client"`
}

type namedGrpcServerConfig struct {
	Reporter grpc.ServerConfig `mapstructure:"reporter"`
}

type namedGrpcClientConfig struct {
	Reporter grpc.ClientConfig `mapstructure:"reporter"`
}

// newDefaultConfig 构造默认配置实例。
//
// 先由结构体 default 标签补齐全部默认值，再覆盖少量常用值：覆盖必须放在默认值之后，
// 才能显式表达 false、空列表等与默认值相反的取值（defaults.Set 会把零值视为未设置）。
func newDefaultConfig() *defaultConfig {
	cfg := &defaultConfig{}
	_ = defaults.Set(cfg)

	// crypto
	cfg.Crypto.Source = "config"
	cfg.Crypto.ParentSecretKey = "<your-32-byte-secret-key>"
	cfg.Crypto.PasswordBookPath = "certs/password-book.bin"

	// api
	cfg.API.FrontendBaseURL = "http://127.0.0.1:8000"
	cfg.API.InternalBaseURL = "http://127.0.0.1:8080"
	cfg.API.Auth.Cookie.Domain = "127.0.0.1"
	cfg.API.Secure.CORS.AllowOrigins = []string{"http://127.0.0.1:8000", "http://localhost:8000"}

	// capture
	cfg.Capture.Resource = reporter_v1.ResourceRaw{Namespace: "default", Name: "test", Version: "v1d"}
	cfg.Capture.Watch.TolerateRatio = 0.5
	cfg.Capture.Feedback.TolerateRatio = 0.5

	// pipeline
	cfg.Pipeline.Scheduler.Location = serializer.Location("Asia/Shanghai")
	cfg.Pipeline.Scheduler.ForceFlush = true
	cfg.Pipeline.Scheduler.Tasks = defaultSchedulerTasks()
	cfg.Pipeline.Custom.Processors = map[string]map[string]any{
		"ipv4-tcp": tcpProcessorDefaults(),
		"ipv6-tcp": tcpProcessorDefaults(),
	}

	// zap：开发环境统一使用 console 编码，便于本地阅读
	for _, loggingCfg := range []*logging.Config{
		&cfg.Zap.Default, &cfg.Zap.Capture, &cfg.Zap.API, &cfg.Zap.Database,
		&cfg.Zap.Etcd, &cfg.Zap.Grpc, &cfg.Zap.Pipeline,
	} {
		loggingCfg.Encoding = "console"
	}

	// database
	cfg.Database.Core.Driver = "postgres"
	cfg.Database.Core.DSN = "host=localhost port=5432 user=postgres password=<db-password> dbname=postgres sslmode=disable TimeZone=Asia/Shanghai"
	cfg.Database.Raw.Driver = "clickhouse"
	cfg.Database.Raw.DSN = "clickhouse://default:<raw-db-password>@127.0.0.1:9000/default"

	// redis：缓存与流水线分库
	cfg.Redis.Pipeline.DB = 1

	// etcd
	cfg.Etcd.Endpoints = []string{"127.0.0.1:2379"}

	// grpc
	cfg.Grpc.Server.Reporter.Host = "0.0.0.0"
	cfg.Grpc.Server.Reporter.TLS.Enabled = true
	cfg.Grpc.Server.Reporter.TLS.KeyFilePath = "./certs/test-key.pem"
	cfg.Grpc.Server.Reporter.TLS.CertFilePath = "./certs/test-cert.pem"
	cfg.Grpc.Server.Reporter.Auth.AccessKeyEnabled = true

	cfg.Grpc.Client.Reporter.TLS.Enabled = true
	cfg.Grpc.Client.Reporter.TLS.Domain = "zhongkui-security.com"
	cfg.Grpc.Client.Reporter.TLS.CertFilePath = "./certs/test-cert.pem"
	cfg.Grpc.Client.Reporter.Auth.Enabled = true
	cfg.Grpc.Client.Reporter.Auth.Authorize = "accesskeySecret"
	cfg.Grpc.Client.Reporter.Auth.AccessKeyID = "<access-key-id>"
	cfg.Grpc.Client.Reporter.Auth.AccessKeySecret = "<access-key-secret>"

	// s3
	cfg.S3.Endpoint = "127.0.0.1:9002"
	cfg.S3.AccessKey = "<s3-access-key>"
	cfg.S3.SecretKey = "<s3-secret-key>"
	cfg.S3.Bucket = "capture"
	cfg.S3.UseSSL = false

	// smtp
	cfg.SMTP.Enabled = true
	cfg.SMTP.Host = "smtp.qq.com"
	cfg.SMTP.Username = "<smtp-username>"
	cfg.SMTP.Password = "<smtp-password>"
	cfg.SMTP.From = "<smtp-from>"

	return cfg
}

// defaultSchedulerTasks 返回内置调度任务的常用配置。
func defaultSchedulerTasks() map[string]pipeline.SchedulerTaskConfig {
	return map[string]pipeline.SchedulerTaskConfig{
		"tag:event:detect": taskWithDefaults(pipeline.SchedulerTaskConfig{
			Interval:       serializer.DurationNS(time.Minute),
			Timeout:        serializer.DurationNS(5 * time.Minute),
			Enabled:        true,
			Lookback:       serializer.DurationNS(5 * time.Minute),
			BacklogEnabled: true,
			BacklogWindow:  serializer.DurationNS(time.Hour),
			BacklogLimit:   60,
		}),
		"tag:event:suppress": taskWithDefaults(pipeline.SchedulerTaskConfig{
			Interval:       serializer.DurationNS(time.Minute),
			Timeout:        serializer.DurationNS(5 * time.Minute),
			Enabled:        true,
			BacklogEnabled: true,
			BacklogWindow:  serializer.DurationNS(time.Hour),
			BacklogLimit:   60,
		}),
		"tag:event:cleanup": taskWithDefaults(pipeline.SchedulerTaskConfig{
			CronSpec:     "0 * * * *",
			Timeout:      serializer.DurationNS(5 * time.Minute),
			Enabled:      true,
			Payload:      map[string]any{"retention": "168h"},
			BacklogLimit: 1,
		}),
		"periodic-report:plan:dispatch": taskWithDefaults(pipeline.SchedulerTaskConfig{
			Interval:     serializer.DurationNS(time.Minute),
			Timeout:      serializer.DurationNS(time.Minute),
			MaxRetry:     1,
			Enabled:      true,
			Lookback:     serializer.DurationNS(0),
			BacklogLimit: 1,
		}),
		"periodic-report:run:cleanup": taskWithDefaults(pipeline.SchedulerTaskConfig{
			CronSpec:     "0 3 * * *",
			Timeout:      serializer.DurationNS(30 * time.Minute),
			MaxRetry:     1,
			Enabled:      true,
			Retention:    serializer.DurationNS(168 * time.Hour),
			BacklogLimit: 100,
		}),
	}
}

// taskWithDefaults 为单个调度任务补齐 default 标签值（defaults.Set 不会递归 map 值）。
func taskWithDefaults(cfg pipeline.SchedulerTaskConfig) pipeline.SchedulerTaskConfig {
	_ = defaults.Set(&cfg)
	return cfg
}

// tcpProcessorDefaults 返回 TCP 处理器常用参数。
func tcpProcessorDefaults() map[string]any {
	return map[string]any{
		"maxReassemblerSize":    1024,
		"maxOutOrderPacketSize": 256,
		"noticeTimeout":         "10s",
		"pollInterval":          "1m",
		"pollOffset":            "1m",
		"vNodeCount":            128,
		"batchSize":             1000,
		"flushInterval":         "5s",
		"saveInterval":          "30s",
		"replayWindow":          "1h",
	}
}

var createDefaultConfigCmd = &cobra.Command{
	Use:   "default-config",
	Short: "cmd.create.default-config.short",
	Long:  "cmd.create.default-config.long",
	// 标记为 bootstrap 命令，无需 config.toml 即可运行。
	Annotations: map[string]string{
		annotationSkipInjectorInit: "",
	},
	Run: func(cmd *cobra.Command, args []string) {
		output, _ := cmd.Flags().GetString("output")
		force, _ := cmd.Flags().GetBool("force")

		absPath, err := filepath.Abs(output)
		if err != nil {
			pterm.Error.Println(err.Error())
			return
		}

		// 检查目标文件是否已存在
		if !force {
			if _, err := os.Stat(absPath); err == nil {
				pterm.Error.Println(translate.T("cli.create.default-config.exists", map[string]any{"Path": absPath}))
				return
			}
		}

		// 由结构体实例生成 TOML，字段与注释随代码结构同步
		body, err := pkgconfig.MarshalTOML(newDefaultConfig())
		if err != nil {
			pterm.Error.Println(err.Error())
			return
		}
		content := append([]byte(defaultConfigHeader), body...)

		// 确保目标目录存在
		if dir := filepath.Dir(absPath); dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				pterm.Error.Println(err.Error())
				return
			}
		}

		if err := os.WriteFile(absPath, content, 0o644); err != nil {
			pterm.Error.Println(err.Error())
			return
		}
		pterm.Success.Println(translate.T("cli.create.default-config.created", map[string]any{"Path": absPath}))
		pterm.Info.Println(translate.T("cli.create.default-config.hint", nil))
	},
}

func init() {
	flags := createDefaultConfigCmd.Flags()
	flags.StringP("output", "o", "config.toml", "flag.create.default-config.output")
	flags.BoolP("force", "f", false, "flag.create.default-config.force")
}
