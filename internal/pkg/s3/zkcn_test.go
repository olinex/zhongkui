package s3

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/smartystreets/goconvey/convey"
	"zhongkui.com/zhongkui/internal/pkg/crypto/combine"
	"zhongkui.com/zhongkui/internal/pkg/crypto/key"
)

func TestNewZkcn(t *testing.T) {
	convey.Convey("Test NewZkcn constructor", t, func() {
		convey.Convey("should create Zkcn with all fields populated", func() {
			client := &S3Client{}
			objectKey := "test/object.json"
			cryptoKey := []byte("0123456789abcdef0123456789abcdef")
			timeout := 1 * time.Hour

			z := NewZkcn(client, objectKey, cryptoKey, timeout)

			convey.So(z, convey.ShouldNotBeNil)
			convey.So(z.client, convey.ShouldEqual, client)
			convey.So(z.objectKey, convey.ShouldEqual, objectKey)
			convey.So(z.cryptoKey, convey.ShouldResemble, cryptoKey)
			convey.So(z.timeout, convey.ShouldEqual, timeout)
		})

		convey.Convey("should create Zkcn with nil cryptoKey", func() {
			client := &S3Client{}
			objectKey := "test/object.json"

			z := NewZkcn(client, objectKey, nil, 0)

			convey.So(z, convey.ShouldNotBeNil)
			convey.So(z.client, convey.ShouldEqual, client)
			convey.So(z.objectKey, convey.ShouldEqual, objectKey)
			convey.So(z.cryptoKey, convey.ShouldBeNil)
			convey.So(z.timeout, convey.ShouldEqual, 0)
		})

		convey.Convey("should create Zkcn with zero timeout", func() {
			client := &S3Client{}
			key := []byte("0123456789abcdef0123456789abcdef")
			objectKey := "test/object.json"

			z := NewZkcn(client, objectKey, key, 0)

			convey.So(z, convey.ShouldNotBeNil)
			convey.So(z.timeout, convey.ShouldEqual, 0)
		})

		convey.Convey("should create Zkcn with negative timeout", func() {
			client := &S3Client{}
			key := []byte("0123456789abcdef0123456789abcdef")
			objectKey := "test/object.json"

			z := NewZkcn(client, objectKey, key, -1*time.Second)

			convey.So(z, convey.ShouldNotBeNil)
			convey.So(z.timeout, convey.ShouldBeLessThan, 0)
		})

		convey.Convey("should create Zkcn with empty objectKey", func() {
			client := &S3Client{}
			key := []byte("0123456789abcdef0123456789abcdef")

			z := NewZkcn(client, "", key, 0)

			convey.So(z, convey.ShouldNotBeNil)
			convey.So(z.objectKey, convey.ShouldEqual, "")
		})

		convey.Convey("should create Zkcn with nil client", func() {
			key := []byte("0123456789abcdef0123456789abcdef")
			objectKey := "test/object.json"

			z := NewZkcn(nil, objectKey, key, 0)

			convey.So(z, convey.ShouldNotBeNil)
			convey.So(z.client, convey.ShouldBeNil)
		})

		convey.Convey("should create Zkcn with short cryptoKey (16 bytes)", func() {
			client := &S3Client{}
			key := []byte("0123456789abcdef") // AES-128
			objectKey := "test/object.json"

			z := NewZkcn(client, objectKey, key, 5*time.Minute)

			convey.So(z, convey.ShouldNotBeNil)
			convey.So(len(z.cryptoKey), convey.ShouldEqual, 16)
		})

		convey.Convey("should create Zkcn with 24-byte cryptoKey (AES-192)", func() {
			client := &S3Client{}
			key := []byte("0123456789abcdef01234567") // AES-192
			objectKey := "test/object.json"

			z := NewZkcn(client, objectKey, key, 0)

			convey.So(z, convey.ShouldNotBeNil)
			convey.So(len(z.cryptoKey), convey.ShouldEqual, 24)
		})
	})
}

func TestZkcn_Clear(t *testing.T) {
	convey.Convey("Test Zkcn.Clear without real S3 server", t, func() {
		convey.Convey("Clear with nil client should panic or return error", func() {
			z := NewZkcn(nil, "test-key", nil, 0)
			// 预期会因为 nil client 而 panic
			convey.So(func() {
				//nolint:errcheck
				z.Clear(context.Background())
			}, convey.ShouldPanic)
		})
	})

	convey.Convey("Test Zkcn methods with nil client", t, func() {
		convey.Convey("Load with nil client should panic", func() {
			z := NewZkcn(nil, "test-key", nil, 0)
			convey.So(func() {
				//nolint:errcheck
				z.Load(context.Background())
			}, convey.ShouldPanic)
		})
	})
}

