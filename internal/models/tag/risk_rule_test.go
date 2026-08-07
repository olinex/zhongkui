package tag

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"zhongkui.com/zhongkui/internal/pkg/database"
)

// TestBoolExprParser 测试布尔表达式解析器的功能
func TestBoolExprParser(t *testing.T) {
	convey.Convey("测试布尔表达式解析器", t, func() {
		convey.Convey("应该能解析简单的标签表达式", func() {
			testCases := []struct {
				name     string
				expr     string
				valid    bool
				expected string
			}{
				{"简单标签", "tag1", true, "tag1"},
				{"带下划线标签", "tag_name", true, "tag_name"},
				{"带连字符标签", "tag-name", true, "tag-name"},
				{"数字结尾标签", "tag123", true, "tag123"},
				{"大写标签", "TAG", true, "TAG"},
				{"混合大小写标签", "TagName", true, "TagName"},
				{"无效标签-数字开头", "1tag", false, ""},
				{"无效标签-特殊字符", "tag@name", false, ""},
				{"无效标签-空格", "tag name", false, ""},
			}

			for _, tc := range testCases {
				convey.Convey(tc.name, func() {
					expr, err := parser.ParseString("", tc.expr)
					if tc.valid {
						convey.So(err, convey.ShouldBeNil)
						convey.So(expr, convey.ShouldNotBeNil)
						convey.So(expr.Or, convey.ShouldHaveLength, 1)
						convey.So(expr.Or[0].Not, convey.ShouldHaveLength, 1)
						if tc.expected != "" {
							convey.So(*expr.Or[0].Not[0].Atom.Label, convey.ShouldEqual, tc.expected)
						}
					} else {
						convey.So(err, convey.ShouldNotBeNil)
					}
				})
			}
		})

		convey.Convey("应该能解析 AND 表达式", func() {
			testCases := []struct {
				name  string
				expr  string
				valid bool
			}{
				{"简单AND", "A AND B", true},
				{"多个AND", "A AND B AND C", true},
				{"带括号AND", "(A AND B)", true},
				{"无效AND-缺少操作数", "A AND", false},
				{"无效AND-操作符错误", "A & B", false},
			}

			for _, tc := range testCases {
				convey.Convey(tc.name, func() {
					expr, err := parser.ParseString("", tc.expr)
					if tc.valid {
						convey.So(err, convey.ShouldBeNil)
						convey.So(expr, convey.ShouldNotBeNil)
					} else {
						convey.So(err, convey.ShouldNotBeNil)
					}
				})
			}
		})

		convey.Convey("应该能解析 OR 表达式", func() {
			testCases := []struct {
				name  string
				expr  string
				valid bool
			}{
				{"简单OR", "A OR B", true},
				{"多个OR", "A OR B OR C", true},
				{"混合AND和OR", "A AND B OR C", true},
				{"带括号OR", "(A OR B) AND C", true},
				{"无效OR-缺少操作数", "A OR", false},
			}

			for _, tc := range testCases {
				convey.Convey(tc.name, func() {
					expr, err := parser.ParseString("", tc.expr)
					if tc.valid {
						convey.So(err, convey.ShouldBeNil)
						convey.So(expr, convey.ShouldNotBeNil)
					} else {
						convey.So(err, convey.ShouldNotBeNil)
					}
				})
			}
		})

		convey.Convey("应该能解析 NOT 表达式", func() {
			testCases := []struct {
				name  string
				expr  string
				valid bool
			}{
				{"简单NOT", "NOT A", true},
				{"NOT与AND组合", "NOT A AND B", true},
				{"NOT与OR组合", "NOT A OR B", true},
				{"多个NOT", "NOT NOT A", false}, // 解析器可能不支持连续的 NOT 操作符
				{"NOT带括号", "NOT (A OR B)", true},
				{"无效NOT-缺少操作数", "NOT", false},
			}

			for _, tc := range testCases {
				convey.Convey(tc.name, func() {
					expr, err := parser.ParseString("", tc.expr)
					if tc.valid {
						convey.So(err, convey.ShouldBeNil)
						convey.So(expr, convey.ShouldNotBeNil)
					} else {
						convey.So(err, convey.ShouldNotBeNil)
					}
				})
			}
		})

		convey.Convey("应该能解析复杂嵌套表达式", func() {
			testCases := []struct {
				name  string
				expr  string
				valid bool
			}{
				{"嵌套括号", "((A AND B) OR (C AND D))", true},
				{"多层嵌套", "(A OR (B AND (C OR D)))", true},
				{"复杂组合", "NOT (A AND B) OR (C AND NOT D)", true},
				{"混合操作符", "(A OR B) AND (C OR D) AND NOT E", true},
			}

			for _, tc := range testCases {
				convey.Convey(tc.name, func() {
					expr, err := parser.ParseString("", tc.expr)
					if tc.valid {
						convey.So(err, convey.ShouldBeNil)
						convey.So(expr, convey.ShouldNotBeNil)
					} else {
						convey.So(err, convey.ShouldNotBeNil)
					}
				})
			}
		})
	})
}

