// Package crypto 提供密码学相关的工具函数，包括密码哈希、随机字符串生成、签名验签以及流式压缩加密等功能。
package crypto

import "io"

// Transform 定义了流式数据变换的抽象接口。
// 压缩器和加密器均实现此接口，可通过组合构建流式处理管道。
//
// WrapWriter 包装 io.Writer，返回一个 WriteCloser：
// 写入的数据经过变换后写入底层 writer。
//
// WrapReader 包装 io.Reader，返回一个 ReadCloser：
// 从底层 reader 读取的数据经过反向变换后返回给调用方。
type Transform interface {
	WrapWriter(w io.Writer) (io.WriteCloser, error)
	WrapReader(r io.Reader) (io.ReadCloser, error)
}

// AlgoIDer 标识算法的类型，用于自描述流头部写入。
type AlgoIDer interface {
	AlgoID() byte
}

// 压缩算法标识常量
const (
	AlgoCompressionZstd byte = 0x01
	AlgoCompressionGzip byte = 0x02
)

// 加密算法标识常量
const (
	AlgoEncryptionAESGCM byte = 0x01
)
