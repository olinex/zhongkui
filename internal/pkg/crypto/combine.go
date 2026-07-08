package crypto

import (
	"github.com/klauspost/compress/zstd"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

func NewZstdAESGCMPipelineWithLevel(secretKey string, compressLevel zstd.EncoderLevel) (*Pipeline, error) {
	if compressor, err := NewZstdCompressorWithLevel(compressLevel); err != nil {
		return nil, errors.WithStack(err)
	} else if encryptor, err := NewAESGCMEncryptor([]byte(secretKey)); err != nil {
		return nil, errors.WithStack(err)
	} else if pipeline, err := NewCompressEncryptPipeline(compressor, encryptor); err != nil {
		return nil, errors.WithStack(err)
	} else {
		return pipeline, nil
	}
}

func NewZstdAESGCMPipeline(secretKey string) (*Pipeline, error) {
	return NewZstdAESGCMPipelineWithLevel(secretKey, zstd.SpeedDefault)
}