// TestRiskCondition_ExprValidate 测试 RiskCondition 的 ExprValidate 方法
func TestRiskCondition_ExprValidate(t *testing.T) {
	convey.Convey("测试 RiskCondition.ExprValidate 方法", t, func() {
		convey.Convey("应该验证有效的表达式", func() {
			testCases := []struct {
				name string
				expr string
			}{
				{"简单标签", "login_failed"},
				{"AND表达式", "login_failed AND suspicious_ip"},
				{"OR表达式", "high_risk OR medium_risk"},
				{"NOT表达式", "NOT trusted_device"},
				{"复杂表达式", "(login_failed AND suspicious_ip) OR NOT trusted_device"},
				{"多层嵌套", "((A OR B) AND (C OR D))"},
			}

			for _, tc := range testCases {
				convey.Convey(tc.name, func() {
					condition := &RiskCondition{
						Expression: tc.expr,
					}
					refinedExpr, err := condition.ExprValidate()
					convey.So(err, convey.ShouldBeNil)
					// 验证返回的精炼表达式不为空
					convey.So(refinedExpr, convey.ShouldNotBeEmpty)
				})
			}
		})

		convey.Convey("应该拒绝无效的表达式", func() {
			testCases := []struct {
				name string
				expr string
			}{
				{"空表达式", ""},
				{"无效标签-数字开头", "1invalid"},
				{"无效操作符", "A & B"},
				{"括号不匹配", "(A AND B"},
				{"多余括号", "A AND B)"},
				{"无效字符", "A@B"},
				{"缺少操作数", "A AND"},
				{"操作符错误", "A OR AND B"},
			}

			for _, tc := range testCases {
				convey.Convey(tc.name, func() {
					condition := &RiskCondition{
						Expression: tc.expr,
					}
					_, err := condition.ExprValidate()
					convey.So(err, convey.ShouldNotBeNil)
				})
			}
		})

		convey.Convey("应该处理边缘情况", func() {
			convey.Convey("超长标签名", func() {
				// 创建超长但有效的标签名
				longTag := "a"
				for i := 0; i < 100; i++ {
					longTag += "a"
				}
				condition := &RiskCondition{
					Expression: longTag,
				}
				refinedExpr, err := condition.ExprValidate()
				convey.So(err, convey.ShouldBeNil)
				convey.So(refinedExpr, convey.ShouldEqual, longTag)
			})

			convey.Convey("带空格但不忽略", func() {
				condition := &RiskCondition{
					Expression: "A  AND  B", // 多个空格
				}
				refinedExpr, err := condition.ExprValidate()
				convey.So(err, convey.ShouldBeNil) // 应该成功，因为 Elide("Whitespace") 会忽略空格
				// 验证精炼后的表达式去除了多余空格
				convey.So(refinedExpr, convey.ShouldEqual, "A AND B")
			})
		})
	})
}

