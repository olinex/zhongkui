// Package s3 提供与S3兼容对象存储服务交互的客户端封装
// 基于minio-go库实现，支持Amazon S3、MinIO等兼容S3协议的对象存储服务
// 主要功能包括：对象上传、下载、删除、列表查询、对象信息获取等
package s3

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"hash"
	"io"
	"net/http"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"zhongkuisec.com/zhongkui/internal/pkg/crypto/combine"
	"zhongkuisec.com/zhongkui/internal/pkg/crypto/key"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// ClientOption 是 S3 客户端的可选配置函数类型。
// 用于在创建 S3Client 时注入自定义配置，如自定义 HTTP Transport。
type ClientOption func(*minio.Options)

// WithTransport 返回一个 ClientOption，用于设置 S3 客户端的底层 HTTP Transport。
//
// 参数:
//   - rt: 自定义的 http.RoundTripper 实现，用于替代默认的 http.DefaultTransport
//
// 使用场景:
//   - 集成 OpenTelemetry 链路追踪
//   - 添加自定义 HTTP 中间件
//   - 配置代理或限流等
//
// 示例:
//
//	client, err := s3.NewS3Client(cfg, s3.WithTransport(s3.NewOtelRoundTripper(http.DefaultTransport)))
func WithTransport(rt http.RoundTripper) ClientOption {
	return func(opts *minio.Options) {
		opts.Transport = rt
	}
}

// S3Client 是S3兼容对象存储服务的客户端封装
// 所有操作都使用初始化时指定的默认存储桶
type S3Client struct {
	// cli 是底层的minio.Client实例
	// 负责实际的S3协议通信
	cli *minio.Client

	// bucket 是默认的存储桶名称
	// 所有操作默认在此存储桶中执行
	bucket string

	// endpoint 是S3服务的端点地址，用于构建 BPF 过滤器以排除上传流量
	// 格式如 "localhost:9000" 或 "s3.amazonaws.com"
	endpoint string

	// pipeline 是加密解密管道，用于对上传下载的对象进行加密解密
	pipeline *combine.Pipeline

	// cfg 是创建此客户端的配置副本，供 Zkcn 等内部组件查询上传策略
	cfg Config
}

// GetEndpoint 返回S3服务的端点地址
// 用于上层构建 BPF 过滤规则，避免抓包时捕获到自身的上传流量
func (s *S3Client) GetEndpoint() string {
	return s.endpoint
}

// NewS3Client 创建并初始化一个新的S3Client实例
// 根据提供的配置连接到S3兼容对象存储服务
//
// 参数:
//   - cfg: S3配置，包含端点、认证信息、存储桶等
//
// 返回值:
//   - *S3Client: 初始化成功的S3客户端实例
//   - error: 连接失败时返回的错误，使用errors.WithStack包装
//
// 示例:
//
//	cfg := &s3.Config{
//	    Endpoint:  "http://localhost:9000",
//	    AccessKey: "minioadmin",
//	    SecretKey: "minioadmin",
//	    Bucket:    "my-bucket",
//	    UseSSL:    false,
//	}
//	client, err := s3.NewS3Client(cfg)
//	if err != nil {
//	    log.Fatal("Failed to create S3 client:", err)
//	}
func NewS3Client(cfg *Config, kd *key.KeyDerivation, opts ...ClientOption) (*S3Client, error) {
	mOpts := &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	}
	for _, opt := range opts {
		opt(mOpts)
	}
	if client, err := minio.New(cfg.Endpoint, mOpts); err != nil {
		return nil, errors.WithStack(err)
	} else if cryptoKey, err := cfg.ExtractCryptoKey(kd); err != nil {
		return nil, errors.WithStack(err)
	} else if pipeline, err := combine.NewZstdAESGCMPipeline(cryptoKey); err != nil {
		return nil, errors.WithStack(err)
	} else {
		return &S3Client{
			cli:      client,
			bucket:   cfg.Bucket,
			endpoint: cfg.Endpoint,
			pipeline: pipeline,
			cfg:      *cfg,
		}, nil
	}
}

