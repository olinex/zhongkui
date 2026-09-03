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
// Package resourcepool provides a resource-agnostic lifecycle-management base for
// third-party resources. It wraps github.com/jolestar/go-commons-pool/v2 as the
// underlying object-pool engine and offers generic capabilities: independent pools
// keyed by an opaque Key that uniformly handle the full lifecycle of "lazy create
// (Make) → acquire → validate (testOnBorrow) → release → idle eviction (Close) →
// graceful shutdown (CloseAll)". An upper layer only needs to supply a Make
// constructor and optionally have the resource type implement Lifespan to get
// reuse, automatic rebuild-on-stale, and idle reclamation.
//
// This base is decoupled from messages, notifications, and concrete resource types:
// the message base uses it to manage channel instances (such as SMTP connections),
// and other future domains (e.g. Redis/gRPC connections or external HTTP clients)
// can reuse it directly.
//
// 适用范围（Scope）：本基座面向「有明确生命周期的外部资源连接」——需要借出校验、
// 闲置驱逐、随 Open/Validate/Close 钩子管理状态的连接型对象。以下对象不应接入：
//   - 库自托管的连接池（GORM/sql.DB、go-redis、etcd、S3/minio 等）——底层已管理连接池，再套一层属反模式；
//   - 单例长连接（如经 DI 长期持有的 gRPC ClientConn）——单条复用已满足，无需池化；
//   - 轻量无状态对象回收（sync.Pool 语义）——无 Open/Validate/Close 生命周期，应由 sync.Pool 承担。
//     The scope of this base is limited to connection-like resources with a clear
//     lifecycle (borrow-time validation, idle eviction, Open/Validate/Close hooks).
//     The following should NOT use it: library-managed pools (GORM/sql.DB, go-redis,
//     etcd, S3/minio), singleton long-lived connections (e.g. a DI-held gRPC
//     ClientConn), and lightweight stateless object recycling (sync.Pool semantics).
package resourcepool

import (
	// stdlib
	"context"
)

// Lifespan 可选生命周期钩子接口。被池化的资源类型可实现该接口，以便资源池在
// 创建、校验与销毁时驱动其底层外部资源（如 SMTP 长连接）的开闭与探活。
// 未实现该接口的资源类型按「纯对象复用 + 始终可用（Validate 恒 true）」处理，
// Destroy 时若类型同时实现 io.Closer 则调用其 Close。
// Lifespan is an optional lifecycle-hook interface. A pooled resource type may
// implement it so the pool drives the open/close and liveness probing of the
// underlying external resource (e.g. a long-lived SMTP connection) on create,
// validate and destroy. A type that does not implement it is treated as "pure
// object reuse + always valid (Validate always true)"; on destroy, if the type
// also implements io.Closer, its Close is called.
type Lifespan interface {
	// Open 惰性建立底层外部资源。在资源对象首次被 Make 创建后立即调用一次；
	// 返回错误将导致本次借出失败并销毁该对象。
	// Open lazily establishes the underlying external resource. It is called once,
	// right after the resource object is created by Make; a returned error fails
	// the current borrow and destroys the object.
	Open(ctx context.Context) error
	// Validate 校验底层外部资源是否仍可用。借出时由 testOnBorrow 触发；
	// 返回 false 表示资源已失效（如连接被服务端断开），池会销毁并重建该对象。
	// Validate reports whether the underlying external resource is still usable.
	// It is triggered by testOnBorrow on borrow; false means the resource is stale
	// (e.g. the connection was dropped by the server) and the pool destroys and
	// rebuilds the object.
	Validate(ctx context.Context) bool
	// Close 销毁底层外部资源。在闲置驱逐与 CloseAll 时调用以释放句柄/内存。
	// Close destroys the underlying external resource, releasing handles/memory
	// on idle eviction and CloseAll.
	Close() error
}

// Key 资源池标识。上层决定其编码内容（如渠道类型 + 规范化配置），
// 语义上不同的配置应映射到不同 Key，以维护相互独立的连接池。
// Key identifies a pool. The upper layer decides its encoding (e.g. channel type
// plus canonicalized config); semantically different configurations should map to
// different keys so that independent connection pools are maintained.
type Key string

// Make 依据 Key 惰性构建一个资源对象。资源池仅在首次需要时调用（不预热），
// 后续复用池中已有对象。返回的 T 若实现 Lifespan，其 Open 会被立即调用。
// Make lazily builds a resource object from a Key. The pool calls it only on first
// demand (no warm-up) and reuses existing objects afterwards. If the returned T
// implements Lifespan, its Open is invoked immediately.
type Make[T any] func(key Key) (T, error)
