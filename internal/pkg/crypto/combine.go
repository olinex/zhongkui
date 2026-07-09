package crypto

import (
	"github.com/klauspost/compress/zstd"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

func NewZstdAESGCMPipelineWithLevel(cryptoKey []byte, compressLevel zstd.EncoderLevel) (*Pipeline, error) {
	if compressor, err := NewZstdCompressorWithLevel(compressLevel); err != nil {
		return nil, errors.WithStack(err)
	} else if encryptor, err := NewAESGCMEncryptor(cryptoKey); err != nil {
		return nil, errors.WithStack(err)
	} else if pipeline, err := NewCompressEncryptPipeline(compressor, encryptor); err != nil {
		return nil, errors.WithStack(err)
	} else {
		return pipeline, nil
	}
}

func NewZstdAESGCMPipeline(cryptoKey []byte) (*Pipeline, error) {
	return NewZstdAESGCMPipelineWithLevel(cryptoKey, zstd.SpeedDefault)
}
