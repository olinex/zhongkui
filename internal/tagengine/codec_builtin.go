// 本文件实现 codec.parse 内置函数：把 Rego 参数转交底层 codec 包，
// 按 HTTP 请求 / 响应头的 Content-Type 解析正文字符串为 Rego 值。
// 正文通常经 crypto.reveal 取得（如 HttpTransport 的 reqBody / rspBody），
// Content-Type 取自同一条数据的 reqHdrContentType / rspHdrContentType。

package tagengine

import (
	// 第三方
	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/types"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/codec"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

const (
	// codecParseName 自定义 OPA 内置函数名：codec.parse(contentType, text)。
	// contentType 为 HTTP 头部原值（可含 charset 等参数），text 为待解析文本。
	codecParseName = "codec.parse"
)

// codecParseDecl 是 codec.parse 内置函数的声明：(string, string) → any。
// 返回 any 是因为解析结果可能是对象、数组、字符串等任意 Rego 值；
// Memoize 开启使单次求值内相同 (contentType, text) 只解析一次。
var codecParseDecl = &rego.Function{
	Name:        codecParseName,
	Description: "parse a payload into a rego value according to its content type",
	Decl: types.NewFunction(
		types.Args(types.S, types.S),
		types.A,
	),
	Memoize: true,
}

// codecParse 是 codec.parse 内置函数的实现：校验参数后交由底层 codec.Parse 解析，
// 并把通用 Go 值转换为 Rego 值。Content-Type 为空、类型不支持或文本解析失败均返回错误；
// 生产识别中错误令规则体不满足，dry-run 与样例校验的 StrictBuiltinErrors 会让错误浮出。
func codecParse(_ rego.BuiltinContext, contentTypeTerm, textTerm *ast.Term) (*ast.Term, error) {
	contentType, ok := contentTypeTerm.Value.(ast.String)
	if !ok {
		return nil, errors.Newf("tagengine: %s: first argument must be a string, got %T", codecParseName, contentTypeTerm.Value)
	}
	text, ok := textTerm.Value.(ast.String)
	if !ok {
		return nil, errors.Newf("tagengine: %s: second argument must be a string, got %T", codecParseName, textTerm.Value)
	}
	value, err := codec.Parse(string(contentType), string(text))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	regoValue, err := ast.InterfaceToValue(value)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return ast.NewTerm(regoValue), nil
}

// init 全局注册 codec.parse：同时注册 ast 声明与 topdown 实现，
// 使校验 / dry-run / 样例校验 / 识别全路径可用。
func init() {
	rego.RegisterBuiltin2(codecParseDecl, codecParse)
}