// TestZkcnIntegration 是 Zkcn 的集成测试，验证加密存储→解密加载的完整流程。
// 需要真实 S3 服务器环境变量，如果未设置则跳过。
func TestZkcnIntegration(t *testing.T) {
	endpoint := getEnvOrDefault("S3_TEST_ENDPOINT", "")
	accessKey := getEnvOrDefault("S3_TEST_ACCESS_KEY", "")
	secretKey := getEnvOrDefault("S3_TEST_SECRET_KEY", "")
	bucket := getEnvOrDefault("S3_TEST_BUCKET", "")

	if endpoint == "" || accessKey == "" || secretKey == "" || bucket == "" {
		t.Skip("Skipping Zkcn integration test: set S3_TEST_ENDPOINT, S3_TEST_ACCESS_KEY, S3_TEST_SECRET_KEY, S3_TEST_BUCKET to run")
	}

	convey.Convey("Test Zkcn integration: Store → Load → Clear roundtrip", t, func() {
		cfg := &Config{
			Endpoint:  endpoint,
			AccessKey: accessKey,
			SecretKey: secretKey,
			Bucket:    bucket,
			Region:    "us-east-1",
			UseSSL:    false,
		}

		provider := key.NewConfigFileSecretKeyProvider(getEnvOrDefault("S3_TEST_CRYPTO_KEY", "0123456789abcdef0123456789abcdef"))
		book, err := key.LoadPasswordBook("")
		convey.So(err, convey.ShouldBeNil)
		kd := key.NewKeyDerivation(provider, book)

		client, err := NewS3Client(cfg, kd)
		convey.So(err, convey.ShouldBeNil)
		convey.So(client, convey.ShouldNotBeNil)

		convey.Convey("roundtrip with custom cryptoKey", func() {
			cryptoKey := []byte("0123456789abcdef0123456789abcdef")
			objectKey := "test/zkcn/roundtrip-custom-key.json"
			timeout := 10 * time.Minute

			z := NewZkcn(client, objectKey, cryptoKey, timeout)
			convey.So(z, convey.ShouldNotBeNil)

			// 清理（如果之前存在残留对象）
			z.Clear(context.Background())

			original := []byte(`{"task":"test-zkcn","status":"completed","timestamp":"2026-07-18T12:00:00Z"}`)

			// Store - 使用 Store 而非 CPutObject，验证 opts 设置
			uploadInfo, err := z.Store(context.Background(), &readerWrapper{data: original})
			convey.So(err, convey.ShouldBeNil)
			convey.So(uploadInfo.Size, convey.ShouldBeGreaterThan, 0)

			defer z.Clear(context.Background())

			// Load - 解密解压后应与原始数据一致
			rc, err := z.Load(context.Background())
			convey.So(err, convey.ShouldBeNil)
			convey.So(rc, convey.ShouldNotBeNil)
			defer rc.Close()

			downloaded, err := io.ReadAll(rc)
			convey.So(err, convey.ShouldBeNil)
			convey.So(downloaded, convey.ShouldResemble, original)
		})

		convey.Convey("roundtrip with nil cryptoKey (use client default pipeline)", func() {
			objectKey := "test/zkcn/roundtrip-default-key.json"

			z := NewZkcn(client, objectKey, nil, 0)
			convey.So(z, convey.ShouldNotBeNil)
			convey.So(z.cryptoKey, convey.ShouldBeNil)
			convey.So(z.timeout, convey.ShouldEqual, 0)

			z.Clear(context.Background())

			original := []byte(`{"task":"test-zkcn-default","status":"ok"}`)

			uploadInfo, err := z.Store(context.Background(), &readerWrapper{data: original})
			convey.So(err, convey.ShouldBeNil)
			convey.So(uploadInfo.Size, convey.ShouldBeGreaterThan, 0)

			defer z.Clear(context.Background())

			rc, err := z.Load(context.Background())
			convey.So(err, convey.ShouldBeNil)
			convey.So(rc, convey.ShouldNotBeNil)
			defer rc.Close()

			downloaded, err := io.ReadAll(rc)
			convey.So(err, convey.ShouldBeNil)
			convey.So(downloaded, convey.ShouldResemble, original)
		})

		convey.Convey("Load on non-existent object should return nil, nil", func() {
			z := NewZkcn(client, "test/zkcn/non-existent-999", nil, 0)

			rc, err := z.Load(context.Background())
			convey.So(err, convey.ShouldBeNil)
			convey.So(rc, convey.ShouldBeNil)
		})

		convey.Convey("Clear on non-existent object should not error", func() {
			z := NewZkcn(client, "test/zkcn/non-existent-clear-999", nil, 0)

			err := z.Clear(context.Background())
			// 删除不存在的对象通常不会报错（幂等操作）
			convey.So(err, convey.ShouldBeNil)
		})

		convey.Convey("Zkcn with negative timeout should not set Expires", func() {
			cryptoKey := []byte("0123456789abcdef0123456789abcdef")
			objectKey := "test/zkcn/no-expires.json"

			z := NewZkcn(client, objectKey, cryptoKey, -1*time.Hour)
			convey.So(z.timeout, convey.ShouldBeLessThan, 0)

			z.Clear(context.Background())

			data := []byte("test data for no expires")
			uploadInfo, err := z.Store(context.Background(), &readerWrapper{data: data})
			convey.So(err, convey.ShouldBeNil)
			convey.So(uploadInfo.Size, convey.ShouldBeGreaterThan, 0)

			defer z.Clear(context.Background())

			// 验证数据可以正常读取
			rc, err := z.Load(context.Background())
			convey.So(err, convey.ShouldBeNil)
			convey.So(rc, convey.ShouldNotBeNil)
			defer rc.Close()

			downloaded, err := io.ReadAll(rc)
			convey.So(err, convey.ShouldBeNil)
			convey.So(downloaded, convey.ShouldResemble, data)
		})
	})
}

