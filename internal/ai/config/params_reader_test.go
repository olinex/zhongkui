package config

import (
	// 标准库
	"strings"
	"testing"
)

// TestSpecs_Consistency 校验注册表自身一致性：键非空且唯一、分组非空、类型合法，
// 且注册表不携带任何出厂值（出厂值只由 migrate fixture 注入 agent_params）。
func TestSpecs_Consistency(t *testing.T) {
	if len(Specs) == 0 {
		t.Fatal("Specs is empty")
	}
	seen := make(map[string]bool, len(Specs))
	for _, spec := range Specs {
		if spec.Key == "" {
			t.Errorf("spec with empty key: %+v", spec)
		}
		if spec.Group == "" {
			t.Errorf("spec %q has an empty group", spec.Key)
		}
		if seen[spec.Key] {
			t.Errorf("duplicate spec key %q", spec.Key)
		}
		seen[spec.Key] = true
		switch spec.Kind {
		case ParamKindString, ParamKindText, ParamKindInt, ParamKindFloat, ParamKindBool:
		default:
			t.Errorf("spec %q has an unknown kind %q", spec.Key, spec.Kind)
		}
	}
}

// TestSpecs_Order 校验注册表声明顺序稳定：首个键为 agent.systemPrompt，且分组首次出现
// 的顺序与约定一致（该顺序即前端展示顺序）。
func TestSpecs_Order(t *testing.T) {
	if Specs[0].Key != "agent.systemPrompt" {
		t.Errorf("Specs[0].Key = %q, want agent.systemPrompt", Specs[0].Key)
	}
	var groups []string
	for _, spec := range Specs {
		if len(groups) == 0 || groups[len(groups)-1] != spec.Group {
			groups = append(groups, spec.Group)
		}
	}
	want := []string{"agent", "retrieval", "session", "memory", "chunking", "eval", "sanitize"}
	if strings.Join(groups, ",") != strings.Join(want, ",") {
		t.Errorf("group order = %v, want %v", groups, want)
	}
}

// TestLookup 校验按键查询参数元信息：命中时返回完整元信息，未声明时返回 ok=false。
func TestLookup(t *testing.T) {
	spec, ok := Lookup("retrieval.topK")
	if !ok {
		t.Fatal("Lookup(retrieval.topK) ok = false, want true")
	}
	if spec.Group != "retrieval" || spec.Kind != ParamKindInt {
		t.Errorf("Lookup(retrieval.topK) = %+v, want group=retrieval kind=int", spec)
	}
	if _, ok := Lookup("not.declared"); ok {
		t.Errorf("Lookup(not.declared) ok = true, want false")
	}
}

// TestValidateValue 校验按类型校验字面量：可解析返回 nil，不可解析返回含键与字面量的
// 英文错误；string / text 类接受任意字面量。
func TestValidateValue(t *testing.T) {
	if err := ValidateValue(ParamSpec{Key: "agent.systemPrompt", Kind: ParamKindText}, "anything"); err != nil {
		t.Errorf("ValidateValue(text) error = %v, want nil", err)
	}
	if err := ValidateValue(ParamSpec{Key: "retrieval.fusion", Kind: ParamKindString}, ""); err != nil {
		t.Errorf("ValidateValue(string) error = %v, want nil", err)
	}
	valid := []ParamSpec{
		{Key: "retrieval.topK", Kind: ParamKindInt},
		{Key: "retrieval.weightVector", Kind: ParamKindFloat},
	}
	literals := map[string]string{
		"retrieval.topK":         "5",
		"retrieval.weightVector": "0.6",
	}
	for _, spec := range valid {
		if err := ValidateValue(spec, literals[spec.Key]); err != nil {
			t.Errorf("ValidateValue(%s) error = %v, want nil", spec.Key, err)
		}
	}
	invalid := []struct {
		spec    ParamSpec
		literal string
		want    string
	}{
		{ParamSpec{Key: "retrieval.topK", Kind: ParamKindInt}, "not-a-number", `parameter "retrieval.topK" is not a valid int`},
		{ParamSpec{Key: "retrieval.weightVector", Kind: ParamKindFloat}, "not-a-float", `parameter "retrieval.weightVector" is not a valid float`},
		{ParamSpec{Key: "sanitize.maxItems", Kind: ParamKindBool}, "not-a-bool", `parameter "sanitize.maxItems" is not a valid bool`},
	}
	for _, c := range invalid {
		err := ValidateValue(c.spec, c.literal)
		if err == nil {
			t.Errorf("ValidateValue(%s, %q) error = nil, want an error", c.spec.Key, c.literal)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("ValidateValue(%s, %q) error = %q, want containing %q", c.spec.Key, c.literal, err, c.want)
		}
	}
}

