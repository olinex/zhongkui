package mail

import (
	"testing"

	uv "github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
)

// testValidate 测试用验证器实例（无业务校验标签，仅用于满足 ExtractConfig 签名）。
var testValidate = uv.New()

// TestExtractConfig 测试从 Viper 提取 [smtp] 配置并映射到各字段。
func TestExtractConfig(t *testing.T) {
	v := viper.New()
	v.Set("smtp.enabled", true)
	v.Set("smtp.host", "smtp.example.com")
	v.Set("smtp.port", 587)
	v.Set("smtp.username", "user")
	v.Set("smtp.password", "secret")
	v.Set("smtp.from", "alert@example.com")

	cfg, err := ExtractConfig(v, testValidate)
	if err != nil {
		t.Fatalf("ExtractConfig() error = %v", err)
	}
	if !cfg.Enabled {
		t.Errorf("Enabled = false, want true")
	}
	if cfg.Host != "smtp.example.com" {
		t.Errorf("Host = %q, want %q", cfg.Host, "smtp.example.com")
	}
	if cfg.Port != 587 {
		t.Errorf("Port = %d, want 587", cfg.Port)
	}
	if cfg.Username != "user" {
		t.Errorf("Username = %q, want %q", cfg.Username, "user")
	}
	if cfg.Password != "secret" {
		t.Errorf("Password = %q, want %q", cfg.Password, "secret")
	}
	if cfg.From != "alert@example.com" {
		t.Errorf("From = %q, want %q", cfg.From, "alert@example.com")
	}
}

// TestExtractConfig_DefaultsBackfill 测试未显式配置的字段由 default 标签回填默认值
// （本包使用 config.ExtractConfig：仅当 [smtp] 段存在时才回填；完全缺省时因 nil sub 返回错误，
// 与 internal/pkg/logging 的既有行为一致）。
func TestExtractConfig_DefaultsBackfill(t *testing.T) {
	// 建立 [smtp] 段但不配置部分字段，验证默认值回填
	v := viper.New()
	v.Set("smtp.enabled", false)

	cfg, err := ExtractConfig(v, testValidate)
	if err != nil {
		t.Fatalf("ExtractConfig() error = %v", err)
	}
	if cfg.Enabled {
		t.Errorf("Enabled = true, want false (default)")
	}
	if cfg.Port != 465 {
		t.Errorf("Port = %d, want 465 (default)", cfg.Port)
	}
	if cfg.Host != "" {
		t.Errorf("Host = %q, want empty (default)", cfg.Host)
	}
	if cfg.From != "" {
		t.Errorf("From = %q, want empty (default)", cfg.From)
	}
}

// TestDescribeConfig 测试 DescribeConfig 输出结构化描述且前缀为 smtp。
func TestDescribeConfig(t *testing.T) {
	descs := DescribeConfig()
	if len(descs) == 0 {
		t.Fatalf("DescribeConfig() returned empty descriptions")
	}
	found := map[string]bool{}
	for _, d := range descs {
		found[d.Path] = true
	}
	for _, key := range []string{"smtp.enabled", "smtp.host", "smtp.port", "smtp.username", "smtp.password", "smtp.from"} {
		if !found[key] {
			t.Errorf("DescribeConfig() missing path %q", key)
		}
	}
}
