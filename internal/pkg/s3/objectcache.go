// Package s3（本文件）提供通用的「S3 加密对象缓存」ObjectCache。
// 它负责把任意数据源上传的第三方文件（如 sqlite .db）以 zstd+AES-GCM 加密长期存于 S3
// （复用 Zkcn 流式管道），写入/删除随数据源配置生命周期管理，读取时按需拉取解密为本地
// 可供打开的文件。该机制不绑定任何具体数据源类型，便于后续第三方数据源接入复用。
//
// 读取时为满足「尽量不加载到本地磁盘」，Materialize 按 size 双路落地：
//   - size <= 内存阈值：落入内存缓存目录（Linux 默认 /dev/shm，tmpfs 即 RAM，不占本地盘）；
//   - size > 内存阈值：落入磁盘缓存目录（os.TempDir 子目录），TTL 到期回收。
//
// 本地文件以 objectKey 哈希为确定性名，进程内按 TTL 缓存复用，避免重复拉取 S3。
// 进程崩溃残留由 CleanupOrphans 在启动时清理：内存与磁盘缓存目录均为本模块专属子目录，
// 直接移除目录内全部文件即可。
//
// This file provides ObjectCache, a generic S3-encrypted object cache. It stores arbitrary
// third-party files uploaded by datasources (e.g. a sqlite .db) encrypted in S3 with
// zstd+AES-GCM (reusing the Zkcn streaming pipeline), writing/removing them together with
// the datasource config lifecycle, and materializing them on demand to a locally-openable
// file when reading. The mechanism is not bound to any datasource type, so it can be reused
// by third-party datasources later.
//
// To avoid loading onto the local disk when reading, Materialize routes by size:
//   - size <= memory threshold: written into the in-memory cache dir (default /dev/shm on
//     Linux, a tmpfs that is RAM, not persistent disk);
//   - size > threshold: spilled to a disk cache dir (os.TempDir subdir) recycled by TTL.
//
// Local files use a deterministic objectKey hash as name, cached per objectKey with a TTL to
// avoid re-fetching S3. Crash leftovers are cleaned by CleanupOrphans at startup: both cache
// dirs are dedicated to this module, so every entry inside them is removed.
package s3

