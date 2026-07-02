package crypto

import (
	"io"

	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// ---------------------------------------------------------------------------
// 头部常量
// ---------------------------------------------------------------------------

const (
	FILE_EXT              = "zkcn"
	FILE_DOT_EXT          = "." + FILE_EXT
	COMPRESS_CONTENT_TYPE = "application/zkcn"
	MAGIC_BYTES           = "\x5a\x4b\x43\x4e" // "ZKCN" — Zhongkui Crypto Notation
	HEADER_VERSION        = 0x01
	HEADER_SIZE           = 7 // magic(4) + version(1) + compressID(1) + encryptID(1)
)

// Pipeline 表示一个流式变换管道，固定按 先压缩(Compress) → 后加密(Encrypt) 的顺序处理数据。
// 输出流包含自描述头部，携带压缩算法和加密算法的标识信息。
type Pipeline struct {
	compressor Transform
	encryptor  Transform
	compressID byte
	encryptID  byte
}

// NewCompressEncryptPipeline 创建先压缩后加密的管道。
// compressor 和 encryptor 需实现 AlgoIDer 接口，否则返回错误。
func NewCompressEncryptPipeline(compressor, encryptor Transform) (*Pipeline, error) {
	cIDer, ok := compressor.(AlgoIDer)
	if !ok {
		return nil, errors.New("compressor does not implement AlgoIDer")
	}
	eIDer, ok := encryptor.(AlgoIDer)
	if !ok {
		return nil, errors.New("encryptor does not implement AlgoIDer")
	}
	return &Pipeline{
		compressor: compressor,
		encryptor:  encryptor,
		compressID: cIDer.AlgoID(),
		encryptID:  eIDer.AlgoID(),
	}, nil
}

// CompressEncrypt 返回一个 WriteCloser：
// 写入的数据依次经 压缩→加密 后写入 w，并自动在数据头部写入自描述信息。
// 必须调用 Close() 以确保所有数据刷新到底层 writer。
func (p *Pipeline) CompressEncrypt(w io.Writer) (io.WriteCloser, error) {
	// 写入自描述头部
	header := make([]byte, HEADER_SIZE)
	copy(header[0:4], MAGIC_BYTES)
	header[4] = HEADER_VERSION
	header[5] = p.compressID
	header[6] = p.encryptID

	if _, err := w.Write(header); err != nil {
		return nil, errors.WithStack(err)
	}

	// 创建加密写入器（包裹 w）
	encWriter, err := p.encryptor.WrapWriter(w)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// 创建压缩写入器（包裹 encWriter）
	compWriter, err := p.compressor.WrapWriter(encWriter)
	if err != nil {
		// 尝试关闭已创建的 encWriter
		encWriter.Close()
		return nil, errors.WithStack(err)
	}

	return &pipelineWriteCloser{
		compWriter: compWriter,
		encWriter:  encWriter,
	}, nil
}

// DecryptDecompress 返回一个 ReadCloser：
// 从 r 读取数据时自动识别头部算法信息，依次 解密→解压 后返回明文。
// 读取完成后调用 Close() 释放资源。
func (p *Pipeline) DecryptDecompress(r io.Reader) (io.ReadCloser, error) {
	// 读取并校验自描述头部
	header := make([]byte, HEADER_SIZE)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, errors.WithStack(err)
	}

	if err := p.validateHeader(header); err != nil {
		return nil, err
	}

	// 创建解密读取器（包裹 r，自动读 nonce + 解密）
	encReader, err := p.encryptor.WrapReader(r)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// 创建解压读取器（包裹 encReader）
	decompReader, err := p.compressor.WrapReader(encReader)
	if err != nil {
		encReader.Close()
		return nil, errors.WithStack(err)
	}

	return decompReader, nil
}

func (p *Pipeline) validateHeader(header []byte) error {
	if string(header[0:4]) != MAGIC_BYTES {
		return errors.New("invalid magic bytes in stream header")
	}
	if header[4] != HEADER_VERSION {
		return errors.Newf("unsupported stream version: %d", header[4])
	}
	if header[5] != p.compressID {
		return errors.Newf("compression algorithm mismatch: header=%d, pipeline=%d", header[5], p.compressID)
	}
	if header[6] != p.encryptID {
		return errors.Newf("encryption algorithm mismatch: header=%d, pipeline=%d", header[6], p.encryptID)
	}
	return nil
}

// ---------------------------------------------------------------------------
// pipelineWriteCloser — 组合写入关闭逻辑
// ---------------------------------------------------------------------------

type pipelineWriteCloser struct {
	compWriter io.WriteCloser
	encWriter  io.WriteCloser
}

func (pwc *pipelineWriteCloser) Write(p []byte) (int, error) {
	return pwc.compWriter.Write(p)
}

func (pwc *pipelineWriteCloser) Close() error {
	// 先关闭压缩写入器（刷新缓冲区，写入剩余压缩数据）
	if err := pwc.compWriter.Close(); err != nil {
		// 仍尝试关闭加密写入器
		pwc.encWriter.Close()
		return errors.WithStack(err)
	}
	// 再关闭加密写入器（写入 nonce + 加密所有数据）
	if err := pwc.encWriter.Close(); err != nil {
		return errors.WithStack(err)
	}
	return nil
}