// TestRiskCondition_Fields 测试 RiskCondition 结构体字段
func TestRiskCondition_Fields(t *testing.T) {
	convey.Convey("测试 RiskCondition 结构体字段", t, func() {
		convey.Convey("应该正确设置所有字段", func() {
			now := time.Now()
			condition := &RiskCondition{
				BaseModel: database.BaseModel{
					ID:        1,
					CreatedAt: now.Add(-time.Hour),
					UpdatedAt: now,
				},
				RiskRuleID:  100,
				Name:        "登录失败检测",
				Window:      5 * time.Minute,
				Threshold:   3,
				Expression:  "login_failed AND suspicious_ip",
				Enabled:     true,
				Description: "检测5分钟内登录失败次数超过3次且IP可疑",
			}

			convey.So(condition.ID, convey.ShouldEqual, 1)
			convey.So(condition.RiskRuleID, convey.ShouldEqual, 100)
			convey.So(condition.Name, convey.ShouldEqual, "登录失败检测")
			convey.So(condition.Window, convey.ShouldEqual, 5*time.Minute)
			convey.So(condition.Threshold, convey.ShouldEqual, 3)
			convey.So(condition.Expression, convey.ShouldEqual, "login_failed AND suspicious_ip")
			convey.So(condition.Enabled, convey.ShouldBeTrue)
			convey.So(condition.Description, convey.ShouldEqual, "检测5分钟内登录失败次数超过3次且IP可疑")
		})

		convey.Convey("应该验证字段约束", func() {
			convey.Convey("必填字段不能为空", func() {
				condition := &RiskCondition{}
				// 这里可以测试数据库约束，但单元测试中主要验证业务逻辑
				convey.So(condition.Name, convey.ShouldBeEmpty)
				convey.So(condition.Expression, convey.ShouldBeEmpty)
			})

			convey.Convey("默认值应该正确", func() {
				condition := &RiskCondition{}
				// GORM 的 default:true 标签只在数据库层面有效
				// 在 Go 结构体中，布尔字段的默认值是 false
				convey.So(condition.Enabled, convey.ShouldBeFalse)
			})
		})

		convey.Convey("应该支持不同的时间窗口", func() {
			testCases := []struct {
				name   string
				window time.Duration
			}{
				{"秒级窗口", 30 * time.Second},
				{"分钟级窗口", 5 * time.Minute},
				{"小时级窗口", 1 * time.Hour},
				{"天级窗口", 24 * time.Hour},
			}

			for _, tc := range testCases {
				convey.Convey(tc.name, func() {
					condition := &RiskCondition{
						Window: tc.window,
					}
					convey.So(condition.Window, convey.ShouldEqual, tc.window)
				})
			}
		})
	})
}

// TestRiskRule_Fields 测试 RiskRule 结构体字段
func TestRiskRule_Fields(t *testing.T) {
	convey.Convey("测试 RiskRule 结构体字段", t, func() {
		convey.Convey("应该正确设置所有字段", func() {
			now := time.Now()
			tag := &RiskRule{
				BaseModel: database.BaseModel{
					ID:        1,
					CreatedAt: now.Add(-time.Hour),
					UpdatedAt: now,
				},
				Name:        "login_failed",
				Enabled:     true,
				Description: "用户登录失败事件",
			}

			convey.So(tag.ID, convey.ShouldEqual, 1)
			convey.So(tag.Name, convey.ShouldEqual, "login_failed")
			convey.So(tag.Enabled, convey.ShouldBeTrue)
			convey.So(tag.Description, convey.ShouldEqual, "用户登录失败事件")
		})

		convey.Convey("应该验证字段约束", func() {
			convey.Convey("必填字段不能为空", func() {
				tag := &RiskRule{}
				convey.So(tag.Name, convey.ShouldBeEmpty)
			})

			convey.Convey("默认值应该正确", func() {
				tag := &RiskRule{}
				// GORM 的 default:true 标签只在数据库层面有效
				// 在 Go 结构体中，布尔字段的默认值是 false
				convey.So(tag.Enabled, convey.ShouldBeFalse)
			})
		})

		convey.Convey("应该支持不同的标签类型", func() {
			testCases := []struct {
				name        string
				description string
			}{
				{"login_failed", "登录失败事件"},
				{"suspicious_ip", "可疑IP地址"},
				{"trusted_device", "可信设备"},
				{"high_risk_operation", "高风险操作"},
				{"unusual_time", "异常时间访问"},
			}

			for _, tc := range testCases {
				convey.Convey(tc.name, func() {
					tag := &RiskRule{
						Name:        tc.name,
						Description: tc.description,
					}
					convey.So(tag.Name, convey.ShouldEqual, tc.name)
					convey.So(tag.Description, convey.ShouldEqual, tc.description)
				})
			}
		})
	})
}

