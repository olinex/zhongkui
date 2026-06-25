package crypto

import (
	"compress/gzip"
	"io"

	"github.com/klauspost/compress/zstd"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// ---------------------------------------------------------------------------
// ZstdCompressor — 基于 Zstandard 的压缩/解压（默认）
// ---------------------------------------------------------------------------

// ZstdCompressor 使用 Zstandard 算法进行流式压缩和解压。
type ZstdCompressor struct {
	level zstd.EncoderLevel
}

// NewZstdCompressor 创建默认级别的 ZstdCompressor。
func NewZstdCompressor() (*ZstdCompressor, error) {
	return NewZstdCompressorWithLevel(zstd.SpeedDefault)
}

// NewZstdCompressorWithLevel 创建指定压缩级别的 ZstdCompressor。
// level: zstd.EncoderLevel，例如 zstd.SpeedDefault、zstd.SpeedBestCompression 等。
func NewZstdCompressorWithLevel(level zstd.EncoderLevel) (*ZstdCompressor, error) {
	return &ZstdCompressor{level: level}, nil
}

// AlgoID 返回 Zstd 压缩算法的标识字节。
func (c *ZstdCompressor) AlgoID() byte { return AlgoCompressionZstd }

// WrapWriter 包装 io.Writer，返回一个写入即压缩的 WriteCloser。
func (c *ZstdCompressor) WrapWriter(w io.Writer) (io.WriteCloser, error) {
	zw, err := zstd.NewWriter(w, zstd.WithEncoderLevel(c.level))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return zw, nil
}

// WrapReader 包装 io.Reader，返回一个读取即解压的 ReadCloser。
func (c *ZstdCompressor) WrapReader(r io.Reader) (io.ReadCloser, error) {
	zr, err := zstd.NewReader(r)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return zr.IOReadCloser(), nil
}

// ---------------------------------------------------------------------------
// GzipCompressor — 基于 Gzip 的压缩/解压（备选）
// ---------------------------------------------------------------------------

// GzipCompressor 使用 Gzip 算法进行流式压缩和解压。
type GzipCompressor struct {
	level int
}

// NewGzipCompressor 创建默认压缩级别（gzip.DefaultCompression）的 GzipCompressor。
func NewGzipCompressor() *GzipCompressor {
	return &GzipCompressor{level: gzip.DefaultCompression}
}

// NewGzipCompressorWithLevel 创建指定压缩级别的 GzipCompressor。
// level: gzip 压缩级别常量，如 gzip.BestSpeed、gzip.BestCompression 等。
// 如果 level 无效，返回错误。
func NewGzipCompressorWithLevel(level int) (*GzipCompressor, error) {
	if level < gzip.HuffmanOnly || level > gzip.BestCompression {
		return nil, errors.Newf("invalid gzip compression level: %d", level)
	}
	return &GzipCompressor{level: level}, nil
}

// AlgoID 返回 Gzip 压缩算法的标识字节。
func (c *GzipCompressor) AlgoID() byte { return AlgoCompressionGzip }

// WrapWriter 包装 io.Writer，返回一个写入即压缩的 WriteCloser。
func (c *GzipCompressor) WrapWriter(w io.Writer) (io.WriteCloser, error) {
	gw, err := gzip.NewWriterLevel(w, c.level)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return gw, nil
}

// WrapReader 包装 io.Reader，返回一个读取即解压的 ReadCloser。
func (c *GzipCompressor) WrapReader(r io.Reader) (io.ReadCloser, error) {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return gr, nil
}
