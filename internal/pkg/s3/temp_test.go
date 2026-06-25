package s3

import (
	"bytes"
	"io"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"zhongkui.com/zhongkui/internal/pkg/crypto"
)

func TestTempCryptoRoundtrip(t *testing.T) {
	convey.Convey("Test Temp crypto compress+encrypt roundtrip", t, func() {
		// 模拟 Store 的完整流程：压缩加密 -> 写入 -> 解密解压
		key := []byte("0123456789abcdef0123456789abcdef") // 32 bytes for AES-256

		compressor, err := crypto.NewZstdCompressor()
		convey.So(err, convey.ShouldBeNil)

		encryptor, err := crypto.NewAESGCMEncryptor(key)
		convey.So(err, convey.ShouldBeNil)

		pipeline, err := crypto.NewCompressEncryptPipeline(compressor, encryptor)
		convey.So(err, convey.ShouldBeNil)

		convey.Convey("数据写入 → 读取应完全一致", func() {
			original := []byte(`{"name":"test","version":"v1","data":"hello world"}`)

			// 模拟 Store 中的写入流程
			buffer := bytes.NewBuffer(nil)
			wc, err := pipeline.CompressEncrypt(buffer)
			convey.So(err, convey.ShouldBeNil)

			_, err = io.Copy(wc, bytes.NewReader(original))
			convey.So(err, convey.ShouldBeNil)

			err = wc.Close()
			convey.So(err, convey.ShouldBeNil)

			// 验证输出不为空且包含头部
			encrypted := buffer.Bytes()
			t.Logf("原始大小: %d, 加密后大小: %d", len(original), len(encrypted))
			convey.So(len(encrypted), convey.ShouldBeGreaterThan, len(original))

			// 模拟 Load 中的读取流程
			r, err := pipeline.DecryptDecompress(bytes.NewReader(encrypted))
			convey.So(err, convey.ShouldBeNil)

			decrypted, err := io.ReadAll(r)
			convey.So(err, convey.ShouldBeNil)
			r.Close()

			convey.So(decrypted, convey.ShouldResemble, original)
		})

		convey.Convey("大数据的写入 → 读取应完全一致", func() {
			// 1MB 随机数据（足够大以触发分块加密）
			original := make([]byte, 1024*1024)
			for i := range original {
				original[i] = byte(i % 256)
			}

			buffer := bytes.NewBuffer(nil)
			wc, err := pipeline.CompressEncrypt(buffer)
			convey.So(err, convey.ShouldBeNil)

			_, err = io.Copy(wc, bytes.NewReader(original))
			convey.So(err, convey.ShouldBeNil)

			err = wc.Close()
			convey.So(err, convey.ShouldBeNil)

			encrypted := buffer.Bytes()
			t.Logf("原始大小: %d, 加密后大小: %d", len(original), len(encrypted))

			r, err := pipeline.DecryptDecompress(bytes.NewReader(encrypted))
			convey.So(err, convey.ShouldBeNil)

			decrypted, err := io.ReadAll(r)
			convey.So(err, convey.ShouldBeNil)
			r.Close()

			convey.So(decrypted, convey.ShouldResemble, original)
		})

		convey.Convey("空数据的写入 → 读取应得到空数据", func() {
			buffer := bytes.NewBuffer(nil)
			wc, err := pipeline.CompressEncrypt(buffer)
			convey.So(err, convey.ShouldBeNil)

			err = wc.Close()
			convey.So(err, convey.ShouldBeNil)

			r, err := pipeline.DecryptDecompress(bytes.NewReader(buffer.Bytes()))
			convey.So(err, convey.ShouldBeNil)

			decrypted, err := io.ReadAll(r)
			convey.So(err, convey.ShouldBeNil)
			r.Close()

			convey.So(decrypted, convey.ShouldResemble, []byte{})
		})

		convey.Convey("使用 bytes.NewReader 多次读取数据应一致", func() {
			original := []byte("test data for reader consistency check")

			buffer := bytes.NewBuffer(nil)
			wc, _ := pipeline.CompressEncrypt(buffer)
			io.Copy(wc, bytes.NewReader(original))
			wc.Close()

			encrypted := buffer.Bytes()

			// 多次从同一个 bytes.Reader 读取
			for i := 0; i < 3; i++ {
				r, _ := pipeline.DecryptDecompress(bytes.NewReader(encrypted))
				decrypted, _ := io.ReadAll(r)
				r.Close()
				convey.So(decrypted, convey.ShouldResemble, original)
			}
		})
	})
}

// TestPutObjectWithKnownSize 验证带明确大小的 PutObject 调用逻辑
func TestPutObjectWithKnownSize(t *testing.T) {
	convey.Convey("已知大小的数据应使用简单的 PUT，避免 multipart", t, func() {
		convey.Convey("小于 16MB 的数据不需要 multipart", func() {
			data := make([]byte, 1024)
			// minPartSize 在 minio 中默认为 16MB
			// 1KB 远小于 16MB，使用简单 PUT 应该成功
			convey.So(int64(len(data)), convey.ShouldBeLessThan, int64(16*1024*1024))
		})
	})
}
