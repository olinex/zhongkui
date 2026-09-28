// Package pipeline 提供基于 asynq 的任务队列与分布式调度功能。
// 此文件提供调度器任务配置的 etcd 增删改查与"立即执行"原语，
// 供前端定时任务管理接口（internal/manager/views/etl.go）与调度循环共用。
package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/creasty/defaults"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/etcd"
)

// SchedulerTaskConfigKey 返回任务配置在 etcd 中对应的键。
// 格式为 "<SchedulerCfgEtcdPrefix><taskType>"，所有任务的配置都存储在此前缀下。
func SchedulerTaskConfigKey(taskType string) string {
	return fmt.Sprintf(`%s%s`, SchedulerCfgEtcdPrefix, taskType)
}

// LoadSchedulerTaskConfigs 从 etcd 读取全部定时任务配置并应用默认值。
// 返回以任务类型为键的配置映射，包含未启用的任务，由调用方按需过滤。
func LoadSchedulerTaskConfigs(ctx context.Context, client *etcd.EtcdClient) (map[string]SchedulerTaskConfig, error) {
	return loadSchedulerTaskConfigs(ctx, client)
}

// SaveSchedulerTaskConfig 将单个任务配置写入 etcd，覆盖已存在的值。
// 写入前会补齐缺失字段的默认值（defaults.Set），保证写入的配置自洽。
func SaveSchedulerTaskConfig(ctx context.Context, client *etcd.EtcdClient, taskType string, cfg SchedulerTaskConfig) error {
	if err := defaults.Set(&cfg); err != nil {
		return errors.WithStack(err)
	}
	value, err := json.Marshal(cfg)
	if err != nil {
		return errors.WithStack(err)
	}
	if _, err := client.Put(ctx, SchedulerTaskConfigKey(taskType), string(value)); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// DeleteSchedulerTaskConfig 从 etcd 删除指定任务的配置。
func DeleteSchedulerTaskConfig(ctx context.Context, client *etcd.EtcdClient, taskType string) error {
	if _, err := client.Delete(ctx, SchedulerTaskConfigKey(taskType)); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// EnqueueSchedulerTaskNow 立即投递一条签到当前时刻作为触发时间点的任务。
// 用于前端"立即执行"能力：以任务当前配置的队列/超时/重试参数将 config 中的
// 固定入参透传给处理器，触发时间点为调用时刻（time.Now）。
func EnqueueSchedulerTaskNow(ctx context.Context, enq *Client, taskType string, cfg SchedulerTaskConfig) error {
	opts := taskOptions(cfg)
	params := payloadParams(cfg)
	task, err := newTriggerTask(taskType, time.Now(), params, opts)
	if err != nil {
		return errors.WithStack(err)
	}
	if _, err := enq.EnqueueContext(ctx, task); err != nil {
		return errors.WithStack(err)
	}
	return nil
}
