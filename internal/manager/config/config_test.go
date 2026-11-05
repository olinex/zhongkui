package config

import (
	"testing"

	uv "github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
)

// testValidate 测试用验证器实例（本包配置项无业务校验标签，仅用于满足 ExtractConfig 签名）。
var testValidate = uv.New()

// TestExtractConfig_TrustedProxiesDefault 测试未配置受信代理时默认取空列表：
// 空列表表示不信任任何代理，ClientIP 退化为 TCP 对端地址。
func TestExtractConfig_TrustedProxiesDefault(t *testing.T) {
	// 仅设置同段内其它键，使 [api] 段存在，trustedProxies 走 default 标签回填
	v := viper.New()
	v.Set("api.host", "127.0.0.1")

	cfg, err := ExtractConfig(v, testValidate)
	if err != nil {
		t.Fatalf("ExtractConfig() error = %v", err)
	}
	if cfg.TrustedProxies == nil {
		t.Fatalf("TrustedProxies = nil, want empty non-nil slice (default \"[]\")")
	}
	if len(cfg.TrustedProxies) != 0 {
		t.Errorf("TrustedProxies = %v, want empty", cfg.TrustedProxies)
	}
}

// TestExtractConfig_TrustedProxies 测试受信代理列表的解析：网段原样保留，
// 裸地址补单主机掩码（/32、/128），掩码外的位归一化为零。
func TestExtractConfig_TrustedProxies(t *testing.T) {
	v := viper.New()
	v.Set("api.trustedProxies", []string{
		"10.0.0.0/8",
		"172.17.0.0/16",
		"192.168.1.1",
		"10.1.2.3/24",
		"2001:db8::1",
	})

	cfg, err := ExtractConfig(v, testValidate)
	if err != nil {
		t.Fatalf("ExtractConfig() error = %v", err)
	}

	want := []string{
		"10.0.0.0/8",
		"172.17.0.0/16",
		"192.168.1.1/32",
		"10.1.2.0/24",
		"2001:db8::1/128",
	}
	if len(cfg.TrustedProxies) != len(want) {
		t.Fatalf("TrustedProxies = %v, want %v", cfg.TrustedProxies, want)
	}
	for i, wantValue := range want {
		if got := cfg.TrustedProxies[i].String(); got != wantValue {
			t.Errorf("TrustedProxies[%d] = %q, want %q", i, got, wantValue)
		}
	}
}

// TestExtractConfig_TrustedProxiesInvalid 测试非法网段在配置提取阶段即报错，
// 使服务启动失败而非带着不可信的来源地址判定继续运行。
func TestExtractConfig_TrustedProxiesInvalid(t *testing.T) {
	v := viper.New()
	v.Set("api.trustedProxies", []string{"10.0.0.0/8", "10.0.0.0/999"})

	if _, err := ExtractConfig(v, testValidate); err == nil {
		t.Fatalf("ExtractConfig() error = nil, want error for invalid cidr")
	}
}
