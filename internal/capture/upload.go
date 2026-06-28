package capture

import (
	"context"
	"os"
	"time"

	"github.com/minio/minio-go/v7"
	"zhongkui.com/zhongkui/internal/pkg/crypto"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

func (c *capturer) uploadOne(ctx context.Context, filePath, objectKey string) error {
	defer func() {
		err := os.Remove(filePath)
		if err != nil {
			c.logger.Ctx(ctx).Errorf(`remove file %s error: %v`, filePath, err)
		} else {
			c.logger.Ctx(ctx).Infof(`remove file %s success`, filePath)
		}
	}()

	// 调用S3客户端上传文件
	// 设置文件内容类型、自动校验和和过期时间
	info, err := c.s3.FPutObject(
		ctx,
		objectKey,
		filePath,
		minio.PutObjectOptions{
			ContentType:  crypto.COMPRESS_CONTENT_TYPE,
			AutoChecksum: minio.ChecksumSHA256,
			Expires:      time.Now().Add(c.uploadCfg.ExpireAge),
		},
	)
	if err != nil {
		return errors.WithStack(err)
	}

	// 记录上传成功的详细日志
	// 包含对象键名、文件大小和SHA256校验和
	c.logger.Ctx(ctx).Infof(`upload file %s to s3, size: %d, checksum: %s`, objectKey, info.Size, info.ChecksumSHA256)
	return nil
}

func (c *capturer) upload(ctx context.Context, sourceChan <-chan notice, notifyChan chan<- notice) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case n := <-sourceChan:
			filePath := c.filePath(n.ts)
			objectKey := c.objectKey(n.ts)
			err := c.uploadOne(ctx, filePath, objectKey)
			if err != nil {
				return errors.WithStack(err)
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
