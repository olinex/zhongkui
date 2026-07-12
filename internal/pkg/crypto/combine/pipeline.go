package combine

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
	cryptor    Transform
	compressID byte
	cryptID    byte
}

// NewCompressEncryptPipeline 创建先压缩后加密的管道。
// compressor 和 encryptor 需实现 AlgoIDer 接口，否则返回错误。
func NewCompressEncryptPipeline(compressor, cryptor Transform) (*Pipeline, error) {
	cIDer, ok := compressor.(ModeIDer)
	if !ok {
		return nil, errors.New("compressor does not implement ModeIDer")
	}
	eIDer, ok := cryptor.(ModeIDer)
	if !ok {
		return nil, errors.New("encryptor does not implement ModeIDer")
	}
	return &Pipeline{
		compressor: compressor,
		cryptor:    cryptor,
		compressID: cIDer.ModeID(),
		cryptID:    eIDer.ModeID(),
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
	header[6] = p.cryptID

	if _, err := w.Write(header); err != nil {
		return nil, errors.WithStack(err)
	}

	// 创建加密写入器（包裹 w）
	encWriter, err := p.cryptor.WrapWriter(w)
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
	encReader, err := p.cryptor.WrapReader(r)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// 创建解压读取器（包裹 encReader）
	decompReader, err := p.compressor.WrapReader(encReader)
	if err != nil {
		encReader.Close()
		return nil, errors.WithStack(err)
	}

	// 收集源 reader 的 Closer（如果实现了的话），用于级联关闭
	var srcCloser io.Closer
	if closer, ok := r.(io.Closer); ok {
		srcCloser = closer
	}

	return &pipelineReadCloser{
		srcCloser:    srcCloser,
		encReader:    encReader,
		decompReader: decompReader,
	}, nil
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
	if header[6] != p.cryptID {
		return errors.Newf("encryption algorithm mismatch: header=%d, pipeline=%d", header[6], p.cryptID)
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

// ---------------------------------------------------------------------------
// pipelineReadCloser — 组合读取关闭逻辑
// ---------------------------------------------------------------------------

type pipelineReadCloser struct {
	srcCloser    io.Closer     // 源 reader 的 Closer，可能为 nil（当源不是 io.Closer 时）
	encReader    io.ReadCloser // 解密读取器（内层）
	decompReader io.ReadCloser // 解压读取器（外层，直接面向调用者）
}

func (prc *pipelineReadCloser) Read(p []byte) (int, error) {
	return prc.decompReader.Read(p)
}

func (prc *pipelineReadCloser) Close() error {
	// 先关闭解压读取器（释放解压器内部资源）
	if err := prc.decompReader.Close(); err != nil {
		// 仍尝试关闭剩余层
		prc.encReader.Close()
		if prc.srcCloser != nil {
			prc.srcCloser.Close()
		}
		return errors.WithStack(err)
	}
	// 再关闭解密读取器（清理加密状态）
	if err := prc.encReader.Close(); err != nil {
		if prc.srcCloser != nil {
			prc.srcCloser.Close()
		}
		return errors.WithStack(err)
	}
	// 最后关闭源 reader（如 *minio.Object，释放 HTTP 连接）
	if prc.srcCloser != nil {
		if err := prc.srcCloser.Close(); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}
