package s3

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/minio/minio-go/v7"
	"github.com/smartystreets/goconvey/convey"
	"zhongkui.com/zhongkui/internal/pkg/crypto/combine"
	"zhongkui.com/zhongkui/internal/pkg/crypto/key"
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
