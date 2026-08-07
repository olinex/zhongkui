package tag

import (
	"strings"
	"time"

	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"

	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"
)

// type Atom struct {
// 	Label *string       `@Ident`        // 标签（标识符）
// 	Expr  *BoolExpr     `| "(" @@ ")"`  // 括号子表达式
// }

// 布尔表达式解析器，用于解析风险规则的条件表达式
// 支持 AND、OR、NOT 操作符和括号分组

var (
	// parser 是布尔表达式的解析器实例，用于解析风险规则的条件表达式
	parser *participle.Parser[BoolExpr]
)

// BoolExpr 表示一个布尔表达式，由多个用 OR 连接的子项组成
// 例如: "A AND B OR C AND D"
type BoolExpr struct {
	Or []*AndTerm `parser:"@@ ( 'OR' @@ )*"` // 用 OR 连接的子项
}

// String 将 BoolExpr 序列化为标准化的字符串表示
// 去除多余的空格，使用统一的格式
func (e *BoolExpr) String() string {
	if e == nil {
		return ""
	}

	result := ""
	for i, term := range e.Or {
		if i > 0 {
			result += " OR "
		}
		result += term.String()
	}
	return result
}

// AndTerm 表示一个 AND 项，由多个用 AND 连接的因子组成
// 例如: "A AND B AND C"
type AndTerm struct {
	Not []*NotFactor `parser:"@@ ( 'AND' @@ )*"` // 用 AND 连接的因子
}

// String 将 AndTerm 序列化为标准化的字符串表示
func (t *AndTerm) String() string {
	if t == nil || len(t.Not) == 0 {
		return ""
	}

	var result strings.Builder
	for i, factor := range t.Not {
		if i > 0 {
			result.WriteString(" AND ")
		}
		result.WriteString(factor.String())
	}
	return result.String()
}

// NotFactor 表示一个 NOT 因子，包含可选的 NOT 操作符和一个原子项
// 例如: "NOT A" 或 "A"
type NotFactor struct {
	Not  bool  `parser:"@'NOT'?"` // 可选 NOT 操作符
	Atom *Atom `parser:"@@"`      // 原子项（标签标识符或括号子表达式）
}

// String 将 NotFactor 序列化为标准化的字符串表示
func (f *NotFactor) String() string {
	if f == nil || f.Atom == nil {
		return ""
	}

	result := ""
	if f.Not {
		result += "NOT "
	}
	result += f.Atom.String()
	return result
}

// Atom 表示一个原子项，可以是标签标识符或括号内的布尔表达式
// 例如: "tag_name" 或 "(A OR B)"
type Atom struct {
	Label *string   `parser:"@Ident"`       // 标签标识符，如 "tag_name"
	Expr  *BoolExpr `parser:"| '(' @@ ')'"` // 括号内的布尔表达式
}

// String 将 Atom 序列化为标准化的字符串表示
func (a *Atom) String() string {
	if a == nil {
		return ""
	}

	if a.Label != nil {
		return *a.Label
	} else if a.Expr != nil {
		return "(" + a.Expr.String() + ")"
	}
	return ""
}

// RiskCondition 表示风险规则条件，定义了触发风险的逻辑条件
// 例如: "在5分钟内，如果标签A和标签B同时出现超过3次，则触发风险"
type RiskCondition struct {
	database.BaseModel
	RiskRuleID  uint          `gorm:"column:risk_rule_id;type:INT"`
	RiskRule    RiskRule      `gorm:"foreignKey:RiskRuleID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Name        string        `gorm:"column:name;type:VARCHAR(255)"`
	Window      time.Duration `gorm:"type:BIGINT"`
	Threshold   int           `gorm:"type:INT"`
	Expression  string        `gorm:"type:VARCHAR(1024)"`
	Enabled     bool          `gorm:"type:BOOLEAN"`
	Description string        `gorm:"type:VARCHAR(256)"`
}

// ExprValidate 验证风险条件中的标签表达式语法是否正确
// 使用布尔表达式解析器检查表达式是否可以被正确解析
// 返回精炼后的表达式字符串（去除无意义的空格和回车）和错误信息
func (c *RiskCondition) ExprValidate() (string, error) {
	expr, err := parser.ParseString("", c.Expression)
	if err != nil {
		return "", errors.WithStack(err)
	}
	// 将解析后的AST序列化为标准化的字符串格式
	refinedExpr := expr.String()
	return refinedExpr, nil
}

// RiskRule 表示风险规则，用于定义特定类型事件或属性的风险判定规则
// 例如: "登录失败"、"异常访问"、"高风险操作"
type RiskRule struct {
	database.BaseModel
	Name        string `gorm:"column:name;type:VARCHAR(255)"`
	Enabled     bool   `gorm:"type:BOOLEAN"`
	Description string `gorm:"type:VARCHAR(256)"`
}

// init 函数在包初始化时执行，用于注册数据模型和初始化布尔表达式解析器
func init() {
	// 初始化布尔表达式解析器
	// 使用 participle 库构建一个支持 AND、OR、NOT 操作符和括号的解析器
	{
		tmp, err := participle.Build[BoolExpr](
			participle.Lexer(lexer.MustSimple([]lexer.SimpleRule{
				{Name: "Ident", Pattern: `[a-zA-Z]([a-zA-Z0-9_-]*[a-zA-Z0-9])?`}, // 标识符规则: 字母开头，可包含字母、数字、下划线和连字符，不能以下划线或连字符结尾
				{Name: "Punct", Pattern: `[()]`},                                 // 标点符号规则: 括号
				{Name: "Whitespace", Pattern: `\s+`},                             // 空白字符规则: 用于忽略空格
			})),
			participle.Elide("Whitespace"), // 忽略空白字符
		)
		if err != nil {
			// 解析器构建失败会导致程序无法启动，因为风险规则功能依赖表达式解析
			panic(err)
		}
		parser = tmp
	}
}
