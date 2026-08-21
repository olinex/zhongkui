package manager

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"

	"zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/pkg/crypto/combine"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/s3"
	"zhongkui.com/zhongkui/internal/tagengine"
)

// S3ClientCtxName 是 S3 客户端在 Gin 上下文中存储的键名。
// 通过此键将 *s3.S3Client 实例注入到请求上下文中，供后续处理函数读取。
const S3ClientCtxName = "s3"

// S3ClientMiddleware 创建一个 Gin 中间件，将 S3 客户端实例注入到请求上下文中。
// 该中间件在请求处理链的入口处调用 c.Set 将 client 实例存储到上下文中，
// 使得后续处理函数可以通过 GetS3Client 函数获取到同一 S3 客户端。
//
// 参数:
//   - client: S3 客户端实例，通常在应用启动时创建
//
// 返回:
//   - gin.HandlerFunc: Gin 中间件处理函数
func S3ClientMiddleware(client *s3.S3Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(S3ClientCtxName, client)
		c.Next()
	}
}

// GetS3Client 从 Gin 请求上下文中获取 S3 客户端实例。
//
// 注意: 如果上下文中不存在 S3 客户端实例，该函数会触发 panic，
// 这通常意味着未正确注册 S3ClientMiddleware 中间件。
//
// 参数:
//   - c: Gin 请求上下文
//
// 返回:
//   - *s3.S3Client: S3 客户端实例
func GetS3Client(c *gin.Context) *s3.S3Client {
	if client, ok := c.Get(S3ClientCtxName); ok {
		return client.(*s3.S3Client)
	}
	panic("s3 client does not exists")
}

// 样例 S3 body 副本管理：
// 创建标签规则样例时将目标模型引用到的 S3 body 对象服务端复制到样例专属前缀
// （sample/{model}/{uid}/{field}.zkcn，uid 为每次创建生成的随机 UUID），
// 该前缀不在桶生命周期规则内，副本永久保留；删除样例时同步删除其引用的全部 S3 对象。

// SampleS3Fields 各目标模型的 S3 body 字段清单（键为 tagengine 类型键，与样例接口 model 入参一致）。
// 字段清单与前端 TrafficS3Fields（web/src/apis/traffic.ts）及明文接口的 field 参数保持一致；
// 新增含 S3 body 字段的模型时在此补充注册。
var SampleS3Fields = map[string][]string{
	tagengine.MustTypeKey(&network.HttpTransport{}): {"reqBody", "rspBody"},
}

// sampleObjectKey 构造样例 S3 body 副本对象键：sample/{model}/{uid}/{field}.zkcn。
// uid 为每次创建样例生成的随机 UUID，保证每个样例独占其副本，删除互不影响。
func sampleObjectKey(model, uid, field string) string {
	return fmt.Sprintf("%s%s/%s/%s.%s", s3.SamplePrefix, model, uid, field, combine.FileExt)
}

// SampleBodyObjectKeys 提取 input 中目标模型各 S3 body 字段的对象键（field → key）。
// 字段缺失或值为空字符串时跳过（空 body 无对象，不参与复制/删除）；
// 字段值非字符串时返回错误。
func SampleBodyObjectKeys(model string, input map[string]any) (map[string]string, error) {
	keys := make(map[string]string, len(SampleS3Fields[model]))
	for _, field := range SampleS3Fields[model] {
		value, ok := input[field]
		if !ok {
			continue
		}
		key, ok := value.(string)
		if !ok {
			return nil, errors.Newf("sample: field %q of model %q is not a string", field, model)
		}
		if key == "" {
			continue
		}
		keys[field] = key
	}
	return keys, nil
}

// CopySampleS3Objects 将 input 中目标模型引用到的 S3 body 对象复制为样例专属副本，
// 并将 input 中的对应字段原地改写为副本对象键。
// 源对象不存在（如已被生命周期过期删除）时返回错误，由调用方拒绝创建。
// 返回的 cleanup 用于复制中途失败或后续落库失败时回滚已生成的副本对象。
func CopySampleS3Objects(ctx context.Context, client *s3.S3Client, model string, input map[string]any, uid string) (cleanup func(), err error) {
	keys, err := SampleBodyObjectKeys(model, input)
	if err != nil {
		return nil, err
	}
	copied := make([]string, 0, len(keys))
	for field, src := range keys {
		dst := sampleObjectKey(model, uid, field)
		if err := client.CopyObject(ctx, src, dst); err != nil {
			return func() { _ = removeSampleObjects(ctx, client, copied) }, errors.WithStack(err)
		}
		copied = append(copied, dst)
		input[field] = dst
	}
	return func() { _ = removeSampleObjects(ctx, client, copied) }, nil
}

// DeleteSampleS3Objects 删除样例引用的全部 S3 body 对象（不区分前缀）。
// 封闭首版开发阶段不考虑历史样例遗留引用；样例副本均为样例独占，删除安全。
func DeleteSampleS3Objects(ctx context.Context, client *s3.S3Client, model string, input map[string]any) error {
	keys, err := SampleBodyObjectKeys(model, input)
	if err != nil {
		return err
	}
	objects := make([]string, 0, len(keys))
	for _, key := range keys {
		objects = append(objects, key)
	}
	return removeSampleObjects(ctx, client, objects)
}

// removeSampleObjects 批量删除 S3 对象；任一删除失败即返回错误。
// minio RemoveObject 对不存在的对象幂等返回成功，重复执行安全。
func removeSampleObjects(ctx context.Context, client *s3.S3Client, keys []string) error {
	for _, key := range keys {
		if err := client.RemoveObject(ctx, key, minio.RemoveObjectOptions{}); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}
