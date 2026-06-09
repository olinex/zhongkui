package capture

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/klauspost/compress/zstd"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

const (
	// compressContentType 压缩文件的内容类型，表示使用zstd压缩算法
	// 该常量用于标识压缩文件的MIME类型，在文件上传或传输时使用
	compressContentType = "application/zstd"
)

func (c *capturer) compressOne(_ context.Context, pcapPath, zstdPath string) error {
	defer func() {
		err := os.Remove(pcapPath)
		if err != nil {
			c.logger.Errorf(`remove pcap file %s error: %v`, pcapPath, err)
		} else {
			c.logger.Infof(`remove pcap file %s success`, pcapPath)
		}
	}()

	// 打开源pcap文件
	inFile, err := os.Open(pcapPath)
	if err != nil {
		return errors.WithStack(err)
	}
	defer inFile.Close()

	// 创建目标压缩文件
	outFile, err := os.Create(zstdPath)
	if err != nil {
		return errors.WithStack(err)
	}
	defer outFile.Close()

	// 记录压缩文件创建日志
	c.logger.Infof(`compress file %s is created`, zstdPath)

	// 初始化zstd编码器，使用配置中的压缩级别
	encoder, err := zstd.NewWriter(outFile, zstd.WithEncoderLevel(c.uploadCfg.EncoderLevel))
	if err != nil {
		return errors.WithStack(err)
	}
	defer encoder.Close()

	// 将源文件内容复制到编码器，实现流式压缩
	if _, err := io.Copy(encoder, inFile); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

// compress 将指定的pcap文件压缩为zstd格式
// 该方法执行以下步骤：
// 1. 打开源pcap文件进行读取
// 2. 创建目标压缩文件
// 3. 初始化zstd编码器，使用配置中的压缩级别（1-4）
// 4. 将源文件内容复制到编码器，实现流式压缩
// 5. 记录压缩完成日志
// 参数：
//   - pcapPath: 源pcap文件的路径
//
// 返回值：
//   - error: 如果压缩过程中出现错误则返回错误，否则返回nil
//
// 错误处理：
//   - 文件打开失败：返回文件系统错误
//   - 编码器创建失败：返回zstd库错误（如压缩级别无效）
//   - 数据复制失败：返回IO错误
func (c *capturer) compress(ctx context.Context, sourceChan <-chan time.Time, notifyChan chan<- time.Time) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case ts := <-sourceChan:
			pcapPath := c.pcapPath(ts)
			zstdPath := c.zstdPath(ts)
			err := c.compressOne(ctx, pcapPath, zstdPath)
			if err != nil {
				return errors.WithStack(err)
			}

			select {
			case <-ctx.Done():
				return nil
			case notifyChan <- ts:
			}
			c.logger.Infof(`compress zstd file %s success`, zstdPath)
		}
	}
}
