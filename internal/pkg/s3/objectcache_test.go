// Package s3（本文件）提供通用对象缓存 ObjectCache 的单测。
// 以注入的 fetch 免真实 S3：验证对象键按 size 分路落地（内存/磁盘目录）、进程内缓存复用
// 与 TTL 失效、Invalidate/Remove 清理，以及未配置 S3 时的明确报错。
package s3

import (
	// stdlib
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	// third-party
	. "github.com/smartystreets/goconvey/convey"
)

// fakeBackend 是测试用的 objectBackend：以函数注入读/写/删，避免依赖真实 S3。
// fakeBackend is a test objectBackend: read/write/remove are injected as functions so tests need
// no real S3.
type fakeBackend struct {
	fetch  func(ctx context.Context, objectKey string) (io.ReadCloser, error)
	store  func(ctx context.Context, objectKey string, r io.Reader) error
	remove func(ctx context.Context, objectKey string) error
}

func (b fakeBackend) Store(ctx context.Context, objectKey string, r io.Reader) error {
	if b.store == nil {
		return nil
	}
	return b.store(ctx, objectKey, r)
}

func (b fakeBackend) Fetch(ctx context.Context, objectKey string) (io.ReadCloser, error) {
	if b.fetch == nil {
		return nil, errors.New("fetch not stubbed")
	}
	return b.fetch(ctx, objectKey)
}

func (b fakeBackend) Remove(ctx context.Context, objectKey string) error {
	if b.remove == nil {
		return nil
	}
	return b.remove(ctx, objectKey)
}

// newObjectCacheForTest 以注入的 fetch 构建通用对象缓存（免真实 S3），供单元测试使用。
// newObjectCacheForTest builds an ObjectCache with an injected fetch (no real S3) for tests.
func newObjectCacheForTest(fetch func(ctx context.Context, objectKey string) (io.ReadCloser, error), cfg Config) *ObjectCache {
	return newObjectCache(fakeBackend{fetch: fetch}, cfg)
}

func TestObjectCacheMaterialize(t *testing.T) {
	Convey("Materialize 按 size 分路落地", t, func() {
		base, err := os.MkdirTemp("", "zk-objcache-")
		So(err, ShouldBeNil)
		// 阈值 1MB：小文件（4B）落内存目录，大文件（超阈且 > 阈值+1，覆盖「前缀缓冲+剩余流」路径）
		// 超阈落磁盘目录。
		cache := newObjectCacheForTest(func(_ context.Context, objectKey string) (io.ReadCloser, error) {
			if strings.HasSuffix(objectKey, "/big") {
				return io.NopCloser(strings.NewReader(string(make([]byte, (1<<20)+4096)))), nil
			}
			return io.NopCloser(strings.NewReader("ABCD")), nil
		}, Config{
			ObjectCacheDir:               filepath.Join(base, "mem"),
			ObjectCacheMemoryThresholdMB: 1,
		})

		Convey("小文件（<= 阈值）落入内存目录，大文件超阈落盘且内容完整", func() {
			small, err := cache.Materialize(context.Background(), "datasource/ns/small")
			So(err, ShouldBeNil)
			So(filepath.Dir(small), ShouldEqual, cache.memDir)

			big, err := cache.Materialize(context.Background(), "datasource/ns/big")
			So(err, ShouldBeNil)
			So(filepath.Dir(big), ShouldEqual, cache.diskDir)
			// 流式分路：已缓冲前缀与剩余流拼接后不得丢失或错位。
			got, err := os.ReadFile(big)
			So(err, ShouldBeNil)
			So(len(got), ShouldEqual, (1<<20)+4096)
		})

		Convey("进程内缓存复用：未过期不重复拉取", func() {
			p1, err := cache.Materialize(context.Background(), "datasource/ns/small")
			So(err, ShouldBeNil)
			p2, err := cache.Materialize(context.Background(), "datasource/ns/small")
			So(err, ShouldBeNil)
			So(p1, ShouldEqual, p2) // 同一对象键复用同一缓存文件
		})
	})

	Convey("Invalidate 失效缓存后重拉，TTL 过期后同样重拉", t, func() {
		base, _ := os.MkdirTemp("", "zk-objcache-")

		Convey("Invalidate 使下次 Materialize 重新拉取", func() {
			fetches := 0
			cache := newObjectCacheForTest(func(_ context.Context, _ string) (io.ReadCloser, error) {
				fetches++
				return io.NopCloser(strings.NewReader("payload")), nil
			}, Config{
				ObjectCacheDir:               filepath.Join(base, "mem"),
				ObjectCacheMemoryThresholdMB: 1024, // 大阈值保证测试文件落内存目录
			})
			ctx := context.Background()
			_, err := cache.Materialize(ctx, "datasource/ns/cache")
			So(err, ShouldBeNil)
			cache.Invalidate("datasource/ns/cache")
			_, err = cache.Materialize(ctx, "datasource/ns/cache")
			So(err, ShouldBeNil)
			So(fetches, ShouldEqual, 2)
		})

		Convey("TTL 过期后下次 Materialize 重新拉取", func() {
			fetches := 0
			cache := newObjectCacheForTest(func(_ context.Context, _ string) (io.ReadCloser, error) {
				fetches++
				return io.NopCloser(strings.NewReader("payload")), nil
			}, Config{
				ObjectCacheDir:               filepath.Join(base, "mem2"),
				ObjectCacheMemoryThresholdMB: 1024,
				ObjectCacheTTL:               time.Nanosecond, // 极短 TTL：第二次调用必然过期
			})
			ctx := context.Background()
			_, err := cache.Materialize(ctx, "datasource/ns/ttl")
			So(err, ShouldBeNil)
			time.Sleep(time.Millisecond)
			_, err = cache.Materialize(ctx, "datasource/ns/ttl")
			So(err, ShouldBeNil)
			So(fetches, ShouldEqual, 2)
		})
	})

	Convey("未配置 S3（client nil）时 Store/Materialize/Remove 明确报错", t, func() {
		base, _ := os.MkdirTemp("", "zk-objcache-")
		cache := NewObjectCache(nil, Config{
			ObjectCacheDir: filepath.Join(base, "mem"),
		})
		ctx := context.Background()
		_, err := cache.Materialize(ctx, "datasource/ns/x")
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "S3 not configured")
		So(cache.Store(ctx, "datasource/ns/x", strings.NewReader("p")), ShouldNotBeNil)
		So(cache.Remove(ctx, "datasource/ns/x"), ShouldNotBeNil)
	})
}

