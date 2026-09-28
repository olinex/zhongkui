package combine

import (
	"bytes"
	"io"
	"math/rand"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

// TestPipelineWithZstd 测试 Zstd 压缩 + AES-GCM 加密管道的往返与自描述头部。
func TestPipelineWithZstd(t *testing.T) {
	Convey("Test Pipeline with ZstdCompressor and AESGCMEncryptor", t, func() {
		key := make([]byte, 32)

		compressor, err := NewZstdCompressor()
		So(err, ShouldBeNil)

		encryptor, err := NewAESGCMCryptor(key)
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
			So(data[5], ShouldEqual, ModeCompressionZstd)
			// encryptID 应为 AES-GCM
			So(data[6], ShouldEqual, ModeEncryptionAesGcm)
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

// TestPipelineWithGzip 测试 Gzip 压缩 + AES-GCM 加密管道的往返与头部算法标识。
func TestPipelineWithGzip(t *testing.T) {
	Convey("Test Pipeline with GzipCompressor and AESGCMEncryptor", t, func() {
		key := make([]byte, 32)

		compressor := NewGzipCompressor()
		encryptor, _ := NewAESGCMCryptor(key)

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
			So(data[5], ShouldEqual, ModeCompressionGzip)
			So(data[6], ShouldEqual, ModeEncryptionAesGcm)
		})
	})
}

// TestPipelineAlgorithmMismatch 测试管道对算法不匹配、非法 magic 与不支持版本的拒绝行为。
func TestPipelineAlgorithmMismatch(t *testing.T) {
	Convey("Test Pipeline algorithm mismatch detection", t, func() {
		key := make([]byte, 32)

		Convey("Should reject mismatched compression algorithm", func() {
			// 用 gzip 压缩
			compressor := NewGzipCompressor()
			encryptor, _ := NewAESGCMCryptor(key)
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
			enc, _ := NewAESGCMCryptor(key)
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
			enc, _ := NewAESGCMCryptor(key)
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

// TestPipelineConstructErrors 测试构造管道时缺少 AlgoIDer 的错误处理。
func TestPipelineConstructErrors(t *testing.T) {
	Convey("Test Pipeline construction errors", t, func() {
		Convey("Should reject compressor without AlgoIDer", func() {
			// 使用匿名 struct 实现 Transform 但没有 AlgoIDer
			badCompressor := &mockTransform{}
			encryptor, _ := NewAESGCMCryptor(make([]byte, 32))

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

// TestPipelineConcurrent 测试同一管道实例在并发场景下的正确性。
func TestPipelineConcurrent(t *testing.T) {
	Convey("Test Pipeline concurrent usage", t, func() {
		key := make([]byte, 32)
		compressor, _ := NewZstdCompressor()
		encryptor, _ := NewAESGCMCryptor(key)
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

// ---------------------------------------------------------------------------
// pipelineReadCloser & DecryptDecompress Close 链测试
// ---------------------------------------------------------------------------

// trackingCloser 实现 io.ReadWriteCloser，将关闭调用记录到共享切片中。
// 写路径时通过 writer 字段透传数据，读路径时通过 reader 字段透传数据。
type trackingCloser struct {
	name    string
	reader  io.Reader
	writer  io.Writer
	records *[]string
}

func (tc *trackingCloser) Read(p []byte) (int, error) {
	return tc.reader.Read(p)
}

func (tc *trackingCloser) Write(p []byte) (int, error) {
	return tc.writer.Write(p)
}

func (tc *trackingCloser) Close() error {
	*tc.records = append(*tc.records, tc.name)
	return nil
}

// trackingTransform 实现 Transform + ModeIDer，WrapReader/WrapWriter 返回指定名称的 trackingCloser
type trackingTransform struct {
	readerName string
	algoID     byte
	records    *[]string
}

func (t *trackingTransform) ModeID() byte { return t.algoID }

func (t *trackingTransform) WrapWriter(w io.Writer) (io.WriteCloser, error) {
	tc := &trackingCloser{name: t.readerName, writer: w, records: t.records}
	return tc, nil
}

func (t *trackingTransform) WrapReader(r io.Reader) (io.ReadCloser, error) {
	tc := &trackingCloser{name: t.readerName, reader: r, records: t.records}
	return tc, nil
}

// TestPipelineReadCloserCloseOrder 测试 pipelineReadCloser 的关闭顺序（decomp → enc → src）。
func TestPipelineReadCloserCloseOrder(t *testing.T) {
	Convey("Test pipelineReadCloser Close order", t, func() {
		Convey("Should close decomp → enc → src in order", func() {
			var records []string

			src := &trackingCloser{name: "src", reader: &bytes.Buffer{}, records: &records}
			enc := &trackingCloser{name: "enc", reader: io.NopCloser(nil), records: &records}
			decomp := &trackingCloser{name: "decomp", reader: io.NopCloser(nil), records: &records}

			prc := &pipelineReadCloser{
				srcCloser:    src,
				encReader:    enc,
				decompReader: decomp,
			}

			err := prc.Close()
			So(err, ShouldBeNil)
			So(records, ShouldResemble, []string{"decomp", "enc", "src"})
		})

		Convey("Should not panic when srcCloser is nil", func() {
			var records []string

			enc := &trackingCloser{name: "enc", reader: io.NopCloser(nil), records: &records}
			decomp := &trackingCloser{name: "decomp", reader: io.NopCloser(nil), records: &records}

			prc := &pipelineReadCloser{
				srcCloser:    nil,
				encReader:    enc,
				decompReader: decomp,
			}

			err := prc.Close()
			So(err, ShouldBeNil)
			So(records, ShouldResemble, []string{"decomp", "enc"})
		})
	})
}

// TestDecryptDecompressCloseAllResources 测试 DecryptDecompress 关闭各层资源（含源 io.Closer）。
func TestDecryptDecompressCloseAllResources(t *testing.T) {
	Convey("Test DecryptDecompress closes all layers including source io.Closer", t, func() {
		var records []string

		// 创建带追踪的 compressor 和 encryptor
		compressor := &trackingTransform{readerName: "decomp", algoID: ModeCompressionZstd, records: &records}
		encryptor := &trackingTransform{readerName: "enc", algoID: ModeEncryptionAesGcm, records: &records}

		pipeline, err := NewCompressEncryptPipeline(compressor, encryptor)
		So(err, ShouldBeNil)

		Convey("When source implements io.Closer, should close src too", func() {
			// 先将数据写入头部（模拟加密压缩后的数据）
			var buf bytes.Buffer
			w, err := pipeline.CompressEncrypt(&buf)
			So(err, ShouldBeNil)
			w.Write([]byte("test data"))
			w.Close()

			// 使用 trackingCloser 包裹源数据，验证关闭链
			records = nil // 重置记录（排除写入阶段的记录）
			src := &trackingCloser{name: "src", reader: &buf, records: &records}

			r, err := pipeline.DecryptDecompress(src)
			So(err, ShouldBeNil)

			_, err = io.ReadAll(r)
			So(err, ShouldBeNil)

			err = r.Close()
			So(err, ShouldBeNil)

			// 验证关闭顺序：decomp → enc → src
			So(records, ShouldResemble, []string{"decomp", "enc", "src"})
		})

		Convey("When source is plain io.Reader, should not panic", func() {
			var buf bytes.Buffer
			w, err := pipeline.CompressEncrypt(&buf)
			So(err, ShouldBeNil)
			w.Write([]byte("hello"))
			w.Close()

			records = nil
			// bytes.Buffer 不实现 io.Closer，作为纯 io.Reader 传入
			r, err := pipeline.DecryptDecompress(&buf)
			So(err, ShouldBeNil)

			_, err = io.ReadAll(r)
			So(err, ShouldBeNil)

			err = r.Close()
			So(err, ShouldBeNil)

			// 只关闭 decomp 和 enc，没有 src
			So(records, ShouldResemble, []string{"decomp", "enc"})
		})
	})
}
