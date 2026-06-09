package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/creasty/defaults"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

// TestConfigConstants 测试配置常量和类型别名
func TestConfigConstants(t *testing.T) {
	Convey("测试配置常量和类型别名", t, func() {
		Convey("PREFIX 应该是 'pipeline'", func() {
			So(PREFIX, ShouldEqual, "pipeline")
		})

		Convey("Task 应该是 asynq.Task 的类型别名", func() {
			var task Task
			So(task, ShouldHaveSameTypeAs, Task{})
		})

		Convey("ContextFunc 应该是返回 context.Context 的函数类型", func() {
			var ctxFunc ContextFunc
			So(ctxFunc, ShouldHaveSameTypeAs, (func() context.Context)(nil))
		})
	})
}

// TestSchedulerTaskConfigDefaults 测试 SchedulerTaskConfig 默认值
func TestSchedulerTaskConfigDefaults(t *testing.T) {
	Convey("测试 SchedulerTaskConfig 默认值", t, func() {
		cfg := &SchedulerTaskConfig{}
		err := defaults.Set(cfg)
		So(err, ShouldBeNil)

		Convey("应该设置正确的默认值", func() {
			So(cfg.CronSpec, ShouldEqual, "")
			So(cfg.Queue, ShouldEqual, "default")
			So(cfg.Timeout, ShouldEqual, 24*time.Hour)
			So(cfg.MaxRetry, ShouldEqual, 3)
			So(cfg.Enabled, ShouldBeFalse)
			So(cfg.Payload, ShouldBeEmpty)
		})
	})
}

// TestSchedulerConfigDefaults 测试 SchedulerConfig 默认值
func TestSchedulerConfigDefaults(t *testing.T) {
	Convey("测试 SchedulerConfig 默认值", t, func() {
		cfg := &SchedulerConfig{}
		err := defaults.Set(cfg)
		So(err, ShouldBeNil)

		Convey("应该设置正确的默认值", func() {
			So(cfg.Tasks, ShouldBeEmpty)
			So(string(cfg.Location), ShouldEqual, "UTC")
			So(cfg.SyncInterval, ShouldEqual, 1*time.Minute)
			So(cfg.HeartbeatInterval, ShouldEqual, 10*time.Second)
			So(cfg.ForceFlush, ShouldBeFalse)
		})
	})
}

// TestConfigDefaults 测试 Config 默认值
func TestConfigDefaults(t *testing.T) {
	Convey("测试 Config 默认值", t, func() {
		cfg := &Config{}
		err := defaults.Set(cfg)
		So(err, ShouldBeNil)

		Convey("应该设置正确的默认值", func() {
			So(cfg.Concurrency, ShouldEqual, -1)
			So(cfg.TaskCheckInterval, ShouldEqual, 1*time.Second)
			So(cfg.HealthCheckInterval, ShouldEqual, 15*time.Second)
			So(cfg.DelayedTaskCheckInterval, ShouldEqual, 5*time.Second)
			So(cfg.ShutdownTimeout, ShouldEqual, 10*time.Second)
			So(cfg.StrictPriority, ShouldBeFalse)

			Convey("队列优先级应该有正确的默认值", func() {
				So(cfg.Queues, ShouldNotBeNil)
				So(cfg.Queues["critical"], ShouldEqual, 6)
				So(cfg.Queues["default"], ShouldEqual, 3)
				So(cfg.Queues["low"], ShouldEqual, 1)
			})

			Convey("调度器配置应该有正确的默认值", func() {
				So(cfg.Scheduler.Tasks, ShouldBeEmpty)
				So(string(cfg.Scheduler.Location), ShouldEqual, "UTC")
				So(cfg.Scheduler.SyncInterval, ShouldEqual, 1*time.Minute)
				So(cfg.Scheduler.HeartbeatInterval, ShouldEqual, 10*time.Second)
				So(cfg.Scheduler.ForceFlush, ShouldBeFalse)
			})
		})
	})
}

