// 限流器：与业务无关的 Redis 固定窗口计数器，供各业务域按自选维度键复用。
package redis

import (
	"context"
	"time"

	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// incrementScript 以 INCR 累加窗口计数，并在首次计数时设置窗口过期时间。
// 「计数 + 设过期」置于同一脚本内原子执行：若拆成两条命令下发，二者之间连接中断或进程退出
// 会让计数器永不设置过期时间，该维度键被永久封禁。
const incrementScript = `
local count = redis.call('INCR', KEYS[1])
if count == 1 then
	redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return count
`

// FixedWindowLimiter 基于 Redis 的固定窗口计数器限流器。
//
// 以固定长度的时间窗口统计「维度键」对应的请求次数，超过上限即判定为超限。
// 计数落在 Redis 而非进程内，使多实例部署下各节点共享同一份计数；
// 本类型不感知业务语义，维度键由调用方按业务含义生成。
type FixedWindowLimiter struct {
	// client 承载窗口计数器的 Redis 客户端。
	client *Client
	// limit 单个窗口内允许的请求次数上限。
	limit int
	// window 窗口时长，窗口届满后计数自动清零重新起算。
	window time.Duration
}

// NewFixedWindowLimiter 创建固定窗口限流器。
//
// 参数：
//   - client：承载窗口计数器的 Redis 客户端
//   - limit：单个窗口内允许的请求次数上限
//   - window：窗口时长
func NewFixedWindowLimiter(client *Client, limit int, window time.Duration) *FixedWindowLimiter {
	return &FixedWindowLimiter{client: client, limit: limit, window: window}
}

// Allow 以 key 为维度统计当前窗口内的请求次数并判断是否放行。
//
// 参数：
//   - ctx：请求上下文
//   - key：限流维度键，调用方负责按业务语义生成
//
// 返回值：
//   - bool：true 表示未超限可放行，false 表示已达上限
//   - error：Redis 访问失败时的错误；失败时是否放行由调用方决定
func (l *FixedWindowLimiter) Allow(ctx context.Context, key string) (bool, error) {
	count, err := l.client.Eval(ctx, incrementScript, []string{key}, l.window.Milliseconds()).Int64()
	if err != nil {
		return false, errors.WithStack(err)
	}
	return count <= int64(l.limit), nil
}
