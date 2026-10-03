package core

import (
	"context"
	"fmt"

	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/protobuf/proto"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/etcd"
	reporter_v1 "zhongkuisec.com/zhongkui/internal/pkg/protobuf/reporter/v1"
)

var (
	// ReporterEtcdPrefix reporter 配置与状态在 etcd 中的存储键前缀
	ReporterEtcdPrefix = etcd.WithPrefix("reporter/")
)

// ReporterConfigKey 生成 reporter 配置在 etcd 中的存储键
// 格式: {prefix}reporter/{namespace}/{name}/{version}/cfg
// 参数:
//   - r: reporter 资源信息，包含命名空间、名称和版本
//
// 返回:
//   - 配置键字符串
func ReporterConfigKey(r *reporter_v1.Resource) string {
	return fmt.Sprintf(`%s%s/%s/%s/cfg`, ReporterEtcdPrefix, r.Namespace, r.Name, r.Version)
}

// ReporterStatusListKey 生成 reporter 状态列表在 etcd 中的存储键
// 格式: {prefix}reporter/{namespace}/{name}/{version}/sts
// 参数:
//   - r: reporter 资源信息，包含命名空间、名称和版本
//
// 返回:
//   - 状态列表键字符串
func ReporterStatusListKey(r *reporter_v1.Resource) string {
	return fmt.Sprintf(`%s%s/%s/%s/sts`, ReporterEtcdPrefix, r.Namespace, r.Name, r.Version)
}

// ReporterStatusDetailKey 生成 reporter 具体状态在 etcd 中的存储键
// 格式: {prefix}reporter/{namespace}/{name}/{version}/sts/{sessionID}
// 参数:
//   - r: reporter 资源信息，包含命名空间、名称和版本
//   - sessionID: 会话标识符
//
// 返回:
//   - 状态详情键字符串
func ReporterStatusDetailKey(r *reporter_v1.Resource, sessionID string) string {
	return fmt.Sprintf(`%s/%s`, ReporterStatusListKey(r), sessionID)
}

// UnmarshalAsReporterConfig 将字节数组反序列化为 reporter 配置对象
// 参数:
//   - value: 包含序列化配置的字节数组
//
// 返回:
//   - *reporter_v1.Config: 反序列化后的配置对象
//   - error: 反序列化过程中的错误
func UnmarshalAsReporterConfig(value []byte) (*reporter_v1.Config, error) {
	config := reporter_v1.Config{}
	err := proto.Unmarshal(value, &config)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &config, nil
}

// UnmarshalAsReporterStatus 将字节数组反序列化为 reporter 状态对象
// 参数:
//   - value: 包含序列化状态的字节数组
//
// 返回:
//   - *reporter_v1.Status: 反序列化后的状态对象
//   - error: 反序列化过程中的错误
func UnmarshalAsReporterStatus(value []byte) (*reporter_v1.Status, error) {
	status := reporter_v1.Status{}
	err := proto.Unmarshal(value, &status)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &status, nil
}

// ReporterConfigMarshalAsBytes 将 reporter 配置对象序列化为字节数组
// 参数:
//   - config: reporter 配置对象
//
// 返回:
//   - []byte: 序列化后的字节数组
//   - error: 序列化过程中的错误
func ReporterConfigMarshalAsBytes(config *reporter_v1.Config) ([]byte, error) {
	return proto.Marshal(config)
}

// Capturer 表示一个数据采集器实体
// 该结构体映射数据库中的 capturer 表，用于存储采集器的元数据信息
// 采集器通过命名空间、名称和版本唯一标识
type Capturer struct {
	database.BaseModel
	// Namespace 所属命名空间关联模型，通过 NamespaceName 关联
	Namespace Namespace `gorm:"foreignKey:NamespaceName;references:Name"`
	// NamespaceName 所属命名空间名称
	NamespaceName string `gorm:"type:VARCHAR(32)"`
	// Name 采集器名称
	Name string `gorm:"type:VARCHAR(32)"`
	// Version 采集器版本
	Version string `gorm:"type:VARCHAR(32)"`
}

// CreateWithConfig 创建采集器记录并存储其配置到 etcd
// 该方法在一个数据库事务中执行，确保数据库记录和 etcd 配置的一致性
// 参数:
//   - db: 数据库连接实例
//   - client: etcd 客户端实例
//   - cfg: reporter 配置对象
//
// 返回:
//   - error: 创建过程中的错误，包括数据库操作错误和 etcd 操作错误
//
// 注意:
//   - 当前实现存在潜在的一致性问题：etcd Put 操作成功后，数据库事务可能因程序崩溃而回滚
//   - 后续需要使用 saga 模式来保证跨系统的事务一致性
func (c *Capturer) CreateWithConfig(db *database.DB, client *etcd.EtcdClient, cfg *reporter_v1.Config) error {
	return db.Transaction(func(tx *database.DB) error {
		key := ReporterConfigKey(&reporter_v1.Resource{
			Namespace: c.NamespaceName,
			Name:      c.Name,
			Version:   c.Version,
		})
		if bytes, err := ReporterConfigMarshalAsBytes(cfg); err != nil {
			return errors.WithStack(err)
		} else if err = tx.Create(c).Error; err != nil {
			return errors.WithStack(err)
			// TODO: 事实上，这里的 Put 操作成功后，tx.Create 也有可能因为程序崩溃而回滚。
			// 而此时 Put 成功，而 Capturer 记录却不存在，这是不一致的状态。
			// 而事实上光靠事务是无法完全保证完整性，后续要使用 saga 模式来保证。
		} else if _, err = client.Put(tx.Statement.Context, key, string(bytes)); err != nil {
			return errors.WithStack(err)
		}
		return nil
	})
}

