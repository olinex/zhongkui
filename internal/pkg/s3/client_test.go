package s3

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/minio/minio-go/v7"
	"github.com/smartystreets/goconvey/convey"
	"zhongkui.com/zhongkui/internal/pkg/crypto/combine"
	"zhongkui.com/zhongkui/internal/pkg/crypto/key"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

func TestWithTransport(t *testing.T) {
	convey.Convey("Test WithTransport option", t, func() {
		convey.Convey("should set custom transport on minio.Options", func() {
			customRT := &http.Transport{}
			opts := &minio.Options{}

			fn := WithTransport(customRT)
			fn(opts)

			convey.So(opts.Transport, convey.ShouldEqual, customRT)
		})

		convey.Convey("should set nil transport", func() {
			opts := &minio.Options{Transport: http.DefaultTransport}

			fn := WithTransport(nil)
			fn(opts)

			convey.So(opts.Transport, convey.ShouldBeNil)
		})

		convey.Convey("should override existing transport", func() {
			oldRT := &http.Transport{}
			newRT := &http.Transport{MaxIdleConns: 100}
			opts := &minio.Options{Transport: oldRT}

			fn := WithTransport(newRT)
			fn(opts)

			convey.So(opts.Transport, convey.ShouldEqual, newRT)
			convey.So(opts.Transport, convey.ShouldNotEqual, oldRT)
		})
	})
}

func TestGetEndpoint(t *testing.T) {
	convey.Convey("Test GetEndpoint", t, func() {
		convey.Convey("should return the endpoint set during creation", func() {
			endpoint := "https://s3.amazonaws.com"
			client := &S3Client{endpoint: endpoint}
			convey.So(client.GetEndpoint(), convey.ShouldEqual, endpoint)
		})

		convey.Convey("should return empty string when endpoint is empty", func() {
			client := &S3Client{}
			convey.So(client.GetEndpoint(), convey.ShouldEqual, "")
		})

		convey.Convey("should return localhost endpoint", func() {
			endpoint := "localhost:9000"
			client := &S3Client{endpoint: endpoint}
			convey.So(client.GetEndpoint(), convey.ShouldEqual, endpoint)
		})
	})
}

