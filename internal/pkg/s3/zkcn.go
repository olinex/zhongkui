// Package s3 提供与S3兼容对象存储服务交互的客户端封装
package s3

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"zhongkuisec.com/zhongkui/internal/pkg/crypto/combine"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// zkcnTempPrefix 是 Zkcn 临时文件的前缀，用于标识和清理 crash 遗留的临时文件。
// 应用程序应在启动时调用 CleanupOrphanedTempFiles 清理上次异常退出残留的文件。
const zkcnTempPrefix = "zkcn-upload-"

// Zkcn 是钟馗加密临时存储（Zhongkui Crypto Notation）的客户端封装。
// 与 S3Client 的核心区别：
//   - Zkcn 始终使用压缩+加密管道（zstd + AES-GCM），数据落盘必定是加密的
//   - 支持设置对象过期时间（TTL），到期后由 S3 服务端自动清理
//   - 每个 Zkcn 实例绑定一个固定的 S3 对象键（objectKey）和加密密钥（cryptoKey）
//
// 典型使用场景：任务临时文件、抓包结果暂存等需要加密存储且可自动过期清理的数据。
type Zkcn struct {
	// client 是底层的 S3 客户端封装，负责实际的 S3 协议通信
	client *S3Client

	// objectKey 是存储对象在 S3 中的唯一标识键
	// 每个 Zkcn 实例对应一个固定的 S3 对象路径
	objectKey string

	// cryptoKey 是用于加密/解密对象内容的对称密钥
	// 传入 combine.NewZstdAESGCMPipeline 构造加密管道
	// 如果为 nil，则使用 S3Client 的默认 pipeline（即从 Config 派生的密钥）
	cryptoKey []byte

	// timeout 是对象的生存时间（Time-To-Live），仅对上传时生效
	// 如果 > 0，Store 时会设置 PutObjectOptions.Expires，到期后 S3 服务端自动删除对象
	// 如果 <= 0，对象永久存储
	timeout time.Duration
}

// NewZkcn 创建一个 Zkcn 加密临时存储实例。
//
// 参数：
//   - client: 已初始化的 S3Client 实例，提供底层的 S3 操作能力
//   - objectKey: 对象在 S3 存储桶中的唯一键名，如 "temp/result.json"
//   - cryptoKey: 对称加密密钥，用于 AES-GCM 加密，长度须为 16/24/32 字节
//     如果为 nil，则回退使用 S3Client 初始化时的默认 pipeline 密钥
//   - timeout: 对象 TTL，用于设置在 Store 时对象的过期时间
//     如果 <= 0，对象不会自动过期
//
// 返回值：
//   - *Zkcn: 新创建的加密临时存储实例
//
// 示例：
//
//	zkcn := s3.NewZkcn(client, "temp/output.json", myCryptoKey, 1*time.Hour)
//	info, err := zkcn.Store(ctx, dataReader)
func NewZkcn(client *S3Client, objectKey string, cryptoKey []byte, timeout time.Duration) *Zkcn {
	return &Zkcn{
		client:    client,
		objectKey: objectKey,
		cryptoKey: cryptoKey,
		timeout:   timeout,
	}
}

// Store 将数据读取器中的内容经过压缩加密后，上传到 S3 对象存储。
//
// 根据 Config.StreamingUpload 选择两种上传策略：
//   - StreamingUpload=true（默认）: 使用 io.Pipe 流式管道 + multipart/chunked encoding 直传
//     适用于成熟的 S3 实现（AWS S3、MinIO），零磁盘暂存
//   - StreamingUpload=false: 加密到临时文件后以已知大小简单 PUT 上传
//     适用于 multipart upload 支持不完整的 S3 实现（如 RustFS Alpha）
//
// 参数：
//   - ctx: 上下文，用于控制上传的生命周期和超时取消
//   - reader: 要上传的原始数据读取器，支持任意大小
//
// 返回值：
//   - minio.UploadInfo: 上传信息，包含 ETag、版本 ID、上传时间等
//   - error: 上传或加密失败时返回的错误
func (t *Zkcn) Store(ctx context.Context, reader io.Reader) (minio.UploadInfo, error) {
	opts := minio.PutObjectOptions{
		ContentType:  combine.CompressContentType,
		AutoChecksum: minio.ChecksumSHA256,
	}
	if t.timeout > 0 {
		opts.Expires = time.Now().Add(t.timeout)
	}

	if t.client.cfg.StreamingUpload {
		// 流式策略：通过 io.Pipe 边加密边上传，零磁盘暂存
		return t.client.CPutObject(ctx, t.objectKey, t.cryptoKey, reader, -1, opts)
	}

	// 降级策略：加密到临时文件，以已知大小上传
	return t.storeViaTempFile(ctx, reader, opts)
}

