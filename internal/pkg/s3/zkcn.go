package s3

import (
	"context"
	"io"
	"time"

	"github.com/minio/minio-go/v7"
	"zhongkui.com/zhongkui/internal/pkg/crypto"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

type Zkcn struct {
	client    *S3Client
	objectKey string
	cryptoKey []byte
	timeout   time.Duration
}

func NewZkcn(client *S3Client, objectKey string, cryptoKey []byte, timeout time.Duration) *Zkcn {
	return &Zkcn{
		client:    client,
		objectKey: objectKey,
		cryptoKey: cryptoKey,
		timeout:   timeout,
	}
}

func (t *Zkcn) Store(ctx context.Context, reader io.Reader) (minio.UploadInfo, error) {
	opts := minio.PutObjectOptions{
		ContentType:  crypto.COMPRESS_CONTENT_TYPE,
		AutoChecksum: minio.ChecksumSHA256,
	}
	if t.timeout > 0 {
		opts.Expires = time.Now().Add(t.timeout)
	}
	return t.client.CPutObject(
		ctx,
		t.objectKey,
		t.cryptoKey,
		reader,
		-1,
		opts,
	)
}

func (t *Zkcn) Load(ctx context.Context) (io.ReadCloser, error) {
	return t.client.CGetOject(ctx, t.objectKey, t.cryptoKey, minio.GetObjectOptions{})
}

func (t *Zkcn) Clear(ctx context.Context) error {
	err := t.client.RemoveObject(ctx, t.objectKey, minio.RemoveObjectOptions{})
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}