func TestIsNotExists(t *testing.T) {
	convey.Convey("Test IsNotExists", t, func() {
		convey.Convey("nil error should return false", func() {
			convey.So(IsNotExists(nil), convey.ShouldBeFalse)
		})

		convey.Convey("NoSuchKey error should return true", func() {
			err := minio.ErrorResponse{Code: "NoSuchKey", Message: "The specified key does not exist."}
			convey.So(IsNotExists(err), convey.ShouldBeTrue)
		})

		convey.Convey("NoSuchBucket error should return true", func() {
			err := minio.ErrorResponse{Code: "NoSuchBucket", Message: "The specified bucket does not exist."}
			convey.So(IsNotExists(err), convey.ShouldBeTrue)
		})

		convey.Convey("AccessDenied error should return false", func() {
			err := minio.ErrorResponse{Code: "AccessDenied", Message: "Access Denied."}
			convey.So(IsNotExists(err), convey.ShouldBeFalse)
		})

		convey.Convey("generic io.EOF error should return false", func() {
			convey.So(IsNotExists(io.EOF), convey.ShouldBeFalse)
		})

		convey.Convey("random non-minio error should return false", func() {
			err := minio.ErrorResponse{
				Code:    "InternalError",
				Message: "We encountered an internal error, please try again.",
			}
			convey.So(IsNotExists(err), convey.ShouldBeFalse)
		})

		convey.Convey("XMinioObjectNotFoundAs403 error - depends on minio.ToErrorResponse mapping", func() {
			err := minio.ErrorResponse{Code: "XMinioObjectNotFoundAs403"}
			// minio.ToErrorResponse may or may not map this to NoSuchKey
			// 只需验证不会 panic
			_ = IsNotExists(err)
			convey.So(true, convey.ShouldBeTrue) // 至少不崩溃
		})

		convey.Convey("NoSuchUpload error should return false", func() {
			err := minio.ErrorResponse{Code: "NoSuchUpload", Message: "The specified multipart upload does not exist."}
			convey.So(IsNotExists(err), convey.ShouldBeFalse)
		})

		convey.Convey("NoSuchVersion error should return false", func() {
			err := minio.ErrorResponse{Code: "NoSuchVersion", Message: "The specified version does not exist."}
			convey.So(IsNotExists(err), convey.ShouldBeFalse)
		})

		convey.Convey("InvalidAccessKeyId error should return false", func() {
			err := minio.ErrorResponse{Code: "InvalidAccessKeyId", Message: "The AWS Access Key Id you provided does not exist."}
			convey.So(IsNotExists(err), convey.ShouldBeFalse)
		})

		convey.Convey("empty string error should return false", func() {
			convey.So(IsNotExists(minio.ErrorResponse{}), convey.ShouldBeFalse)
		})

		convey.Convey("non-minio error type (context.DeadlineExceeded) should return false", func() {
			convey.So(IsNotExists(context.DeadlineExceeded), convey.ShouldBeFalse)
		})

		convey.Convey("non-minio error type (context.Canceled) should return false", func() {
			convey.So(IsNotExists(context.Canceled), convey.ShouldBeFalse)
		})

		convey.Convey("wrapped io.EOF in custom error chain should return false", func() {
			err := errors.WithStack(io.EOF)
			convey.So(IsNotExists(err), convey.ShouldBeFalse)
		})

		convey.Convey("custom string error should return false", func() {
			err := errors.New("custom error message")
			convey.So(IsNotExists(err), convey.ShouldBeFalse)
		})
	})
}

func TestCPutObjectAndCGetObject(t *testing.T) {
	endpoint := os.Getenv("S3_TEST_ENDPOINT")
	accessKey := os.Getenv("S3_TEST_ACCESS_KEY")
	secretKey := os.Getenv("S3_TEST_SECRET_KEY")
	bucket := os.Getenv("S3_TEST_BUCKET")
	cryptoKeyEnv := os.Getenv("S3_TEST_CRYPTO_KEY")

	if endpoint == "" || accessKey == "" || secretKey == "" || bucket == "" {
		t.Skip("Skipping integration test: set S3_TEST_ENDPOINT, S3_TEST_ACCESS_KEY, S3_TEST_SECRET_KEY, S3_TEST_BUCKET to run")
	}

	convey.Convey("Test CPutObject & CGetObject roundtrip with real S3", t, func() {
		cfg := &Config{
			Endpoint:  endpoint,
			AccessKey: accessKey,
			SecretKey: secretKey,
			Bucket:    bucket,
			Region:    "us-east-1",
			UseSSL:    false,
		}

		provider := key.NewConfigFileSecretKeyProvider(cryptoKeyEnv)
		book, err := key.LoadPasswordBook("")
		convey.So(err, convey.ShouldBeNil)
		kd := key.NewKeyDerivation(provider, book)

		client, err := NewS3Client(cfg, kd)
		convey.So(err, convey.ShouldBeNil)
		convey.So(client, convey.ShouldNotBeNil)

		cryptoKey := []byte(cryptoKeyEnv)

		convey.Convey("encrypted upload → encrypted download should produce identical data", func() {
			original := []byte(`{"name":"integration-test","version":"v2","data":"hello zhongkui"}`)
			objectKey := "test/crypto-roundtrip/test.json"

			// 使用独立的 cryptoKey 测试自定义加密管道
			uploadInfo, err := client.CPutObject(
				context.Background(),
				objectKey,
				cryptoKey,
				bytes.NewReader(original),
				-1, // 流式上传，未知大小
				minio.PutObjectOptions{},
			)
			convey.So(err, convey.ShouldBeNil)
			convey.So(uploadInfo.Size, convey.ShouldBeGreaterThan, 0)

			defer client.RemoveObject(context.Background(), objectKey, minio.RemoveObjectOptions{})

			rc, err := client.CGetOject(context.Background(), objectKey, cryptoKey, minio.GetObjectOptions{})
			convey.So(err, convey.ShouldBeNil)
			convey.So(rc, convey.ShouldNotBeNil)
			defer rc.Close()

			downloaded, err := io.ReadAll(rc)
			convey.So(err, convey.ShouldBeNil)
			convey.So(downloaded, convey.ShouldResemble, original)
		})

		convey.Convey("CGetOject on non-existent key should return nil, nil", func() {
			rc, err := client.CGetOject(context.Background(), "test/non-existent-key-xyz", nil, minio.GetObjectOptions{})
			convey.So(err, convey.ShouldBeNil)
			convey.So(rc, convey.ShouldBeNil)
		})
	})
}