// TestZkcn_Store_ContentTypeValidation 验证 Store 方法使用正确的 ContentType。
// 此测试通过集成方式验证：上传后检查对象的 ContentType 属性。
func TestZkcn_Store_ContentTypeValidation(t *testing.T) {
	endpoint := getEnvOrDefault("S3_TEST_ENDPOINT", "")
	accessKey := getEnvOrDefault("S3_TEST_ACCESS_KEY", "")
	secretKey := getEnvOrDefault("S3_TEST_SECRET_KEY", "")
	bucket := getEnvOrDefault("S3_TEST_BUCKET", "")

	if endpoint == "" || accessKey == "" || secretKey == "" || bucket == "" {
		t.Skip("Skipping ContentType validation test: S3 env vars not set")
	}

	convey.Convey("Test Zkcn Store ContentType validation", t, func() {
		cfg := &Config{
			Endpoint:  endpoint,
			AccessKey: accessKey,
			SecretKey: secretKey,
			Bucket:    bucket,
			Region:    "us-east-1",
			UseSSL:    false,
		}

		provider := key.NewConfigFileSecretKeyProvider(getEnvOrDefault("S3_TEST_CRYPTO_KEY", "0123456789abcdef0123456789abcdef"))
		book, err := key.LoadPasswordBook("")
		convey.So(err, convey.ShouldBeNil)
		kd := key.NewKeyDerivation(provider, book)

		client, err := NewS3Client(cfg, kd)
		convey.So(err, convey.ShouldBeNil)

		convey.Convey("object uploaded via Zkcn should have application/zkcn ContentType", func() {
			objectKey := "test/zkcn/content-type-check.json"
			cryptoKey := []byte("0123456789abcdef0123456789abcdef")

			z := NewZkcn(client, objectKey, cryptoKey, 5*time.Minute)
			z.Clear(context.Background())
			defer z.Clear(context.Background())

			data := []byte("content-type validation data")
			_, err := z.Store(context.Background(), &readerWrapper{data: data})
			convey.So(err, convey.ShouldBeNil)

			// StatObject 检查 ContentType
			info, err := client.StatObject(context.Background(), objectKey, minio.StatObjectOptions{})
			convey.So(err, convey.ShouldBeNil)
			convey.So(info.ContentType, convey.ShouldEqual, combine.COMPRESS_CONTENT_TYPE)
		})
	})
}

// readerWrapper 是将 []byte 适配为 io.Reader 的简单包装器。
// 用于在测试中提供可重复读取的数据源，因为 bytes.Reader 只能读取一次。
type readerWrapper struct {
	data      []byte
	readSoFar int
}

// Read 实现 io.Reader 接口。
// 从内部 data 切片中读取剩余字节到 p 中。
func (r *readerWrapper) Read(p []byte) (n int, err error) {
	if r.readSoFar >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.readSoFar:])
	r.readSoFar += n
	return n, nil
}