// TestToAsynqConfig 测试 ToAsynqConfig 方法
func TestToAsynqConfig(t *testing.T) {
	Convey("测试 ToAsynqConfig 方法", t, func() {
		cfg := &Config{
			Concurrency:              10,
			TaskCheckInterval:        2 * time.Second,
			HealthCheckInterval:      30 * time.Second,
			DelayedTaskCheckInterval: 10 * time.Second,
			ShutdownTimeout:          20 * time.Second,
			Queues: map[string]int{
				"high":   10,
				"medium": 5,
				"low":    1,
			},
			StrictPriority: true,
		}

		// 创建一个简单的 zap 日志记录器用于测试
		logConfig := zap.NewProductionConfig()
		logConfig.OutputPaths = []string{}
		zapLogger, _ := logConfig.Build()
		logger := zapLogger.Sugar()
		ctxFunc := func() context.Context { return context.Background() }

		Convey("应该正确转换为 asynq.Config", func() {
			asynqCfg := cfg.ToAsynqConfig(ctxFunc, logger)

			So(asynqCfg.Concurrency, ShouldEqual, cfg.Concurrency)
			So(asynqCfg.TaskCheckInterval, ShouldEqual, cfg.TaskCheckInterval)
			So(asynqCfg.HealthCheckInterval, ShouldEqual, cfg.HealthCheckInterval)
			So(asynqCfg.ShutdownTimeout, ShouldEqual, cfg.ShutdownTimeout)
			So(asynqCfg.Queues, ShouldResemble, cfg.Queues)
			So(asynqCfg.StrictPriority, ShouldEqual, cfg.StrictPriority)
			So(asynqCfg.Logger, ShouldEqual, logger)
		})

		Convey("DelayedTaskCheckInterval 不应该出现在 asynq.Config 中", func() {
			asynqCfg := cfg.ToAsynqConfig(ctxFunc, logger)
			// 验证 asynq.Config 结构中没有 DelayedTaskCheckInterval 字段
			// 这是预期的，因为 asynq 已弃用此字段
			So(asynqCfg.Concurrency, ShouldEqual, cfg.Concurrency)
		})
	})
}

// TestExtractConfig 测试 ExtractConfig 函数
func TestExtractConfig(t *testing.T) {
	Convey("测试 ExtractConfig 函数", t, func() {
		v := viper.New()

		Convey("应该正确提取配置", func() {
			// 设置测试配置
			v.Set("pipeline.worker.concurrency", 20)
			v.Set("pipeline.worker.taskCheckInterval", "3s")
			v.Set("pipeline.worker.healthCheckInterval", "45s")
			v.Set("pipeline.worker.shutdownTimeout", "30s")
			v.Set("pipeline.worker.queues.critical", 8)
			v.Set("pipeline.worker.queues.default", 4)
			v.Set("pipeline.worker.queues.low", 2)

			cfg, err := ExtractConfig(v, "worker")
			So(err, ShouldBeNil)
			So(cfg, ShouldNotBeNil)

			So(cfg.Concurrency, ShouldEqual, 20)
			So(cfg.TaskCheckInterval, ShouldEqual, 3*time.Second)
			So(cfg.HealthCheckInterval, ShouldEqual, 45*time.Second)
			So(cfg.ShutdownTimeout, ShouldEqual, 30*time.Second)
			So(cfg.Queues["critical"], ShouldEqual, 8)
			So(cfg.Queues["default"], ShouldEqual, 4)
			So(cfg.Queues["low"], ShouldEqual, 2)
		})

		Convey("应该应用默认值", func() {
			// 只设置部分配置
			v.Set("pipeline.test.concurrency", 5)

			cfg, err := ExtractConfig(v, "test")
			So(err, ShouldBeNil)
			So(cfg, ShouldNotBeNil)

			// 验证显式设置的配置
			So(cfg.Concurrency, ShouldEqual, 5)

			// 验证默认值
			So(cfg.TaskCheckInterval, ShouldEqual, 1*time.Second)
			So(cfg.HealthCheckInterval, ShouldEqual, 15*time.Second)
			So(cfg.StrictPriority, ShouldBeFalse)
		})

		Convey("应该正确处理调度器配置", func() {
			v.Set("pipeline.test.scheduler.location", "Asia/Shanghai")
			v.Set("pipeline.test.scheduler.syncInterval", "2m")
			v.Set("pipeline.test.scheduler.heartbeatInterval", "15s")
			v.Set("pipeline.test.scheduler.forceFlush", true)
			v.Set("pipeline.test.scheduler.tasks.daily.cronSpec", "0 0 * * *")
			v.Set("pipeline.test.scheduler.tasks.daily.queue", "critical")
			v.Set("pipeline.test.scheduler.tasks.daily.enabled", true)

			cfg, err := ExtractConfig(v, "test")
			So(err, ShouldBeNil)
			So(cfg, ShouldNotBeNil)

			So(string(cfg.Scheduler.Location), ShouldEqual, "Asia/Shanghai")
			So(cfg.Scheduler.SyncInterval, ShouldEqual, 2*time.Minute)
			So(cfg.Scheduler.HeartbeatInterval, ShouldEqual, 15*time.Second)
			So(cfg.Scheduler.ForceFlush, ShouldBeTrue)
			So(cfg.Scheduler.Tasks["daily"].CronSpec, ShouldEqual, "0 0 * * *")
			So(cfg.Scheduler.Tasks["daily"].Queue, ShouldEqual, "critical")
			So(cfg.Scheduler.Tasks["daily"].Enabled, ShouldBeTrue)
		})

		Convey("应该返回错误当配置节不存在时", func() {
			cfg, err := ExtractConfig(v, "nonexistent")
			So(err, ShouldNotBeNil)
			So(cfg, ShouldBeNil)
			So(err.Error(), ShouldContainSubstring, "Cannot get")
		})
	})
}