// GetObject 从S3存储桶中获取指定对象的读取流
// 返回一个minio.Object实例，可以通过该实例读取对象内容
// 注意：调用者有责任在读取完成后关闭返回的对象
//
// 参数:
//   - ctx: 上下文，用于控制请求的生命周期和取消
//   - objectKey: 对象键，即对象在存储桶中的唯一标识
//   - opts: 获取对象的选项，如服务器端加密、部分读取范围等
//
// 返回值:
//   - *minio.Object: 对象读取流，可以从中读取对象数据
//   - error: 获取对象失败时返回的错误，如对象不存在或权限不足
//
// 示例:
//
//	obj, err := client.GetObject(ctx, "my-file.txt", minio.GetObjectOptions{})
//	if err != nil {
//	    log.Fatal("Failed to get object:", err)
//	}
//	defer obj.Close()
//	data, err := io.ReadAll(obj)
//	if err != nil {
//	    log.Fatal("Failed to read object:", err)
//	}
func (s *S3Client) GetObject(ctx context.Context, objectKey string, opts minio.GetObjectOptions) (*minio.Object, error) {
	return s.cli.GetObject(ctx, s.bucket, objectKey, opts)
}

// CGetOject 从 S3 存储桶中获取加密对象并自动解密解压，返回明文读取流。
// C 前缀代表 Crypto（加密），区别于普通的 GetObject。
//
// 内部流程（if-else 级联）：
//  1. 调用 GetObject 获取 S3 对象流
//  2. 对获取的对象调用 Stat() 检查状态：
//     a. 如果对象不存在（NoSuchKey/NoSuchBucket），返回 (nil, nil)
//     b. 如果对象大小为 0（空对象），返回 (nil, nil)
//  3. 根据 cryptoKey 参数决定解密策略：
//     a. 如果 cryptoKey == nil，使用 S3Client 初始化时的默认 pipeline 解密
//     b. 如果 cryptoKey != nil，使用该密钥创建新的 pipeline 解密
//
// 参数：
//   - ctx: 上下文，用于控制请求的生命周期和取消
//   - objectKey: 对象键，即对象在存储桶中的唯一标识
//   - cryptoKey: 解密密钥，用于 AES-GCM 解密，长度须为 16/24/32 字节
//     如果为 nil，使用 S3Client 初始化时配置的默认解密管道
//   - opts: 获取对象的选项，可配置服务器端加密、版本 ID 等
//
// 返回值：
//   - io.ReadCloser: 解密解压后的明文数据读取流，调用者使用完毕后必须关闭
//     nil 表示对象不存在或为空（此时 error 也为 nil）
//   - error: 获取或解密失败时返回的错误
//
// 注意：
//   - 返回的 ReadCloser 关闭时会依次关闭解密器、解压器以及源 S3 对象流
//   - 调用者需要自行判断 nil 返回值来区分"对象不存在"和"解密失败"
func (s *S3Client) CGetOject(ctx context.Context, objectKey string, cryptoKey []byte, opts minio.GetObjectOptions) (io.ReadCloser, error) {
	if obj, err := s.GetObject(ctx, objectKey, opts); err != nil {
		return nil, errors.WithStack(err)
	} else if info, err := obj.Stat(); err != nil {
		if IsNotExists(err) {
			return nil, nil
		}
		return nil, errors.WithStack(err)
	} else if info.Size == 0 {
		return nil, nil
	} else if cryptoKey == nil {
		return s.pipeline.DecryptDecompress(obj)
	} else if pipeline, err := combine.NewZstdAESGCMPipeline(cryptoKey); err != nil {
		return nil, errors.WithStack(err)
	} else {
		return pipeline.DecryptDecompress(obj)
	}
}

// FGetObject 从S3存储桶下载对象到本地文件系统
// 将指定对象的内容直接保存到本地文件，适用于大文件下载
// 此方法会自动创建文件并写入对象内容，无需手动处理流
//
// 参数:
//   - ctx: 上下文，用于控制请求的生命周期和取消
//   - objectKey: 对象键，即对象在存储桶中的唯一标识
//   - filePath: 本地文件路径，对象内容将保存到此文件
//   - opts: 获取对象的选项，如服务器端加密、部分读取范围等
//
// 返回值:
//   - error: 下载失败时返回的错误，如对象不存在、权限不足或文件系统错误
//
// 示例:
//
//	err := client.FGetObject(ctx, "large-file.zip", "/tmp/large-file.zip", minio.GetObjectOptions{})
//	if err != nil {
//	    log.Fatal("Failed to download object:", err)
//	}
func (s *S3Client) FGetObject(ctx context.Context, objectKey, filePath string, opts minio.GetObjectOptions) error {
	return s.cli.FGetObject(ctx, s.bucket, objectKey, filePath, opts)
}

