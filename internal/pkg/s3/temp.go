package s3

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/minio/minio-go/v7"
	"zhongkui.com/zhongkui/internal/pkg/crypto"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

type Temp struct {
	client    *S3Client
	secretKey string
	Name      string
	Version   string
	timeout   time.Duration
}

func NewTemp(client *S3Client, secretKey, name, version string) *Temp {
	return &Temp{
		client:    client,
		secretKey: secretKey,
		Name:      name,
		Version:   version,
	}
}

func (t *Temp) ObjectKey() string {
	return fmt.Sprintf(`temp/%s/%s.%s`, t.Name, t.Version, crypto.FILE_EXT)
}

func (t *Temp) Store(ctx context.Context, reader io.Reader) (*minio.UploadInfo, error) {
	if compressor, err := crypto.NewZstdCompressor(); err != nil {
		return nil, errors.WithStack(err)
	} else if encryptor, err := crypto.NewAESGCMEncryptor([]byte(t.secretKey)); err != nil {
		return nil, errors.WithStack(err)
	} else if pipeline, err := crypto.NewCompressEncryptPipeline(compressor, encryptor); err != nil {
		return nil, errors.WithStack(err)
	} else {
		buffer := bytes.NewBuffer(nil)
		if wc, err := pipeline.CompressEncrypt(buffer); err != nil {
			return nil, errors.WithStack(err)
		} else if _, err := io.Copy(wc, reader); err != nil {
			wc.Close()
			return nil, errors.WithStack(err)
		} else if err := wc.Close(); err != nil {
			return nil, errors.WithStack(err)
		} else {
			data := buffer.Bytes()
			info, err := t.client.PutObject(
				ctx,
				t.ObjectKey(),
				bytes.NewReader(data),
				int64(len(data)),
				minio.PutObjectOptions{
					ContentType:  crypto.COMPRESS_CONTENT_TYPE,
					AutoChecksum: minio.ChecksumSHA256,
					Expires:      time.Now().Add(t.timeout),
				},
			)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			return &info, nil
		}
	}
}

func (t *Temp) Load(ctx context.Context) (io.ReadCloser, error) {
	if compressor, err := crypto.NewZstdCompressor(); err != nil {
		return nil, errors.WithStack(err)
	} else if encryptor, err := crypto.NewAESGCMEncryptor([]byte(t.secretKey)); err != nil {
		return nil, errors.WithStack(err)
	} else if pipeline, err := crypto.NewCompressEncryptPipeline(compressor, encryptor); err != nil {
		return nil, errors.WithStack(err)
	} else if obj, err := t.client.GetObject(
		ctx,
		t.ObjectKey(),
		minio.GetObjectOptions{},
	); err != nil {
		return nil, errors.WithStack(err)
	} else if info, err := obj.Stat(); err != nil {
		if IsNotExists(err) {
			return nil, nil
		}
		return nil, errors.WithStack(err)
	} else if info.Size == 0 {
		return nil, nil
	} else {
		return pipeline.DecryptDecompress(obj)
	}
}

func (t *Temp) Clear(ctx context.Context) error {
	err := t.client.RemoveObject(ctx, t.ObjectKey(), minio.RemoveObjectOptions{})
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}
