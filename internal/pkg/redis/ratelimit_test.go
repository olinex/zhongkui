package redis

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	. "github.com/smartystreets/goconvey/convey"
)

// newTestFixedWindowLimiter 用内存版 Redis（miniredis）构造限流器，使窗口计数无需真实 Redis 即可生效，
// 并返回内存实例以便测试快进时钟驱动窗口过期。Redis 客户端与内存实例随测试结束自动关闭。
func newTestFixedWindowLimiter(t *testing.T, limit int, window time.Duration) (*FixedWindowLimiter, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := NewClient(&Config{Type: NormalType, Addr: mr.Addr()})
	t.Cleanup(func() {
		// 此处已脱离 goconvey 断言上下文，直接忽略关闭错误即可
		_ = client.Shutdown()
	})
	return NewFixedWindowLimiter(client, limit, window), mr
}

// TestFixedWindowLimiter_Allow 测试固定窗口限流器的放行判定：上限边界、维度独立性与窗口过期重置。
func TestFixedWindowLimiter_Allow(t *testing.T) {
	Convey("测试 FixedWindowLimiter 的 Allow", t, func() {
		ctx := context.Background()

		Convey("同一维度键在窗口内前 limit 次放行，其后拒绝", func() {
			limiter, _ := newTestFixedWindowLimiter(t, 2, time.Hour)

			allowed, err := limiter.Allow(ctx, "dimension-key")
			So(err, ShouldBeNil)
			So(allowed, ShouldBeTrue)

			allowed, err = limiter.Allow(ctx, "dimension-key")
			So(err, ShouldBeNil)
			So(allowed, ShouldBeTrue)

			// 第 limit+1 次达到上限被拒绝
			allowed, err = limiter.Allow(ctx, "dimension-key")
			So(err, ShouldBeNil)
			So(allowed, ShouldBeFalse)
		})

		Convey("不同维度键各自独立计数", func() {
			limiter, _ := newTestFixedWindowLimiter(t, 1, time.Hour)

			allowed, err := limiter.Allow(ctx, "key-a")
			So(err, ShouldBeNil)
			So(allowed, ShouldBeTrue)

			// 另一维度键不受 key-a 计数影响
			allowed, err = limiter.Allow(ctx, "key-b")
			So(err, ShouldBeNil)
			So(allowed, ShouldBeTrue)

			allowed, err = limiter.Allow(ctx, "key-a")
			So(err, ShouldBeNil)
			So(allowed, ShouldBeFalse)
		})

		Convey("窗口届满后计数清零重新起算", func() {
			limiter, mr := newTestFixedWindowLimiter(t, 1, time.Minute)

			allowed, err := limiter.Allow(ctx, "dimension-key")
			So(err, ShouldBeNil)
			So(allowed, ShouldBeTrue)

			allowed, err = limiter.Allow(ctx, "dimension-key")
			So(err, ShouldBeNil)
			So(allowed, ShouldBeFalse)

			// 快进超过窗口时长，键过期后计数重新起算
			mr.FastForward(time.Minute + time.Second)

			allowed, err = limiter.Allow(ctx, "dimension-key")
			So(err, ShouldBeNil)
			So(allowed, ShouldBeTrue)
		})
	})
}
