// 本文件覆盖 internal/pkg/codec 的 Content-Type 解析能力：JSON、表单、XML、HTML、
// 纯文本与字符集转换，以及媒体类型不支持、结构非法与规模超限的错误路径。

package codec

import (
	// 标准库
	"encoding/json"
	"strings"
	"testing"

	// 第三方
	"golang.org/x/text/encoding/simplifiedchinese"
)

// TestParseJSON 覆盖 JSON 解析：嵌套字段、数组、+json 后缀、整数精度、
// 尾随数据、非法文本与 charset 转码。
func TestParseJSON(t *testing.T) {
	t.Run("嵌套对象与整数精度", func(t *testing.T) {
		got := parseOK(t, "application/json; charset=utf-8",
			`{"user":{"role":"admin"},"tags":["a","b"],"count":2}`)
		obj := asMap(t, got)
		user := asMap(t, obj["user"])
		if user["role"] != "admin" {
			t.Fatalf("user.role = %#v, want admin", user["role"])
		}
		tags := asSlice(t, obj["tags"])
		if len(tags) != 2 || tags[1] != "b" {
			t.Fatalf("tags = %#v, want [a b]", tags)
		}
		count, ok := obj["count"].(json.Number)
		if !ok || count.String() != "2" {
			t.Fatalf("count = %#v (%T), want json.Number 2", obj["count"], obj["count"])
		}
	})

	t.Run("数组", func(t *testing.T) {
		got := parseOK(t, "application/json", `[{"id":1},{"id":2}]`)
		items := asSlice(t, got)
		if len(items) != 2 {
			t.Fatalf("len(items) = %d, want 2", len(items))
		}
		if asMap(t, items[1])["id"].(json.Number).String() != "2" {
			t.Fatalf("items[1].id = %#v, want 2", asMap(t, items[1])["id"])
		}
	})

	t.Run("+json 后缀", func(t *testing.T) {
		got := parseOK(t, "application/vnd.api+json", `{"data":null}`)
		if _, ok := asMap(t, got)["data"]; !ok {
			t.Fatalf("data key missing in %#v", got)
		}
	})

	t.Run("charset 转码", func(t *testing.T) {
		raw, err := simplifiedchinese.GBK.NewEncoder().String(`{"name":"中文"}`)
		if err != nil {
			t.Fatalf("encode gbk: %v", err)
		}
		got := parseOK(t, "application/json; charset=gbk", raw)
		if asMap(t, got)["name"] != "中文" {
			t.Fatalf("name = %#v, want 中文", asMap(t, got)["name"])
		}
	})

	t.Run("尾随数据报错", func(t *testing.T) {
		requireParseError(t, "application/json", `{"a":1}{"b":2}`, "trailing data")
	})

	t.Run("非法文本报错", func(t *testing.T) {
		requireParseError(t, "application/json", `{"a":`, "invalid JSON payload")
	})

	t.Run("尾部非法内容报错", func(t *testing.T) {
		requireParseError(t, "application/json", `{"a":1}}`, "invalid JSON payload")
	})
}

// TestParseForm 覆盖表单解析：单值、同名多值、空值与非法编码。
func TestParseForm(t *testing.T) {
	t.Run("单值与同名多值", func(t *testing.T) {
		got := parseOK(t, "application/x-www-form-urlencoded",
			"username=admin&scope=read&scope=write&empty=")
		obj := asMap(t, got)
		if obj["username"] != "admin" {
			t.Fatalf("username = %#v, want admin", obj["username"])
		}
		scope := asSlice(t, obj["scope"])
		if len(scope) != 2 || scope[0] != "read" || scope[1] != "write" {
			t.Fatalf("scope = %#v, want [read write]", scope)
		}
		if obj["empty"] != "" {
			t.Fatalf("empty = %#v, want empty string", obj["empty"])
		}
	})

	t.Run("非法编码报错", func(t *testing.T) {
		requireParseError(t, "application/x-www-form-urlencoded", "%zz", "invalid form payload")
	})
}

