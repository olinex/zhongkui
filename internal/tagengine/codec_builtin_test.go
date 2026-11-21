package tagengine_test

import (
	// 标准库
	"context"
	"testing"

	// 第三方
	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/rego"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/text/encoding/simplifiedchinese"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/models/tag"
	"zhongkuisec.com/zhongkui/internal/tagengine"
)

// evalCodecRule 编译单条规则并以给定输入文档求值，返回命中的标签集合。
// strict 为 true 时启用 StrictBuiltinErrors，使内置函数错误浮出为评估错误。
func evalCodecRule(t *testing.T, source string, input map[string]any, strict bool) ([]string, error) {
	t.Helper()
	return evalCodecRuleWith(t, source, rego.EvalInput(input), strict)
}

// evalCodecRuleParsed 与 evalCodecRule 相同，但以已解析的 AST 值作为输入，
// 避免 OPA 将输入 JSON 往返序列化（GBK 等非 UTF-8 字节会被替换为 U+FFFD）。
func evalCodecRuleParsed(t *testing.T, source string, input map[string]any, strict bool) ([]string, error) {
	t.Helper()
	return evalCodecRuleWith(t, source, rego.EvalParsedInput(ast.MustInterfaceToValue(input)), strict)
}

// evalCodecRuleWith 编译单条规则并以指定求值选项执行，返回命中的标签集合。
func evalCodecRuleWith(t *testing.T, source string, option rego.EvalOption, strict bool) ([]string, error) {
	t.Helper()
	group, err := tagengine.CompileGroup(
		context.Background(),
		[]*tag.TagRule{{Name: "codec", Rego: source}},
		rego.StrictBuiltinErrors(strict),
	)
	if err != nil {
		return nil, err
	}
	return group.Recognize(context.Background(), option)
}

