package tag

import (
	"context"
	"fmt"

	"github.com/minio/minio-go/v7"

	"zhongkuisec.com/zhongkui/internal/manager/response"
	"zhongkuisec.com/zhongkui/internal/models/network"
	"zhongkuisec.com/zhongkui/internal/pkg/crypto/combine"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/s3"
	"zhongkuisec.com/zhongkui/internal/tagengine"
)

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
			return nil, response.CreateTagRuleSampleInputError("sampleFieldNotString", field, model)
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
		return nil, errors.WithStack(err)
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
		return errors.WithStack(err)
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