// UpdateConfig 更新采集器的配置到 etcd
// 该方法仅更新 etcd 中的配置，不修改数据库中的采集器记录
// 参数:
//   - ctx: 上下文对象，用于控制请求的生命周期
//   - client: etcd 客户端实例
//   - cfg: 新的 reporter 配置对象
//
// 返回:
//   - error: 更新过程中的错误，包括序列化错误和 etcd 操作错误
func (c *Capturer) UpdateConfig(ctx context.Context, client *etcd.EtcdClient, cfg *reporter_v1.Config) error {
	key := ReporterConfigKey(&reporter_v1.Resource{
		Namespace: c.NamespaceName,
		Name:      c.Name,
		Version:   c.Version,
	})
	if bytes, err := ReporterConfigMarshalAsBytes(cfg); err != nil {
		return errors.WithStack(err)
	} else if _, err = client.Put(ctx, key, string(bytes)); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// DeleteWithConfig 删除采集器记录及其在 etcd 中的配置
// 该方法在一个数据库事务中执行，确保数据库记录和 etcd 配置的一致性删除
// 参数:
//   - db: 数据库连接实例
//   - client: etcd 客户端实例
//
// 返回:
//   - error: 删除过程中的错误，包括数据库操作错误和 etcd 操作错误
//
// 注意:
//   - 先删除数据库记录，再删除 etcd 配置，确保即使 etcd 删除失败，数据库记录已被删除
//   - 与 CreateWithConfig 类似，存在跨系统事务一致性问题
func (c *Capturer) DeleteWithConfig(db *database.DB, client *etcd.EtcdClient) error {
	return db.Transaction(func(tx *database.DB) error {
		key := ReporterConfigKey(&reporter_v1.Resource{
			Namespace: c.NamespaceName,
			Name:      c.Name,
			Version:   c.Version,
		})
		if err := tx.Delete(c).Error; err != nil {
			return errors.WithStack(err)
		}
		if _, err := client.Delete(db.Statement.Context, key); err != nil {
			return errors.WithStack(err)
		}
		return nil
	})
}

// GetConfig 从 etcd 获取采集器的配置
// 该方法查询 etcd 中存储的采集器配置，并将其反序列化为配置对象
// 参数:
//   - ctx: 上下文对象，用于控制请求的生命周期
//   - client: etcd 客户端实例
//
// 返回:
//   - *reporter_v1.Config: 采集器配置对象，如果配置不存在则返回 nil
//   - error: 获取过程中的错误，包括 etcd 操作错误和反序列化错误
//
// 注意:
//   - 如果配置不存在，返回 (nil, nil) 而不是错误
//   - 该方法仅从 etcd 读取配置，不涉及数据库操作
func (c *Capturer) GetConfig(ctx context.Context, client *etcd.EtcdClient) (*reporter_v1.Config, error) {
	key := ReporterConfigKey(&reporter_v1.Resource{
		Namespace: c.NamespaceName,
		Name:      c.Name,
		Version:   c.Version,
	})
	if rsp, err := client.Get(ctx, key); err != nil {
		return nil, errors.WithStack(err)
	} else if len(rsp.Kvs) == 0 {
		return nil, nil
	} else if cfg, err := UnmarshalAsReporterConfig(rsp.Kvs[0].Value); err != nil {
		return nil, errors.WithStack(err)
	} else {
		return cfg, nil
	}
}

// GetStatusList 从 etcd 获取采集器的状态列表
// 该方法查询 etcd 中存储的所有采集器状态记录，使用前缀匹配获取所有会话状态
// 参数:
//   - ctx: 上下文对象，用于控制请求的生命周期
//   - client: etcd 客户端实例
//
// 返回:
//   - []*reporter_v1.Status: 状态对象列表，如果没有状态记录则返回 nil
//   - error: 获取过程中的错误，包括 etcd 操作错误和反序列化错误
//
// 注意:
//   - 使用 clientv3.WithPrefix() 选项获取所有以状态列表键为前缀的键值对
//   - 每个状态记录对应一个采集器会话
//   - 如果没有任何状态记录，返回 (nil, nil) 而不是错误
func (c *Capturer) GetStatusList(ctx context.Context, client *etcd.EtcdClient) ([]*reporter_v1.Status, error) {
	key := ReporterStatusListKey(&reporter_v1.Resource{
		Namespace: c.NamespaceName,
		Name:      c.Name,
		Version:   c.Version,
	})
	rsp, err := client.Get(
		ctx,
		key,
		clientv3.WithPrefix(),
	)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if len(rsp.Kvs) == 0 {
		return nil, nil
	}
	statusList := make([]*reporter_v1.Status, 0, len(rsp.Kvs))
	for _, kv := range rsp.Kvs {
		if status, err := UnmarshalAsReporterStatus(kv.Value); err != nil {
			return nil, errors.WithStack(err)
		} else {
			statusList = append(statusList, status)
		}
	}
	return statusList, nil
}