// TestCodecParseBuiltin 覆盖 codec.parse 的 JSON / 表单 / XML / HTML / 纯文本解析、
// 字符集转换与错误路径。
func TestCodecParseBuiltin(t *testing.T) {
	Convey("codec.parse 自定义内置函数", t, func() {
		Convey("函数元信息与校验链路可见", func() {
			found := false
			for _, fn := range tagengine.CustomFunctions() {
				if fn.Name != "codec.parse" {
					continue
				}
				found = true
				So(fn.Signature, ShouldEqual, "codec.parse(contentType, text)")
				So(fn.Description, ShouldEqual, "tagengine.custom-functions.codec.parse.description")
			}
			So(found, ShouldBeTrue)

			errs, ok := tagengine.ValidateRegoRules(context.Background(), []*tag.TagRule{{
				Name: "json",
				Rego: `package zki

tags contains "ok" if {
	codec.parse(input.contentType, input.body) != null
}`,
			}})
			So(ok, ShouldBeTrue)
			So(errs, ShouldBeEmpty)
		})

		Convey("JSON 对象可访问嵌套字段与数组", func() {
			source := `package zki

tags contains "json-ok" if {
	body := codec.parse(input.contentType, input.body)
	body.user.role == "admin"
	body.tags[1] == "b"
	body.count == 2
}`
			tags, err := evalCodecRule(t, source, map[string]any{
				"contentType": "application/json; charset=utf-8",
				"body":        `{"user":{"role":"admin"},"tags":["a","b"],"count":2}`,
			}, false)
			So(err, ShouldBeNil)
			So(tags, ShouldResemble, []string{"json-ok"})
		})

		Convey("JSON 数组可遍历元素", func() {
			source := `package zki

tags contains "json-array" if {
	body := codec.parse(input.contentType, input.body)
	some item in body
	item.id == 2
}`
			tags, err := evalCodecRule(t, source, map[string]any{
				"contentType": "application/json",
				"body":        `[{"id":1},{"id":2}]`,
			}, false)
			So(err, ShouldBeNil)
			So(tags, ShouldResemble, []string{"json-array"})
		})

		Convey("非法 JSON 在严格模式下报错", func() {
			source := `package zki

tags contains "x" if {
	codec.parse(input.contentType, input.body) != null
}`
			_, err := evalCodecRule(t, source, map[string]any{
				"contentType": "application/json",
				"body":        `{"user":`,
			}, true)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "invalid JSON payload")
		})

		Convey("表单解析单值与同名多值参数", func() {
			source := `package zki

tags contains "form-ok" if {
	body := codec.parse(input.contentType, input.body)
	body.username == "admin"
	count(body.scope) == 2
	body.scope[1] == "write"
}`
			tags, err := evalCodecRule(t, source, map[string]any{
				"contentType": "application/x-www-form-urlencoded",
				"body":        "username=admin&scope=read&scope=write",
			}, false)
			So(err, ShouldBeNil)
			So(tags, ShouldResemble, []string{"form-ok"})
		})

		Convey("XML 解析为根节点树", func() {
			source := `package zki

tags contains "xml-ok" if {
	node := codec.parse(input.contentType, input.body)
	node.name == "root"
	node.attrs.id == "r"
	count(node.children) == 2
	node.children[0].attrs.id == "1"
	node.children[1].text == "second"
}`
			tags, err := evalCodecRule(t, source, map[string]any{
				"contentType": "application/xml",
				"body":        `<root id="r"><item id="1">first</item><item id="2">second</item></root>`,
			}, false)
			So(err, ShouldBeNil)
			So(tags, ShouldResemble, []string{"xml-ok"})
		})

		Convey("HTML 解析为 #document 节点树", func() {
			source := `package zki

tags contains "html-ok" if {
	doc := codec.parse(input.contentType, input.body)
	doc.name == "#document"
	some page in doc.children
	page.name == "html"
	some body in page.children
	body.name == "body"
	some form in body.children
	form.name == "form"
	form.attrs.action == "/login"
	some field in form.children
	field.name == "input"
	field.attrs.name == "user"
}`
			tags, err := evalCodecRule(t, source, map[string]any{
				"contentType": "text/html",
				"body":        `<html><body><form action="/login" method="post"><input name="user"></form></body></html>`,
			}, false)
			So(err, ShouldBeNil)
			So(tags, ShouldResemble, []string{"html-ok"})
		})

		Convey("application/xhtml+xml 走 HTML 解析", func() {
			source := `package zki

tags contains "xhtml-ok" if {
	doc := codec.parse(input.contentType, input.body)
	doc.name == "#document"
}`
			tags, err := evalCodecRule(t, source, map[string]any{
				"contentType": "application/xhtml+xml",
				"body":        `<html><body><p>x</p></body></html>`,
			}, false)
			So(err, ShouldBeNil)
			So(tags, ShouldResemble, []string{"xhtml-ok"})
		})

		Convey("text/plain 原样返回并按 charset 转码", func() {
			source := `package zki

tags contains "text-ok" if {
	codec.parse(input.contentType, input.body) == "中文"
}`
			gbk, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte("中文"))
			So(err, ShouldBeNil)

			tags, err := evalCodecRuleParsed(t, source, map[string]any{
				"contentType": "text/plain; charset=gbk",
				"body":        string(gbk),
			}, false)
			So(err, ShouldBeNil)
			So(tags, ShouldResemble, []string{"text-ok"})
		})

		Convey("XML 按 Content-Type charset 与声明 encoding 转码且不重复解码", func() {
			source := `package zki

tags contains "xml-charset" if {
	node := codec.parse(input.contentType, input.body)
	node.name == "root"
	node.text == "中文"
}`
			gbk, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(`<?xml version="1.0" encoding="gbk"?><root>中文</root>`))
			So(err, ShouldBeNil)

			// Content-Type 与 XML 声明同时指定 gbk：只转码一次
			tags, err := evalCodecRuleParsed(t, source, map[string]any{
				"contentType": "application/xml; charset=gbk",
				"body":        string(gbk),
			}, false)
			So(err, ShouldBeNil)
			So(tags, ShouldResemble, []string{"xml-charset"})

			// 仅 XML 声明指定 gbk：由 CharsetReader 转码
			tags, err = evalCodecRuleParsed(t, source, map[string]any{
				"contentType": "application/xml",
				"body":        string(gbk),
			}, false)
			So(err, ShouldBeNil)
			So(tags, ShouldResemble, []string{"xml-charset"})
		})

		Convey("不支持的 Content-Type 与空 Content-Type 报错", func() {
			source := `package zki

tags contains "x" if {
	codec.parse(input.contentType, input.body) != null
}`
			_, err := evalCodecRule(t, source, map[string]any{
				"contentType": "application/octet-stream",
				"body":        "binary",
			}, true)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "unsupported content type")

			_, err = evalCodecRule(t, source, map[string]any{
				"contentType": "",
				"body":        "text",
			}, true)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "content type must not be empty")
		})
	})
}