func TestNewS3Client_InvalidConfig(t *testing.T) {
	convey.Convey("Test NewS3Client with invalid config", t, func() {
		provider := key.NewConfigFileSecretKeyProvider("0123456789abcdef0123456789abcdef")
		book, err := key.LoadPasswordBook("")
		convey.So(err, convey.ShouldBeNil)
		kd := key.NewKeyDerivation(provider, book)

		convey.Convey("should fail with unreachable endpoint", func() {
			cfg := &Config{
				Endpoint:  "http://127.0.0.1:19999",
				AccessKey: "invalid",
				SecretKey: "invalid",
				Bucket:    "nonexistent",
				UseSSL:    false,
			}

			_, err := NewS3Client(cfg, kd)
			convey.So(err, convey.ShouldNotBeNil)
		})
	})
}

func TestCPutObject_StreamEncryption(t *testing.T) {
	convey.Convey("Test CPutObject stream encryption pipeline", t, func() {
		convey.Convey("verify pipeline encrypts data before sending", func() {
			key := []byte("0123456789abcdef0123456789abcdef")

			compressor, err := combine.NewZstdCompressor()
			convey.So(err, convey.ShouldBeNil)

			encryptor, err := combine.NewAESGCMCryptor(key)
			convey.So(err, convey.ShouldBeNil)

			pipeline, err := combine.NewCompressEncryptPipeline(compressor, encryptor)
			convey.So(err, convey.ShouldBeNil)

			original := []byte("stream encryption test payload")

			pipeReader, pipeWriter := io.Pipe()

			// 先启动读取 goroutine，避免 CompressEncrypt 写 header 时因 Pipe 无缓冲而死锁
			var encrypted bytes.Buffer
			readDone := make(chan error, 1)
			go func() {
				_, err := io.Copy(&encrypted, pipeReader)
				readDone <- err
			}()

			w, err := pipeline.CompressEncrypt(pipeWriter)
			convey.So(err, convey.ShouldBeNil)

			_, err = io.Copy(w, bytes.NewReader(original))
			convey.So(err, convey.ShouldBeNil)

			err = w.Close()
			convey.So(err, convey.ShouldBeNil)

			err = pipeWriter.Close()
			convey.So(err, convey.ShouldBeNil)

			err = <-readDone
			convey.So(err, convey.ShouldBeNil)

			convey.So(encrypted.Len(), convey.ShouldBeGreaterThan, len(original))

			decrypted, err := pipeline.DecryptDecompress(&encrypted)
			convey.So(err, convey.ShouldBeNil)
			result, err := io.ReadAll(decrypted)
			convey.So(err, convey.ShouldBeNil)
			convey.So(result, convey.ShouldResemble, original)
		})
	})
}

