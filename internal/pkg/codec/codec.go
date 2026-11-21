// Package codec 依据 MIME 媒体类型解析 HTTP 请求 / 响应正文，把文本转换为通用 Go 值：
// JSON 返回对象 / 数组等原生值，application/x-www-form-urlencoded 返回键值对象，
// XML / HTML 返回通用节点树，text/plain 原样返回字符串。
//
// 供 Rego 内置函数 codec.parse 等按 Content-Type 解析文本的场景复用；
// 解析结果统一为可交给 ast.InterfaceToValue 的 map / slice / 标量结构。
package codec

import (
	// 标准库
	"encoding/json"
	"encoding/xml"
	"io"
	"mime"
	"net/url"
	"strings"

	// 第三方
	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/base"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

const (
	// maxDepth 解析 XML / HTML 时节点树的最大嵌套深度，
	// 防止恶意深嵌套文档耗尽栈或内存。
	maxDepth = 256
	// maxNodes 解析 XML / HTML 时节点树的最大节点数，
	// 防止超大文档生成巨大数据结构。
	maxNodes = 100000
	// jsonSuffix 结构化语法后缀：JSON（如 application/vnd.api+json）。
	jsonSuffix = "+json"
	// xmlSuffix 结构化语法后缀：XML（如 application/atom+xml）。
	xmlSuffix = "+xml"
)

// node 是 XML / HTML 解析出的节点，转换为通用 Go 值时含
// name（标签名）、attrs（属性对象）、text（直接文本）、children（子节点数组）。
type node struct {
	// name 标签名；HTML 文档根为 #document
	name string
	// attrs 属性名 → 属性值
	attrs map[string]any
	// text 节点的直接文本内容（不含子元素文本）
	text strings.Builder
	// children 子元素节点
	children []*node
}

// toGo 将节点转换为通用的 map 结构：children 始终为数组、attrs 始终为对象，
// 便于调用方统一按四键访问。
func (n *node) toGo() map[string]any {
	children := make([]any, len(n.children))
	for i, child := range n.children {
		children[i] = child.toGo()
	}
	return map[string]any{
		"name":     n.name,
		"attrs":    n.attrs,
		"text":     strings.TrimSpace(n.text.String()),
		"children": children,
	}
}

// Parse 依据 Content-Type 解析文本，返回可交给 ast.InterfaceToValue 的通用 Go 值。
// 支持 JSON（application/json、text/json、+json 后缀）、application/x-www-form-urlencoded、
// XML（application/xml、text/xml、+xml 后缀）、HTML（text/html、application/xhtml+xml）
// 与 text/plain（原样返回）。Content-Type 为空、媒体类型不支持或文本解析失败均返回错误。
func Parse(contentType, text string) (any, error) {
	trimmed := strings.TrimSpace(contentType)
	if trimmed == "" {
		return nil, errors.New("codec: content type must not be empty")
	}
	mediaType, params, err := mime.ParseMediaType(trimmed)
	if err != nil {
		return nil, errors.Newf("codec: invalid content type %q: %v", contentType, err)
	}
	switch {
	case isJSONMediaType(mediaType):
		decoded, err := decodeCharset(text, params)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return parseJSON(decoded)
	case mediaType == base.MediaTypeForm:
		decoded, err := decodeCharset(text, params)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return parseForm(decoded)
	case mediaType == base.MediaTypeHTML || mediaType == base.MediaTypeXHTML:
		return parseHTML(text, trimmed)
	case mediaType == base.MediaTypeXML || mediaType == base.MediaTypeTextXML || strings.HasSuffix(mediaType, xmlSuffix):
		label := strings.TrimSpace(params["charset"])
		decoded, err := decodeCharset(text, params)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return parseXML(decoded, label)
	case mediaType == base.MediaTypeTextPlain:
		decoded, err := decodeCharset(text, params)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return decoded, nil
	default:
		return nil, errors.Newf("codec: unsupported content type %q", mediaType)
	}
}

// isJSONMediaType 判断媒体类型是否属于 JSON 家族（application/json、text/json 或 +json 后缀）。
func isJSONMediaType(mediaType string) bool {
	return mediaType == base.MediaTypeJSON || mediaType == base.MediaTypeTextJSON || strings.HasSuffix(mediaType, jsonSuffix)
}

// isUTF8Label 判断字符集标签是否表示 UTF-8（大小写不敏感比较）。
func isUTF8Label(label string) bool {
	return strings.EqualFold(label, "utf-8") || strings.EqualFold(label, "utf8")
}

// decodeCharset 在 Content-Type 显式声明非 UTF-8 字符集时把文本转为 UTF-8。
// 未声明字符集或已是 UTF-8 时原样返回，避免无谓拷贝。
func decodeCharset(text string, params map[string]string) (string, error) {
	label := strings.TrimSpace(params["charset"])
	if label == "" || isUTF8Label(label) {
		return text, nil
	}
	reader, err := charset.NewReaderLabel(label, strings.NewReader(text))
	if err != nil {
		return "", errors.Newf("codec: unsupported charset %q: %v", label, err)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return string(data), nil
}

// parseJSON 解析 JSON 文本，使用 UseNumber 保留整数精度，
// 并在首个值之后要求 EOF，拒绝尾随的多余内容。
func parseJSON(text string) (any, error) {
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, errors.Newf("codec: invalid JSON payload: %v", err)
	}
	var trailing any
	switch err := decoder.Decode(&trailing); {
	case err == io.EOF:
		return value, nil
	case err == nil:
		return nil, errors.New("codec: invalid JSON payload: trailing data")
	default:
		return nil, errors.Newf("codec: invalid JSON payload: %v", err)
	}
}

// parseForm 解析 application/x-www-form-urlencoded 文本：
// 单值参数映射为字符串，同名多值参数映射为字符串数组。
func parseForm(text string) (any, error) {
	values, err := url.ParseQuery(text)
	if err != nil {
		return nil, errors.Newf("codec: invalid form payload: %v", err)
	}
	obj := make(map[string]any, len(values))
	for key, items := range values {
		if len(items) == 1 {
			obj[key] = items[0]
			continue
		}
		arr := make([]any, len(items))
		for i, item := range items {
			arr[i] = item
		}
		obj[key] = arr
	}
	return obj, nil
}

// parseXML 流式解析 XML 文本并构建节点树（无递归），返回根元素节点。
// appliedCharset 为 Content-Type 中已用于预转码的字符集标签：
// 若 XML 声明中的 encoding 与之相同，则不再重复转码；其余声明交由 CharsetReader 处理。
// 注释、指令与处理指令不进入节点树。
func parseXML(text, appliedCharset string) (any, error) {
	decoder := xml.NewDecoder(strings.NewReader(text))
	decoder.CharsetReader = func(label string, input io.Reader) (io.Reader, error) {
		if appliedCharset != "" && strings.EqualFold(strings.TrimSpace(label), appliedCharset) {
			return input, nil
		}
		return charset.NewReaderLabel(label, input)
	}
	var root *node
	var stack []*node
	nodes := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.Newf("codec: invalid XML payload: %v", err)
		}
		switch item := token.(type) {
		case xml.StartElement:
			nodes++
			if nodes > maxNodes {
				return nil, errors.Newf("codec: XML payload exceeds the limit of %d elements", maxNodes)
			}
			if len(stack) >= maxDepth {
				return nil, errors.Newf("codec: XML payload nesting exceeds the limit of %d", maxDepth)
			}
			current := &node{name: item.Name.Local, attrs: make(map[string]any, len(item.Attr))}
			for _, attr := range item.Attr {
				current.attrs[attr.Name.Local] = attr.Value
			}
			if len(stack) == 0 {
				if root != nil {
					return nil, errors.New("codec: XML payload has multiple root elements")
				}
				root = current
			} else {
				parent := stack[len(stack)-1]
				parent.children = append(parent.children, current)
			}
			stack = append(stack, current)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].text.Write(item)
			}
		}
	}
	if root == nil {
		return nil, errors.New("codec: XML payload has no root element")
	}
	return root.toGo(), nil
}

