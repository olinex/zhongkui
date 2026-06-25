package crypto

import (
	"bytes"
	"io"
	"math/rand"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPipelineWithZstd(t *testing.T) {
	Convey("Test Pipeline with ZstdCompressor and AESGCMEncryptor", t, func() {
		key := make([]byte, 32)

		compressor, err := NewZstdCompressor()
		So(err, ShouldBeNil)

		encryptor, err := NewAESGCMEncryptor(key)
		So(err, ShouldBeNil)

		pipeline, err := NewCompressEncryptPipeline(compressor, encryptor)
		So(err, ShouldBeNil)

		Convey("Should compress-encrypt and decrypt-decompress correctly", func() {
			original := []byte("hello world, this is a streaming compress-then-encrypt test!")

			var buf bytes.Buffer
			w, err := pipeline.CompressEncrypt(&buf)
			So(err, ShouldBeNil)

			_, err = w.Write(original)
			So(err, ShouldBeNil)
			err = w.Close()
			So(err, ShouldBeNil)

			// 输出应包含头部 + nonce + 密文
			So(buf.Len(), ShouldBeGreaterThan, len(original))

			// 读取方
			r, err := pipeline.DecryptDecompress(&buf)
			So(err, ShouldBeNil)

			result, err := io.ReadAll(r)
			So(err, ShouldBeNil)
			err = r.Close()
			So(err, ShouldBeNil)

			So(result, ShouldResemble, original)
		})

		Convey("Output should contain self-describing header", func() {
			original := []byte("test data")
			var buf bytes.Buffer

			w, _ := pipeline.CompressEncrypt(&buf)
			w.Write(original)
			w.Close()

			data := buf.Bytes()
			// 前 4 字节应为 magic
			So(string(data[0:4]), ShouldEqual, "\x5a\x4b\x43\x4e")
			// version 应为 1
			So(data[4], ShouldEqual, 0x01)
			// compressID 应为 Zstd
			So(data[5], ShouldEqual, AlgoCompressionZstd)
			// encryptID 应为 AES-GCM
			So(data[6], ShouldEqual, AlgoEncryptionAESGCM)
		})

		Convey("Should handle empty data", func() {
			var buf bytes.Buffer
			w, _ := pipeline.CompressEncrypt(&buf)
			err := w.Close()
			So(err, ShouldBeNil)

			r, _ := pipeline.DecryptDecompress(&buf)
			result, err := io.ReadAll(r)
			So(err, ShouldBeNil)
			r.Close()

			So(result, ShouldResemble, []byte{})
		})

		Convey("Should handle large data", func() {
			large := make([]byte, 512*1024) // 512KB
			_, err := rand.Read(large)
			So(err, ShouldBeNil)

			var buf bytes.Buffer
			w, _ := pipeline.CompressEncrypt(&buf)
			_, err = w.Write(large)
			So(err, ShouldBeNil)
			w.Close()

			r, _ := pipeline.DecryptDecompress(&buf)
			result, err := io.ReadAll(r)
			So(err, ShouldBeNil)
			r.Close()

			So(result, ShouldResemble, large)
		})

		Convey("Should handle multiple write chunks", func() {
			var buf bytes.Buffer
			w, _ := pipeline.CompressEncrypt(&buf)

			w.Write([]byte("chunk1"))
			w.Write([]byte("-chunk2"))
			w.Write([]byte("-chunk3"))
			w.Close()

			r, _ := pipeline.DecryptDecompress(&buf)
			result, _ := io.ReadAll(r)
			r.Close()

			So(string(result), ShouldEqual, "chunk1-chunk2-chunk3")
		})
	})
}

func TestPipelineWithGzip(t *testing.T) {
	Convey("Test Pipeline with GzipCompressor and AESGCMEncryptor", t, func() {
		key := make([]byte, 32)

		compressor := NewGzipCompressor()
		encryptor, _ := NewAESGCMEncryptor(key)

		pipeline, err := NewCompressEncryptPipeline(compressor, encryptor)
		So(err, ShouldBeNil)

		Convey("Should compress-encrypt and decrypt-decompress correctly", func() {
			original := []byte("hello world from gzip pipeline!")

			var buf bytes.Buffer
			w, _ := pipeline.CompressEncrypt(&buf)
			w.Write(original)
			w.Close()

			r, _ := pipeline.DecryptDecompress(&buf)
			result, _ := io.ReadAll(r)
			r.Close()

			So(result, ShouldResemble, original)
		})

		Convey("Header should contain gzip algo ID", func() {
			var buf bytes.Buffer
			w, _ := pipeline.CompressEncrypt(&buf)
			w.Write([]byte("test"))
			w.Close()

			data := buf.Bytes()
			So(data[5], ShouldEqual, AlgoCompressionGzip)
			So(data[6], ShouldEqual, AlgoEncryptionAESGCM)
		})
	})
}

func TestPipelineAlgorithmMismatch(t *testing.T) {
	Convey("Test Pipeline algorithm mismatch detection", t, func() {
		key := make([]byte, 32)

		Convey("Should reject mismatched compression algorithm", func() {
			// 用 gzip 压缩
			compressor := NewGzipCompressor()
			encryptor, _ := NewAESGCMEncryptor(key)
			pipeline1, _ := NewCompressEncryptPipeline(compressor, encryptor)

			var buf bytes.Buffer
			w, _ := pipeline1.CompressEncrypt(&buf)
			w.Write([]byte("data"))
			w.Close()

			// 用 zstd 解压，应失败
			zstdCompressor, _ := NewZstdCompressor()
			pipeline2, _ := NewCompressEncryptPipeline(zstdCompressor, encryptor)
			_, err := pipeline2.DecryptDecompress(&buf)
			So(err, ShouldNotBeNil)
		})

		Convey("Should reject invalid magic bytes", func() {
			compressor := NewGzipCompressor()
			enc, _ := NewAESGCMEncryptor(key)
			pipeline, _ := NewCompressEncryptPipeline(compressor, enc)

			invalidReader := bytes.NewReader([]byte{
				0x00, 0x00, 0x00, 0x00, // 错误的 magic
				0x01, // version
				0x01, // compressID
				0x01, // encryptID
			})
			_, err := pipeline.DecryptDecompress(invalidReader)
			So(err, ShouldNotBeNil)
		})

		Convey("Should reject unsupported version", func() {
			compressor := NewGzipCompressor()
			enc, _ := NewAESGCMEncryptor(key)
			pipeline, _ := NewCompressEncryptPipeline(compressor, enc)

			invalidReader := bytes.NewReader([]byte{
				0x5a, 0x4b, 0x43, 0x4e, // 正确的 magic
				0xFF, // 不支持的 version
				0x02, // compressID
				0x01, // encryptID
			})
			_, err := pipeline.DecryptDecompress(invalidReader)
			So(err, ShouldNotBeNil)
		})
	})
}

func TestPipelineConstructErrors(t *testing.T) {
	Convey("Test Pipeline construction errors", t, func() {
		Convey("Should reject compressor without AlgoIDer", func() {
			// 使用匿名 struct 实现 Transform 但没有 AlgoIDer
			badCompressor := &mockTransform{}
			encryptor, _ := NewAESGCMEncryptor(make([]byte, 32))

			_, err := NewCompressEncryptPipeline(badCompressor, encryptor)
			So(err, ShouldNotBeNil)
		})

		Convey("Should reject encryptor without AlgoIDer", func() {
			compressor := NewGzipCompressor()
			badEncryptor := &mockTransform{}

			_, err := NewCompressEncryptPipeline(compressor, badEncryptor)
			So(err, ShouldNotBeNil)
		})
	})
}

func TestPipelineConcurrent(t *testing.T) {
	Convey("Test Pipeline concurrent usage", t, func() {
		key := make([]byte, 32)
		compressor, _ := NewZstdCompressor()
		encryptor, _ := NewAESGCMEncryptor(key)
		pipeline, _ := NewCompressEncryptPipeline(compressor, encryptor)

		var wg sync.WaitGroup
		errs := make([]error, 20)
		results := make([][]byte, 20)

		for i := 0; i < 20; i++ {
			wg.Add(1)
			idx := i
			go func() {
				defer wg.Done()
				original := []byte("concurrent pipeline test data")

				var buf bytes.Buffer
				w, err := pipeline.CompressEncrypt(&buf)
				if err != nil {
					errs[idx] = err
					return
				}
				w.Write(original)
				w.Close()

				r, err := pipeline.DecryptDecompress(&buf)
				if err != nil {
					errs[idx] = err
					return
				}
				result, err := io.ReadAll(r)
				r.Close()
				if err != nil {
					errs[idx] = err
					return
				}

				results[idx] = result
			}()
		}
		wg.Wait()

		for i, err := range errs {
			So(err, ShouldBeNil)
			So(results[i], ShouldResemble, []byte("concurrent pipeline test data"))
		}
	})
}

// mockTransform 实现 Transform 但不实现 AlgoIDer，用于测试构造错误
type mockTransform struct{}

func (m *mockTransform) WrapWriter(w io.Writer) (io.WriteCloser, error) {
	return struct {
		io.Writer
		io.Closer
	}{w, io.NopCloser(nil)}, nil
}

func (m *mockTransform) WrapReader(r io.Reader) (io.ReadCloser, error) {
	return io.NopCloser(r), nil
}
