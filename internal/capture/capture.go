package capture

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/gopacket/gopacket/layers"
	"golang.org/x/sync/errgroup"
	"zhongkui.com/zhongkui/internal/capture/totel"
	"zhongkui.com/zhongkui/internal/capture/wire"
	"zhongkui.com/zhongkui/internal/pkg/crypto/combine"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	reporter_v1 "zhongkui.com/zhongkui/internal/pkg/protobuf/reporter/v1"
	"zhongkui.com/zhongkui/internal/pkg/s3"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

// capturer 负责抓取网络数据包并将其导出、压缩、上传到S3存储
// 该结构体封装了完整的抓包流程，包括：
// 1. 数据包捕获和导出到pcap文件
// 2. 文件压缩（使用zstd算法）
// 3. 上传到S3兼容的对象存储
// 4. 清理临时文件
// 每个capturer实例对应一次完整的抓包会话，包含唯一的会话ID和时间戳
type capturer struct {
	resource *reporter_v1.ResourceRaw

	uploadCfg *UploadConfig

	captureCfg *reporter_v1.Config

	// logger 日志记录器，用于记录抓包过程中的关键事件和错误信息
	// 支持不同级别的日志记录（DEBUG、INFO、WARN、ERROR）
	logger *logging.Logger

	// s3 S3客户端，用于与S3兼容的对象存储服务进行交互
	// 负责文件上传、元数据设置和过期策略管理
	s3 *s3.S3Client

	// sessionID 会话标识符，唯一标识本次抓包会话
	// 使用HexUUID格式，用于生成文件名和S3对象键名
	sessionID serializer.HexUUID

	// linkType 链路类型，定义网络接口的数据链路层协议类型
	// 如Ethernet、PPP、Raw等，影响pcap文件头的写入
	linkType layers.LinkType

	// trackFile 文件追踪回调，在 dump 阶段创建临时文件后调用，将文件路径注册到 CaptureClient 的待清理集合
	trackFile func(string)

	// untrackFile 文件追踪回调，在 upload 阶段成功删除临时文件后调用，将文件路径从待清理集合中移除
	untrackFile func(string)
}

// filePath 生成压缩文件的完整路径
// 该方法根据配置中的临时目录、会话ID和时间戳生成唯一的压缩文件名
// 返回值：压缩文件的完整绝对路径
func (c *capturer) filePath(ts time.Time) string {
	return filepath.Join(
		c.uploadCfg.DumpDir,
		fmt.Sprintf(
			"%d.%s.%s",
			ts.UnixMilli(),
			c.sessionID.String(),
			combine.FILE_EXT,
		),
	)
}

// 构建唯一的对象存储键名
func (c *capturer) objectKey(ts time.Time) string {
	return BuildObjectKey(
		c.resource.Namespace,
		c.resource.Name,
		c.resource.Version,
		ts,
		c.sessionID,
	)
}

// capture 执行完整的抓包流程：导出数据包到pcap文件、压缩、上传到S3存储
// 该方法实现了数据包处理的三阶段流水线：
// 1. 导出阶段：将原始数据包写入pcap格式的临时文件
// 2. 压缩阶段：使用zstd算法压缩pcap文件，减少存储和传输开销
// 3. 上传阶段：将压缩后的文件上传到S3对象存储，并设置过期时间
//
// 参数：
//
//	packets <- 数据包通道，提供需要处理的数据包流
//	  通道关闭表示数据包流结束，触发后续处理流程
//
// 返回值：
//
//	error <- 如果任何阶段失败，返回包含堆栈信息的错误
//	  成功时返回nil，并通过uploadChan通知上传完成
//
// 错误处理：
//   - 每个阶段都有独立的错误处理，确保错误信息包含堆栈跟踪
//   - 使用defer确保临时文件在函数退出时被清理
//   - 如果中间阶段失败，后续阶段不会执行，但已创建的临时文件会被清理
//
// 并发安全：
//   - 该方法不是线程安全的，每个capturer实例应独立使用
//   - 数据包通道的读取是阻塞的，直到通道关闭或上下文取消
func (c *capturer) capture(ctx context.Context, ws wire.Wire, uploadChan chan<- notice) error {
	ctx, span := totel.StartCaptureSpan(ctx)
	var err error
	defer totel.EndSpan(span, &err)

	wg, ctx := errgroup.WithContext(ctx)
	dumpChan := make(chan notice, 10)
	wg.Go(func() error {
		defer close(dumpChan)
		// 第一阶段：数据包捕获/压缩/加密到指定文件
		return c.dump(ctx, ws, dumpChan)
	})
	wg.Go(func() error {
		// 第三阶段：文件上传到S3
		return c.upload(ctx, dumpChan, uploadChan)
	})
	if err = wg.Wait(); err != nil {
		return errors.WithStack(err)
	}

	// 返回nil表示所有阶段都成功完成
	return nil
}
