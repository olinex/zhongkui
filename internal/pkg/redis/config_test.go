package redis

import (
	"testing"
	"time"

	"github.com/creasty/defaults"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

// TestConfigConstants 测试配置常量
func TestConfigConstants(t *testing.T) {
	Convey("测试配置常量", t, func() {
		Convey("PREFIX应该是'redis'", func() {
			So(Prefix, ShouldEqual, "redis")
		})

		Convey("NORMAL_TYPE应该是'normal'", func() {
			So(NormalType, ShouldEqual, "normal")
		})

		Convey("SENTINEL_TYPE应该是'sentinel'", func() {
			So(SentinelType, ShouldEqual, "sentinel")
		})
	})
}

// TestConfigDefaultValues 测试配置默认值
func TestConfigDefaultValues(t *testing.T) {
	Convey("测试配置默认值", t, func() {
		cfg := &Config{}
		err := defaults.Set(cfg)
		So(err, ShouldBeNil)

		Convey("应该设置正确的默认值", func() {
			So(cfg.Type, ShouldEqual, "normal")
			So(cfg.Network, ShouldEqual, "tcp")
			So(cfg.Addr, ShouldEqual, "localhost:6379")
			So(cfg.DB, ShouldEqual, 0)
			So(cfg.PoolSize, ShouldEqual, 10)
			So(cfg.MinIdleConns, ShouldEqual, 3)
			So(cfg.MaxIdleConns, ShouldEqual, 5)
			So(cfg.MaxActiveConns, ShouldEqual, 10)
			So(cfg.MaxRetries, ShouldEqual, 5)
			So(cfg.MinRetryBackoff, ShouldEqual, 8*time.Millisecond)
			So(cfg.MaxRetryBackoff, ShouldEqual, 512*time.Millisecond)
			So(cfg.DialTimeout, ShouldEqual, 5*time.Second)
			So(cfg.ReadTimeout, ShouldEqual, 3*time.Second)
			So(cfg.WriteTimeout, ShouldEqual, 3*time.Second)
			So(cfg.ContextTimeoutEnabled, ShouldBeTrue)

			So(cfg.MasterName, ShouldEqual, "")
			So(cfg.SentinelAddrs, ShouldBeEmpty)
			So(cfg.SentinelUsername, ShouldEqual, "")
			So(cfg.SentinelPassword, ShouldEqual, "")
			So(cfg.RouteByLatency, ShouldBeFalse)
			So(cfg.RouteRandomly, ShouldBeTrue)
			So(cfg.ReplicaOnly, ShouldBeTrue)
			So(cfg.UseDisconnectedReplicas, ShouldBeFalse)

			So(cfg.Username, ShouldEqual, "")
			So(cfg.Password, ShouldEqual, "")
			So(cfg.ClientName, ShouldEqual, "")
		})
	})
}

// TestToOptionsNormalConfig 测试普通配置转换为选项
func TestToOptionsNormalConfig(t *testing.T) {
	Convey("测试普通配置转换为选项", t, func() {
		cfg := &Config{}
		err := defaults.Set(cfg)
		So(err, ShouldBeNil)

		cfg.Type = NormalType
		cfg.Network = "tcp"
		cfg.Addr = "127.0.0.1:6380"
		cfg.Username = "testuser"
		cfg.Password = "testpass"
		cfg.ClientName = "test-client"
		cfg.DB = 1
		cfg.PoolSize = 20
		cfg.MinIdleConns = 5
		cfg.MaxIdleConns = 10
		cfg.MaxActiveConns = 30
		cfg.MaxRetries = 3
		cfg.MinRetryBackoff = 10 * time.Millisecond
		cfg.MaxRetryBackoff = 1 * time.Second
		cfg.DialTimeout = 10 * time.Second
		cfg.ReadTimeout = 5 * time.Second
		cfg.WriteTimeout = 5 * time.Second
		cfg.ContextTimeoutEnabled = false

		Convey("应该正确转换为redis.Options", func() {
			options := cfg.ToOptions()
			So(options, ShouldNotBeNil)

			So(options.Network, ShouldEqual, cfg.Network)
			So(options.Addr, ShouldEqual, cfg.Addr)
			So(options.Username, ShouldEqual, cfg.Username)
			So(options.Password, ShouldEqual, cfg.Password)
			So(options.ClientName, ShouldEqual, cfg.ClientName)
			So(options.DB, ShouldEqual, cfg.DB)
			So(options.PoolSize, ShouldEqual, cfg.PoolSize)
			So(options.MinIdleConns, ShouldEqual, cfg.MinIdleConns)
			So(options.MaxIdleConns, ShouldEqual, cfg.MaxIdleConns)
			So(options.MaxActiveConns, ShouldEqual, cfg.MaxActiveConns)
			So(options.MaxRetries, ShouldEqual, cfg.MaxRetries)
			So(options.MinRetryBackoff, ShouldEqual, cfg.MinRetryBackoff)
			So(options.MaxRetryBackoff, ShouldEqual, cfg.MaxRetryBackoff)
			So(options.DialTimeout, ShouldEqual, cfg.DialTimeout)
			So(options.ReadTimeout, ShouldEqual, cfg.ReadTimeout)
			So(options.WriteTimeout, ShouldEqual, cfg.WriteTimeout)
			So(options.ContextTimeoutEnabled, ShouldEqual, cfg.ContextTimeoutEnabled)
		})
	})
}

// TestToSentinelOptionsSentinelConfig 测试Sentinel配置转换为选项
func TestToSentinelOptionsSentinelConfig(t *testing.T) {
	Convey("测试Sentinel配置转换为选项", t, func() {
		cfg := &Config{}
		err := defaults.Set(cfg)
		So(err, ShouldBeNil)

		cfg.Type = SentinelType
		cfg.MasterName = "mymaster"
		cfg.SentinelAddrs = []string{"sentinel1:26379", "sentinel2:26379"}
		cfg.SentinelUsername = "sentineluser"
		cfg.SentinelPassword = "sentinelpass"
		cfg.RouteByLatency = false
		cfg.RouteRandomly = false
		cfg.ReplicaOnly = false
		cfg.UseDisconnectedReplicas = true
		cfg.Username = "redisuser"
		cfg.Password = "redispass"
		cfg.ClientName = "sentinel-client"
		cfg.DB = 2
		cfg.PoolSize = 15
		cfg.MinIdleConns = 4
		cfg.MaxRetries = 2
		cfg.MinRetryBackoff = 20 * time.Millisecond
		cfg.MaxRetryBackoff = 2 * time.Second
		cfg.DialTimeout = 8 * time.Second
		cfg.ReadTimeout = 4 * time.Second
		cfg.WriteTimeout = 4 * time.Second
		cfg.ContextTimeoutEnabled = true

		Convey("应该正确转换为redis.FailoverOptions", func() {
			options := cfg.ToSentinelOptions()
			So(options, ShouldNotBeNil)

			So(options.MasterName, ShouldEqual, cfg.MasterName)
			So(options.SentinelAddrs, ShouldResemble, cfg.SentinelAddrs)
			So(options.SentinelUsername, ShouldEqual, cfg.SentinelUsername)
			So(options.SentinelPassword, ShouldEqual, cfg.SentinelPassword)
			So(options.RouteByLatency, ShouldEqual, cfg.RouteByLatency)
			So(options.RouteRandomly, ShouldEqual, cfg.RouteRandomly)
			So(options.ReplicaOnly, ShouldEqual, cfg.ReplicaOnly)
			So(options.UseDisconnectedReplicas, ShouldEqual, cfg.UseDisconnectedReplicas)
			So(options.Username, ShouldEqual, cfg.Username)
			So(options.Password, ShouldEqual, cfg.Password)
			So(options.ClientName, ShouldEqual, cfg.ClientName)
			So(options.DB, ShouldEqual, cfg.DB)
			So(options.PoolSize, ShouldEqual, cfg.PoolSize)
			So(options.MinIdleConns, ShouldEqual, cfg.MinIdleConns)
			So(options.MaxRetries, ShouldEqual, cfg.MaxRetries)
			So(options.MinRetryBackoff, ShouldEqual, cfg.MinRetryBackoff)
			So(options.MaxRetryBackoff, ShouldEqual, cfg.MaxRetryBackoff)
			So(options.DialTimeout, ShouldEqual, cfg.DialTimeout)
			So(options.ReadTimeout, ShouldEqual, cfg.ReadTimeout)
			So(options.WriteTimeout, ShouldEqual, cfg.WriteTimeout)
			So(options.ContextTimeoutEnabled, ShouldEqual, cfg.ContextTimeoutEnabled)
		})
	})
}

// TestExtractConfigFromViper 测试从Viper提取配置
func TestExtractConfigFromViper(t *testing.T) {
	Convey("测试从Viper提取配置", t, func() {
		v := viper.New()

		v.Set("redis.test.type", "normal")
		v.Set("redis.test.addr", "localhost:6380")
		v.Set("redis.test.db", 3)
		v.Set("redis.test.poolSize", 25)
		v.Set("redis.test.password", "extracted-pass")

		Convey("应该成功提取配置", func() {
			cfg, err := ExtractConfig(v, "test")
			So(err, ShouldBeNil)
			So(cfg, ShouldNotBeNil)

			So(cfg.Type, ShouldEqual, "normal")
			So(cfg.Addr, ShouldEqual, "localhost:6380")
			So(cfg.DB, ShouldEqual, 3)
			So(cfg.PoolSize, ShouldEqual, 25)
			So(cfg.Password, ShouldEqual, "extracted-pass")
		})
	})
}

// TestExtractConfigSentinelFromViper 测试从Viper提取Sentinel配置
func TestExtractConfigSentinelFromViper(t *testing.T) {
	Convey("测试从Viper提取Sentinel配置", t, func() {
		v := viper.New()

		v.Set("redis.sentinel_test.type", "sentinel")
		v.Set("redis.sentinel_test.masterName", "testmaster")
		v.Set("redis.sentinel_test.sentinelAddrs", []string{"s1:26379", "s2:26379"})
		v.Set("redis.sentinel_test.sentinelPassword", "sentinel-secret")
		v.Set("redis.sentinel_test.routeRandomly", false)

		Convey("应该成功提取Sentinel配置", func() {
			cfg, err := ExtractConfig(v, "sentinel_test")
			So(err, ShouldBeNil)
			So(cfg, ShouldNotBeNil)

			So(cfg.Type, ShouldEqual, "sentinel")
			So(cfg.MasterName, ShouldEqual, "testmaster")
			So(cfg.SentinelAddrs, ShouldResemble, []string{"s1:26379", "s2:26379"})
			So(cfg.SentinelPassword, ShouldEqual, "sentinel-secret")
			So(cfg.RouteRandomly, ShouldBeFalse)
		})
	})
}

// TestExtractConfigInvalidPrefix 测试提取无效前缀的配置
func TestExtractConfigInvalidPrefix(t *testing.T) {
	Convey("测试提取无效前缀的配置", t, func() {
		v := viper.New()

		Convey("应该对不存在的配置返回错误", func() {
			cfg, err := ExtractConfig(v, "nonexistent")
			So(err, ShouldNotBeNil)
			So(cfg, ShouldBeNil)
			So(err.Error(), ShouldContainSubstring, "Cannot get")
		})
	})
}

// TestDescribeConfig 测试描述配置
func TestDescribeConfig(t *testing.T) {
	Convey("测试描述配置", t, func() {
		descriptions := DescribeConfig("default", "backup")
		So(descriptions, ShouldNotBeEmpty)

		Convey("应该包含正确的配置描述", func() {
			foundType := false
			foundAddr := false
			foundDB := false

			for _, desc := range descriptions {
				if desc.Path == "redis.{default,backup}.type" {
					foundType = true
					So(desc.Kind, ShouldEqual, "string")
					So(desc.Default, ShouldEqual, "normal")
				}
				if desc.Path == "redis.{default,backup}.addr" {
					foundAddr = true
					So(desc.Kind, ShouldEqual, "string")
					So(desc.Default, ShouldEqual, "localhost:6379")
				}
				if desc.Path == "redis.{default,backup}.db" {
					foundDB = true
					So(desc.Kind, ShouldEqual, "int")
					So(desc.Default, ShouldEqual, "0")
				}
			}

			So(foundType, ShouldBeTrue)
			So(foundAddr, ShouldBeTrue)
			So(foundDB, ShouldBeTrue)
		})
	})
}

// TestConfigValidation 测试配置验证
func TestConfigValidation(t *testing.T) {
	Convey("测试配置验证", t, func() {
		v := viper.New()
		v.Set("redis.invalid.type", "invalid_type")

		Convey("应该拒绝无效类型", func() {
			cfg, err := ExtractConfig(v, "invalid")
			So(err, ShouldNotBeNil)
			So(cfg, ShouldBeNil)
		})
	})
}

// TestConfigDurationFields 测试配置持续时间字段
func TestConfigDurationFields(t *testing.T) {
	Convey("测试配置持续时间字段", t, func() {
		cfg := &Config{}
		err := defaults.Set(cfg)
		So(err, ShouldBeNil)

		Convey("应该设置正确的持续时间默认值", func() {
			So(cfg.MinRetryBackoff, ShouldEqual, 8*time.Millisecond)
			So(cfg.MaxRetryBackoff, ShouldEqual, 512*time.Millisecond)
			So(cfg.DialTimeout, ShouldEqual, 5*time.Second)
			So(cfg.ReadTimeout, ShouldEqual, 3*time.Second)
			So(cfg.WriteTimeout, ShouldEqual, 3*time.Second)

			options := cfg.ToOptions()
			So(options.MinRetryBackoff, ShouldEqual, cfg.MinRetryBackoff)
			So(options.MaxRetryBackoff, ShouldEqual, cfg.MaxRetryBackoff)
			So(options.DialTimeout, ShouldEqual, cfg.DialTimeout)
			So(options.ReadTimeout, ShouldEqual, cfg.ReadTimeout)
			So(options.WriteTimeout, ShouldEqual, cfg.WriteTimeout)
		})
	})
}

// TestConfigEmptyStrings 测试配置空字符串字段
func TestConfigEmptyStrings(t *testing.T) {
	Convey("测试配置空字符串字段", t, func() {
		cfg := &Config{}
		err := defaults.Set(cfg)
		So(err, ShouldBeNil)

		cfg.Username = ""
		cfg.Password = ""
		cfg.ClientName = ""
		cfg.MasterName = ""
		cfg.SentinelUsername = ""
		cfg.SentinelPassword = ""

		Convey("应该正确处理空字符串", func() {
			So(cfg.Username, ShouldBeEmpty)
			So(cfg.Password, ShouldBeEmpty)
			So(cfg.ClientName, ShouldBeEmpty)
			So(cfg.MasterName, ShouldBeEmpty)
			So(cfg.SentinelUsername, ShouldBeEmpty)
			So(cfg.SentinelPassword, ShouldBeEmpty)

			options := cfg.ToOptions()
			So(options.Username, ShouldBeEmpty)
			So(options.Password, ShouldBeEmpty)
			So(options.ClientName, ShouldBeEmpty)

			sentinelOptions := cfg.ToSentinelOptions()
			So(sentinelOptions.MasterName, ShouldBeEmpty)
			So(sentinelOptions.SentinelUsername, ShouldBeEmpty)
			So(sentinelOptions.SentinelPassword, ShouldBeEmpty)
			So(sentinelOptions.Username, ShouldBeEmpty)
			So(sentinelOptions.Password, ShouldBeEmpty)
			So(sentinelOptions.ClientName, ShouldBeEmpty)
		})
	})
}
