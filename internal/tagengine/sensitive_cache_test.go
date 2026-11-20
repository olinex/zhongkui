package tagengine

import (
	"sync"
	"sync/atomic"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

// TestRevealCacheSingleFlight 验证同一字段在一次识别缓存内只执行一次解密，
// 并发调用由 sync.Once 阻塞等待首个结果。
func TestRevealCacheSingleFlight(t *testing.T) {
	Convey("Test reveal cache single flight", t, func() {
		cache := newRevealCache()
		var calls int32
		release := make(chan struct{})
		started := make(chan struct{}, 8)
		fn := func() (string, error) {
			<-release
			atomic.AddInt32(&calls, 1)
			return "plain", nil
		}

		var wg sync.WaitGroup
		results := make([]string, 8)
		errs := make([]error, 8)
		for i := range results {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				started <- struct{}{}
				results[i], errs[i] = cache.do("body", fn)
			}(i)
		}
		// 等全部 goroutine 进入缓存调用后再放行，确保真正并发
		for range results {
			<-started
		}
		close(release)
		wg.Wait()

		So(atomic.LoadInt32(&calls), ShouldEqual, 1)
		for i := range results {
			So(errs[i], ShouldBeNil)
			So(results[i], ShouldEqual, "plain")
		}

		Convey("再次调用命中缓存，不再执行", func() {
			value, err := cache.do("body", fn)
			So(err, ShouldBeNil)
			So(value, ShouldEqual, "plain")
			So(atomic.LoadInt32(&calls), ShouldEqual, 1)
		})

		Convey("不同字段各自独立计算", func() {
			value, err := cache.do("auth", func() (string, error) { return "token", nil })
			So(err, ShouldBeNil)
			So(value, ShouldEqual, "token")
		})
	})
}