// TestValidateRelations 校验跨键关系：融合权重非负且不同时为零，工具结果字节上限不小于整篇
// 展开预算；缺失的键跳过，存在但不可解析的键返回错误。
func TestValidateRelations(t *testing.T) {
	cases := []struct {
		name   string
		values map[string]string
		want   string
	}{
		{
			name:   "factory values",
			values: map[string]string{"retrieval.weightVector": "0.6", "retrieval.weightKeyword": "0.4", "agent.maxToolResultBytes": "32768", "retrieval.expandDocLimit": "26000"},
		},
		{
			name:   "missing keys skipped",
			values: map[string]string{"retrieval.weightVector": "0.6"},
		},
		{
			name:   "negative weight",
			values: map[string]string{"retrieval.weightVector": "-0.1", "retrieval.weightKeyword": "0.4"},
			want:   "must be non-negative",
		},
		{
			name:   "both weights zero",
			values: map[string]string{"retrieval.weightVector": "0", "retrieval.weightKeyword": "0"},
			want:   "must not both be zero",
		},
		{
			name:   "tool result budget below expansion budget",
			values: map[string]string{"agent.maxToolResultBytes": "1024", "retrieval.expandDocLimit": "26000"},
			want:   "must be at least retrieval.expandDocLimit",
		},
		{
			name:   "invalid float literal",
			values: map[string]string{"retrieval.weightVector": "not-a-float"},
			want:   "is not a valid float",
		},
	}
	for _, c := range cases {
		err := ValidateRelations(c.values)
		if c.want == "" {
			if err != nil {
				t.Errorf("%s: ValidateRelations() error = %v, want nil", c.name, err)
			}
			continue
		}
		if err == nil {
			t.Errorf("%s: ValidateRelations() error = nil, want containing %q", c.name, c.want)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: ValidateRelations() error = %q, want containing %q", c.name, err, c.want)
		}
	}
}

// TestParams_Values 校验生效值按类型读取（string/text/int/float/bool），值全部来自
// 构造时传入的映射（即数据库 agent_params.value）。
func TestParams_Values(t *testing.T) {
	p := NewParams(map[string]string{
		"retrieval.fusion":       "weighted",
		"agent.systemPrompt":     "custom agent prompt",
		"retrieval.topK":         "9",
		"retrieval.weightVector": "0.25",
	})
	if got := p.String("retrieval.fusion"); got != "weighted" {
		t.Errorf("String(retrieval.fusion) = %q, want weighted", got)
	}
	if got := p.Text("agent.systemPrompt"); got != "custom agent prompt" {
		t.Errorf("Text(agent.systemPrompt) = %q, want custom agent prompt", got)
	}
	if got := p.Int("retrieval.topK"); got != 9 {
		t.Errorf("Int(retrieval.topK) = %d, want 9", got)
	}
	if got := p.Float("retrieval.weightVector"); got != 0.25 {
		t.Errorf("Float(retrieval.weightVector) = %v, want 0.25", got)
	}
}

// TestParams_EmptyValueIsMissing 校验空串值在构造时被丢弃，读取时按「库内无值」panic
// （本包已无出厂值可回落，缺值即数据未按迁移种子化）。
func TestParams_EmptyValueIsMissing(t *testing.T) {
	p := NewParams(map[string]string{"retrieval.fusion": "", "retrieval.topK": ""})
	assertPanicContains(t, "empty string value", func() { p.String("retrieval.fusion") },
		`parameter retrieval.fusion has no value in the database`)
	assertPanicContains(t, "empty int value", func() { p.Int("retrieval.topK") },
		`parameter retrieval.topK has no value in the database`)
	assertPanicContains(t, "nil params", func() { var nilParams *Params; nilParams.Int("retrieval.topK") },
		`parameter retrieval.topK has no value in the database`)
}

// TestParams_InvalidValuePanics 校验不可解析的生效值 panic（英文消息，含键与字面量），
// 不再回落任何默认值。
func TestParams_InvalidValuePanics(t *testing.T) {
	p := NewParams(map[string]string{
		"retrieval.topK":         "not-a-number",
		"retrieval.weightVector": "not-a-float",
	})
	assertPanicContains(t, "invalid int", func() { p.Int("retrieval.topK") },
		`parameter retrieval.topK is not a valid int: not-a-number`)
	assertPanicContains(t, "invalid float", func() { p.Float("retrieval.weightVector") },
		`parameter retrieval.weightVector is not a valid float: not-a-float`)
}

// TestParams_Panics 校验编程缺陷快速失败：读取未声明键或类型不符时 panic（英文消息）。
func TestParams_Panics(t *testing.T) {
	p := NewParams(map[string]string{"retrieval.topK": "5"})
	cases := []struct {
		name string
		call func()
		want string
	}{
		{"undeclared int", func() { p.Int("not.declared") }, "undeclared parameter key: not.declared"},
		{"undeclared string", func() { p.String("not.declared") }, "undeclared parameter key: not.declared"},
		{"kind mismatch string", func() { p.String("retrieval.topK") }, "is int, not string"},
		{"kind mismatch text", func() { p.Text("retrieval.topK") }, "is int, not text"},
		{"kind mismatch bool", func() { p.Bool("retrieval.topK") }, "is int, not bool"},
	}
	for _, c := range cases {
		assertPanicContains(t, c.name, c.call, c.want)
	}
}

// assertPanicContains 断言 call 触发 panic 且 panic 值（字符串）包含 want。
func assertPanicContains(t *testing.T, name string, call func(), want string) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Errorf("%s: no panic, want a panic containing %q", name, want)
			return
		}
		msg, ok := r.(string)
		if !ok {
			t.Errorf("%s: panic value = %v (%T), want a string", name, r, r)
			return
		}
		if !strings.Contains(msg, want) {
			t.Errorf("%s: panic message = %q, want containing %q", name, msg, want)
		}
	}()
	call()
}
