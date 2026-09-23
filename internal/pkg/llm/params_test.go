package llm

import (
	// stdlib
	"strings"
	"testing"
)

// TestSpecs_Consistency 校验注册表自身一致性：键非空且唯一、分组非空、类型合法，
// 且注册表不携带任何出厂值（出厂值只由 migrate fixture 注入 agent_params）。
// TestSpecs_Consistency verifies the registry consistency: keys are non-empty and unique,
// groups are non-empty, kinds are known, and the registry carries no factory value at all
// (factory values are injected only by the migrate fixtures into agent_params).
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
// TestSpecs_Order verifies the declaration order is stable: the first key is
// agent.systemPrompt and the groups first appear in the agreed order (that order is the
// frontend display order).
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
	want := []string{"agent", "retrieval", "session", "chunking", "eval", "sanitize"}
	if strings.Join(groups, ",") != strings.Join(want, ",") {
		t.Errorf("group order = %v, want %v", groups, want)
	}
}

// TestLookup 校验按键查询参数元信息：命中时返回完整元信息，未声明时返回 ok=false。
// TestLookup verifies the metadata lookup: a hit returns the full metadata and an
// undeclared key returns ok=false.
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
// TestValidateValue verifies the per-kind literal validation: a parsable literal yields nil,
// an unparsable one yields an English error carrying the key and the literal; the string and
// text kinds accept any literal.
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

// TestParams_Values 校验生效值按类型读取（string/text/int/float/bool），值全部来自
// 构造时传入的映射（即数据库 agent_params.value）。
// TestParams_Values verifies the typed reads of the effective values (string/text/int/float/
// bool): every value comes from the map given at construction (agent_params.value).
func TestParams_Values(t *testing.T) {
	p := NewParams(map[string]string{
		"retrieval.fusion":            "weighted",
		"agent.systemPrompt":          "custom agent prompt",
		"retrieval.topK":              "9",
		"retrieval.weightVector":      "0.25",
		"agent.retrievalSectionTitle": "## KB",
	})
	if got := p.String("retrieval.fusion"); got != "weighted" {
		t.Errorf("String(retrieval.fusion) = %q, want weighted", got)
	}
	if got := p.Text("agent.systemPrompt"); got != "custom agent prompt" {
		t.Errorf("Text(agent.systemPrompt) = %q, want custom agent prompt", got)
	}
	if got := p.String("agent.retrievalSectionTitle"); got != "## KB" {
		t.Errorf("String(agent.retrievalSectionTitle) = %q, want ## KB", got)
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
// TestParams_EmptyValueIsMissing verifies an empty value is dropped at construction and
// panics as "no value in the database" when read (the package holds no factory value to fall
// back on, so a missing value means the data was not seeded by the migration).
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
// TestParams_InvalidValuePanics verifies an unparsable effective value panics (an English
// message carrying the key and the literal) instead of falling back to a default.
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
// TestParams_Panics verifies the fail-fast of programming defects: reading an undeclared
// key or a kind mismatch panics (with an English message).
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
// assertPanicContains asserts call panics with a string value containing want.
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
