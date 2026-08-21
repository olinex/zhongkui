package capture

import (
	"context"
	"os"

	"github.com/minio/minio-go/v7"
	"zhongkui.com/zhongkui/internal/capture/totel"
	"zhongkui.com/zhongkui/internal/pkg/crypto/combine"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

func (c *capturer) uploadOne(ctx context.Context, objectKey, filePath string, n notice) (err error) {
	ctx, span := totel.StartUploadOneSpan(ctx, objectKey)
	var uploadedSize int64
	defer func() {
		if err != nil {
			totel.RecordUploadError(ctx)
		} else {
			totel.RecordSegmentUploaded(ctx, uploadedSize)
		}
		totel.EndSpan(span, &err)
	}()

	defer func() {
		e := os.Remove(filePath)
		if e != nil {
			c.logger.Ctx(ctx).Errorf(`remove file %s error: %v`, filePath, e)
		} else {
			c.logger.Ctx(ctx).Infof(`remove file %s success`, filePath)
			if c.untrackFile != nil {
				c.untrackFile(filePath)
			}
		}
	}()

	// 调用 S3 客户端上传文件
	// SendContentMd5 使 SDK 自动计算文件 MD5 并通过 Content-MD5 头发送，
	// 服务端校验通过后将 MD5 作为 ETag 返回。此机制在所有 S3 兼容存储上通用。
	// 下载侧可通过 StatObject 获取 ETag 并计算下载数据 MD5 进行一致性比对。
	// 注意：对象过期策略统一由 `zki s3 lifecycle` 工具按桶生命周期规则管理，
	// 代码层不再设置对象级 Expires。
	var info minio.UploadInfo
	info, err = c.s3.FPutObject(
		ctx,
		objectKey,
		filePath,
		minio.PutObjectOptions{
			ContentType:    combine.CompressContentType,
			SendContentMd5: true,
		},
	)
	if err != nil {
		return errors.WithStack(err)
	}
	uploadedSize = info.Size

	// 记录上传成功日志
	// ETag 在单次上传中等于文件内容的 MD5 哈希，下载侧可据此校验完整性
	c.logger.Ctx(ctx).Infof(
		`upload file %s to s3, size: %d, packets: %d, bytes: %d, etag: %s`,
		objectKey, info.Size, n.packetCount, n.byteCount, info.ETag,
	)
	return nil
}

func (c *capturer) upload(ctx context.Context, sourceChan <-chan notice, notifyChan chan<- notice) (err error) {
	ctx, span := totel.StartUploadFileSpan(ctx)
	defer totel.EndSpan(span, &err)
	for {
		select {
		case <-ctx.Done():
			return nil
		case n := <-sourceChan:
			filePath := c.filePath(n.ts)
			objectKey := c.objectKey(n.ts)
			e := c.uploadOne(ctx, objectKey, filePath, n)
			if e != nil {
				err = e
				return errors.WithStack(e)
			}
			select {
			case <-ctx.Done():
				return nil
			case notifyChan <- n:
			}
			c.logger.Ctx(ctx).Infof(`upload file %s to %s success`, filePath, objectKey)
		}
	}
}