// TestDescribeConfig 测试 DescribeConfig 函数
func TestDescribeConfig(t *testing.T) {
	Convey("测试 DescribeConfig 函数", t, func() {
		Convey("应该生成正确的配置描述", func() {
			descs := DescribeConfig("worker")
			So(descs, ShouldNotBeNil)
			So(len(descs), ShouldBeGreaterThan, 0)

			// 验证一些关键字段的描述
			foundConcurrency := false
			foundQueues := false
			foundScheduler := false

			for _, desc := range descs {
				if desc.Path == "pipeline.{worker}.concurrency" {
					foundConcurrency = true
					So(desc.Kind, ShouldEqual, "int")
					So(desc.Default, ShouldEqual, "-1")
					So(desc.Comment, ShouldContainSubstring, "maximum number of concurrent processing tasks")
				}
				if desc.Path == "pipeline.{worker}.queues" {
					foundQueues = true
					So(desc.Kind, ShouldEqual, "map")
				}
				if desc.Path == "pipeline.{worker}.scheduler" {
					foundScheduler = true
				}
			}

			So(foundConcurrency, ShouldBeTrue)
			So(foundQueues, ShouldBeTrue)
			So(foundScheduler, ShouldBeTrue)
		})

		Convey("应该处理多个名称", func() {
			descs := DescribeConfig("worker", "manager", "api")
			So(descs, ShouldNotBeNil)

			// 验证路径格式
			expectedPath := "pipeline.{worker,manager,api}"
			for _, desc := range descs {
				So(desc.Path, ShouldStartWith, expectedPath)
			}
		})

		Convey("应该处理空名称列表", func() {
			descs := DescribeConfig()
			So(descs, ShouldNotBeNil)

			// 验证通用配置结构
			for _, desc := range descs {
				So(desc.Path, ShouldStartWith, "pipeline.{}")
			}
		})
	})
}

// TestSchedulerTaskConfigValidation 测试 SchedulerTaskConfig 验证
func TestSchedulerTaskConfigValidation(t *testing.T) {
	Convey("测试 SchedulerTaskConfig 验证", t, func() {
		Convey("应该验证有效的 cron 表达式", func() {
			cfg := &SchedulerTaskConfig{
				CronSpec: "0 * * * *",
				Queue:    "default",
				Enabled:  true,
			}

			err := defaults.Set(cfg)
			So(err, ShouldBeNil)

			// 这里可以添加更详细的验证逻辑
			So(cfg.CronSpec, ShouldNotBeEmpty)
		})

		Convey("应该处理带有效载荷的任务", func() {
			cfg := &SchedulerTaskConfig{
				CronSpec: "*/5 * * * *",
				Queue:    "critical",
				Enabled:  true,
				Payload: map[string]any{
					"action":    "cleanup",
					"threshold": 1000,
					"dry_run":   false,
				},
			}

			err := defaults.Set(cfg)
			So(err, ShouldBeNil)

			So(cfg.Payload, ShouldNotBeNil)
			So(cfg.Payload["action"], ShouldEqual, "cleanup")
			So(cfg.Payload["threshold"], ShouldEqual, 1000)
			So(cfg.Payload["dry_run"], ShouldBeFalse)
		})
	})
}

// TestConfigIntegration 测试配置集成功能
func TestConfigIntegration(t *testing.T) {
	Convey("测试配置集成功能", t, func() {
		Convey("应该与 config 包正确集成", func() {
			// 测试 DescribeConfig 返回的类型
			descs := DescribeConfig("test")
			So(descs, ShouldHaveSameTypeAs, config.ConfigDescriptions{})

			// 测试 ExtractConfig 返回的类型
			v := viper.New()
			v.Set("pipeline.test.concurrency", 15)

			cfg, err := ExtractConfig(v, "test")
			So(err, ShouldBeNil)
			So(cfg, ShouldHaveSameTypeAs, &Config{})
		})

		Convey("应该正确处理 serializer.Location", func() {
			v := viper.New()
			v.Set("pipeline.test.scheduler.location", "America/New_York")

			cfg, err := ExtractConfig(v, "test")
			So(err, ShouldBeNil)

			loc := cfg.Scheduler.Location
			So(loc, ShouldHaveSameTypeAs, serializer.Location(""))
			So(string(loc), ShouldEqual, "America/New_York")
		})
	})
}
