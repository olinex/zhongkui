package message

import (
	// stdlib
	"context"
	"encoding/json"
	"sort"
	"strings"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/resourcepool"
)

// 本文件基于独立的 generic 资源池基座 resourcepool 提供消息渠道的统一生命周期入口：
// 按「渠道类型 + 规范化渠道级配置」维护相互独立的连接池，Send 走统一的借出/归还路径，
// 使 SMTP 等有状态渠道复用底层连接、失效时自动重建、闲置时被定时回收。
// 进程优雅关闭时调用 ClosePools 释放全部存活连接。

// channelManager 消息渠道生命周期管理器：资源类型为 message.Channel，
// 依据 Key（类型 + 规范化配置）惰性建池并由 go-commons-pool 托管连接复用与回收。
// channelManager is the channel lifecycle manager over message.Channel: it lazily
// builds pools keyed by (type + canonicalized config) and lets go-commons-pool
// handle connection reuse and reclamation.
var channelManager = resourcepool.NewManager(func(key resourcepool.Key) (Channel, error) {
	typ, config := decodeKey(key)
	// 复用既有工厂注册表：未注册类型/配置非法原样返回错误
	return New(typ, config)
}, resourcepool.NewDefaultConfig())

// keySep 分隔渠道类型与配置的边界字节。JSON 不包含 NUL 字节，故用作安全分隔符。
// keySep delimits the channel type from its config. JSON never contains a NUL byte,
// so it is a safe separator.
const keySep = "\x00"

// makeKey 构造资源池 Key：规范化渠道级配置后与类型拼接，使语义相同的配置复用同一池。
// makeKey builds the pool Key: it canonicalizes the channel-level config and joins it
// with the type so semantically identical configs share the same pool.
func makeKey(typ string, config json.RawMessage) resourcepool.Key {
	return resourcepool.Key(typ + keySep + canonicalJSON(config))
}

// decodeKey 从资源池 Key 还原渠道类型与渠道级配置（makeKey 的逆运算）。
// decodeKey recovers the channel type and channel-level config from the Key
// (the inverse of makeKey).
func decodeKey(key resourcepool.Key) (string, json.RawMessage) {
	raw := string(key)
	if before, after, ok := strings.Cut(raw, keySep); ok {
		return before, json.RawMessage(after)
	}
	return raw, json.RawMessage(`{}`)
}

// canonicalJSON 将渠道级配置规范化为稳定字节序的 JSON（map 键升序），
// 保证字段顺序不同的等价配置复用同一连接池。
// canonicalJSON normalizes the channel-level config into a stable-byte-order JSON
// (map keys sorted) so equivalent configs with different field orders share a pool.
func canonicalJSON(config json.RawMessage) string {
	if len(config) == 0 {
		return "{}"
	}
	var obj map[string]any
	if err := json.Unmarshal(config, &obj); err != nil {
		// 非法 JSON 按原文参与构建（工厂将在 New 阶段报错）
		return string(config)
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		kv, _ := json.Marshal(k)
		vv, _ := json.Marshal(obj[k])
		b.Write(kv)
		b.WriteByte(':')
		b.Write(vv)
	}
	b.WriteByte('}')
	return b.String()
}

// Send 经统一资源池向目标投递渲染后的消息：借出与配置匹配的渠道实例并复用底层连接，
// 发送完成后归还；渠道实例借出（含建连/校验）失败或发送失败均原样返回错误。
// 发送失败仍归还对象，由下一次借出的 testOnBorrow 校验并在底层连接失效后自动重建。
// Send delivers the rendered message to the target through the unified pool: it
// borrows a channel instance matched to the config (reusing the underlying
// connection), sends, then returns the instance. A borrow failure (including
// connect/validation) or a send failure is returned as-is. A send failure still
// returns the object; the next borrow validates it via testOnBorrow and rebuilds
// the underlying connection once it is stale.
func Send(ctx context.Context, typ string, config json.RawMessage, target *Target, msg *RenderedMessage) (string, error) {
	key := makeKey(typ, config)
	ch, err := channelManager.Acquire(ctx, key)
	if err != nil {
		return "", err
	}
	defer channelManager.Release(ctx, key, ch)
	return ch.Send(ctx, target, msg)
}

// ClosePools 关闭全部渠道资源池并销毁存活连接，供进程优雅关闭时调用。
// ClosePools closes all channel pools and destroys live connections; call it on
// graceful process shutdown.
func ClosePools(ctx context.Context) {
	channelManager.CloseAll(ctx)
}