// CPutObject 将数据读取器中的内容经过压缩加密后，以流式方式上传到 S3 存储桶。
// C 前缀代表 Crypto（加密），区别于普通的 PutObject。
//
// 内部流程（如果-否则 级联）：
//  1. 创建 io.Pipe 作为加密管道和 S3 上传之间的桥梁
//  2. 根据 cryptoKey 参数选择加密管道：
//     a. 如果 cryptoKey == nil，使用 S3Client 初始化时的默认 pipeline
//     b. 如果 cryptoKey != nil，使用该密钥创建新的 pipeline
//  3. 调用 pipeline.CompressEncrypt 创建加密写入器
//  4. 将加密后的数据通过 pipeReader 传递给 minio.PutObject 进行上传
//  5. io.Copy 将明文 reader 的数据流式写入加密管道
//
// 使用 defer 确保所有资源正确释放（pipeReader、pipeWriter、加密写入器、数据拷贝）。
//
// 参数：
//   - ctx: 上下文，用于控制上传的生命周期和取消
//   - objectKey: 对象键，即对象在存储桶中的唯一标识
//   - cryptoKey: 加密密钥，用于 AES-GCM 加密，长度须为 16/24/32 字节
//     如果为 nil，使用 S3Client 初始化时配置的默认加密管道
//   - reader: 要上传的原始数据读取器，通过管道流式加密后上传
//   - objectSize: 对象大小（字节数），-1 表示未知大小（流式上传）
//   - opts: 上传选项，如内容类型、元数据、服务器端加密等
//
// 返回值：
//   - minio.UploadInfo: 上传信息，包含 ETag、版本 ID、上传时间等
//   - error: 上传或加密失败时返回的错误
//
// 注意：
//   - 加密是流式进行的，不会将整个数据加载到内存中
//   - pipeReader 和 pipeWriter 分别由 minio 上传线程和加密线程并发操作
func (s *S3Client) CPutObject(ctx context.Context, objectKey string, cryptoKey []byte, reader io.Reader, objectSize int64, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
	pipeReader, pipeWriter := io.Pipe()
	var pipeline *combine.Pipeline
	if cryptoKey == nil {
		pipeline = s.pipeline
	} else if tmp, err := combine.NewZstdAESGCMPipeline(cryptoKey); err != nil {
		pipeReader.Close()
		pipeWriter.Close()
		return minio.UploadInfo{}, errors.WithStack(err)
	} else {
		pipeline = tmp
	}
	w, err := pipeline.CompressEncrypt(pipeWriter)
	if err != nil {
		pipeReader.Close()
		pipeWriter.Close()
		return minio.UploadInfo{}, errors.WithStack(err)
	}

	// 在 goroutine 中将 reader 的数据通过加密管道写入 pipeWriter。
	// 必须放在 PutObject 之前启动，因为 io.Pipe 是同步的：
	// 读端（pipeReader）被 PutObject 消费时，写端必须在另一个 goroutine 中并发写入，
	// 否则会形成死锁：PutObject 等数据 → defer io.Copy 等函数返回 → 函数等 PutObject 返回。
	//
	// 注意 defer 的执行顺序（LIFO）：
	//   - pipeWriter.Close() 必须在 w.Close() 之后执行
	//   - w.Close() 需要向 pipeWriter 写入 GCM 认证标签（finalization block）
	//   - 如果 pipeWriter 先关闭，w.Close() 会写入失败，导致数据不完整
	// copyErr 回传写侧（io.Copy / 加密写入器 Close）的错误：写侧失败会让上传数据不完整，
	// 必须与 PutObject 的错误一起上报，不能静默丢弃。
	copyErrCh := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(w, reader)
		if closeErr := w.Close(); copyErr == nil {
			copyErr = closeErr
		}
		if copyErr != nil {
			// 让 PutObject 侧的读端感知写端失败，避免其把截断数据当成完整对象上传。
			_ = pipeWriter.CloseWithError(copyErr)
		} else {
			_ = pipeWriter.Close()
		}
		copyErrCh <- copyErr
	}()

	uploadInfo, err := s.cli.PutObject(ctx, s.bucket, objectKey, pipeReader, objectSize, opts)
	// 先关闭读端以解除写侧可能的阻塞，再等待写侧收敛并取回其错误。
	_ = pipeReader.Close()
	copyErr := <-copyErrCh
	if err != nil {
		return minio.UploadInfo{}, errors.WithStack(err)
	}
	if copyErr != nil {
		return minio.UploadInfo{}, errors.WithStack(copyErr)
	}
	return uploadInfo, nil
}

