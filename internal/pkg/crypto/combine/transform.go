package combine

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
	// WrapWriter 用该变换包装 io.Writer，返回写入即变换的 WriteCloser。
	WrapWriter(w io.Writer) (io.WriteCloser, error)
	// WrapReader 用该变换包装 io.Reader，返回读取即反向变换的 ReadCloser。
	WrapReader(r io.Reader) (io.ReadCloser, error)
}

// ModeIDer 标识算法的类型，用于自描述流头部写入。
type ModeIDer interface {
	// ModeID 返回该变换算法的模式标识字节，用于写入自描述流头部。
	ModeID() byte
}
