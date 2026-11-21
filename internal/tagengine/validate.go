package tagengine

import (
	// 标准库
	"context"
	"strings"

	// 第三方
	"github.com/open-policy-agent/opa/ast"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/models/tag"
	"zhongkuisec.com/zhongkui/internal/pkg/base"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

const (
	// regoPackageRef 契约包名对应的完整引用，OPA 解析 package zki 后路径为 data.zki
	regoPackageRef = "data." + base.GlobalPrefix
	// regoTagsRuleName 规则契约要求的标签规则名
	regoTagsRuleName = "tags"
	// regoModuleName 校验使用的 Rego 模块文件名
	regoModuleName = "rule.rego"

	// Rego 校验错误消息的 i18n 消息键（接口层按请求语言翻译 RegoError.Message/Hint）。
	// 翻译文案维护在 internal/pkg/translate/locales/ 的 zh-Hans.toml 与 en.toml。
	// regoInvalidKey 规则不符合标签识别契约的消息键
	regoInvalidKey = "tagrule.rego-invalid"
	// lintFieldNotExistKey input.<field> 引用字段不存在的告警消息键（翻译模板含 %s 字段名，由 Args 提供）
	lintFieldNotExistKey = "tagrule.lint-field-not-exist"
	// lintFieldHintKey 字段 lint 告警的提示消息键
	lintFieldHintKey = "tagrule.lint-field-hint"
	// lintSensitiveFieldKey 直接引用敏感字段（不出现在 input 中）的告警消息键，
	// 翻译模板含两个 %s（均填字段名），由 Args 提供
	lintSensitiveFieldKey = "tagrule.lint-sensitive-field"
	// lintSensitiveFieldHintKey 敏感字段引用的提示消息键（引导改用 crypto.reveal）
	lintSensitiveFieldHintKey = "tagrule.lint-sensitive-field-hint"
)

type unwrapper interface{ Unwrap() error }

// ValidateRego 校验 Rego 规则源码是否符合标签识别契约。
//
// 校验内容:
//  1. Rego 语法正确，解析失败返回带行号定位的错误；
//  2. 包名必须为 zki；
//  3. 模块必须产出 tags 规则（contains / 整体赋值 / 部分集赋值均可）；
//  4. 语义合法，例如同一规则名不得同时存在完整赋值与部分定义（CompileModulesWithOpt）。
//
// 注意：OPA v1.19.0 的 ast.ParseModule 默认按 Rego v0 语法解析，
// 而契约要求的 tags contains 属于 Rego v1 语法，因此这里显式指定
// RegoV1 模式解析，以支持 contains 规则形式；语义编译同样需显式 RegoV1。
func ValidateRego(source string) error {
	module, err := ast.ParseModuleWithOpts(regoModuleName, source, ast.ParserOptions{RegoVersion: ast.RegoV1})
	if err != nil {
		return errors.WithStack(err)
	}
	if module == nil || module.Package == nil || module.Package.Path.String() != regoPackageRef {
		return ErrInvalidRego
	}
	hasTags := false
	for _, rule := range module.Rules {
		if rule.Head.Name.String() == regoTagsRuleName {
			hasTags = true
			break
		}
	}
	if !hasTags {
		return ErrInvalidRego
	}
	// 语义编译：捕获解析阶段检测不到的冲突，
	// 如 tags := {...} 与 tags contains ... 同名完整/部分定义并存。
	if _, err := ast.CompileModulesWithOpt(map[string]string{regoModuleName: source}, ast.CompileOpts{
		ParserOptions: ast.ParserOptions{RegoVersion: ast.RegoV1},
	}); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// RegoError 描述 Rego 规则源码中的一处校验错误，带行号/列号定位。
// tagengine 生成的条目 Message/Hint 为 i18n 消息键（如 tagrule.lint-field-not-exist），
// 文案维护在 internal/pkg/translate/locales/ 下，由接口层按请求语言统一翻译；
// OPA 原生消息（如解析/编译错误原文）不是消息键，翻译不命中时原样保留。
type RegoError struct {
	// Line 错误所在行号（1 起），无法定位时为 0
	Line int `json:"line"`
	// Column 错误所在列号（1 起），无法定位时为 0
	Column int `json:"column"`
	// Message 错误信息。tagengine 生成的条目为 i18n 消息键（带 Args 时翻译后渲染），
	// OPA 原生消息为原文；接口层统一按请求语言翻译
	Message string `json:"message"`
	// Type 错误类型：error（默认）或 warning（字段感知 lint 的告警）
	Type string `json:"type,omitempty"`
	// Hint 针对常见 OPA 解析/编译/类型错误的友好提示的 i18n 消息键，可为空；
	// 接口层按请求语言翻译后输出
	Hint string `json:"hint,omitempty"`
	// Args Message 消息模板的格式化参数（如 lint 告警中的字段名），
	// 接口层翻译 Message 后与 Args 一并渲染
	Args []any `json:"-"`
}

// InspectRego 校验 Rego 规则源码是否符合标签识别契约，返回结构化错误列表与是否合法。
// 与 ValidateRego 的校验语义一致，但错误带行号/列号，供 validate / dry-run 接口输出。
func InspectRego(source string) ([]RegoError, bool) {
	module, err := ast.ParseModuleWithOpts(regoModuleName, source, ast.ParserOptions{RegoVersion: ast.RegoV1})
	if err != nil {
		return extractRegoErrors(err), false
	}
	if module == nil || module.Package == nil || module.Package.Path.String() != regoPackageRef {
		return []RegoError{{Message: regoInvalidKey}}, false
	}
	hasTags := false
	for _, rule := range module.Rules {
		if rule.Head.Name.String() == regoTagsRuleName {
			hasTags = true
			break
		}
	}
	if !hasTags {
		return []RegoError{{Message: regoInvalidKey}}, false
	}
	if _, err := ast.CompileModulesWithOpt(map[string]string{regoModuleName: source}, ast.CompileOpts{
		ParserOptions: ast.ParserOptions{RegoVersion: ast.RegoV1},
	}); err != nil {
		return extractRegoErrors(err), false
	}
	return nil, true
}

// extractRegoErrors 将 OPA 解析/编译错误提取为结构化错误列表。
// ast.Errors 是 []*ast.Error 切片类型，errors.As 不支持切片目标，需类型断言；
// 同时沿错误链向上遍历，兼容 errors.WithStack 等包装。
func extractRegoErrors(err error) []RegoError {
	for e := err; e != nil; {
		if errs, ok := e.(ast.Errors); ok && len(errs) > 0 {
			out := make([]RegoError, 0, len(errs))
			for _, item := range errs {
				if item == nil {
					continue
				}
				out = append(out, newRegoError(item))
			}
			if len(out) > 0 {
				return out
			}
		} else if item, ok := e.(*ast.Error); ok && item != nil {
			return []RegoError{newRegoError(item)}
		}
		if u, ok := e.(unwrapper); ok {
			e = u.Unwrap()
			continue
		}
		break
	}
	return []RegoError{{Message: err.Error()}}
}

// newRegoError 将单个 ast.Error 转换为 RegoError，提取行列定位并匹配提示消息键。
func newRegoError(e *ast.Error) RegoError {
	re := RegoError{
		Type:    "error",
		Message: e.Message,
	}
	if key := regoHintFor(e); key != "" {
		re.Hint = key
	}
	if e.Location != nil {
		re.Line = e.Location.Row
		re.Column = e.Location.Col
	}
	return re
}

// regoHintRule 单条提示映射规则：错误码匹配且消息以 prefix 开头时命中。
// prefix 为空串表示该错误码下的兜底规则（匹配任意消息）。
// key 为提示的 i18n 消息键，接口层按请求语言翻译后填充 Hint。
type regoHintRule struct {
	prefix string
	key    string
}

// regoHintRules 常见 OPA 解析/编译/类型错误的提示消息键映射表（按错误码分组）。
// 匹配按顺序取第一条消息前缀命中的规则；未命中任何规则时返回空键。
// 文案维护在 internal/pkg/translate/locales/ 的 tagrule.hint.* 键下。
var regoHintRules = map[string][]regoHintRule{
	ast.ParseErr: {
		{prefix: "unexpected", key: "tagrule.hint.parse-unexpected"},
		{prefix: "rule name conflicts with built-in function", key: "tagrule.hint.parse-builtin-conflict"},
		{prefix: "package", key: "tagrule.hint.parse-package"},
		{prefix: "`if` keyword is required", key: "tagrule.hint.parse-if"},
		{prefix: "`contains` keyword is required", key: "tagrule.hint.parse-contains"},
		{prefix: "empty module", key: "tagrule.hint.parse-empty"},
		{prefix: "illegal", key: "tagrule.hint.parse-illegal"},
		{key: "tagrule.hint.parse-fallback"},
	},
	ast.CompileErr: {
		{prefix: "rules must not shadow", key: "tagrule.hint.compile-shadow"},
		{prefix: "import must not shadow", key: "tagrule.hint.compile-import-shadow"},
		{prefix: "variables must not shadow", key: "tagrule.hint.compile-var-shadow"},
		{key: "tagrule.hint.compile-fallback"},
	},
	ast.TypeErr: {
		{prefix: "undefined function", key: "tagrule.hint.type-undefined-function"},
		{prefix: "undefined ref", key: "tagrule.hint.type-undefined-ref"},
		{prefix: "conflicting rules", key: "tagrule.hint.type-conflicting-rules"},
		{prefix: "match error", key: "tagrule.hint.type-match-error"},
		{prefix: "function", key: "tagrule.hint.type-function"},
		{key: "tagrule.hint.type-fallback"},
	},
	ast.UnsafeVarErr: {
		{key: "tagrule.hint.unsafe-var"},
	},
	ast.RecursionErr: {
		{key: "tagrule.hint.recursion"},
	},
}

// regoHintFor 按错误码与消息前缀为 ast.Error 匹配提示消息键，返回 i18n 消息键。
// 未命中任何规则时返回空串，调用方据此不附加提示。
func regoHintFor(e *ast.Error) string {
	for _, rule := range regoHintRules[e.Code] {
		if rule.prefix == "" || strings.HasPrefix(e.Message, rule.prefix) {
			return rule.key
		}
	}
	return ""
}

// ValidateRegoRules 校验一组候选规则的 Rego 合法性，作为 validate / dry-run /
// 写路径校验统一复用的公共入口：逐条做语法与契约校验（InspectRego），
// 全部通过后再对整组做语义编译校验（ValidateGroup），返回结构化错误列表与是否合法。
// 仅针对规则源码本身，不依赖数据库与验证样例，可对任意候选规则集合（含未落库的
// dry-run 输入）复用；单条校验时传入单元素组即可。
func ValidateRegoRules(ctx context.Context, rules []*tag.TagRule) ([]RegoError, bool) {
	for _, r := range rules {
		if errs, ok := InspectRego(r.Rego); !ok {
			return errs, false
		}
	}
	if err := ValidateGroup(ctx, rules); err != nil {
		return extractRegoErrors(err), false
	}
	return nil, true
}

// ValidateGroup 校验一组规则能否作为一个整体语义编译通过。
// 用于拦截跨规则冲突（如两条规则同时对 tags 整体赋值，或
// 一条规则 tags := 与另一条 tags contains 并存），
// 在规则增删改/启停/回滚前基于"变更后的组"调用。
func ValidateGroup(ctx context.Context, rules []*tag.TagRule) error {
	if _, err := CompileGroup(ctx, rules); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// ExtractRegoErrors 从 OPA 解析/编译错误中提取结构化校验错误列表（带行列号）。
// 供路由层在组级校验（ValidateGroup）失败后输出带行列号的错误详情。
func ExtractRegoErrors(err error) []RegoError {
	return extractRegoErrors(err)
}
