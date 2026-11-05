package skill

import (
	// 标准库
	"strings"
	"testing"
)

// testBuiltinSkill 返回一条合法的内置技能代码定义，供各用例复用。
func testBuiltinSkill() BuiltinSkill {
	return BuiltinSkill{
		Name:    "test_skill",
		Content: "## 指引\n按此执行。",
	}
}

// TestValidateBuiltinSkills 覆盖代码注册表一致性校验：合法清单与各类残缺清单。
func TestValidateBuiltinSkills(t *testing.T) {
	valid := testBuiltinSkill()
	cases := []struct {
		name    string
		skills  []BuiltinSkill
		wantErr bool
	}{
		{name: "empty registry", skills: nil},
		{name: "valid", skills: []BuiltinSkill{valid}},
		{name: "empty name", skills: []BuiltinSkill{{Name: "", Content: "c"}}, wantErr: true},
		{name: "invalid name", skills: []BuiltinSkill{{Name: "Alpha", Content: "c"}}, wantErr: true},
		{name: "empty content", skills: []BuiltinSkill{{Name: "a", Content: " "}}, wantErr: true},
		{name: "duplicate name", skills: []BuiltinSkill{valid, valid}, wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateBuiltinSkills(c.skills)
			if c.wantErr && err == nil {
				t.Fatalf("validateBuiltinSkills(%+v) = nil, want error", c.skills)
			}
			if !c.wantErr && err != nil {
				t.Fatalf("validateBuiltinSkills(%+v) error = %v", c.skills, err)
			}
		})
	}
}

// TestBuiltinPeriodicReportCoCreateSkill 守卫「周期报告技能共创」内置技能的关键段落：正文必须同时
// 承载周期报告定位、多轮共创流程、满分 100 的评分标准、创建技能能力与最终交付指引，并明确
// 报告技能运行在后台无人值守环境（运行期禁止 ask_user、信息须在共创期收集、审批口径与访问密钥
// 前置条件），任一段落缺失即视为能力失效。
func TestBuiltinPeriodicReportCoCreateSkill(t *testing.T) {
	spec, ok := LookupBuiltin(builtinSkillNamePeriodicReportCoCreate)
	if !ok {
		t.Fatalf("builtin skill %q is not registered", builtinSkillNamePeriodicReportCoCreate)
	}
	for _, marker := range []string{
		"周期报告", "多轮对话", "评分标准（满分 100）", "ai.skill.create", `"complete": true`,
		"arguments are incomplete", "最终交付",
		// 后台无人值守与信息前置收集
		"后台无人值守", "运行期没有用户在场", "运行期禁止调用 ask_user",
		"固定参数（fixedFields）", "输入字段（inputFields", "一次性或分批",
		// 运行期无审批但共创期仍需确认，且不得越权
		"自动通过审批", "以审批卡片挂起", "RBAC",
		// 访问密钥前置确认
		"访问密钥", "启用且未过期",
	} {
		if !strings.Contains(spec.Content, marker) {
			t.Errorf("builtin skill %q content misses %q", builtinSkillNamePeriodicReportCoCreate, marker)
		}
	}
}

// TestBuiltinRiskTagRegoRuleSkill 守卫「风险标签 Rego 规则共创」内置技能的关键段落：正文必须同时
// 承载风险标签定位、Rego 契约、校验与 dry-run、评分标准、创建能力与最终交付指引，任一段落缺失
// 即视为能力失效。
func TestBuiltinRiskTagRegoRuleSkill(t *testing.T) {
	spec, ok := LookupBuiltin(builtinSkillNameRiskTagRegoRule)
	if !ok {
		t.Fatalf("builtin skill %q is not registered", builtinSkillNameRiskTagRegoRule)
	}
	for _, marker := range []string{
		"风险标签", "package zki", "tag.rule.validate", "tag.rule.single/dry-run",
		"评分标准（满分 100）", "tag.rule.create", "最终交付",
	} {
		if !strings.Contains(spec.Content, marker) {
			t.Errorf("builtin skill %q content misses %q", builtinSkillNameRiskTagRegoRule, marker)
		}
	}
}

// TestLookupBuiltin 校验按名查询与名称清单：注册表可临时追加测试项，查询与清单随之一致。
func TestLookupBuiltin(t *testing.T) {
	original := append([]BuiltinSkill(nil), BuiltinSkills...)
	defer func() { BuiltinSkills = original }()

	BuiltinSkills = append(append([]BuiltinSkill(nil), original...), testBuiltinSkill())

	spec, ok := LookupBuiltin("test_skill")
	if !ok || spec.Content != "## 指引\n按此执行。" {
		t.Fatalf("LookupBuiltin(test_skill) = %+v, ok = %v", spec, ok)
	}
	if _, ok := LookupBuiltin("missing"); ok {
		t.Fatal("LookupBuiltin(missing) should miss")
	}

	names := BuiltinSkillNames()
	if len(names) != len(original)+1 || names[len(names)-1] != "test_skill" {
		t.Fatalf("BuiltinSkillNames() = %v, want the original names plus test_skill", names)
	}
}
