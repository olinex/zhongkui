package validator

import (
	"testing"

	"github.com/go-playground/validator/v10"
)

// cronTestIn 是 binding 标签测试结构体：cron 字段使用自定义 cron 标签。
type cronTestIn struct {
	// CronSpec 待校验的 cron 表达式
	CronSpec string `validate:"cron"`
}

// TestIsValidCronSpec 覆盖纯函数对合法与非法 cron 表达式的判定。
func TestIsValidCronSpec(t *testing.T) {
	cases := []struct {
		name string
		spec string
		want bool
	}{
		{"standard five fields", "0 9 * * *", true},
		{"every minute", "* * * * *", true},
		{"descriptor daily", "@daily", true},
		{"surrounding spaces", "  0 9 * * *  ", true},
		{"empty", "", false},
		{"blank", "   ", false},
		{"too few fields", "0 9 *", false},
		{"invalid token", "0 9 * * x", false},
		{"six fields with seconds", "0 0 9 * * *", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsValidCronSpec(tc.spec); got != tc.want {
				t.Fatalf("IsValidCronSpec(%q) = %v, want %v", tc.spec, got, tc.want)
			}
		})
	}
}

// TestValidateCronSpec 验证 `cron` 标签接入 go-playground/validator 后对字段的判定。
func TestValidateCronSpec(t *testing.T) {
	v := validator.New()
	if err := v.RegisterValidation(TagCron, ValidateCronSpec); err != nil {
		t.Fatalf("register cron validation: %v", err)
	}
	for _, spec := range []string{"0 9 * * *", "@daily"} {
		if err := v.Struct(cronTestIn{CronSpec: spec}); err != nil {
			t.Fatalf("Struct(%q) error = %v, want nil", spec, err)
		}
	}
	for _, spec := range []string{"", "not-a-cron", "0 0 9 * * *"} {
		if err := v.Struct(cronTestIn{CronSpec: spec}); err == nil {
			t.Fatalf("Struct(%q) error = nil, want a validation error", spec)
		}
	}
}