// PutObject 将数据流上传到S3存储桶
// 从提供的io.Reader读取数据并上传到指定的对象键
// 适用于从内存、网络或其他流式源上传数据
//
// 参数:
//   - ctx: 上下文，用于控制请求的生命周期和取消
//   - objectKey: 对象键，即对象在存储桶中的唯一标识
//   - reader: 数据读取器，提供要上传的数据流
//   - objectSize: 对象大小（字节数），-1表示未知大小（流式上传）
//   - opts: 上传选项，如内容类型、元数据、服务器端加密等
//
// 返回值:
//   - minio.UploadInfo: 上传信息，包含ETag、版本ID（如果启用）、上传时间等
//   - error: 上传失败时返回的错误，如网络错误、权限不足或存储桶不存在
//
// 示例:
//
//	data := strings.NewReader("Hello, S3!")
//	info, err := client.PutObject(ctx, "hello.txt", data, int64(data.Len()), minio.PutObjectOptions{
//	    ContentType: "text/plain",
//	})
//	if err != nil {
//	    log.Fatal("Failed to upload object:", err)
//	}
//	fmt.Printf("Uploaded with ETag: %s\n", info.ETag)
func (s *S3Client) PutObject(ctx context.Context, objectKey string, reader io.Reader, objectSize int64, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
	return s.cli.PutObject(ctx, s.bucket, objectKey, reader, objectSize, opts)
}

// FPutObject 将本地文件上传到S3存储桶
// 读取指定路径的本地文件内容并上传到S3对象
// 适用于从文件系统直接上传文件，自动处理文件读取和上传
//
// 参数:
//   - ctx: 上下文，用于控制请求的生命周期和取消
//   - objectKey: 对象键，即对象在存储桶中的唯一标识
//   - filePath: 本地文件路径，从此文件读取内容上传
//   - opts: 上传选项，如内容类型、元数据、服务器端加密等
//
// 返回值:
//   - minio.UploadInfo: 上传信息，包含ETag、版本ID（如果启用）、上传时间等
//   - error: 上传失败时返回的错误，如文件不存在、读取错误、网络错误或权限不足
//
// 示例:
//
//	info, err := client.FPutObject(ctx, "backup.tar.gz", "/var/backups/backup.tar.gz", minio.PutObjectOptions{
//	    ContentType: "application/gzip",
//	})
//	if err != nil {
//	    log.Fatal("Failed to upload file:", err)
//	}
//	fmt.Printf("File uploaded successfully, size: %d bytes\n", info.Size)
func (s *S3Client) FPutObject(ctx context.Context, objectKey, filePath string, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
	return s.cli.FPutObject(ctx, s.bucket, objectKey, filePath, opts)
}

// RemoveObject 从S3存储桶中删除指定对象
// 永久删除存储桶中的对象，此操作不可逆
// 如果启用了版本控制，可能需要指定版本ID来删除特定版本
//
// 参数:
//   - ctx: 上下文，用于控制请求的生命周期和取消
//   - objectKey: 对象键，即要删除的对象在存储桶中的唯一标识
//   - opts: 删除选项，如版本ID、治理模式等
//
// 返回值:
//   - error: 删除失败时返回的错误，如对象不存在、权限不足或网络错误
//
// 示例:
//
//	err := client.RemoveObject(ctx, "old-file.txt", minio.RemoveObjectOptions{})
//	if err != nil {
//	    log.Fatal("Failed to remove object:", err)
//	}
//	fmt.Println("Object removed successfully")
func (s *S3Client) RemoveObject(ctx context.Context, objectKey string, opts minio.RemoveObjectOptions) error {
	return s.cli.RemoveObject(ctx, s.bucket, objectKey, opts)
}