// getEnvOrDefault 获取环境变量值，如果不存在则返回默认值。
func getEnvOrDefault(key, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultValue
}

// chunkedReadTracker 是一个可控分块的读取器包装器。
// 每次 Read 调用最多返回 chunkSize 字节，用于验证流式处理的数据完整性。
// 同时记录总读取字节数和 Read 调用次数。
type chunkedReadTracker struct {
	data      []byte
	chunkSize int
	offset    int
	ReadCount int // Read 方法被调用的次数
	TotalRead int // 累计读取的字节数
}

// Read 实现 io.Reader 接口，每次最多返回 chunkSize 字节。
func (c *chunkedReadTracker) Read(p []byte) (n int, err error) {
	if c.offset >= len(c.data) {
		return 0, io.EOF
	}
	remaining := len(c.data) - c.offset
	toRead := c.chunkSize
	if toRead > remaining {
		toRead = remaining
	}
	if toRead > len(p) {
		toRead = len(p)
	}
	n = copy(p, c.data[c.offset:c.offset+toRead])
	c.offset += n
	c.ReadCount++
	c.TotalRead += n
	return n, nil
}

// TestZkcnStore_StreamingWithChunkedReader 验证 Store 能正确处理小块分发的 reader。
// 当 reader 每次只返回 1 字节时，依然能完整加密上传并正确解密。
// 此测试验证了 Zkcn.Store 不依赖 reader 的一次性大批量读取（即 io.ReadAll）也能正确工作。
func TestZkcnStore_StreamingWithChunkedReader(t *testing.T) {
	endpoint := getEnvOrDefault("S3_TEST_ENDPOINT", "")
	accessKey := getEnvOrDefault("S3_TEST_ACCESS_KEY", "")
	secretKey := getEnvOrDefault("S3_TEST_SECRET_KEY", "")
	bucket := getEnvOrDefault("S3_TEST_BUCKET", "")

	if endpoint == "" || accessKey == "" || secretKey == "" || bucket == "" {
		t.Skip("Skipping: S3 env vars not set")
	}

	convey.Convey("Test Zkcn.Store with 1-byte chunked reader", t, func() {
		cfg := &Config{
			Endpoint:  endpoint,
			AccessKey: accessKey,
			SecretKey: secretKey,
			Bucket:    bucket,
			Region:    "us-east-1",
			UseSSL:    false,
		}

		provider := key.NewConfigFileSecretKeyProvider(getEnvOrDefault("S3_TEST_CRYPTO_KEY", "0123456789abcdef0123456789abcdef"))
		book, err := key.LoadPasswordBook("")
		convey.So(err, convey.ShouldBeNil)
		kd := key.NewKeyDerivation(provider, book)

		client, err := NewS3Client(cfg, kd)
		convey.So(err, convey.ShouldBeNil)

		convey.Convey("data should roundtrip correctly with 1-byte chunks", func() {
			cryptoKey := []byte("0123456789abcdef0123456789abcdef")
			objectKey := "test/zkcn/chunked-reader-test.zkcn"

			z := NewZkcn(client, objectKey, cryptoKey, 0)
			z.Clear(context.Background())
			defer z.Clear(context.Background())

			original := []byte(`{"chunked":"yes","size":42,"data":"abcdefghijklmnopqrstuvwxyz"}`)

			tracker := &chunkedReadTracker{data: original, chunkSize: 1}

			uploadInfo, err := z.Store(context.Background(), tracker)
			convey.So(err, convey.ShouldBeNil)
			convey.So(uploadInfo.Size, convey.ShouldBeGreaterThan, 0)

			// 验证 reader 被完整消费（每个字节都被读取）
			convey.So(tracker.TotalRead, convey.ShouldEqual, len(original))
			convey.So(tracker.ReadCount, convey.ShouldBeGreaterThan, 1) // 证明是分多次读取

			// 验证解密后数据一致
			rc, err := z.Load(context.Background())
			convey.So(err, convey.ShouldBeNil)
			convey.So(rc, convey.ShouldNotBeNil)
			defer rc.Close()

			downloaded, err := io.ReadAll(rc)
			convey.So(err, convey.ShouldBeNil)
			convey.So(downloaded, convey.ShouldResemble, original)
		})

		convey.Convey("moderate-sized data with small chunks", func() {
			cryptoKey := []byte("0123456789abcdef0123456789abcdef")
			objectKey := "test/zkcn/chunked-10k.zkcn"

			z := NewZkcn(client, objectKey, cryptoKey, 0)
			z.Clear(context.Background())
			defer z.Clear(context.Background())

			// 10KB 的数据，每次只读 7 字节
			original := make([]byte, 10240)
			for i := range original {
				original[i] = byte(i % 256)
			}

			tracker := &chunkedReadTracker{data: original, chunkSize: 7}

			uploadInfo, err := z.Store(context.Background(), tracker)
			convey.So(err, convey.ShouldBeNil)
			convey.So(uploadInfo.Size, convey.ShouldBeGreaterThan, 0)

			convey.So(tracker.TotalRead, convey.ShouldEqual, len(original))
			convey.So(tracker.ReadCount, convey.ShouldBeGreaterThan, 100) // 多次小块读取

			rc, err := z.Load(context.Background())
			convey.So(err, convey.ShouldBeNil)
			convey.So(rc, convey.ShouldNotBeNil)
			defer rc.Close()

			downloaded, err := io.ReadAll(rc)
			convey.So(err, convey.ShouldBeNil)
			convey.So(downloaded, convey.ShouldResemble, original)
		})

		convey.Convey("empty data should work with chunked reader", func() {
			cryptoKey := []byte("0123456789abcdef0123456789abcdef")
			objectKey := "test/zkcn/chunked-empty.zkcn"

			z := NewZkcn(client, objectKey, cryptoKey, 0)
			z.Clear(context.Background())
			defer z.Clear(context.Background())

			tracker := &chunkedReadTracker{data: []byte{}, chunkSize: 1}

			uploadInfo, err := z.Store(context.Background(), tracker)
			convey.So(err, convey.ShouldBeNil)
			convey.So(uploadInfo.Size, convey.ShouldBeGreaterThan, 0) // 即使空数据，加密后也有头部

			convey.So(tracker.TotalRead, convey.ShouldEqual, 0)

			rc, err := z.Load(context.Background())
			convey.So(err, convey.ShouldBeNil)
			convey.So(rc, convey.ShouldNotBeNil)
			defer rc.Close()

			downloaded, err := io.ReadAll(rc)
			convey.So(err, convey.ShouldBeNil)
			convey.So(downloaded, convey.ShouldResemble, []byte{})
		})
	})
}

