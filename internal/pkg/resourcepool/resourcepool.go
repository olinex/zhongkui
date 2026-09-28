// Package resourcepool 提供不依赖具体业务的第三方资源生命周期管理基座。
// 它封装 github.com/jolestar/go-commons-pool/v2 作为底层对象池引擎，为上层提供
// 泛型能力：按不透明 Key 维护相互独立的对象池，统一处理「惰性创建（Make）→ 借出
// （Acquire）→ 校验（Validate，testOnBorrow）→ 归还（Release）→ 闲置驱逐（Close）
// → 优雅关闭（CloseAll）」的完整生命周期。上层只需提供 Make 构造函数并让资源类型
// 可选实现 Lifespan 接口，即可获得资源复用、失效自动重建与闲置回收。
//
// 本基座与消息、通知、具体资源类型完全解耦：消息基座用其管理渠道实例（SMTP 长连接等），
// 未来其他业务（如 Redis/grpc 连接、外部 HTTP 客户端）也可直接复用。
//
// 适用范围（Scope）：本基座面向「有明确生命周期的外部资源连接」——需要借出校验、
// 闲置驱逐、随 Open/Validate/Close 钩子管理状态的连接型对象。以下对象不应接入：
//   - 库自托管的连接池（GORM/sql.DB、go-redis、etcd、S3/minio 等）——底层已管理连接池，再套一层属反模式；
//   - 单例长连接（如经 DI 长期持有的 gRPC ClientConn）——单条复用已满足，无需池化；
//   - 轻量无状态对象回收（sync.Pool 语义）——无 Open/Validate/Close 生命周期，应由 sync.Pool 承担。
package resourcepool

import (
	// 标准库
	"context"
)

// Lifespan 可选生命周期钩子接口。被池化的资源类型可实现该接口，以便资源池在
// 创建、校验与销毁时驱动其底层外部资源（如 SMTP 长连接）的开闭与探活。
// 未实现该接口的资源类型按「纯对象复用 + 始终可用（Validate 恒 true）」处理，
// Destroy 时若类型同时实现 io.Closer 则调用其 Close。
type Lifespan interface {
	// Open 惰性建立底层外部资源。在资源对象首次被 Make 创建后立即调用一次；
	// 返回错误将导致本次借出失败并销毁该对象。
	Open(ctx context.Context) error
	// Validate 校验底层外部资源是否仍可用。借出时由 testOnBorrow 触发；
	// 返回 false 表示资源已失效（如连接被服务端断开），池会销毁并重建该对象。
	Validate(ctx context.Context) bool
	// Close 销毁底层外部资源。在闲置驱逐与 CloseAll 时调用以释放句柄/内存。
	Close() error
}

// Key 资源池标识。上层决定其编码内容（如渠道类型 + 规范化配置），
// 语义上不同的配置应映射到不同 Key，以维护相互独立的连接池。
type Key string

// Make 依据 Key 惰性构建一个资源对象。资源池仅在首次需要时调用（不预热），
// 后续复用池中已有对象。返回的 T 若实现 Lifespan，其 Open 会被立即调用。
type Make[T any] func(key Key) (T, error)