// parseHTML 解析 HTML 文本并构建节点树，返回 #document 虚拟根节点。
// 使用 charset.NewReader 同时覆盖显式 charset、BOM 与 <meta charset> 嗅探。
func parseHTML(text, contentType string) (any, error) {
	source, err := charset.NewReader(strings.NewReader(text), contentType)
	if err != nil {
		return nil, errors.Newf("codec: unsupported charset in content type %q: %v", contentType, err)
	}
	document, err := html.Parse(source)
	if err != nil {
		return nil, errors.Newf("codec: invalid HTML payload: %v", err)
	}
	nodes := 0
	root, err := htmlNodeToCodec(document, 0, &nodes)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return root.toGo(), nil
}

// htmlNodeToCodec 递归转换 html.Node 为 node：
// 文本节点并入父节点 text，元素节点递归进入 children，注释与 Doctype 忽略。
// 递归过程校验嵌套深度与节点总数，超限返回错误。
func htmlNodeToCodec(source *html.Node, depth int, nodes *int) (*node, error) {
	if depth > maxDepth {
		return nil, errors.Newf("codec: HTML payload nesting exceeds the limit of %d", maxDepth)
	}
	(*nodes)++
	if *nodes > maxNodes {
		return nil, errors.Newf("codec: HTML payload exceeds the limit of %d elements", maxNodes)
	}
	current := &node{name: htmlNodeName(source), attrs: make(map[string]any, len(source.Attr))}
	for _, attr := range source.Attr {
		current.attrs[attr.Key] = attr.Val
	}
	for child := source.FirstChild; child != nil; child = child.NextSibling {
		switch child.Type {
		case html.TextNode:
			current.text.WriteString(child.Data)
		case html.ElementNode:
			converted, err := htmlNodeToCodec(child, depth+1, nodes)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			current.children = append(current.children, converted)
		}
	}
	return current, nil
}

// htmlNodeName 返回节点在节点树中的名称：文档节点为 #document，元素节点为标签名。
func htmlNodeName(source *html.Node) string {
	if source.Type == html.DocumentNode {
		return "#document"
	}
	return source.Data
}