import (
	// stdlib
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	// third-party

	// internal
	"github.com/creasty/defaults"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// objectCachePrefix 本模块缓存目录的共同名称组件（prefix）：内存与磁盘缓存目录都由它拼接
// 而来——Linux 内存目录为 /dev/shm/{prefix}，其余平台及磁盘缓存目录为 os.TempDir()/{prefix}。
// 因两个缓存目录均默认落于该 prefix 构成且专属本模块的子目录，启动清理可安全移除目录内
// 全部文件，无需逐文件甄别。
// objectCachePrefix is the shared name component of this module's cache dirs: both the memory
// and disk cache dirs are derived from it (Linux memory dir /dev/shm/{prefix}, disk and other
// memory dirs os.TempDir()/{prefix}). Since both dirs live under this prefix's dedicated
// subdir by default, startup cleanup can safely remove every entry inside, with no per-file
// filtering.
const objectCachePrefix = "zhongkui-objcache"

// defaultObjectCacheMemDir 返回平台相关的内存缓存目录：Linux 优先 /dev/shm/{prefix}（tmpfs
// 即内存），其余平台回退系统临时目录下的 prefix 专属子目录。
// defaultObjectCacheMemDir returns the platform-dependent in-memory cache dir: Linux prefers
// /dev/shm/{prefix} (a tmpfs that is RAM); others fall back to the prefix-dedicated subdir
// under the OS temp dir.
func defaultObjectCacheMemDir() string {
	if runtime.GOOS == "linux" {
		return filepath.Join("/dev/shm", objectCachePrefix)
	}
	return filepath.Join(os.TempDir(), objectCachePrefix)
}

// ObjectCache 负责把数据源上传的文件加密存于 S3，读取时按需拉取解密为本地文件。
// ObjectCache stores datasource-uploaded files encrypted in S3 and materializes them to
// local files on demand when reading.
type ObjectCache struct {
	// client 底层的 S3 客户端（nil 表示 S3 未配置）。
	// client the underlying S3 client (nil means S3 is not configured).
	client *S3Client
	// fetch 拉取+解密对象明文读取流的实现（默认 fetchFromS3，测试可注入替换）。
	// fetch the object fetch+decrypt implementation (defaults to fetchFromS3; injectable).
	fetch func(ctx context.Context, objectKey string) (io.ReadCloser, error)
	// memDir 内存缓存目录（tmpfs，如 Linux /dev/shm），承载 <= 阈值的小文件。
	// memDir the in-memory cache dir (tmpfs, e.g. /dev/shm) for files <= threshold.
	memDir string
	// diskDir 磁盘缓存目录，承载 > 阈值的大文件（TTL 回收）。
	// diskDir the disk cache dir for files > threshold (TTL-recycled).
	diskDir string
	// thresh 内存优先阈值（字节）。
	// thresh the memory-first threshold in bytes.
	thresh int64
	// ttl 本地缓存存活时长。
	// ttl the local cache TTL.
	ttl time.Duration

	// mu 保护 files。
	// mu guards files.
	mu sync.Mutex
	// files objectKey → 本地缓存条目。
	// files objectKey → local cache entry.
	files map[string]cacheEntry
}

// cacheEntry 描述一个已落地的本地缓存文件。
// cacheEntry describes a materialized local cache file.
type cacheEntry struct {
	path     string
	lastUsed time.Time
}

// NewObjectCache 以 S3 客户端与 Config 的通用对象缓存项构建对象缓存。缓存默认值（内存目录、
// 内存优先阈值 64MB、本地缓存 TTL 1h）统一由 Config 的 default tag 经 defaults 库注入，
// 不再手写分支；内存目录为空时按平台默认，不可写时回退磁盘缓存目录。client 可为 nil
// （S3 未启用，此时 Store/Materialize/Remove 会返回「S3 未配置」错误）。
// NewObjectCache builds an ObjectCache from an S3 client and the generic object cache options
// in Config. Cache defaults (mem dir, 64MB memory-first threshold, 1h local-cache TTL) are
// injected from Config's default tags via the defaults library instead of hand-written
// branches; an empty mem dir falls back to the platform default and an unwritable one to the
// disk cache dir. client may be nil (S3 disabled, in which case Store/Materialize/Remove
// return a "S3 not configured" error).
func NewObjectCache(client *S3Client, cfg Config) *ObjectCache {
	// 与项目约定一致：默认值统一经 defaults 库的 default tag 注入，而非手写 if-else。
	if err := defaults.Set(&cfg); err != nil {
		panic("s3 objectcache: apply defaults: " + err.Error())
	}
	c := &ObjectCache{
		client:  client,
		memDir:  cfg.ObjectCacheDir,
		diskDir: filepath.Join(os.TempDir(), objectCachePrefix),
		thresh:  int64(cfg.ObjectCacheMemoryThresholdMB) << 20,
		ttl:     cfg.ObjectCacheTTL,
		files:   make(map[string]cacheEntry),
	}
	if c.memDir == "" {
		c.memDir = defaultObjectCacheMemDir()
	}
	// 内存目录不可写时回退磁盘缓存目录，避免内存优先策略在受限环境直接失败。
	if err := os.MkdirAll(c.memDir, 0o700); err != nil {
		c.memDir = c.diskDir
	}
	if err := os.MkdirAll(c.diskDir, 0o700); err != nil {
		panic(err)
	}
	return c
}

// Store 把 reader 中的明文内容流式加密上传到 S3（复用 Zkcn 流式上传，零磁盘暂存）。
// 对象永久存储（无 TTL），生命周期随数据源配置管理。
// Store streams and encrypts the reader's plaintext content into S3 (reusing Zkcn's
// streaming upload, zero disk staging). The object is stored permanently (no TTL), with a
// lifecycle tied to the datasource config.
func (c *ObjectCache) Store(ctx context.Context, objectKey string, r io.Reader) error {
	if c == nil || c.client == nil {
		return errors.New("s3 objectcache: S3 not configured to store object")
	}
	if _, err := NewZkcn(c.client, objectKey, nil, 0).Store(ctx, r); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// fetchFromS3 从 S3 下载并解密对象，返回明文读取流。
// fetchFromS3 downloads and decrypts the S3 object, returning a plaintext reader.
func (c *ObjectCache) fetchFromS3(ctx context.Context, objectKey string) (io.ReadCloser, error) {
	rc, err := NewZkcn(c.client, objectKey, nil, 0).Load(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if rc == nil {
		return nil, errors.Newf("s3 objectcache: object %q not found", objectKey)
	}
	return rc, nil
}

// Materialize 确保 objectKey 对应的明文已落地为本地可用文件，返回其路径。
// 先命中 TTL 缓存复用；未缓存则拉取解密，按 size 决定落内存目录还是磁盘目录。
// 进程内并发安全。
// Materialize ensures the plaintext of objectKey is materialized as a local file and returns
// its path. A live TTL cache hit is reused; otherwise it fetches, decrypts and writes to the
// mem or disk cache dir by size. Safe for concurrent use.
func (c *ObjectCache) Materialize(ctx context.Context, objectKey string) (string, error) {
	if c == nil {
		return "", errors.New("s3 objectcache: unavailable")
	}
	if path, hit := c.cached(objectKey); hit {
		return path, nil
	}
	fetch := c.fetch
	if fetch == nil {
		if c.client == nil {
			return "", errors.New("s3 objectcache: S3 not configured to load object")
		}
		fetch = c.fetchFromS3
	}
	rc, err := fetch(ctx, objectKey)
	if err != nil {
		return "", err
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return "", errors.WithStack(err)
	}
	dir := c.diskDir
	if int64(len(data)) <= c.thresh {
		dir = c.memDir
	}
	full := filepath.Join(dir, hashObjectKey(objectKey))
	if err := os.WriteFile(full, data, 0o600); err != nil {
		return "", errors.WithStack(err)
	}
	c.setCache(objectKey, full)
	return full, nil
}

// Remove 删除 S3 中 objectKey 对应的对象并失效本地缓存。用于数据源删除时清理无归属文件。
// Remove deletes the S3 object for objectKey and invalidates the local cache. Used to clean
// up ownerless files when a datasource is deleted.
func (c *ObjectCache) Remove(ctx context.Context, objectKey string) error {
	if c == nil || c.client == nil {
		return errors.New("s3 objectcache: S3 not configured to remove object")
	}
	c.Invalidate(objectKey)
	return errors.WithStack(NewZkcn(c.client, objectKey, nil, 0).Clear(ctx))
}

// Invalidate 仅失效本地缓存文件（不触碰 S3 对象）。供注销/更新后释放内存目录占用。
// Invalidate drops only the local cached file (leaving the S3 object untouched), to free
// memory-dir space after unregister/update.
func (c *ObjectCache) Invalidate(objectKey string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.files[objectKey]; ok {
		_ = os.Remove(e.path)
		delete(c.files, objectKey)
	}
}

// cached 返回未过期的本地缓存路径；缓存过期或文件缺失时清理并返回未命中。
// cached returns a live local cache path; on TTL expiry or missing file it cleans up and
// reports a miss.
func (c *ObjectCache) cached(objectKey string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.files[objectKey]
	if !ok {
		return "", false
	}
	if time.Since(e.lastUsed) > c.ttl {
		_ = os.Remove(e.path)
		delete(c.files, objectKey)
		return "", false
	}
	if _, err := os.Stat(e.path); err != nil {
		delete(c.files, objectKey)
		return "", false
	}
	e.lastUsed = time.Now()
	c.files[objectKey] = e
	return e.path, true
}

// setCache 记录 objectKey 的本地缓存路径。
// setCache records the local cache path of objectKey.
func (c *ObjectCache) setCache(objectKey, path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.files[objectKey] = cacheEntry{path: path, lastUsed: time.Now()}
}

// CleanupOrphans 清理崩溃/异常退出残留的本地缓存文件。内存与磁盘缓存目录均为本模块专属
// 子目录，故移除目录内全部文件即安全；建议在进程启动时调用。
// CleanupOrphans removes leftover local cache files from crashes/abnormal exits. Since both
// cache dirs are dedicated to this module, removing every entry inside them is safe; call it
// at process startup.
func (c *ObjectCache) CleanupOrphans() {
	if c == nil {
		return
	}
	for _, dir := range []string{c.memDir, c.diskDir} {
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
	c.mu.Lock()
	c.files = make(map[string]cacheEntry)
	c.mu.Unlock()
}

// hashObjectKey 取 objectKey 的 SHA-256 前 16 个十六进制字符作为本地文件名，保证目录内
// 命名确定性且不泄漏原文。
// hashObjectKey takes the first 16 hex chars of objectKey's SHA-256 as the local filename,
// keeping names deterministic and not leaking the raw key.
func hashObjectKey(objectKey string) string {
	sum := sha256.Sum256([]byte(objectKey))
	return hex.EncodeToString(sum[:])[:16]
}
