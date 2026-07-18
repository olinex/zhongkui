// Package combine 提供流式数据压缩与加密的组合管道。
//
// 本包实现了"先压缩后加密"的数据处理管道，支持：
//   - 压缩算法：Zstandard（默认）、Gzip
//   - 加密算法：AES-GCM（认证加密）
//
// 数据流格式：
//
//	写入：明文 → 压缩 → 加密 → 输出（带自描述头部）
//	读取：输入 → 解密 → 解压 → 明文
//
// 输出流头部格式（7 字节）：
//
//	[4B magic "ZKCN"][1B version][1B compressID][1B encryptID]
//
// 加密采用分块模式（64KB 一块），每块格式：
//
//	[4B 密文长度][12B nonce][密文+认证标签]
//
// Pipeline 结构支持自描述：读取端根据头部信息自动选择正确的算法。
package combine

import (
	"github.com/klauspost/compress/zstd"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// NewZstdAESGCMPipelineWithLevel 创建使用指定压缩级别的 Zstd+AES-GCM 管道。
//
// 参数：
//   - cryptoKey: AES 密钥，长度必须为 16、24 或 32 字节
//   - compressLevel: Zstd 压缩级别
//
// 返回值：
//   - *Pipeline: 成功时返回管道实例
//   - error: 压缩器或加密器创建失败时返回错误
func NewZstdAESGCMPipelineWithLevel(cryptoKey []byte, compressLevel zstd.EncoderLevel) (*Pipeline, error) {
	if compressor, err := NewZstdCompressorWithLevel(compressLevel); err != nil {
		return nil, errors.WithStack(err)
	} else if encryptor, err := NewAESGCMCryptor(cryptoKey); err != nil {
		return nil, errors.WithStack(err)
	} else if pipeline, err := NewCompressEncryptPipeline(compressor, encryptor); err != nil {
		return nil, errors.WithStack(err)
	} else {
		return pipeline, nil
	}
}

// NewZstdAESGCMPipeline 创建使用默认压缩级别的 Zstd+AES-GCM 管道。
//
// 这是最常用的工厂函数，使用 Zstd 默认压缩级别和 AES-GCM 加密。
//
// 参数：
//   - cryptoKey: AES 密钥，长度必须为 16、24 或 32 字节
//
// 返回值：
//   - *Pipeline: 成功时返回管道实例
//   - error: 创建失败时返回错误
func NewZstdAESGCMPipeline(cryptoKey []byte) (*Pipeline, error) {
	return NewZstdAESGCMPipelineWithLevel(cryptoKey, zstd.SpeedDefault)
}
