// 本文件声明「内置技能」的代码注册表，与内置工具的链路同构：内置技能的
// 注册信息（名称、明文描述、启停）由 migrate fixture 注入 agent_skills 表的系统命名空间行，
// 代码侧（BuiltinSkills）只承载按名对齐的技能正文，二者漂移由守卫测试拦截。内置技能跨命名
// 空间恒可用、不落租户表；命名空间级自定义技能仍由本包的 Store 持久化。内置与自定义技能
// 都以 agent_skills 行 ID 表达一次选择，不再区分引用前缀。
package skill

import (
	// 标准库
	"regexp"
	"strings"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// builtinSkillNameExpr 是内置技能名称的取值表达式：小写字母开头，仅含小写字母、数字与下划线，
// 与内置工具的 snake_case 命名风格一致，用于校验代码注册表的名称。
const builtinSkillNameExpr = `[a-z][a-z0-9_]*`

// builtinSkillNamePattern 校验内置技能名称须整串匹配 builtinSkillNameExpr。
var builtinSkillNamePattern = regexp.MustCompile(`^` + builtinSkillNameExpr + `$`)

// BuiltinSkill 是一条内置技能的代码侧定义：Name 与 agent_skills 系统命名空间注册行对齐，
// Content 是模型指引正文（Markdown 文本，不落库）。展示描述以数据库行的明文 description 为准。
type BuiltinSkill struct {
	// Name 与 agent_skills 注册行对齐的稳定键（snake_case、全局唯一）
	Name string
	// Content 技能正文（模型指引，Markdown 文本）
	Content string
}

// BuiltinSkills 是全部内置技能的代码侧正文注册表。新增内置技能时需同时在此追加一条
// BuiltinSkill 并在 002_fixtures.sql 的 agent_skills 系统命名空间出厂段追加对应注册行，
// 二者由守卫测试双向校验。当前登记「周期报告技能共创」与「风险标签 Rego 规则共创」技能，
// 正文分别见 builtin_periodic_report_cocreate.go 与 builtin_risk_tag_rego_rule.go。
var BuiltinSkills = []BuiltinSkill{
	{Name: builtinSkillNamePeriodicReportCoCreate, Content: builtinSkillPeriodicReportCoCreateContent},
	{Name: builtinSkillNameRiskTagRegoRule, Content: builtinSkillRiskTagRegoRuleContent},
}

// LookupBuiltin 按名查询内置技能的代码侧定义，未登记时返回 ok=false。
func LookupBuiltin(name string) (BuiltinSkill, bool) {
	for _, skill := range BuiltinSkills {
		if skill.Name == name {
			return skill, true
		}
	}
	return BuiltinSkill{}, false
}

// BuiltinSkillNames 返回代码注册表中的内置技能名（声明顺序），供守卫测试与诊断使用。
func BuiltinSkillNames() []string {
	names := make([]string, len(BuiltinSkills))
	for i, skill := range BuiltinSkills {
		names[i] = skill.Name
	}
	return names
}

// validateBuiltinSkills 校验代码注册表自身一致性：名称非空且符合命名规范、名称唯一、正文非空。
// 返回 nil 表示注册表合法。
func validateBuiltinSkills(skills []BuiltinSkill) error {
	seen := make(map[string]struct{}, len(skills))
	for _, skill := range skills {
		if skill.Name == "" {
			return errors.New("ai/skill: builtin skill with an empty name")
		}
		if !builtinSkillNamePattern.MatchString(skill.Name) {
			return errors.Newf("ai/skill: builtin skill name %q must match %s", skill.Name, builtinSkillNamePattern.String())
		}
		if _, dup := seen[skill.Name]; dup {
			return errors.Newf("ai/skill: duplicate builtin skill name: %s", skill.Name)
		}
		seen[skill.Name] = struct{}{}
		if strings.TrimSpace(skill.Content) == "" {
			return errors.Newf("ai/skill: builtin skill %s has empty content", skill.Name)
		}
	}
	return nil
}

// init 校验内置技能代码注册表自身一致性（fail fast）：任一项不满足即 panic，避免非法注册表
// 在生产中以残缺形态运行。
func init() {
	if err := validateBuiltinSkills(BuiltinSkills); err != nil {
		panic(err)
	}
}
