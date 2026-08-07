package tagengine

import (
	"context"

	"zhongkui.com/zhongkui/internal/models/tag"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// RuleStore 规则数据访问层，从 core 数据库读取 OPA 规则。
type RuleStore struct {
	db *database.DB
}

// NewRuleStore 创建规则存储。
func NewRuleStore(db *database.DB) *RuleStore {
	return &RuleStore{db: db}
}

// ListByGroup 返回目标类型与标签类型下启用的规则。
func (s *RuleStore) ListByGroup(ctx context.Context, targetType, tagType string) ([]*tag.TagRule, error) {
	var rules []*tag.TagRule
	if err := s.db.WithContext(ctx).
		Where("target_type = ? AND tag_type = ? AND enabled = ?", targetType, tagType, true).
		Find(&rules).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return rules, nil
}
