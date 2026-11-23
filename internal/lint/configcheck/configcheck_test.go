package configcheck

import (
	"os"
	"path/filepath"
	"testing"
)

// writeTemp 在临时目录写入测试文件并返回路径。
func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写入临时文件失败: %v", err)
	}
	return path
}

func TestLoadTOMLKeys(t *testing.T) {
	path := writeTemp(t, "config.toml", `
[api]
port = 8080
enabled = true
ratio = 1.5

[api.cors]
[api.cors.headers]
allow = ['X-A']

[crypto.kms]
command = []

[pipeline.scheduler.tasks.'tag:event:detect']
interval = '1m0s'
payload = {}
`)

	keys, err := LoadTOMLKeys(path)
	if err != nil {
		t.Fatalf("LoadTOMLKeys 失败: %v", err)
	}

	want := map[string]Kind{
		"api":                    KindTable,
		"api.port":               KindInt,
		"api.enabled":            KindBool,
		"api.ratio":              KindFloat,
		"api.cors":               KindTable,
		"api.cors.headers":       KindTable,
		"api.cors.headers.allow": KindArray,
		"crypto.kms":             KindTable,
		"crypto.kms.command":     KindArray,
		"pipeline.scheduler.tasks.tag:event:detect":          KindTable,
		"pipeline.scheduler.tasks.tag:event:detect.interval": KindString,
		"pipeline.scheduler.tasks.tag:event:detect.payload":  KindTable,
	}
	for key, kind := range want {
		got, ok := keys[key]
		if !ok {
			t.Errorf("缺少键 %s", key)
			continue
		}
		if got != kind {
			t.Errorf("键 %s 类型 = %s，期望 %s", key, got, kind)
		}
	}
}

func TestCompare(t *testing.T) {
	authority := map[string]Kind{
		"api":        KindTable,
		"api.port":   KindInt,
		"api.host":   KindString,
		"crypto.kms": KindTable,
	}
	target := map[string]Kind{
		"api":      KindTable,
		"api.port": KindString,
		"stale":    KindBool,
	}

	diff := Compare(authority, target)
	if diff.Empty() {
		t.Fatal("期望存在差异")
	}
	if len(diff.Missing) != 2 || diff.Missing[0] != "api.host" || diff.Missing[1] != "crypto.kms" {
		t.Fatalf("Missing = %v", diff.Missing)
	}
	if len(diff.Extra) != 1 || diff.Extra[0] != "stale" {
		t.Errorf("Extra = %v", diff.Extra)
	}
	if len(diff.TypeMismatch) != 1 || diff.TypeMismatch[0].Path != "api.port" ||
		diff.TypeMismatch[0].Authority != KindInt || diff.TypeMismatch[0].Target != KindString {
		t.Errorf("TypeMismatch = %+v", diff.TypeMismatch)
	}
}

func TestCompareIdentical(t *testing.T) {
	authority := map[string]Kind{"a": KindString, "b": KindTable}
	target := map[string]Kind{"a": KindString, "b": KindTable}
	if diff := Compare(authority, target); !diff.Empty() {
		t.Fatalf("相同键集合不应有差异: %+v", diff)
	}
}

func TestCheckEnvOverrides(t *testing.T) {
	authority := map[string]Kind{
		"database.core.dsn":            KindString,
		"redis.cache.addr":             KindString,
		"s3.bucket":                    KindString,
		"api.secure.cors.allowOrigins": KindArray,
	}

	compose := writeTemp(t, "docker-compose.yml", `
x-zhongkui-env: &env
  ZKI_DATABASE_CORE_DSN: "host=${ZKI_DATABASE_CORE_DSN:-postgres}"
  ZKI_S3_BUCKET: "capture"
  ZKI_ENVIRONMENT: "production"
  ZKI_NOT_A_REAL_KEY: "1"
`)
	helm := writeTemp(t, "helpers.tpl", `
- name: ZKI_REDIS_CACHE_ADDR
  value: "redis:6379"
- name: ZKI_GHOST_KEY
  value: "x"
`)

	issues, err := CheckEnvOverrides(authority, []EnvFile{
		{Label: "compose-env", Path: compose},
		{Label: "helm-env", Path: helm},
	}, []string{"ZKI_ENVIRONMENT"})
	if err != nil {
		t.Fatalf("CheckEnvOverrides 失败: %v", err)
	}

	if len(issues) != 2 {
		t.Fatalf("未知环境变量数量 = %d，期望 2：%+v", len(issues), issues)
	}
	if issues[0].Name != "ZKI_NOT_A_REAL_KEY" || issues[0].Label != "compose-env" {
		t.Errorf("第一个问题 = %+v", issues[0])
	}
	if issues[1].Name != "ZKI_GHOST_KEY" || issues[1].Label != "helm-env" {
		t.Errorf("第二个问题 = %+v", issues[1])
	}
}

func TestCheckEnvOverridesCollision(t *testing.T) {
	authority := map[string]Kind{"foo.bar": KindString, "foo_bar": KindString}
	file := writeTemp(t, "env.yml", "  ZKI_FOOBAR: x\n")

	if _, err := CheckEnvOverrides(authority, []EnvFile{{Label: "x", Path: file}}, nil); err == nil {
		t.Fatal("归一化碰撞应返回错误")
	}
}