// TestParseXML 覆盖 XML 解析：节点树、属性、命名空间、后缀类型、charset 转码，
// 以及多根、空文档、非法文本与规模限制。
func TestParseXML(t *testing.T) {
	t.Run("节点树与属性", func(t *testing.T) {
		got := parseOK(t, "application/xml",
			`<root id="r"><item id="1">first</item><item id="2">second</item></root>`)
		root := asMap(t, got)
		if root["name"] != "root" {
			t.Fatalf("root.name = %#v, want root", root["name"])
		}
		if asMap(t, root["attrs"])["id"] != "r" {
			t.Fatalf("root.attrs.id = %#v, want r", asMap(t, root["attrs"])["id"])
		}
		children := asSlice(t, root["children"])
		if len(children) != 2 {
			t.Fatalf("len(children) = %d, want 2", len(children))
		}
		second := asMap(t, children[1])
		if second["text"] != "second" || asMap(t, second["attrs"])["id"] != "2" {
			t.Fatalf("children[1] = %#v, want item id=2 text=second", second)
		}
	})

	t.Run("命名空间取本地名", func(t *testing.T) {
		got := parseOK(t, "application/xml", `<r xmlns:x="urn:test" x:a="1"/>`)
		root := asMap(t, got)
		if root["name"] != "r" {
			t.Fatalf("root.name = %#v, want r", root["name"])
		}
		if asMap(t, root["attrs"])["a"] != "1" {
			t.Fatalf("attrs = %#v, want a=1", root["attrs"])
		}
	})

	t.Run("+xml 后缀", func(t *testing.T) {
		got := parseOK(t, "application/atom+xml", `<feed><title>x</title></feed>`)
		if asMap(t, got)["name"] != "feed" {
			t.Fatalf("root.name = %#v, want feed", asMap(t, got)["name"])
		}
	})

	t.Run("XML 声明 encoding 转码", func(t *testing.T) {
		raw, err := simplifiedchinese.GBK.NewEncoder().String(
			`<?xml version="1.0" encoding="gbk"?><root>中文</root>`)
		if err != nil {
			t.Fatalf("encode gbk: %v", err)
		}
		got := parseOK(t, "application/xml", raw)
		if asMap(t, got)["text"] != "中文" {
			t.Fatalf("text = %#v, want 中文", asMap(t, got)["text"])
		}
	})

	t.Run("Content-Type charset 与声明 encoding 不重复解码", func(t *testing.T) {
		raw, err := simplifiedchinese.GBK.NewEncoder().String(
			`<?xml version="1.0" encoding="gbk"?><root>中文</root>`)
		if err != nil {
			t.Fatalf("encode gbk: %v", err)
		}
		got := parseOK(t, "application/xml; charset=gbk", raw)
		if asMap(t, got)["text"] != "中文" {
			t.Fatalf("text = %#v, want 中文", asMap(t, got)["text"])
		}
	})

	t.Run("多根元素报错", func(t *testing.T) {
		requireParseError(t, "application/xml", `<a/><b/>`, "multiple root elements")
	})

	t.Run("空文档报错", func(t *testing.T) {
		requireParseError(t, "application/xml", "", "no root element")
	})

	t.Run("非法文本报错", func(t *testing.T) {
		requireParseError(t, "application/xml", `<a>`, "invalid XML payload")
	})

	t.Run("嵌套深度超限报错", func(t *testing.T) {
		text := strings.Repeat("<a>", maxDepth+1) + strings.Repeat("</a>", maxDepth+1)
		requireParseError(t, "application/xml", text, "nesting exceeds the limit")
	})

	t.Run("节点数超限报错", func(t *testing.T) {
		text := "<r>" + strings.Repeat("<a/>", maxNodes) + "</r>"
		requireParseError(t, "application/xml", text, "exceeds the limit")
	})
}

// TestParseHTML 覆盖 HTML 解析：#document 根、节点树查询、文本聚合、注释忽略、
// XHTML 分支、meta charset 嗅探与嵌套深度限制。
func TestParseHTML(t *testing.T) {
	t.Run("文档树与表单属性", func(t *testing.T) {
		got := parseOK(t, "text/html",
			`<html><body><form action="/login" method="post"><input name="user"></form></body></html>`)
		root := asMap(t, got)
		if root["name"] != "#document" {
			t.Fatalf("root.name = %#v, want #document", root["name"])
		}
		form := requireNode(t, root, "form")
		if asMap(t, form["attrs"])["action"] != "/login" {
			t.Fatalf("form.attrs = %#v, want action=/login", form["attrs"])
		}
		input := requireNode(t, form, "input")
		if asMap(t, input["attrs"])["name"] != "user" {
			t.Fatalf("input.attrs = %#v, want name=user", input["attrs"])
		}
	})

	t.Run("文本聚合与注释忽略", func(t *testing.T) {
		got := parseOK(t, "text/html", `<div>a<!--c-->b<span>c</span></div>`)
		div := requireNode(t, asMap(t, got), "div")
		if div["text"] != "ab" {
			t.Fatalf("div.text = %#v, want ab", div["text"])
		}
		if len(asSlice(t, div["children"])) != 1 {
			t.Fatalf("div.children = %#v, want one span", div["children"])
		}
	})

	t.Run("XHTML 走 HTML 解析", func(t *testing.T) {
		got := parseOK(t, "application/xhtml+xml", `<html><body><p>x</p></body></html>`)
		if asMap(t, got)["name"] != "#document" {
			t.Fatalf("root.name = %#v, want #document", asMap(t, got)["name"])
		}
	})

	t.Run("meta charset 嗅探", func(t *testing.T) {
		raw, err := simplifiedchinese.GBK.NewEncoder().String(
			`<html><head><meta charset="gbk"></head><body><p>中文</p></body></html>`)
		if err != nil {
			t.Fatalf("encode gbk: %v", err)
		}
		got := parseOK(t, "text/html", raw)
		p := requireNode(t, asMap(t, got), "p")
		if p["text"] != "中文" {
			t.Fatalf("p.text = %#v, want 中文", p["text"])
		}
	})

	t.Run("嵌套深度超限报错", func(t *testing.T) {
		open := strings.Repeat("<div>", maxDepth+2)
		closeTags := strings.Repeat("</div>", maxDepth+2)
		requireParseError(t, "text/html", open+closeTags, "nesting exceeds the limit")
	})
}