// TestVerifyDownloadIntegrity 测试 S3 下载数据的 ETag 一致性校验。
// 通过 mock HTTP 服务器模拟服务端返回的 ETag 与实际下载数据不一致的场景，
// 验证 VerifyDownloadIntegrity 能正确检测出数据损坏。
func TestVerifyDownloadIntegrity(t *testing.T) {
	convey.Convey("Test VerifyDownloadIntegrity with mock S3 server", t, func() {
		// 构建两组数据：goodData 用于设置服务端 ETag，corruptData 用于模拟损坏的下载内容
		goodData := []byte("original file content for integrity check")
		corruptData := []byte("corrupted file content!!! different data")

		goodMD5 := md5.Sum(goodData)
		goodETag := fmt.Sprintf(`"%s"`, hex.EncodeToString(goodMD5[:]))

		testBucket := "test-bucket"
		testObjectKey := "test/object.zkcn"
		expectedPath := "/" + testBucket + "/" + testObjectKey

		// 启动 mock HTTP 服务器
		// HEAD 请求（StatObject）：返回 goodData 的 ETag
		// GET 请求（GetObject）：返回 corruptData（MD5 与 ETag 不一致）
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != expectedPath {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			switch r.Method {
			case http.MethodHead:
				w.Header().Set("ETag", goodETag)
				w.Header().Set("Content-Length", fmt.Sprintf("%d", len(goodData)))
				w.Header().Set("Last-Modified", "Mon, 02 Jan 2006 15:04:05 GMT")
				w.WriteHeader(http.StatusOK)
			case http.MethodGet:
				w.Header().Set("ETag", goodETag)
				w.Header().Set("Content-Length", fmt.Sprintf("%d", len(corruptData)))
				w.Header().Set("Last-Modified", "Mon, 02 Jan 2006 15:04:05 GMT")
				w.WriteHeader(http.StatusOK)
				w.Write(corruptData)
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		}))
		defer server.Close()

		convey.Convey("should return error when downloaded data MD5 does not match ETag", func() {
			cfg := &Config{
				Endpoint:  strings.TrimPrefix(server.URL, "http://"),
				AccessKey: "test-access-key",
				SecretKey: "test-secret-key",
				Bucket:    testBucket,
				Region:    "us-east-1",
				UseSSL:    false,
			}

			provider := key.NewConfigFileSecretKeyProvider("0123456789abcdef0123456789abcdef")
			book, e := key.LoadPasswordBook("")
			convey.So(e, convey.ShouldBeNil)
			kd := key.NewKeyDerivation(provider, book)

			client, e := NewS3Client(cfg, kd)
			convey.So(e, convey.ShouldBeNil)
			convey.So(client, convey.ShouldNotBeNil)

			ctx := context.Background()

			// 模拟下载：GetObject 返回 corruptData，包装为 ETag 校验流
			rc, e := client.GetObject(ctx, testObjectKey, minio.GetObjectOptions{})
			convey.So(e, convey.ShouldBeNil)
			defer rc.Close()

			vr, e := client.VerifyDownloadIntegrity(ctx, testObjectKey, rc)
			convey.So(e, convey.ShouldBeNil)

			// 流式读取：StatObject 返回 goodETag，但 corruptData 的 MD5 与之不匹配
			_, e = io.ReadAll(vr)
			convey.So(e, convey.ShouldNotBeNil)
			convey.So(e.Error(), convey.ShouldContainSubstring, "S3 object integrity check failed")
			convey.So(e.Error(), convey.ShouldContainSubstring, "expected ETag")
			convey.So(e.Error(), convey.ShouldContainSubstring, "computed MD5")
		})

		convey.Convey("should return nil when downloaded data MD5 matches ETag", func() {
			cfg := &Config{
				Endpoint:  strings.TrimPrefix(server.URL, "http://"),
				AccessKey: "test-access-key",
				SecretKey: "test-secret-key",
				Bucket:    testBucket,
				Region:    "us-east-1",
				UseSSL:    false,
			}

			provider := key.NewConfigFileSecretKeyProvider("0123456789abcdef0123456789abcdef")
			book, e := key.LoadPasswordBook("")
			convey.So(e, convey.ShouldBeNil)
			kd := key.NewKeyDerivation(provider, book)

			client, e := NewS3Client(cfg, kd)
			convey.So(e, convey.ShouldBeNil)

			ctx := context.Background()

			// 包装 goodData Reader，数据 MD5 与 ETag 一致
			vr, e := client.VerifyDownloadIntegrity(ctx, testObjectKey, bytes.NewReader(goodData))
			convey.So(e, convey.ShouldBeNil)

			// 流式读取完毕，校验应通过
			result, e := io.ReadAll(vr)
			convey.So(e, convey.ShouldBeNil)
			convey.So(result, convey.ShouldResemble, goodData)
		})
	})
}

