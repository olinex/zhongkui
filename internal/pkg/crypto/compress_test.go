package crypto

import (
	"bytes"
	"io"
	"math/rand"
	"testing"

	"github.com/klauspost/compress/zstd"
	. "github.com/smartystreets/goconvey/convey"
)

func TestZstdCompressor(t *testing.T) {
	Convey("Test ZstdCompressor", t, func() {
		Convey("Should create compressor with default level", func() {
			c, err := NewZstdCompressor()
			So(err, ShouldBeNil)
			So(c.AlgoID(), ShouldEqual, AlgoCompressionZstd)
		})

		Convey("Should compress and decompress data correctly", func() {
			original := []byte("hello world, this is a test of zstd compression!")

			var buf bytes.Buffer

			c, err := NewZstdCompressor()
			So(err, ShouldBeNil)

			w, err := c.WrapWriter(&buf)
			So(err, ShouldBeNil)

			_, err = w.Write(original)
			So(err, ShouldBeNil)
			err = w.Close()
			So(err, ShouldBeNil)

			// 压缩后的数据应与原始数据不同
			So(buf.Bytes(), ShouldNotResemble, original)

			// 解压
			r, err := c.WrapReader(&buf)
			So(err, ShouldBeNil)

			result, err := io.ReadAll(r)
			So(err, ShouldBeNil)
			err = r.Close()
			So(err, ShouldBeNil)

			So(result, ShouldResemble, original)
		})

		Convey("Should handle empty data", func() {
			var buf bytes.Buffer

			c, _ := NewZstdCompressor()
			w, _ := c.WrapWriter(&buf)
			err := w.Close()
			So(err, ShouldBeNil)

			r, _ := c.WrapReader(&buf)
			result, err := io.ReadAll(r)
			So(err, ShouldBeNil)
			r.Close()

			So(result, ShouldResemble, []byte{})
		})

		Convey("Should handle large data", func() {
			large := make([]byte, 1024*1024) // 1MB
			_, err := rand.Read(large)
			So(err, ShouldBeNil)

			var buf bytes.Buffer
			c, _ := NewZstdCompressor()

			w, _ := c.WrapWriter(&buf)
			_, err = w.Write(large)
			So(err, ShouldBeNil)
			w.Close()

			r, _ := c.WrapReader(&buf)
			result, err := io.ReadAll(r)
			So(err, ShouldBeNil)
			r.Close()

			So(result, ShouldResemble, large)
		})

		Convey("Should support different compression levels", func() {
			levels := []int{
				int(zstd.SpeedFastest),
				int(zstd.SpeedDefault),
				int(zstd.SpeedBestCompression),
			}

			for _, lvl := range levels {
				c, err := NewZstdCompressorWithLevel(zstd.EncoderLevel(lvl))
				So(err, ShouldBeNil)

				var buf bytes.Buffer
				w, _ := c.WrapWriter(&buf)
				w.Write([]byte("test data for compression level"))
				w.Close()

				r, _ := c.WrapReader(&buf)
				result, _ := io.ReadAll(r)
				r.Close()

				So(string(result), ShouldEqual, "test data for compression level")
			}
		})
	})
}

func TestGzipCompressor(t *testing.T) {
	Convey("Test GzipCompressor", t, func() {
		Convey("Should create compressor with default level", func() {
			c := NewGzipCompressor()
			So(c, ShouldNotBeNil)
			So(c.AlgoID(), ShouldEqual, AlgoCompressionGzip)
		})

		Convey("Should compress and decompress data correctly", func() {
			original := []byte("hello world, this is a test of gzip compression!")

			var buf bytes.Buffer

			c := NewGzipCompressor()

			w, err := c.WrapWriter(&buf)
			So(err, ShouldBeNil)

			_, err = w.Write(original)
			So(err, ShouldBeNil)
			err = w.Close()
			So(err, ShouldBeNil)

			// 压缩后的数据应与原始数据不同
			So(buf.Bytes(), ShouldNotResemble, original)

			// 解压
			r, err := c.WrapReader(&buf)
			So(err, ShouldBeNil)

			result, err := io.ReadAll(r)
			So(err, ShouldBeNil)
			err = r.Close()
			So(err, ShouldBeNil)

			So(result, ShouldResemble, original)
		})

		Convey("Should handle empty data", func() {
			var buf bytes.Buffer

			c := NewGzipCompressor()
			w, _ := c.WrapWriter(&buf)
			err := w.Close()
			So(err, ShouldBeNil)

			r, _ := c.WrapReader(&buf)
			result, err := io.ReadAll(r)
			So(err, ShouldBeNil)
			r.Close()

			So(result, ShouldResemble, []byte{})
		})

		Convey("Should reject invalid compression level", func() {
			_, err := NewGzipCompressorWithLevel(99)
			So(err, ShouldNotBeNil)

			_, err = NewGzipCompressorWithLevel(-5)
			So(err, ShouldNotBeNil)
		})
	})
}
