package tag

import (
	"strings"

	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"

	"zhongkuisec.com/zhongkui/internal/models/core"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// 布尔表达式解析器，用于解析标签事件条件中的标签组合表达式
// 支持 AND、OR、NOT 操作符和括号分组

var (
	// parser 是布尔表达式的解析器实例，用于解析标签事件条件的标签组合表达式
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
// Label 为值类型 string，标识符分支未匹配时保持零值空串，与 Expr 分支由 nil 区分
type Atom struct {
	Label string    `parser:"@Ident"`       // 标签标识符，如 "tag_name"、"登录失败"（支持中文）
	Expr  *BoolExpr `parser:"| '(' @@ ')'"` // 括号内的布尔表达式
}

// String 将 Atom 序列化为标准化的字符串表示
func (a *Atom) String() string {
	if a == nil {
		return ""
	}

	if a.Label != "" {
		return a.Label
	} else if a.Expr != nil {
		return "(" + a.Expr.String() + ")"
	}
	return ""
}

// TagEventCondition 表示标签事件条件，是标签事件规则（TagEventRule）中单条触发条件的判定逻辑。
// 在时间窗口（Window）内，若标签组合表达式（Expression）的命中次数达到阈值（Threshold），
// 则触发该条件对应的事件。
// Expression 为标签标识符的布尔组合（AND / OR / NOT / 括号），其中的标签由 TagRule
// 识别产生，与具体标签维度（risk / asset / behavior）解耦。
// 条件与规则为多对一关系，命名空间由所属 TagEventRule 承载，条件自身不冗余存储。
// 例如: "在5分钟内，如果标签A和标签B同时出现超过3次，则触发事件"
type TagEventCondition struct {
	database.BaseModel
	// Model 目标模型类型键过滤条件，对应 TagSumming 的 model 维度，为空表示不限制（如 zhongkuisec.com/zhongkui/internal/models/network.HttpTransport）
	Model string `gorm:"column:model;type:VARCHAR(255)"`
	// Dimension 标签维度类型过滤条件，对应 TagSumming 的 dimension 维度，为空表示不限制（如 "fpTcpClientSig"、"fpTcpServerSig"）
	Dimension string `gorm:"column:dimension;type:VARCHAR(128)"`
	// Item 标签维度具体值过滤条件，对应 TagSumming 的 item 维度，为空表示不限制（如具体的指纹签名串）
	Item string `gorm:"column:item;type:VARCHAR(255)"`
	// TagEventRuleID 所属标签事件规则 ID
	TagEventRuleID uint `gorm:"column:tag_event_rule_id;type:INT"`
	// TagEventRule 所属标签事件规则关联模型，随规则级联更新/删除
	TagEventRule TagEventRule `gorm:"foreignKey:TagEventRuleID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	// Name 条件名称
	Name string `gorm:"column:name;type:VARCHAR(255)"`
	// Window 统计时间窗口（纳秒），在该窗口内统计标签命中次数
	Window serializer.DurationNS `gorm:"type:BIGINT"`
	// Threshold 触发阈值，窗口内命中次数超过该值即触发
	Threshold int `gorm:"type:INT"`
	// Expression 标签组合布尔表达式（AND / OR / NOT / 括号）
	Expression string `gorm:"type:VARCHAR(1024)"`
	// Enabled 是否启用该条件
	Enabled bool `gorm:"type:BOOLEAN"`
	// Description 条件描述
	Description string `gorm:"type:VARCHAR(256)"`
}

// ExprValidate 验证标签事件条件中的标签组合表达式语法是否正确
// 使用布尔表达式解析器检查表达式是否可以被正确解析
// 返回精炼后的表达式字符串（去除无意义的空格和回车）和错误信息
func (c *TagEventCondition) ExprValidate() (string, error) {
	expr, err := parser.ParseString("", c.Expression)
	if err != nil {
		return "", errors.WithStack(err)
	}
	// 将解析后的AST序列化为标准化的字符串格式
	refinedExpr := expr.String()
	return refinedExpr, nil
}

// TagEventRule 表示标签事件规则，是基于已识别标签的高层次识别规则。
// 由一条或多条标签事件条件（TagEventCondition）组成，条件中的标签来自
// TagRule 的识别结果，因此该规则与具体标签维度（risk / asset / behavior）无关。
// 规则归属命名空间（NamespaceName），不同租户的事件规则相互隔离。
// 例如: "登录失败"、"异常访问"、"高风险操作"等事件判定
type TagEventRule struct {
	database.BaseModel
	// Namespace 所属命名空间的关联模型，用于多租户隔离
	Namespace core.Namespace `gorm:"foreignKey:NamespaceName;references:Name"`
	// NamespaceName 所属命名空间名称
	NamespaceName string `gorm:"column:namespace_name;type:VARCHAR(32)"`
	// Name 规则名称
	Name string `gorm:"column:name;type:VARCHAR(255)"`
	// Enabled 是否启用该规则
	Enabled bool `gorm:"type:BOOLEAN"`
	// MatchAll 是否要求规则下所有条件均命中才触发事件；为 false（默认）时任一条件命中即触发
	MatchAll bool `gorm:"column:match_all;type:BOOLEAN;default:false"`
	// NotificationGroupID 规则绑定的通知组 ID，nil 表示未绑定，此时使用命名空间默认通知组；
	// 通知组删除时置空（ON DELETE SET NULL）
	NotificationGroupID *uint `gorm:"column:notification_group_id;type:INT;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	// NotificationTargetID 规则直绑的通知目标 ID，nil 表示未直绑目标；用于向单个目标直接发送通知而无需建组。
	// 通知目标删除时置空（ON DELETE SET NULL）
	NotificationTargetID *uint `gorm:"column:notification_target_id;type:INT;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	// SuppressWindow 规则级抑制窗口（纳秒），同一事件键在此窗口内不重复告警；0 表示不抑制（每个事件都告警）
	// 显式零值必须原样落库，故模型不设 default tag（业务默认 1 小时由 API 输入层 defaults 填充）
	SuppressWindow serializer.DurationNS `gorm:"column:suppress_window;type:BIGINT"`
	// Description 规则描述
	Description string `gorm:"column:description;type:VARCHAR(256)"`
}

// init 函数在包初始化时执行，用于初始化布尔表达式解析器
// 使用 participle 库构建一个支持 AND、OR、NOT 操作符和括号的解析器
func init() {
	{
		tmp, err := participle.Build[BoolExpr](
			participle.Lexer(lexer.MustSimple([]lexer.SimpleRule{
				// 标识符规则: 以 Unicode 字母开头（含中文），可包含字母、数字、下划线和连字符，不能以连字符结尾。
				// 标签名可能为中文（如 "登录失败"），故用 \p{L}/\p{N} 而非仅 ASCII
				{Name: "Ident", Pattern: `[\p{L}]([\p{L}\p{N}_-]*[\p{L}\p{N}])?`},
				{Name: "Punct", Pattern: `[()]`},     // 标点符号规则: 括号
				{Name: "Whitespace", Pattern: `\s+`}, // 空白字符规则: 用于忽略空格
			})),
			participle.Elide("Whitespace"), // 忽略空白字符
		)
		if err != nil {
			// 解析器构建失败会导致程序无法启动，因为标签事件规则功能依赖表达式解析
			panic(err)
		}
		parser = tmp
	}
}

// ParseBoolExpr 解析标签组合布尔表达式，返回语法树。
// 供 tagengine 将条件表达式翻译为 ClickHouse 查询谓词复用，与 ExprValidate 使用同一解析器。
func ParseBoolExpr(expr string) (*BoolExpr, error) {
	return parser.ParseString("", expr)
}
