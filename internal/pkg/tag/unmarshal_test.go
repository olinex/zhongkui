package tag

import (
	"testing"
)

// 测试按结构体 tag 解析。
func TestUnmarshal_StructTags(t *testing.T) {
	type Config struct {
		Mode   string `tag:"mode"`
		Length int    `tag:"length"`
		Offset int    `tag:"offset"`
		Salt   string `tag:"salt"`
	}

	var cfg Config
	input := `mode="aes-gcm", length=32, offset=10, salt="randomsalt"`
	if err := Unmarshal(input, &cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Mode != "aes-gcm" {
		t.Errorf("expected mode='aes-gcm', got %q", cfg.Mode)
	}
	if cfg.Length != 32 {
		t.Errorf("expected length=32, got %d", cfg.Length)
	}
	if cfg.Offset != 10 {
		t.Errorf("expected offset=10, got %d", cfg.Offset)
	}
	if cfg.Salt != "randomsalt" {
		t.Errorf("expected salt='randomsalt', got %q", cfg.Salt)
	}
}

// 测试按字段名解析。
func TestUnmarshal_FieldNames(t *testing.T) {
	type Config struct {
		Mode   string
		Length int
		Offset int
	}

	var cfg Config
	input := `Mode="aes-gcm", Length=32, Offset=10`
	if err := Unmarshal(input, &cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Mode != "aes-gcm" {
		t.Errorf("expected Mode='aes-gcm', got %q", cfg.Mode)
	}
	if cfg.Length != 32 {
		t.Errorf("expected Length=32, got %d", cfg.Length)
	}
}

// 测试整数字面量解析。
func TestUnmarshal_IntValuesAsLiteral(t *testing.T) {
	type Config struct {
		Count int
	}

	var cfg Config
	input := `Count=42`
	if err := Unmarshal(input, &cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Count != 42 {
		t.Errorf("expected Count=42, got %d", cfg.Count)
	}
}

// 测试仅部分字段赋值。
func TestUnmarshal_PartialFields(t *testing.T) {
	type Config struct {
		Mode   string `tag:"mode"`
		Length int    `tag:"length"`
		Offset int    `tag:"offset"`
	}

	var cfg Config
	input := `mode="aes-gcm"`
	if err := Unmarshal(input, &cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Mode != "aes-gcm" {
		t.Errorf("expected mode='aes-gcm', got %q", cfg.Mode)
	}
	if cfg.Length != 0 {
		t.Errorf("expected Length=0 (not set), got %d", cfg.Length)
	}
}

// 测试布尔字段解析。
func TestUnmarshal_BoolField(t *testing.T) {
	type Config struct {
		Enabled bool `tag:"enabled"`
	}

	t.Run("string true", func(t *testing.T) {
		var cfg Config
		if err := Unmarshal(`enabled="true"`, &cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !cfg.Enabled {
			t.Error("expected Enabled=true")
		}
	})

	t.Run("int 1", func(t *testing.T) {
		var cfg Config
		if err := Unmarshal(`enabled=1`, &cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !cfg.Enabled {
			t.Error("expected Enabled=true")
		}
	})

	t.Run("int 0", func(t *testing.T) {
		var cfg Config
		if err := Unmarshal(`enabled=0`, &cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Enabled {
			t.Error("expected Enabled=false")
		}
	})
}

// 测试浮点字段解析。
func TestUnmarshal_FloatField(t *testing.T) {
	type Config struct {
		Ratio float64 `tag:"ratio"`
	}

	t.Run("int value", func(t *testing.T) {
		var cfg Config
		if err := Unmarshal(`ratio=3`, &cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Ratio != 3.0 {
			t.Errorf("expected Ratio=3.0, got %f", cfg.Ratio)
		}
	})

	t.Run("string value", func(t *testing.T) {
		var cfg Config
		if err := Unmarshal(`ratio="0.75"`, &cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Ratio != 0.75 {
			t.Errorf("expected Ratio=0.75, got %f", cfg.Ratio)
		}
	})
}

// 测试无符号整数字段解析。
func TestUnmarshal_UintField(t *testing.T) {
	type Config struct {
		Port uint16 `tag:"port"`
	}

	var cfg Config
	if err := Unmarshal(`port=8080`, &cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != 8080 {
		t.Errorf("expected Port=8080, got %d", cfg.Port)
	}
}

// 测试非法输入返回错误。
func TestUnmarshal_Errors(t *testing.T) {
	t.Run("nil target", func(t *testing.T) {
		if err := Unmarshal(`key="val"`, nil); err == nil {
			t.Error("expected error for nil target")
		}
	})

	t.Run("non-pointer", func(t *testing.T) {
		type Config struct {
			Key string `tag:"key"`
		}
		var cfg Config
		if err := Unmarshal(`key="val"`, cfg); err == nil {
			t.Error("expected error for non-pointer target")
		}
	})

	t.Run("pointer to non-struct", func(t *testing.T) {
		var s string
		if err := Unmarshal(`key="val"`, &s); err == nil {
			t.Error("expected error for pointer to non-struct")
		}
	})

	t.Run("invalid tag string", func(t *testing.T) {
		type Config struct {
			Key string `tag:"key"`
		}
		var cfg Config
		if err := Unmarshal(`not a valid tag`, &cfg); err == nil {
			t.Error("expected parse error for invalid tag string")
		}
	})
}

// 测试忽略未导出字段。
func TestUnmarshal_IgnoresUnexportedFields(t *testing.T) {
	type Config struct {
		Exported   string `tag:"exported"`
		unexported string `tag:"unexported"`
	}

	var cfg Config
	input := `exported="hello", unexported="secret"`
	if err := Unmarshal(input, &cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Exported != "hello" {
		t.Errorf("expected Exported='hello', got %q", cfg.Exported)
	}
	if cfg.unexported != "" {
		t.Error("expected unexported field to be empty")
	}
}
