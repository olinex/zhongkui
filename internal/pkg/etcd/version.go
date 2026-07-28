// Package etcd 提供了 etcd 客户端的配置管理与连接创建功能。
// 该包封装了 etcd client/v3 客户端的配置结构，支持 TLS 安全连接，
// 并提供了配置描述和配置提取的便捷方法，方便从 viper 配置中加载 etcd 相关配置。
//
// 此外，该包还基于 etcd concurrency 包实现了分布式领导者选举机制和版本控制机制，
// 支持多实例部署场景下的主备切换、高可用调度和版本化管理。
package etcd

import (
	"context"
	"fmt"
	"time"

	"go.etcd.io/etcd/client/v3/concurrency"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

// VERSION_ETCD_PREFIX 是版本信息在 etcd 中的键前缀，
// 通过 WithPrefix 函数拼接全局前缀，最终格式为 "<GLOBAL_PREFIX>/version/"。
// 所有版本信息都将存储在该前缀下，键格式为 "<VERSION_ETCD_PREFIX>/<handlerName>"。
var (
	VERSION_ETCD_PREFIX = WithPrefix("version/")
	// VERSION_LOCK_ETCD_PREFIX 是版本锁在 etcd 中的键前缀，
	// 通过 WithPrefix 函数拼接全局前缀，最终格式为 "<GLOBAL_PREFIX>/version/lock/"。
	// 用于分布式锁，确保同一时刻只有一个实例执行版本更新操作。
	VERSION_LOCK_ETCD_PREFIX = WithPrefix("version/lock/")
)

// VersionHandler 定义了版本更新处理器需要实现的接口。
// 该接口用于实现基于版本号的增量处理逻辑，如数据库迁移、配置升级等。
//
// 工作流程：
//  1. Version.Commit 获取分布式锁
//  2. 从 etcd 读取上一个版本号（lastVersion）
//  3. 调用 handler.Handle 执行版本更新逻辑
//  4. 将新的版本号（nextVersion）写回 etcd
//
// 方法说明：
//   - Name: 返回版本处理器名称，用于区分不同的版本管理场景（如 "database"、"config" 等），
//     同一名称的版本处理器共享同一个版本号和锁
//   - Handle: 执行版本更新逻辑，接收上一个版本号，返回下一个版本号
//     该方法应根据 lastVersion 判断需要执行哪些升级步骤，并返回新的版本号
type VersionHandler interface {
	// Name 返回版本处理器名称
	Name() string
	// Handle 执行版本更新逻辑
	//
	// 参数：
	//   - ctx: 上下文对象，用于控制操作的生命周期
	//   - logger: 日志记录器，用于记录版本更新过程中的关键事件
	//   - lastVersion: 上一个版本号，为空字符串表示首次执行
	//
	// 返回值：
	//   - nextVersion: 更新后的版本号
	//   - err: 执行过程中的错误
	Handle(ctx context.Context, logger *logging.Logger, lastVersion string) (nextVersion string, err error)
}

// Version 封装了基于 etcd 的版本控制逻辑。
// 它利用 etcd 的分布式锁和键值存储实现版本号的原子性读取和更新，
// 确保在多实例部署场景下，版本更新操作不会被并发执行。
//
// 工作流程：
//  1. 创建 etcd Session（带 TTL 的租约，自动心跳保活）
//  2. 基于 Session 创建分布式锁，确保同一时刻只有一个实例执行版本更新
//  3. 从 etcd 读取上一个版本号
//  4. 调用 handler.Handle 执行版本更新逻辑
//  5. 将新的版本号写回 etcd
//  6. 释放分布式锁
//
// 字段说明：
//   - client: etcd 客户端实例，用于与 etcd 集群通信
//   - logger: 日志记录器，用于记录版本更新过程中的关键事件
//   - handler: 版本处理器，定义了版本更新的具体业务逻辑
type Version struct {
	client  *EtcdClient
	logger  *logging.Logger
	handler VersionHandler
}

// NewVersion 创建一个新的版本控制实例。
//
// 该函数仅初始化版本控制所需的结构体，不会立即执行版本更新。
// 版本更新过程在调用 Commit 方法后才开始。
//
// 参数：
//   - client: etcd 客户端实例，必须已成功连接到 etcd 集群
//   - logger: 日志记录器，用于记录版本更新过程中的状态变更和错误信息
//   - handler: 版本处理器，定义了版本名称和版本更新的业务逻辑
//
// 返回值：
//   - *Version: 初始化完成的版本控制实例
func NewVersion(client *EtcdClient, logger *logging.Logger, handler VersionHandler) *Version {
	return &Version{
		client:  client,
		logger:  logger,
		handler: handler,
	}
}

// Commit 执行一次版本更新操作，该方法会阻塞直到版本更新完成或上下文被取消。
//
// 执行流程：
//  1. 创建 etcd Session（TTL 为 30 秒），Session 会在后台自动保持心跳。
//     如果当前实例挂掉，Session 过期后锁自动释放，其他实例可以获取锁。
//  2. 基于 Session 创建分布式锁（路径为 versionLockPrefix + handler.Name()），
//     使用独立的 5 秒超时 context 获取锁，避免长时间阻塞。
//  3. 从 etcd 读取上一个版本号（键为 versionPrefix + handler.Name()），
//     如果键不存在则 lastVersion 为空字符串。
//  4. 调用 handler.Handle 执行版本更新逻辑，传入 lastVersion，获取 nextVersion。
//  5. 将 nextVersion 写回 etcd，完成版本更新。
//  6. 释放分布式锁，关闭 Session。
//
// 错误处理：
//   - Session 创建失败：返回错误，可能是 etcd 连接问题
//   - 锁获取超时（5 秒）：返回错误，可能是其他实例正在执行版本更新
//   - 读取版本号失败：返回错误，可能是 etcd 连接问题
//   - handler.Handle 执行失败：返回错误，版本更新逻辑出错
//   - 写入版本号失败：返回错误，可能是 etcd 连接问题
//
// 参数：
//   - ctx: 上下文对象，用于控制操作的生命周期，取消该上下文将中断版本更新
//
// 返回值：
//   - error: 版本更新过程中的错误，所有错误均通过 errors.WithStack 包装，保留调用栈信息
func (v *Version) Commit(ctx context.Context) error {
	name := v.handler.Name()
	// Session会在后台自动保持心跳。如果此实例挂掉，Session过期，锁自动释放。
	session, err := concurrency.NewSession(
		v.client,
		concurrency.WithContext(ctx),
		concurrency.WithTTL(30),
	)
	if err != nil {
		return errors.WithStack(err)
	}
	defer session.Close()
	// 获取锁（使用独立的短期超时 context，不影响后续业务逻辑）
	mu := concurrency.NewMutex(session, fmt.Sprintf("%s%s", VERSION_LOCK_ETCD_PREFIX, name))
	lockCtx, lockCancel := context.WithTimeout(ctx, 5*time.Second)
	defer lockCancel()
	v.logger.Ctx(ctx).Infof("get %s's version lock...", name)
	err = mu.Lock(lockCtx)
	if err != nil {
		return errors.WithStack(err)
	}
	defer mu.Unlock(ctx)
	v.logger.Ctx(ctx).Infof("get %s's version lock success", name)
	// 获取上一个版本号
	rsp, err := v.client.Get(ctx, fmt.Sprintf("%s%s", VERSION_ETCD_PREFIX, name))
	if err != nil {
		return errors.WithStack(err)
	}
	lastVersion := ""
	if len(rsp.Kvs) != 0 {
		lastVersion = string(rsp.Kvs[0].Value)
	}
	v.logger.Ctx(ctx).Infof("get %s's last version %s success", name, lastVersion)
	nextVersion, err := v.handler.Handle(ctx, v.logger, lastVersion)
	if err != nil {
		return errors.WithStack(err)
	}
	// 写入下一个版本号
	_, err = v.client.Put(ctx, fmt.Sprintf("%s%s", VERSION_ETCD_PREFIX, name), nextVersion)
	if err != nil {
		return errors.WithStack(err)
	}
	v.logger.Ctx(ctx).Infof("handle %s's next version %s success", name, nextVersion)
	return nil
}
