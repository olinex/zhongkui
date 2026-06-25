// Package s3 提供与S3兼容对象存储服务交互的客户端封装
// 基于minio-go库实现，支持Amazon S3、MinIO等兼容S3协议的对象存储服务
// 主要功能包括：对象上传、下载、删除、列表查询、对象信息获取等
package s3

import (
	"context"
	"io"
	"net/http"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"zhongkui.com/zhongkui/internal/pkg/errors"
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
// 封装了minio.Client并提供统一的接口，简化S3操作
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
func NewS3Client(cfg *Config, opts ...ClientOption) (*S3Client, error) {
	mOpts := &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	}
	for _, opt := range opts {
		opt(mOpts)
	}
	client, err := minio.New(cfg.Endpoint, mOpts)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &S3Client{
		cli:      client,
		bucket:   cfg.Bucket,
		endpoint: cfg.Endpoint,
	}, nil
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