// TestParsePlainAndErrors 覆盖纯文本透传、charset 转码与 Content-Type 错误路径。
func TestParsePlainAndErrors(t *testing.T) {
	t.Run("纯文本原样返回", func(t *testing.T) {
		got := parseOK(t, "text/plain", "hello 世界")
		if got != "hello 世界" {
			t.Fatalf("got = %#v, want hello 世界", got)
		}
	})

	t.Run("纯文本 charset 转码", func(t *testing.T) {
		raw, err := simplifiedchinese.GBK.NewEncoder().String("中文")
		if err != nil {
			t.Fatalf("encode gbk: %v", err)
		}
		got := parseOK(t, "text/plain; charset=GBK", raw)
		if got != "中文" {
			t.Fatalf("got = %#v, want 中文", got)
		}
	})

	t.Run("空 Content-Type 报错", func(t *testing.T) {
		requireParseError(t, "  ", "x", "content type must not be empty")
	})

	t.Run("非法 Content-Type 报错", func(t *testing.T) {
		requireParseError(t, "application/json; charset", "x", "invalid content type")
	})

	t.Run("不支持的字符集报错", func(t *testing.T) {
		requireParseError(t, "text/plain; charset=no-such-charset", "x", "unsupported charset")
	})

	t.Run("不支持的媒体类型报错", func(t *testing.T) {
		requireParseError(t, "application/octet-stream", "x", "unsupported content type")
		requireParseError(t, "multipart/form-data; boundary=x", "x", "unsupported content type")
	})
}

// parseOK 调用 Parse 并断言成功，返回解析结果。
func parseOK(t *testing.T, contentType, text string) any {
	t.Helper()
	got, err := Parse(contentType, text)
	if err != nil {
		t.Fatalf("Parse(%q, ...) error = %v", contentType, err)
	}
	return got
}

// requireParseError 调用 Parse 并断言返回包含指定子串的错误。
func requireParseError(t *testing.T, contentType, text, wantSubstring string) {
	t.Helper()
	_, err := Parse(contentType, text)
	if err == nil {
		t.Fatalf("Parse(%q, ...) error = nil, want substring %q", contentType, wantSubstring)
	}
	if !strings.Contains(err.Error(), wantSubstring) {
		t.Fatalf("Parse(%q, ...) error = %q, want substring %q", contentType, err.Error(), wantSubstring)
	}
}

// asMap 断言值为 map[string]any 并返回。
func asMap(t *testing.T, value any) map[string]any {
	t.Helper()
	if value == nil {
		t.Fatal("value = nil, want map[string]any")
	}
	obj, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("value = %#v (%T), want map[string]any", value, value)
	}
	return obj
}

// asSlice 断言值为 []any 并返回。
func asSlice(t *testing.T, value any) []any {
	t.Helper()
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("value = %#v (%T), want []any", value, value)
	}
	return items
}

// requireNode 在节点树中递归查找指定名称的节点，未命中则失败。
func requireNode(t *testing.T, root map[string]any, name string) map[string]any {
	t.Helper()
	if root["name"] == name {
		return root
	}
	for _, child := range asSlice(t, root["children"]) {
		if hit, ok := findNode(asMap(t, child), name); ok {
			return hit
		}
	}
	t.Fatalf("node %q not found under %#v", name, root)
	return nil
}

// findNode 递归查找指定名称的节点。
func findNode(current map[string]any, name string) (map[string]any, bool) {
	if current["name"] == name {
		return current, true
	}
	for _, child := range current["children"].([]any) {
		if hit, ok := findNode(child.(map[string]any), name); ok {
			return hit, true
		}
	}
	return nil, false
}
