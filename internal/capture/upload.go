package capture

import (
	"context"
	"encoding/hex"
	"os"
	"time"

	"github.com/minio/minio-go/v7"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/utils"
)

func (c *capturer) uploadOne(ctx context.Context, zstdPath, objectKey string) error {
	defer func() {
		err := os.Remove(zstdPath)
		if err != nil {
			c.logger.Errorf(`remove zstd file %s error: %v`, zstdPath, err)
		} else {
			c.logger.Infof(`remove zstd file %s success`, zstdPath)
		}
	}()
	// 计算本地文件的SHA256校验和
	// 用于验证文件完整性和数据一致性
	sha256, err := utils.CalculateFileSHA256(zstdPath)
	if err != nil {
		return errors.WithStack(err)
	}

	// 调用S3客户端上传文件
	// 设置文件内容类型、自动校验和和过期时间
	info, err := c.s3.FPutObject(
		ctx,
		objectKey,
		zstdPath,
		minio.PutObjectOptions{
			ContentType:  compressContentType,
			AutoChecksum: minio.ChecksumSHA256,
			Expires:      time.Now().Add(c.uploadCfg.ExpireAge),
		},
	)
	if err != nil {
		return errors.WithStack(err)
	}

	// 记录上传成功的详细日志
	// 包含对象键名、文件大小和SHA256校验和
	c.logger.Infof(`upload file %s to s3, size: %d, checksum: %s`, objectKey, info.Size, hex.EncodeToString(sha256))
	return nil
}

// upload 将本地文件上传到S3对象存储
// 该方法执行完整的文件上传流程，包括：
// 1. 构建唯一的对象存储键名
// 2. 计算文件的SHA256校验和
// 3. 调用S3客户端上传文件
// 4. 记录上传成功或失败的日志
//
// 参数:
//   - localPath: 本地文件路径，需要上传的文件在本地文件系统中的完整路径
//
// 返回值:
//   - error: 如果上传过程中发生错误，返回详细的错误信息；否则返回nil
//
// 上传流程:
//  1. 使用BuildObjectKey函数构建唯一的对象键名，格式为：namespace/name/version/timestamp_sessionID.zstd
//  2. 计算本地文件的SHA256校验和，用于数据完整性验证
//  3. 调用S3客户端的FPutObject方法上传文件，设置以下选项：
//     - ContentType: 设置为compressContentType（application/zstd）
//     - AutoChecksum: 启用SHA256校验和自动计算
//     - Expires: 设置对象的过期时间，基于配置中的ExpireAge参数
//  4. 如果上传成功，记录包含对象键名、文件大小和校验和的详细日志
//  5. 如果任何步骤失败，返回包装了原始错误的错误信息
//
// 错误处理:
//   - 文件SHA256计算失败: 返回包含原始错误的包装错误
//   - S3上传失败: 返回包含原始错误的包装错误
//   - 其他内部错误: 返回相应的错误信息
//
// 示例:
//
//	u := &uploader{
//	    ctx:       context.Background(),
//	    logger:    logging.NewLogger(),
//	    cfg:       &Config{...},
//	    s3:        s3Client,
//	    sessionID: sessionID,
//	    timestamp: time.Now(),
//	}
//	err := u.upload("/tmp/data.zstd")
//	if err != nil {
//	    // 处理上传错误
//	}
func (c *capturer) upload(ctx context.Context, sourceChan <-chan time.Time, notifyChan chan<- time.Time) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case ts := <-sourceChan:
			zstdPath := c.zstdPath(ts)
			objectKey := c.objectKey(ts)
			err := c.uploadOne(ctx, zstdPath, objectKey)
			if err != nil {
				return errors.WithStack(err)
			}
			select {
			case <-ctx.Done():
				return nil
			case notifyChan <- ts:
			}
			c.logger.Infof(`upload zstd file %s to %s success`, zstdPath, objectKey)
		}
	}
}