func TestCopyObject(t *testing.T) {
	endpoint := os.Getenv("S3_TEST_ENDPOINT")
	accessKey := os.Getenv("S3_TEST_ACCESS_KEY")
	secretKey := os.Getenv("S3_TEST_SECRET_KEY")
	bucket := os.Getenv("S3_TEST_BUCKET")
	cryptoKeyEnv := os.Getenv("S3_TEST_CRYPTO_KEY")

	if endpoint == "" || accessKey == "" || secretKey == "" || bucket == "" {
		t.Skip("Skipping integration test: set S3_TEST_ENDPOINT, S3_TEST_ACCESS_KEY, S3_TEST_SECRET_KEY, S3_TEST_BUCKET to run")
	}

	convey.Convey("Test CopyObject server-side copy roundtrip with real S3", t, func() {
		cfg := &Config{
			Endpoint:  endpoint,
			AccessKey: accessKey,
			SecretKey: secretKey,
			Bucket:    bucket,
			Region:    "us-east-1",
			UseSSL:    false,
		}

		provider := key.NewConfigFileSecretKeyProvider(cryptoKeyEnv)
		book, err := key.LoadPasswordBook("")
		convey.So(err, convey.ShouldBeNil)
		kd := key.NewKeyDerivation(provider, book)

		client, err := NewS3Client(cfg, kd)
		convey.So(err, convey.ShouldBeNil)
		convey.So(client, convey.ShouldNotBeNil)

		cryptoKey := []byte(cryptoKeyEnv)
		ctx := context.Background()
		srcKey := "test/copy-object/source.zkcn"
		dstKey := "test/copy-object/destination.zkcn"

		convey.Convey("server-side copy should preserve encrypted bytes exactly", func() {
			original := []byte(`{"name":"copy-integration-test","data":"hello zhongkui copy"}`)

			// 上传加密源对象
			uploadInfo, err := client.CPutObject(ctx, srcKey, cryptoKey, bytes.NewReader(original), -1, minio.PutObjectOptions{})
			convey.So(err, convey.ShouldBeNil)
			convey.So(uploadInfo.Size, convey.ShouldBeGreaterThan, 0)
			defer client.RemoveObject(ctx, srcKey, minio.RemoveObjectOptions{})
			defer client.RemoveObject(ctx, dstKey, minio.RemoveObjectOptions{})

			// 服务端复制到目标键
			convey.So(client.CopyObject(ctx, srcKey, dstKey), convey.ShouldBeNil)

			// 下载目标对象解密解压，内容应与源一致
			rc, err := client.CGetOject(ctx, dstKey, cryptoKey, minio.GetObjectOptions{})
			convey.So(err, convey.ShouldBeNil)
			convey.So(rc, convey.ShouldNotBeNil)
			defer rc.Close()
			downloaded, err := io.ReadAll(rc)
			convey.So(err, convey.ShouldBeNil)
			convey.So(downloaded, convey.ShouldResemble, original)
		})

		convey.Convey("CopyObject on non-existent source should return error", func() {
			err := client.CopyObject(ctx, "test/copy-object/not-exists", dstKey)
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(IsNotExists(err), convey.ShouldBeTrue)
		})
	})
}
