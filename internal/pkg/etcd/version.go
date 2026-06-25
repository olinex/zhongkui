package etcd

import (
	"context"
	"fmt"
	"time"

	"go.etcd.io/etcd/client/v3/concurrency"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

var (
	versionPrefix     = WithPrefix("version/")
	versionLockPrefix = WithPrefix("version/lock/")
)

type VersionHandler interface {
	Name() string
	Handle(ctx context.Context, logger *logging.Logger, lastVersion string) (nextVersion string, err error)
}

type Version struct {
	client  *EtcdClient
	logger  *logging.Logger
	handler VersionHandler
}

func NewVersion(client *EtcdClient, logger *logging.Logger, handler VersionHandler) *Version {
	return &Version{
		client:  client,
		logger:  logger,
		handler: handler,
	}
}

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
	mu := concurrency.NewMutex(session, fmt.Sprintf("%s%s/", versionLockPrefix, name))
	lockCtx, lockCancel := context.WithTimeout(ctx, 5*time.Second)
	defer lockCancel()
	v.logger.Infof("get %s's version lock...", name)
	err = mu.Lock(lockCtx)
	if err != nil {
		return errors.WithStack(err)
	}
	defer mu.Unlock(ctx)
	v.logger.Infof("get %s's version lock success", name)
	// 获取上一个版本号
	rsp, err := v.client.Get(ctx, fmt.Sprintf("%s%s", versionPrefix, name))
	if err != nil {
		return errors.WithStack(err)
	}
	lastVersion := ""
	if len(rsp.Kvs) != 0 {
		lastVersion = string(rsp.Kvs[0].Value)
	}
	v.logger.Infof("get %s's last version %s success", name, lastVersion)
	nextVersion, err := v.handler.Handle(ctx, v.logger, lastVersion)
	if err != nil {
		return errors.WithStack(err)
	}
	// 写入下一个版本号
	_, err = v.client.Put(ctx, fmt.Sprintf("%s%s", versionPrefix, name), nextVersion)
	if err != nil {
		return errors.WithStack(err)
	}
	v.logger.Infof("handle %s's next version %s success", name, nextVersion)
	return nil
}
