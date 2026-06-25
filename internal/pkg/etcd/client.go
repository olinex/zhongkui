// Package etcd 提供了 etcd 客户端的配置管理与连接创建功能。
// 该包封装了 etcd client/v3 客户端的配置结构，支持 TLS 安全连接，
// 并提供了配置描述和配置提取的便捷方法，方便从 viper 配置中加载 etcd 相关配置。
package etcd

import (
	"context"
	"fmt"
	"strings"

	clientv3 "go.etcd.io/etcd/client/v3"
	g "google.golang.org/grpc"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

// EtcdClient 是 etcd client/v3 客户端的类型别名，指向 clientv3.Client。
// 使用类型别名便于在项目内统一引用，同时保留 clientv3.Client 的全部功能。
type EtcdClient = clientv3.Client

// WithPrefix 为 etcd 键添加全局前缀。
// 该函数将项目全局前缀（base.GLOBAL_PREFIX）与用户提供的 key 拼接，
// 生成格式为 "<GLOBAL_PREFIX>/<key>" 的完整键路径，用于隔离不同应用的 etcd 命名空间。
//
// 参数 key 必须满足以下约束，否则将触发 panic：
//   - 不能以斜杠 '/' 开头（避免与全局前缀拼接后产生双斜杠）
//   - 必须以斜杠 '/' 结尾（确保键代表一个目录前缀，而非叶子节点）
//
// 参数:
//   - key: 用户定义的键前缀，必须不以 '/' 开头且以 '/' 结尾
//
// 返回值:
//   - string: 拼接全局前缀后的完整键路径，格式为 "<GLOBAL_PREFIX>/<key>"
//
// 示例:
//
//	WithPrefix("myapp/")  -> "zki/myapp/"  (假设 GLOBAL_PREFIX = "zki")
//	WithPrefix("config/") -> "zki/config/"
func WithPrefix(key string) string {
	if strings.HasPrefix(key, "/") {
		panic(fmt.Sprintf(`Etcd key %s cannot startswith slash '/'`, key))
	}
	if !strings.HasSuffix(key, "/") {
		panic(fmt.Sprintf(`Etcd key %s must endswith slash '/'`, key))
	}
	return fmt.Sprintf(`%s/%s`, base.GLOBAL_PREFIX, key)
}

// NewEtcdClient 根据提供的配置创建并返回一个新的 etcd 客户端实例。
//
// 支持通过 dialOpts 注入额外的 gRPC 拨号选项（如 OpenTelemetry 拦截器）。
//
// 创建流程:
//  1. 调用 cfg.ToEtcdConfig() 将内部配置转换为 etcd SDK 配置
//  2. 注入 dialOpts 到 DialOptions 中
//  3. 调用 clientv3.New() 根据配置创建客户端连接
//  4. 所有错误均通过 errors.WithStack 包装，保留完整的调用栈信息
//
// 参数:
//   - ctx: 上下文对象，用于控制 etcd 客户端的生命周期和取消操作
//   - cfg: etcd 客户端配置，包含端点、认证、TLS 等信息
//   - logger: 日志记录器，会传递给 etcd SDK 用于内部日志输出
//   - dialOpts: 额外的 gRPC 拨号选项（可选），如 otelgrpc 拦截器
//
// 返回值:
//   - *EtcdClient: 成功时返回 etcd 客户端实例
//   - error: 创建过程中的错误，包括配置转换失败或连接建立失败
func NewEtcdClient(ctx context.Context, cfg *Config, logger *logging.Logger, dialOpts ...g.DialOption) (*EtcdClient, error) {
	ecfg, err := cfg.ToEtcdConfig(ctx, logger)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	ecfg.DialOptions = append(ecfg.DialOptions, dialOpts...)
	client, err := clientv3.New(ecfg)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return client, nil
}
