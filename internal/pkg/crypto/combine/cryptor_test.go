package combine

import (
	"bytes"
	"io"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAESGCMEncryptor(t *testing.T) {
	Convey("Test AESGCMEncryptor", t, func() {
		Convey("Should create encryptor with valid key lengths", func() {
			for _, length := range []int{16, 24, 32} {
				key := make([]byte, length)
				e, err := NewAESGCMCryptor(key)
				So(err, ShouldBeNil)
				So(e, ShouldNotBeNil)
				So(e.ModeID(), ShouldEqual, MODE_ENCRYPTION_AES_GCM)
			}
		})

		Convey("Should reject invalid key lengths", func() {
			for _, length := range []int{0, 1, 8, 15, 17, 20, 31, 33} {
				key := make([]byte, length)
				_, err := NewAESGCMCryptor(key)
				So(err, ShouldNotBeNil)
			}
		})

		Convey("Should encrypt and decrypt data correctly", func() {
			key := make([]byte, 32)
			original := []byte("hello world, this is sensitive data!")

			e, err := NewAESGCMCryptor(key)
			So(err, ShouldBeNil)

			// 加密
			var encrypted bytes.Buffer
			w, err := e.WrapWriter(&encrypted)
			So(err, ShouldBeNil)

			_, err = w.Write(original)
			So(err, ShouldBeNil)
			err = w.Close()
			So(err, ShouldBeNil)

			// 加密后的数据应与原始数据不同
			So(encrypted.Bytes(), ShouldNotResemble, original)
			// 加密数据应包含 nonce + ciphertext
			So(encrypted.Len(), ShouldBeGreaterThan, len(original))

			// 解密
			r, err := e.WrapReader(&encrypted)
			So(err, ShouldBeNil)

			result, err := io.ReadAll(r)
			So(err, ShouldBeNil)
			err = r.Close()
			So(err, ShouldBeNil)

			So(result, ShouldResemble, original)
		})

		Convey("Should handle empty data", func() {
			key := make([]byte, 32)
			e, _ := NewAESGCMCryptor(key)

			var encrypted bytes.Buffer
			w, _ := e.WrapWriter(&encrypted)
			err := w.Close()
			So(err, ShouldBeNil)

			r, _ := e.WrapReader(&encrypted)
			result, err := io.ReadAll(r)
			So(err, ShouldBeNil)
			r.Close()

			So(result, ShouldResemble, []byte{})
		})

		Convey("Should handle large data", func() {
			key := make([]byte, 32)
			large := make([]byte, 1024*1024) // 1MB
			for i := range large {
				large[i] = byte(i % 256)
			}

			e, _ := NewAESGCMCryptor(key)

			var encrypted bytes.Buffer
			w, _ := e.WrapWriter(&encrypted)
			_, err := w.Write(large)
			So(err, ShouldBeNil)
			w.Close()

			r, _ := e.WrapReader(&encrypted)
			result, err := io.ReadAll(r)
			So(err, ShouldBeNil)
			r.Close()

			So(result, ShouldResemble, large)
		})

		Convey("Should fail to decrypt with wrong key", func() {
			key1 := make([]byte, 32)
			key1[31] = 0x01
			key2 := make([]byte, 32)
			key2[31] = 0x02
			original := []byte("secret data")

			e1, _ := NewAESGCMCryptor(key1)
			var encrypted bytes.Buffer
			w, _ := e1.WrapWriter(&encrypted)
			w.Write(original)
			w.Close()

			e2, _ := NewAESGCMCryptor(key2)
			r, _ := e2.WrapReader(&encrypted)
			_, err := io.ReadAll(r)
			So(err, ShouldNotBeNil)
			r.Close()
		})

		Convey("Each encryption should produce different ciphertext for same plaintext", func() {
			key := make([]byte, 32)
			original := []byte("same data")

			e, _ := NewAESGCMCryptor(key)

			var enc1, enc2 bytes.Buffer
			w1, _ := e.WrapWriter(&enc1)
			w1.Write(original)
			w1.Close()

			w2, _ := e.WrapWriter(&enc2)
			w2.Write(original)
			w2.Close()

			// 由于 nonce 随机，两次加密结果应不同
			So(enc1.Bytes(), ShouldNotResemble, enc2.Bytes())
		})
	})
}

func TestAESGCMEncryptorConcurrent(t *testing.T) {
	Convey("Test AESGCMEncryptor concurrent safety", t, func() {
		key := make([]byte, 32)
		e, _ := NewAESGCMCryptor(key)

		for i := 0; i < 10; i++ {
			var buf bytes.Buffer
			w, _ := e.WrapWriter(&buf)
			w.Write([]byte("concurrent test"))
			w.Close()

			r, _ := e.WrapReader(&buf)
			result, _ := io.ReadAll(r)
			r.Close()

			So(string(result), ShouldEqual, "concurrent test")
		}
	})
}
