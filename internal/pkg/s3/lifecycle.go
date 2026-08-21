package s3

import (
	"context"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/lifecycle"

	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// S3 对象路径前缀常量，是各上传路径与生命周期规则的单一事实来源。
// 新增对象类型时必须在此补充前缀，并让对应上传路径引用常量而非手写字面量。
const (
	// BodyRequestPrefix 请求体对象前缀（internal/models/network 上传）
	BodyRequestPrefix = "body/request/"
	// BodyResponsePrefix 响应体对象前缀（internal/models/network 上传）
	BodyResponsePrefix = "body/response/"
	// PcapPrefix 抓包文件对象前缀（internal/capture 上传）
	PcapPrefix = "pcap/"
	// SamplePrefix 标签规则样例 body 副本对象前缀（样例创建时复制生成，样例独占）。
	// 该前缀不在 LifecyclePrefixes 内，不受桶生命周期规则约束，副本永久保留；
	// 新增对象类型时必须在此补充前缀（本文件是前缀的唯一事实来源）。
	SamplePrefix = "sample/"
)

// LifecyclePrefixes 所有可纳入生命周期管理的 S3 对象路径前缀。
// CLI 交互式配置时以此为枚举选项，避免手工输入。
var LifecyclePrefixes = []string{BodyRequestPrefix, BodyResponsePrefix, PcapPrefix}

// LifecycleRuleConfig 描述一条 S3 桶生命周期规则。
// 与 minio-go lifecycle.Rule 语义对齐，首版仅暴露过期删除（Expiration.Days）；
// 过渡存储、非当前版本过期等策略后续按需扩展。
type LifecycleRuleConfig struct {
	// ID 规则唯一标识，建议形如 "pcap-expire"；与平台既有规则 ID 冲突时应用失败
	ID string `yaml:"id" json:"id"`
	// Status 规则状态：Enabled 或 Disabled（缺省视为 Enabled）
	Status string `yaml:"status" json:"status"`
	// Prefix 匹配的对象键前缀，如 "pcap/"、"body/request/"
	Prefix string `yaml:"prefix" json:"prefix"`
	// ExpireDays 对象创建后的过期天数，>0 表示到期自动删除；0 表示不设置过期
	ExpireDays int `yaml:"expireDays" json:"expireDays"`
}

// LifecycleConfig 是 S3 桶生命周期规则清单
type LifecycleConfig struct {
	Rules []LifecycleRuleConfig `yaml:"rules" json:"rules"`
}

// ToMinioConfig 将规则清单转换为 minio-go 生命周期配置。
// 校验规则合法性（ID 必填、Status 取值受限），任一规则非法整体拒绝，避免部分应用。
func (c *LifecycleConfig) ToMinioConfig() (*lifecycle.Configuration, error) {
	config := lifecycle.NewConfiguration()
	for _, rule := range c.Rules {
		if rule.ID == "" {
			return nil, errors.New("lifecycle rule id is required")
		}
		status := strings.TrimSpace(rule.Status)
		if status == "" {
			status = "Enabled"
		}
		if status != "Enabled" && status != "Disabled" {
			return nil, errors.Newf("lifecycle rule %q: unsupported status %q", rule.ID, rule.Status)
		}
		// 使用 Filter.Prefix 而非顶层 Prefix：minio-go 的 Rule.RuleFilter 是值类型，
		// 零值也会被序列化为 <Filter>，与顶层 <Prefix> 同时出现会触发
		// S3 "Legacy Prefix and Filter cannot both be present" 校验错误。
		entry := lifecycle.Rule{
			ID:         rule.ID,
			Status:     status,
			RuleFilter: lifecycle.Filter{Prefix: rule.Prefix},
		}
		if rule.ExpireDays > 0 {
			entry.Expiration.Days = lifecycle.ExpirationDays(rule.ExpireDays)
		}
		config.Rules = append(config.Rules, entry)
	}
	return config, nil
}

// Bucket 返回默认存储桶名称
func (s *S3Client) Bucket() string {
	return s.bucket
}

// SetLifecycle 将规则清单应用到默认存储桶，整体替换桶的既有生命周期规则。
func (s *S3Client) SetLifecycle(ctx context.Context, cfg *LifecycleConfig) error {
	minioCfg, err := cfg.ToMinioConfig()
	if err != nil {
		return errors.WithStack(err)
	}
	if err := s.cli.SetBucketLifecycle(ctx, s.bucket, minioCfg); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// GetLifecycle 获取默认存储桶当前的生命周期规则；未配置时返回 (nil, nil)。
func (s *S3Client) GetLifecycle(ctx context.Context) (*lifecycle.Configuration, error) {
	cfg, err := s.cli.GetBucketLifecycle(ctx, s.bucket)
	if err != nil {
		if IsNoSuchLifecycle(err) {
			return nil, nil
		}
		return nil, errors.WithStack(err)
	}
	return cfg, nil
}

// RemoveLifecycle 移除默认存储桶的全部生命周期规则。
// 依赖 minio-go 语义：SetBucketLifecycle 收到空配置时执行删除操作。
func (s *S3Client) RemoveLifecycle(ctx context.Context) error {
	if err := s.cli.SetBucketLifecycle(ctx, s.bucket, lifecycle.NewConfiguration()); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// IsNoSuchLifecycle 判断错误是否为"存储桶未配置生命周期规则"（NoSuchLifecycleConfiguration）。
func IsNoSuchLifecycle(err error) bool {
	if err == nil {
		return false
	}
	return minio.ToErrorResponse(err).Code == "NoSuchLifecycleConfiguration"
}