// storeViaTempFile 将 reader 的内容流式加密写入临时文件，再以已知大小上传。
//
// 临时文件生命周期保证：
//   - 正常返回／error 返回：defer os.Remove 自动清理
//   - panic：defer os.Remove 在 unwind 时执行
//   - 进程崩溃（SIGKILL／电源故障）：文件残留在 OS 临时目录，
//     应用应在启动时调用 CleanupOrphanedTempFiles() 清理上次残留
//
// 实现流式磁盘暂存而非全量内存缓冲，适用于不支持 multipart upload 的 S3 服务端。
func (t *Zkcn) storeViaTempFile(ctx context.Context, reader io.Reader, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
	var pipeline *combine.Pipeline
	if t.cryptoKey == nil {
		pipeline = t.client.pipeline
	} else if tmp, err := combine.NewZstdAESGCMPipeline(t.cryptoKey); err != nil {
		return minio.UploadInfo{}, errors.WithStack(err)
	} else {
		pipeline = tmp
	}

	tmpFile, err := os.CreateTemp("", zkcnTempPrefix+"*.tmp")
	if err != nil {
		return minio.UploadInfo{}, errors.WithStack(err)
	}
	tmpPath := tmpFile.Name()
	// defer 保证函数在任何返回路径（正常/error/panic）下都会删除临时文件。
	// 唯一无法覆盖的场景是进程被 SIGKILL 或电源故障——此时需依赖 CleanupOrphanedTempFiles。
	defer os.Remove(tmpPath)

	w, err := pipeline.CompressEncrypt(tmpFile)
	if err != nil {
		tmpFile.Close()
		return minio.UploadInfo{}, errors.WithStack(err)
	}
	if _, err := io.Copy(w, reader); err != nil {
		w.Close()
		tmpFile.Close()
		return minio.UploadInfo{}, errors.WithStack(err)
	}
	if err := w.Close(); err != nil {
		tmpFile.Close()
		return minio.UploadInfo{}, errors.WithStack(err)
	}
	// FPutObject 需要可读的文件，必须在调用前关闭 write fd
	if err := tmpFile.Close(); err != nil {
		return minio.UploadInfo{}, errors.WithStack(err)
	}

	return t.client.FPutObject(ctx, t.objectKey, tmpPath, opts)
}

// Load 从 S3 对象存储中下载并解密解压指定对象，返回明文数据的读取流。
//
// 内部流程：
//  1. 调用 S3Client.CGetOject 获取对象
//  2. CGetOject 内部会检测对象的加密头部（ZKCN 魔数），自动解密解压
//  3. 如果对象不存在或为空，返回 (nil, nil)
//
// 参数：
//   - ctx: 上下文，用于控制下载的生命周期和超时取消
//
// 返回值：
//   - io.ReadCloser: 解密解压后的明文数据读取流，调用者使用完毕后必须关闭
//   - error: 下载或解密失败时返回的错误
//
// 注意：返回的 ReadCloser 包含解密层和源 S3 对象层，Close() 会逐层关闭。
func (t *Zkcn) Load(ctx context.Context) (io.ReadCloser, error) {
	return t.client.CGetOject(ctx, t.objectKey, t.cryptoKey, minio.GetObjectOptions{})
}

// Clear 从 S3 存储桶中删除 Zkcn 关联的对象。
// 用于清理过期或不再需要的临时存储数据。
//
// 参数：
//   - ctx: 上下文，用于控制删除操作的生命周期和超时取消
//
// 返回值：
//   - error: 删除失败时返回的错误，如对象不存在、权限不足或网络错误
//     错误会通过 errors.WithStack 包装，保留完整的调用栈信息
//
// 注意：删除操作是不可逆的，调用前请确认数据不再需要。
func (t *Zkcn) Clear(ctx context.Context) error {
	err := t.client.RemoveObject(ctx, t.objectKey, minio.RemoveObjectOptions{})
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// Exists 判断 Zkcn 关联的 S3 对象是否已存在。
// 用于内容寻址（content-addressed）场景下跳过重复上传，节省带宽与存储。
//
// 参数：
//   - ctx: 上下文，用于控制查询的生命周期和取消
//
// 返回值：
//   - bool: 对象存在返回 true，不存在返回 false
//   - error: 查询失败（对象不存在以外的错误）时返回错误
func (t *Zkcn) Exists(ctx context.Context) (bool, error) {
	_, err := t.client.StatObject(ctx, t.objectKey, minio.StatObjectOptions{})
	if err == nil {
		return true, nil
	}
	if IsNotExists(err) {
		return false, nil
	}
	return false, errors.WithStack(err)
}

// NewStagingTempFile 创建一个可被 CleanupOrphanedTempFiles 清理的本地暂存文件。
// 用于"先落盘、后上传"的场景（如先计算内容摘要再决定是否上传），
// 崩溃遗留的暂存文件会在应用启动时由 CleanupOrphanedTempFiles 统一清理。
func NewStagingTempFile() (*os.File, error) {
	return os.CreateTemp("", zkcnTempPrefix+"*.tmp")
}

// CleanupOrphanedTempFiles 清理因进程异常退出（SIGKILL／崩溃）而残留的临时文件。
//
// 遍历 OS 临时目录（os.TempDir()），删除所有以 "zkcn-upload-" 为前缀的 .tmp 文件。
// 建议在应用启动时调用，例如在 main 或 PersistentPreRunE 中：
//
//	if n, err := s3.CleanupOrphanedTempFiles(); err != nil {
//	    log.Warnf("failed to cleanup orphaned temp files: %v", err)
//	} else if n > 0 {
//	    log.Infof("cleaned up %d orphaned temp file(s)", n)
//	}
//
// 返回值：
//   - int: 成功删除的残留文件数量
//   - error: 读取目录失败时返回错误（删除单个文件失败仅记录警告，不中断）
func CleanupOrphanedTempFiles() (int, error) {
	dir := os.TempDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("read temp dir %s: %w", dir, err)
	}

	var cleaned int
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, zkcnTempPrefix) || !strings.HasSuffix(name, ".tmp") {
			continue
		}
		path := filepath.Join(dir, name)
		if err := os.Remove(path); err != nil {
			// 删除失败不中断，可能是权限问题或文件正在使用
			// 大部分场景下 OS 定期清理机制（tmpwatch/systemd-tmpfiles）会兜底
			continue
		}
		cleaned++
	}
	return cleaned, nil
}