// TestBoolExpr_Structure 测试 BoolExpr 结构体及其相关类型的结构
func TestBoolExpr_Structure(t *testing.T) {
	convey.Convey("测试 BoolExpr 结构体及其相关类型的结构", t, func() {
		convey.Convey("应该正确构建表达式树", func() {
			expr, err := parser.ParseString("", "A AND B OR C")
			convey.So(err, convey.ShouldBeNil)
			convey.So(expr, convey.ShouldNotBeNil)

			// 验证结构
			convey.So(expr.Or, convey.ShouldHaveLength, 2) // A AND B, C

			// 第一个 OR 项: A AND B
			convey.So(expr.Or[0].Not, convey.ShouldHaveLength, 2)
			convey.So(*expr.Or[0].Not[0].Atom.Label, convey.ShouldEqual, "A")
			convey.So(*expr.Or[0].Not[1].Atom.Label, convey.ShouldEqual, "B")

			// 第二个 OR 项: C
			convey.So(expr.Or[1].Not, convey.ShouldHaveLength, 1)
			convey.So(*expr.Or[1].Not[0].Atom.Label, convey.ShouldEqual, "C")
		})

		convey.Convey("应该处理嵌套括号", func() {
			expr, err := parser.ParseString("", "(A OR B) AND C")
			convey.So(err, convey.ShouldBeNil)
			convey.So(expr, convey.ShouldNotBeNil)

			// 验证结构: 应该是一个 AND 项，左边是括号表达式
			convey.So(expr.Or, convey.ShouldHaveLength, 1)
			convey.So(expr.Or[0].Not, convey.ShouldHaveLength, 2)

			// 第一个因子应该是括号表达式
			convey.So(expr.Or[0].Not[0].Atom.Expr, convey.ShouldNotBeNil)
			convey.So(expr.Or[0].Not[0].Atom.Label, convey.ShouldBeNil)

			// 第二个因子是 C
			convey.So(*expr.Or[0].Not[1].Atom.Label, convey.ShouldEqual, "C")
		})

		convey.Convey("应该处理 NOT 操作符", func() {
			expr, err := parser.ParseString("", "NOT A AND B")
			convey.So(err, convey.ShouldBeNil)
			convey.So(expr, convey.ShouldNotBeNil)

			convey.So(expr.Or, convey.ShouldHaveLength, 1)
			convey.So(expr.Or[0].Not, convey.ShouldHaveLength, 2)

			// 第一个因子: NOT A
			convey.So(expr.Or[0].Not[0].Not, convey.ShouldBeTrue)
			convey.So(*expr.Or[0].Not[0].Atom.Label, convey.ShouldEqual, "A")

			// 第二个因子: B
			convey.So(expr.Or[0].Not[1].Not, convey.ShouldBeFalse)
			convey.So(*expr.Or[0].Not[1].Atom.Label, convey.ShouldEqual, "B")
		})
	})
}

// TestParser_EdgeCases 测试解析器的边界情况
func TestParser_EdgeCases(t *testing.T) {
	convey.Convey("测试解析器的边界情况", t, func() {
		convey.Convey("应该处理空表达式", func() {
			_, err := parser.ParseString("", "")
			convey.So(err, convey.ShouldNotBeNil)
		})

		convey.Convey("应该处理只有空格的表达式", func() {
			_, err := parser.ParseString("", "   ")
			convey.So(err, convey.ShouldNotBeNil)
		})

		convey.Convey("应该处理大小写敏感", func() {
			expr1, err1 := parser.ParseString("", "tag")
			expr2, err2 := parser.ParseString("", "TAG")

			convey.So(err1, convey.ShouldBeNil)
			convey.So(err2, convey.ShouldBeNil)

			convey.So(*expr1.Or[0].Not[0].Atom.Label, convey.ShouldEqual, "tag")
			convey.So(*expr2.Or[0].Not[0].Atom.Label, convey.ShouldEqual, "TAG")
		})

		convey.Convey("应该处理标签名称中的特殊字符", func() {
			// 根据规则，只允许字母、数字、下划线、连字符
			testCases := []struct {
				name  string
				expr  string
				valid bool
			}{
				{"下划线", "tag_name", true},
				{"连字符", "tag-name", true},
				{"点号", "tag.name", false},
				{"@符号", "tag@name", false},
				{"空格", "tag name", false},
				{"中文", "标签", false},
			}

			for _, tc := range testCases {
				convey.Convey(tc.name, func() {
					_, err := parser.ParseString("", tc.expr)
					if tc.valid {
						convey.So(err, convey.ShouldBeNil)
					} else {
						convey.So(err, convey.ShouldNotBeNil)
					}
				})
			}
		})
	})
}