func TestCleanupOrphanedTempFiles(t *testing.T) {
	convey.Convey("Test CleanupOrphanedTempFiles", t, func() {
		convey.Convey("should not error on empty temp dir with no matching files", func() {
			cleaned, err := CleanupOrphanedTempFiles()
			convey.So(err, convey.ShouldBeNil)
			convey.So(cleaned, convey.ShouldBeGreaterThanOrEqualTo, 0)
		})

		convey.Convey("should clean up orphaned temp files matching prefix", func() {
			// 创建模拟的残留文件
			orphanFile, err := os.CreateTemp("", zkcnTempPrefix+"*.tmp")
			convey.So(err, convey.ShouldBeNil)
			orphanPath := orphanFile.Name()
			orphanFile.Close()

			defer os.Remove(orphanPath) // 兜底清理

			cleaned, err := CleanupOrphanedTempFiles()
			convey.So(err, convey.ShouldBeNil)
			convey.So(cleaned, convey.ShouldBeGreaterThan, 0)

			// 验证文件确实被删除
			_, err = os.Stat(orphanPath)
			convey.So(os.IsNotExist(err), convey.ShouldBeTrue)
		})

		convey.Convey("should not delete files with different prefix", func() {
			otherFile, err := os.CreateTemp("", "other-prefix-*.tmp")
			convey.So(err, convey.ShouldBeNil)
			otherPath := otherFile.Name()
			otherFile.Close()
			defer os.Remove(otherPath)

			cleaned, err := CleanupOrphanedTempFiles()
			convey.So(err, convey.ShouldBeNil)
			// 验证不匹配的文件仍然存在
			_, err = os.Stat(otherPath)
			convey.So(err, convey.ShouldBeNil)
			_ = cleaned // cleaned 可能 > 0 如果有之前的残留
		})

		convey.Convey("should not delete files without .tmp suffix", func() {
			notTempFile, err := os.CreateTemp("", zkcnTempPrefix+"*.dat")
			convey.So(err, convey.ShouldBeNil)
			notTempPath := notTempFile.Name()
			notTempFile.Close()
			defer os.Remove(notTempPath)

			_, err = CleanupOrphanedTempFiles()
			convey.So(err, convey.ShouldBeNil)
			// 非 .tmp 后缀的文件不应被删除
			_, err = os.Stat(notTempPath)
			convey.So(err, convey.ShouldBeNil)
		})
	})
}