func TestObjectCacheOpen(t *testing.T) {
	Convey("Open 返回对象明文读取流且不落地本地文件", t, func() {
		base, _ := os.MkdirTemp("", "zk-objcache-")

		Convey("注入 fetch 时读到明文，且不写本地缓存", func() {
			cache := newObjectCacheForTest(func(_ context.Context, objectKey string) (io.ReadCloser, error) {
				So(objectKey, ShouldEqual, "datasource/ns/file")
				return io.NopCloser(strings.NewReader("plaintext")), nil
			}, Config{ObjectCacheDir: filepath.Join(base, "mem")})
			rc, err := cache.Open(context.Background(), "datasource/ns/file")
			So(err, ShouldBeNil)
			defer rc.Close()
			data, err := io.ReadAll(rc)
			So(err, ShouldBeNil)
			So(string(data), ShouldEqual, "plaintext")
			So(len(cache.files), ShouldEqual, 0) // Open 不写本地缓存表
		})

		Convey("fetch 报错时透传错误并返回 nil 流", func() {
			cache := newObjectCacheForTest(func(_ context.Context, _ string) (io.ReadCloser, error) {
				return nil, errors.New("boom")
			}, Config{ObjectCacheDir: filepath.Join(base, "mem2")})
			rc, err := cache.Open(context.Background(), "datasource/ns/file")
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "boom")
			So(rc, ShouldBeNil)
		})
	})

	Convey("nil 缓存或未配置 S3（client nil）时 Open 明确报错", t, func() {
		base, _ := os.MkdirTemp("", "zk-objcache-")
		cache := NewObjectCache(nil, Config{ObjectCacheDir: filepath.Join(base, "mem")})
		rc, err := cache.Open(context.Background(), "datasource/ns/x")
		So(err, ShouldNotBeNil)
		So(rc, ShouldBeNil)
		So(err.Error(), ShouldContainSubstring, "S3 not configured")

		var nilCache *ObjectCache
		_, err = nilCache.Open(context.Background(), "datasource/ns/x")
		So(err, ShouldNotBeNil)
	})
}

func TestObjectCleanupOrphans(t *testing.T) {
	Convey("CleanupOrphans 移除专属缓存目录内全部文件并重置缓存", t, func() {
		base, _ := os.MkdirTemp("", "zk-objcache-")
		cache := newObjectCacheForTest(func(_ context.Context, _ string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("payload")), nil
		}, Config{
			ObjectCacheDir:               filepath.Join(base, "mem"),
			ObjectCacheMemoryThresholdMB: 1024,
		})
		path, err := cache.Materialize(context.Background(), "datasource/ns/keep")
		So(err, ShouldBeNil)

		// 缓存目录为本模块专属，目录内文件（含并存文件）都会被整体清理。
		other := filepath.Join(cache.memDir, "other.bin")
		So(os.WriteFile(other, []byte("x"), 0o600), ShouldBeNil)

		cache.CleanupOrphans()
		_, statErr := os.Stat(path)
		So(statErr, ShouldNotBeNil) // 本模块残留被清理
		_, statErr = os.Stat(other)
		So(statErr, ShouldNotBeNil)          // 目录内并存文件一并清理
		So(len(cache.files), ShouldEqual, 0) // 缓存表清空
	})
}
