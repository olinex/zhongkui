// Package s3（本文件）提供通用的「S3 加密对象缓存」ObjectCache。
// 它负责把任意数据源上传的第三方文件（如 sqlite .db）以 zstd+AES-GCM 加密长期存于 S3
// （复用 Zkcn 流式管道），写入/删除随数据源配置生命周期管理，读取时按需拉取解密为本地
// 可供打开的文件。该机制不绑定任何具体数据源类型，便于后续第三方数据源接入复用。
//
// 读取时为满足「尽量不加载到本地磁盘」，Materialize 按 size 流式双路落地，分流过程内存占用不超过
// 阈值（不会把整个对象缓冲进内存）：
//   - size <= 内存阈值：落入内存缓存目录（Linux 默认 /dev/shm，tmpfs 即 RAM，不占本地盘）；
//   - size > 内存阈值：落入磁盘缓存目录（os.TempDir 子目录），TTL 到期回收。
//
// 本地文件以 objectKey 哈希为确定性名，进程内按 TTL 缓存复用，避免重复拉取 S3。
// 进程崩溃残留由 CleanupOrphans 在启动时清理：内存与磁盘缓存目录均为本模块专属子目录，
// 直接移除目录内全部文件即可。
package s3

import (
	// 标准库

	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	// 第三方

	// 项目内部
	"github.com/creasty/defaults"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// objectCachePrefix 本模块缓存目录的共同名称组件（prefix）：内存与磁盘缓存目录都由它拼接
// 而来——Linux 内存目录为 /dev/shm/{prefix}，其余平台及磁盘缓存目录为 os.TempDir()/{prefix}。
// 因两个缓存目录均默认落于该 prefix 构成且专属本模块的子目录，启动清理可安全移除目录内
// 全部文件，无需逐文件甄别。
const objectCachePrefix = "zhongkui-objcache"

// defaultObjectCacheMemDir 返回平台相关的内存缓存目录：Linux 优先 /dev/shm/{prefix}（tmpfs
// 即内存），其余平台回退系统临时目录下的 prefix 专属子目录。
func defaultObjectCacheMemDir() string {
	if runtime.GOOS == "linux" {
		return filepath.Join("/dev/shm", objectCachePrefix)
	}
	return filepath.Join(os.TempDir(), objectCachePrefix)
}

// objectBackend 抽象底层「加密对象存储」的读/写/删，使 ObjectCache 不直接依赖具体 *S3Client：
// 生产实现为 s3Backend（经 Zkcn 管道加解密读写 S3）；S3 未配置时以 disabledBackend 空对象替代
// 原先散落在各方法里的 nil client 判断；测试可注入内存实现。
type objectBackend interface {
	Store(ctx context.Context, objectKey string, r io.Reader) error
	Fetch(ctx context.Context, objectKey string) (io.ReadCloser, error)
	Remove(ctx context.Context, objectKey string) error
}

// s3Backend 是 objectBackend 的生产实现：对象经 Zkcn（zstd+AES-GCM）加密管道写入 S3，
// 读取时加密下载并解密为明文流。
type s3Backend struct {
	client *S3Client
}

// Store 经 Zkcn 加密管道把明文流写入 S3。
func (b s3Backend) Store(ctx context.Context, objectKey string, r io.Reader) error {
	if _, err := NewZkcn(b.client, objectKey, nil, 0).Store(ctx, r); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// Fetch 下载并解密对象，返回明文读取流；对象不存在时返回明确错误。
func (b s3Backend) Fetch(ctx context.Context, objectKey string) (io.ReadCloser, error) {
	rc, err := NewZkcn(b.client, objectKey, nil, 0).Load(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if rc == nil {
		return nil, errors.Newf("s3 objectcache: object %q not found", objectKey)
	}
	return rc, nil
}

// Remove 删除 S3 中的加密对象。
func (b s3Backend) Remove(ctx context.Context, objectKey string) error {
	return errors.WithStack(NewZkcn(b.client, objectKey, nil, 0).Clear(ctx))
}

// disabledBackend 是 S3 未配置时的空对象后端：三个方法各自返回带操作语义的「S3 未配置」错误，
// 使 Store/Materialize/Open/Remove 无需再各自判空。
type disabledBackend struct{}

// Store 返回「S3 未配置」错误。
func (disabledBackend) Store(context.Context, string, io.Reader) error {
	return errors.New("s3 objectcache: S3 not configured to store object")
}

// Fetch 返回「S3 未配置」错误。
func (disabledBackend) Fetch(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("s3 objectcache: S3 not configured to load object")
}

// Remove 返回「S3 未配置」错误。
func (disabledBackend) Remove(context.Context, string) error {
	return errors.New("s3 objectcache: S3 not configured to remove object")
}

// ObjectCache 负责把数据源上传的文件加密存于 S3，读取时按需拉取解密为本地文件。
type ObjectCache struct {
	// backend 底层加密对象存储的读写删端口（生产为 s3Backend，S3 未配置为 disabledBackend）。
	backend objectBackend
	// memDir 内存缓存目录（tmpfs，如 Linux /dev/shm），承载 <= 阈值的小文件。
	memDir string
	// diskDir 磁盘缓存目录，承载 > 阈值的大文件（TTL 回收）。
	diskDir string
	// thresh 内存优先阈值（字节）。
	thresh int64
	// ttl 本地缓存存活时长。
	ttl time.Duration

	// mu 保护 files。
	mu sync.Mutex
	// files objectKey → 本地缓存条目。
	files map[string]cacheEntry
}

// cacheEntry 描述一个已落地的本地缓存文件。
type cacheEntry struct {
	path     string
	lastUsed time.Time
}

// NewObjectCache 以 S3 客户端与 Config 的通用对象缓存项构建对象缓存。缓存默认值（内存目录、
// 内存优先阈值、本地缓存 TTL）统一由 Config 的 default tag 经 defaults 库注入，不再手写分支；
// 内存目录为空时按平台默认，不可写时回退磁盘缓存目录。client 为 nil 表示 S3 未启用，此时注入
// disabledBackend 空对象，Store/Materialize/Open/Remove 统一返回「S3 未配置」错误。
func NewObjectCache(client *S3Client, cfg Config) *ObjectCache {
	if client == nil {
		return newObjectCache(disabledBackend{}, cfg)
	}
	return newObjectCache(s3Backend{client: client}, cfg)
}

// newObjectCache 以给定后端端口构建对象缓存；NewObjectCache 与包内测试均经此构造，使测试可注入
// 内存实现替代真实 S3。
func newObjectCache(backend objectBackend, cfg Config) *ObjectCache {
	// 与项目约定一致：默认值统一经 defaults 库的 default tag 注入，而非手写 if-else。
	if err := defaults.Set(&cfg); err != nil {
		panic("s3 objectcache: apply defaults: " + err.Error())
	}
	c := &ObjectCache{
		backend: backend,
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
func (c *ObjectCache) Store(ctx context.Context, objectKey string, r io.Reader) error {
	if c == nil {
		return errors.New("s3 objectcache: unavailable")
	}
	return c.backend.Store(ctx, objectKey, r)
}

// Materialize 确保 objectKey 对应的明文已落地为本地可用文件，返回其路径。
// 先命中 TTL 缓存复用；未缓存则拉取解密，按 size 流式分路落地（内存占用不超过阈值）。
// 进程内并发安全。
func (c *ObjectCache) Materialize(ctx context.Context, objectKey string) (string, error) {
	if c == nil {
		return "", errors.New("s3 objectcache: unavailable")
	}
	if path, hit := c.cached(objectKey); hit {
		return path, nil
	}
	rc, err := c.backend.Fetch(ctx, objectKey)
	if err != nil {
		return "", errors.WithStack(err)
	}
	defer rc.Close()
	full, err := c.materialize(rc, objectKey)
	if err != nil {
		return "", errors.WithStack(err)
	}
	c.setCache(objectKey, full)
	return full, nil
}

// materialize 把明文流 rc 按 size 分路写入本地缓存文件并返回最终路径：先读至多「阈值+1」字节到
// 内存，未超阈即小文件，直接写内存目录（不触碰本地磁盘）；一旦超阈则把已缓冲前缀与剩余流写入
// 磁盘目录的暂存文件，再原子改名到最终路径。全程内存占用不超过阈值，不会把整个对象缓冲进内存。
func (c *ObjectCache) materialize(rc io.Reader, objectKey string) (string, error) {
	name := hashObjectKey(objectKey)

	var head bytes.Buffer
	if _, err := io.CopyN(&head, rc, c.thresh+1); err != nil && err != io.EOF {
		return "", errors.WithStack(err)
	}
	if int64(head.Len()) <= c.thresh {
		full := filepath.Join(c.memDir, name)
		if err := os.WriteFile(full, head.Bytes(), 0o600); err != nil {
			return "", errors.WithStack(err)
		}
		return full, nil
	}

	tmp, err := os.CreateTemp(c.diskDir, name+".tmp-*")
	if err != nil {
		return "", errors.WithStack(err)
	}
	tmpPath := tmp.Name()
	// 成功后 rename 已把该文件移走；失败路径由 defer 清理，避免残留半成品被后续误用。
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(head.Bytes()); err != nil {
		tmp.Close()
		return "", errors.WithStack(err)
	}
	if _, err := io.Copy(tmp, rc); err != nil {
		tmp.Close()
		return "", errors.WithStack(err)
	}
	if err := tmp.Close(); err != nil {
		return "", errors.WithStack(err)
	}
	full := filepath.Join(c.diskDir, name)
	if err := os.Rename(tmpPath, full); err != nil {
		return "", errors.WithStack(err)
	}
	return full, nil
}

// Open 拉取并解密 objectKey 对应的对象，返回明文读取流，供流式下载使用，避免整对象落地。
// 直接走底层后端端口，不写入本地缓存文件，调用方负责关闭返回的 ReadCloser。c 为 nil 或 S3
// 未配置时返回明确错误。
func (c *ObjectCache) Open(ctx context.Context, objectKey string) (io.ReadCloser, error) {
	if c == nil {
		return nil, errors.New("s3 objectcache: unavailable")
	}
	return c.backend.Fetch(ctx, objectKey)
}

// Remove 删除 S3 中 objectKey 对应的对象并失效本地缓存。用于数据源删除时清理无归属文件。
func (c *ObjectCache) Remove(ctx context.Context, objectKey string) error {
	if c == nil {
		return errors.New("s3 objectcache: unavailable")
	}
	c.Invalidate(objectKey)
	return c.backend.Remove(ctx, objectKey)
}

// Invalidate 仅失效本地缓存文件（不触碰 S3 对象）。供注销/更新后释放内存目录占用。
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
func (c *ObjectCache) setCache(objectKey, path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.files[objectKey] = cacheEntry{path: path, lastUsed: time.Now()}
}

// CleanupOrphans 清理崩溃/异常退出残留的本地缓存文件。内存与磁盘缓存目录均为本模块专属
// 子目录，故移除目录内全部文件即安全；建议在进程启动时调用。
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
func hashObjectKey(objectKey string) string {
	sum := sha256.Sum256([]byte(objectKey))
	return hex.EncodeToString(sum[:])[:16]
}
