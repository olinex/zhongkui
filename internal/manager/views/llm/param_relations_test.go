package llm

import (
	// 标准库
	"strings"
	"testing"

	// 第三方
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	// 项目内部
	aimodel "zhongkuisec.com/zhongkui/internal/models/ai"
)

// newParamRelationTestDB 构造跨键参数校验用的 sqlite 内存库并迁移 agent_params 表。
func newParamRelationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("gorm.Open(sqlite) error = %v", err)
	}
	if err := db.AutoMigrate(&aimodel.AgentParam{}); err != nil {
		t.Fatalf("AutoMigrate error = %v", err)
	}
	return db
}

// seedParamRelationRows 写入跨键校验涉及的四条参数：当前值与出厂值相同。
func seedParamRelationRows(t *testing.T, db *gorm.DB) {
	t.Helper()
	rows := []aimodel.AgentParam{
		{Key: "retrieval.weightVector", Value: "0.6", DefaultValue: "0.6"},
		{Key: "retrieval.weightKeyword", Value: "0.4", DefaultValue: "0.4"},
		{Key: "agent.maxToolResultBytes", Value: "32768", DefaultValue: "32768"},
		{Key: "retrieval.expandDocLimit", Value: "26000", DefaultValue: "26000"},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("seed agent_params error = %v", err)
	}
}

// TestValidateParamRelations 校验写入前的关系检查：非法组合被拒绝，恢复出厂值与合法更新通过。
func TestValidateParamRelations(t *testing.T) {
	cases := []struct {
		name    string
		updates []paramUpdate
		want    string
	}{
		{
			name:    "valid update",
			updates: []paramUpdate{{key: "retrieval.weightVector", value: "0.7"}},
		},
		{
			name: "reset falls back to default",
			updates: []paramUpdate{
				{key: "retrieval.weightKeyword", value: "0"},
				{key: "retrieval.weightVector", value: ""},
			},
		},
		{
			name: "both weights zero",
			updates: []paramUpdate{
				{key: "retrieval.weightVector", value: "0"},
				{key: "retrieval.weightKeyword", value: "0"},
			},
			want: "must not both be zero",
		},
		{
			name:    "negative weight",
			updates: []paramUpdate{{key: "retrieval.weightVector", value: "-0.5"}},
			want:    "must be non-negative",
		},
		{
			name:    "tool result budget below expansion budget",
			updates: []paramUpdate{{key: "agent.maxToolResultBytes", value: "1024"}},
			want:    "must be at least retrieval.expandDocLimit",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db := newParamRelationTestDB(t)
			seedParamRelationRows(t, db)
			err := validateParamRelations(db, c.updates)
			if c.want == "" {
				if err != nil {
					t.Fatalf("validateParamRelations() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("validateParamRelations() error = %v, want containing %q", err, c.want)
			}
		})
	}
}

// TestValidateParamRelations_MissingRow 校验重置一个尚未种子化的键时返回与启动期一致的错误。
func TestValidateParamRelations_MissingRow(t *testing.T) {
	db := newParamRelationTestDB(t)
	err := validateParamRelations(db, []paramUpdate{{key: "retrieval.weightVector", value: ""}})
	if err == nil || !strings.Contains(err.Error(), "has no seeded row") {
		t.Fatalf("validateParamRelations() error = %v, want a missing-row error", err)
	}
}