// CopyObject 在默认桶内执行服务端复制，将源对象完整复制为新的目标对象。
// 对象为客户端加密密文（zkcn），复制保持字节不变，下载侧仍用同一密钥解密，无需在此加解密。
// 源对象不存在时返回 NoSuchKey 错误（可用 IsNotExists 判断）。
//
// 参数:
//   - ctx: 上下文，用于控制请求的生命周期和取消
//   - srcKey: 源对象键
//   - dstKey: 目标对象键
//
// 返回值:
//   - error: 复制失败时返回的错误，如源对象不存在、权限不足或网络错误
func (s *S3Client) CopyObject(ctx context.Context, srcKey, dstKey string) error {
	if _, err := s.cli.CopyObject(ctx,
		minio.CopyDestOptions{Bucket: s.bucket, Object: dstKey},
		minio.CopySrcOptions{Bucket: s.bucket, Object: srcKey},
	); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// ListObjects 列出S3存储桶中的对象
// 返回一个只读通道，通过该通道可以接收存储桶中的对象信息
// 通道会在所有对象列出后自动关闭，或者当上下文取消时提前关闭
//
// 参数:
//   - ctx: 上下文，用于控制请求的生命周期和取消
//   - opts: 列表选项，如前缀、分隔符、递归模式、起始标记等
//
// 返回值:
//   - <-chan minio.ObjectInfo: 对象信息通道，每个元素包含对象的键、大小、最后修改时间等元数据
//
// 示例:
//
//	objects := client.ListObjects(ctx, minio.ListObjectsOptions{
//	    Prefix:    "images/",
//	    Recursive: true,
//	})
//	for obj := range objects {
//	    fmt.Printf("Object: %s, Size: %d, LastModified: %v\n",
//	        obj.Key, obj.Size, obj.LastModified)
//	}
func (s *S3Client) ListObjects(ctx context.Context, opts minio.ListObjectsOptions) <-chan minio.ObjectInfo {
	return s.cli.ListObjects(ctx, s.bucket, opts)
}

// StatObject 获取S3对象的元数据信息
// 查询指定对象的详细信息，包括大小、ETag、最后修改时间、内容类型等
// 此操作不会下载对象内容，只获取对象的元数据
//
// 参数:
//   - ctx: 上下文，用于控制请求的生命周期和取消
//   - objectKey: 对象键，即要查询的对象在存储桶中的唯一标识
//   - opts: 查询选项，如版本ID、服务器端加密等
//
// 返回值:
//   - minio.ObjectInfo: 对象信息，包含对象的完整元数据
//   - error: 查询失败时返回的错误，如对象不存在、权限不足或网络错误
//
// 示例:
//
//	info, err := client.StatObject(ctx, "document.pdf", minio.StatObjectOptions{})
//	if err != nil {
//	    log.Fatal("Failed to stat object:", err)
//	}
//	fmt.Printf("Object size: %d bytes, Content-Type: %s, LastModified: %v\n",
//	    info.Size, info.ContentType, info.LastModified)
func (s *S3Client) StatObject(ctx context.Context, objectKey string, opts minio.StatObjectOptions) (minio.ObjectInfo, error) {
	return s.cli.StatObject(ctx, s.bucket, objectKey, opts)
}

// IsNotExists 判断错误是否表示 S3 对象或存储桶不存在（NoSuchKey / NoSuchBucket）。
func IsNotExists(err error) bool {
	if err == nil {
		return false
	}
	rsp := minio.ToErrorResponse(err)
	if rsp.Code == minio.NoSuchKey || rsp.Code == minio.NoSuchBucket {
		return true
	}
	return false
}

// etagVerifyingReader 对流经的数据计算 MD5，在读取完毕时自动校验 ETag 一致性。
// 包装一个 io.Reader，所有 Read 调用透传到底层 reader，同时将数据写入内部 MD5 哈希。
// 当底层 reader 返回 io.EOF 时，自动比对计算出的 MD5 与期望的 ETag。
type etagVerifyingReader struct {
	src         io.Reader
	expectedMD5 string
	objectKey   string
	hash        hash.Hash
	verified    bool
}

// Read 从底层 reader 读取数据并增量计算 MD5，读取结束时校验 ETag 一致性。
func (r *etagVerifyingReader) Read(p []byte) (n int, err error) {
	n, err = r.src.Read(p)
	if n > 0 {
		r.hash.Write(p[:n])
	}
	if err == io.EOF && !r.verified {
		r.verified = true
		computedMD5 := hex.EncodeToString(r.hash.Sum(nil))
		if computedMD5 != r.expectedMD5 {
			return n, errors.Newf(
				"S3 object integrity check failed for %s: expected ETag %s, computed MD5 %s",
				r.objectKey, r.expectedMD5, computedMD5,
			)
		}
	}
	return n, errors.WithStack(err)
}

// VerifyDownloadIntegrity 返回一个包装了 src 的 Reader，对流经数据进行 MD5 校验。
// 先通过 StatObject 获取 S3 对象存储的 ETag（单次上传中等于文件 MD5），
// 然后在流式读取过程中增量计算 MD5，读取完毕时自动与 ETag 比对。
//
// 所有数据均为流式处理，不会将整个对象加载到内存中。校验失败时，错误会在
// 底层 reader 返回 io.EOF 的同一 Read 调用中返回。
//
// 参数：
//   - ctx: 上下文
//   - objectKey: S3 对象键
//   - src: 已下载的对象数据流
//
// 返回值：
//   - io.Reader: 包装后的 Reader，可直接传递给下游消费者
//   - error: StatObject 失败时返回错误
func (s *S3Client) VerifyDownloadIntegrity(ctx context.Context, objectKey string, src io.Reader) (io.Reader, error) {
	stat, err := s.StatObject(ctx, objectKey, minio.StatObjectOptions{})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &etagVerifyingReader{
		src:         src,
		expectedMD5: strings.Trim(stat.ETag, `"`),
		objectKey:   objectKey,
		hash:        md5.New(),
	}, nil
}
