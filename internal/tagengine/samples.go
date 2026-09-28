// 本文件实现目标模型系统样例的懒供给与列表查询，
// 供前端 dry-run 样例选择器使用：模型无样例时自动落库零值文档
// default 样例，HttpTransport 额外预置一条 http-example 真实样例。
package tagengine

import (
	// 标准库
	"context"
	"encoding/json"

	// 第三方
	"gorm.io/datatypes"

	// 项目内部
	"zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/models/tag"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

const (
	// defaultSampleName 懒供给默认样例的名称：目标模型零值实例的输入文档。
	defaultSampleName = "default"

	// httpExampleSampleName HttpTransport 预置真实样例的名称。
	httpExampleSampleName = "http-example"
)

// httpTransportExample HttpTransport 预置真实样例的输入文档。
// 模拟一条真实 HTTP 登录请求，键为 HttpTransport 的 json tag 名，
// 可反序列化回 *network.HttpTransport（加密字段 reqHdrAuthorization 为 {}）。
var httpTransportExample = map[string]any{
	"namespace":           "default",
	"capture":             "eth0",
	"version":             "1",
	"sessionId":           "c81d4e2e-bcf2-11e6-869b-7df92533d2db",
	"traceId":             "f47ac10b-58cc-4372-a567-0e02b2c3d479",
	"timestamp":           "2026-08-16T10:00:00+08:00",
	"clientIp":            "192.168.1.10",
	"clientPort":          53452,
	"serverIp":            "93.184.216.34",
	"serverPort":          443,
	"httpVersion":         "HTTP/1.1",
	"method":              "POST",
	"url":                 "https://api.example.com/v1/login",
	"scheme":              "https",
	"host":                "api.example.com",
	"path":                "/v1/login",
	"fileExt":             "json",
	"reqBodyHash":         "4a2e3f9c1b8d7e6f5a4b3c2d1e0f9a8b7c6d5e4f3a2b1c0d9e8f7a6b5c4d3e2f",
	"reqBodyLength":       86,
	"reqHdrUserAgent":     "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
	"reqHdrContentType":   "application/json",
	"reqHdrAccept":        "application/json",
	"reqHdrAuthorization": map[string]any{},
	"reqHdrHost":          "api.example.com",
	"rspHdrContentType":   "application/json",
	"rspHdrServer":        "nginx",
	"rspHdrContentLength": 2048,
	"tags":                []string{"http", "login"},
}

// ListModelSamples 列出目标模型在指定命名空间下的全部系统样例（按 name 排序）。
// 未注册模型返回 ErrModelNotRegistered。
func ListModelSamples(ctx context.Context, db *database.DB, namespace, typeKey string) ([]*tag.TagRuleSample, error) {
	if !IsRegistered(typeKey) {
		return nil, errors.WithStack(ErrModelNotRegistered)
	}
	return listModelSamples(ctx, db, namespace, typeKey)
}

// EnsureModelSamples 确保目标模型在指定命名空间下已有系统样例，无样例时懒供给并落库，
// 返回最新样例列表。
// 懒供给内容：模型零值实例经 BuildInput 得到的输入文档 JSON，作为 name="default" 的样例；
// 模型有预置样例（如 HttpTransport 的 http-example）时一并供给。
// 每个命名空间各自懒供给，样例归属命名空间，租户间相互隔离。
// 并发幂等：插入遇唯一约束冲突（uq_tag_rule_samples_namespace_name_model_name）时
// 忽略错误后重查，保证不重复。
// 未注册模型返回 ErrModelNotRegistered。
func EnsureModelSamples(ctx context.Context, db *database.DB, namespace, typeKey string) ([]*tag.TagRuleSample, error) {
	target, ok := TargetInfo(typeKey)
	if !ok {
		return nil, errors.WithStack(ErrModelNotRegistered)
	}
	// 收集缺失的系统样例：default 恒需供给；HttpTransport 额外供给 http-example
	pending := make([]*tag.TagRuleSample, 0, 2)
	if exists, err := sampleExists(ctx, db, namespace, typeKey, defaultSampleName); err != nil {
		return nil, errors.WithStack(err)
	} else if !exists {
		input, err := BuildInput(target.Type)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		raw, err := json.Marshal(input)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		pending = append(pending, &tag.TagRuleSample{
			NamespaceName: namespace,
			Model:         typeKey,
			Name:          defaultSampleName,
			Input:         datatypes.JSON(raw),
			Description:   "目标模型零值输入文档",
		})
	}
	if typeKey == MustTypeKey(&network.HttpTransport{}) {
		if exists, err := sampleExists(ctx, db, namespace, typeKey, httpExampleSampleName); err != nil {
			return nil, err
		} else if !exists {
			raw, err := json.Marshal(httpTransportExample)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			pending = append(pending, &tag.TagRuleSample{
				NamespaceName: namespace,
				Model:         typeKey,
				Name:          httpExampleSampleName,
				Input:         datatypes.JSON(raw),
				Description:   "HTTP 登录请求真实样例（模拟典型流量字段）",
			})
		}
	}
	for _, sample := range pending {
		if err := db.WithContext(ctx).Create(sample).Error; err != nil {
			// 并发幂等：唯一约束冲突说明其他调用方已供给同名样例，
			// 忽略该错误后以重查结果为准；若重查仍无此样例则视为真实错误返回
			if exists, existsErr := sampleExists(ctx, db, namespace, typeKey, sample.Name); existsErr != nil {
				return nil, existsErr
			} else if !exists {
				return nil, errors.WithStack(err)
			}
		}
	}
	return listModelSamples(ctx, db, namespace, typeKey)
}

// listModelSamples 按 name 排序查询目标模型在指定命名空间下的全部系统样例。
func listModelSamples(ctx context.Context, db *database.DB, namespace, typeKey string) ([]*tag.TagRuleSample, error) {
	var samples []*tag.TagRuleSample
	if err := db.WithContext(ctx).Model(&tag.TagRuleSample{}).
		Where("namespace_name = ? AND model = ?", namespace, typeKey).
		Order("name").
		Find(&samples).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return samples, nil
}

// sampleExists 判断目标模型在指定命名空间下指定名称的样例是否已存在。
func sampleExists(ctx context.Context, db *database.DB, namespace, typeKey, name string) (bool, error) {
	var count int64
	if err := db.WithContext(ctx).Model(&tag.TagRuleSample{}).
		Where("namespace_name = ? AND model = ? AND name = ?", namespace, typeKey, name).
		Count(&count).Error; err != nil {
		return false, errors.WithStack(err)
	}
	return count > 0, nil
}
