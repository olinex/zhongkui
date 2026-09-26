// 本文件把 HTTP 请求方法与协议版本注册为枚举类 httpMethod / httpVersion。两者的取值空间由本文件
// 唯一声明（HTTP 流量检索的 method / httpVersion 字段即取此处常量），类名须与 search.toml 字段绑定的
// 引用一致。本包是对应模型（HttpRequestBody.Method / HttpBase.HttpVersion）的归属包。
//
// This file registers the HTTP request methods and protocol versions as the enum classes httpMethod /
// httpVersion. Their value spaces are declared solely here (the method / httpVersion fields of the HTTP
// traffic search take these very constants), and the class names must match the references in the
// search.toml field bindings. This package owns the corresponding models (HttpRequestBody.Method /
// HttpBase.HttpVersion).
package network

import (
	// internal
	"zhongkui.com/zhongkui/internal/pkg/enum"
)

// HTTPMethod 是 HTTP 请求方法的取值类型（HttpRequestBody.Method 字段）。
// HTTPMethod is the value type of an HTTP request method (the HttpRequestBody.Method field).
type HTTPMethod string

// HTTP 请求方法常量。
// HTTP request method constants.
const (
	// HTTPMethodGet GET 方法
	// HTTPMethodGet the GET method
	HTTPMethodGet HTTPMethod = "GET"
	// HTTPMethodPost POST 方法
	// HTTPMethodPost the POST method
	HTTPMethodPost HTTPMethod = "POST"
	// HTTPMethodPut PUT 方法
	// HTTPMethodPut the PUT method
	HTTPMethodPut HTTPMethod = "PUT"
	// HTTPMethodDelete DELETE 方法
	// HTTPMethodDelete the DELETE method
	HTTPMethodDelete HTTPMethod = "DELETE"
	// HTTPMethodPatch PATCH 方法
	// HTTPMethodPatch the PATCH method
	HTTPMethodPatch HTTPMethod = "PATCH"
	// HTTPMethodHead HEAD 方法
	// HTTPMethodHead the HEAD method
	HTTPMethodHead HTTPMethod = "HEAD"
	// HTTPMethodOptions OPTIONS 方法
	// HTTPMethodOptions the OPTIONS method
	HTTPMethodOptions HTTPMethod = "OPTIONS"
	// HTTPMethodTrace TRACE 方法
	// HTTPMethodTrace the TRACE method
	HTTPMethodTrace HTTPMethod = "TRACE"
	// HTTPMethodConnect CONNECT 方法
	// HTTPMethodConnect the CONNECT method
	HTTPMethodConnect HTTPMethod = "CONNECT"
)

// httpMethods 是 HTTP 请求方法的取值空间，声明顺序即枚举类条目顺序，也是前端下拉的展示顺序。
// httpMethods is the value space of the HTTP request methods; the declaration order is the entry order
// of the enum class and therefore the display order of the frontend select.
var httpMethods = []HTTPMethod{
	HTTPMethodGet,
	HTTPMethodPost,
	HTTPMethodPut,
	HTTPMethodDelete,
	HTTPMethodPatch,
	HTTPMethodHead,
	HTTPMethodOptions,
	HTTPMethodTrace,
	HTTPMethodConnect,
}

// HTTPVersion 是 HTTP 协议版本的取值类型（HttpBase.HttpVersion 字段）。
// HTTPVersion is the value type of an HTTP protocol version (the HttpBase.HttpVersion field).
type HTTPVersion string

// HTTP 协议版本常量。
// HTTP protocol version constants.
const (
	// HTTPVersion10 HTTP/1.0 版本
	// HTTPVersion10 the HTTP/1.0 version
	HTTPVersion10 HTTPVersion = "HTTP/1.0"
	// HTTPVersion11 HTTP/1.1 版本
	// HTTPVersion11 the HTTP/1.1 version
	HTTPVersion11 HTTPVersion = "HTTP/1.1"
	// HTTPVersion2 HTTP/2 版本
	// HTTPVersion2 the HTTP/2 version
	HTTPVersion2 HTTPVersion = "HTTP/2"
	// HTTPVersion3 HTTP/3 版本
	// HTTPVersion3 the HTTP/3 version
	HTTPVersion3 HTTPVersion = "HTTP/3"
)

// httpVersions 是 HTTP 协议版本的取值空间，声明顺序即枚举类条目顺序，也是前端下拉的展示顺序。
// httpVersions is the value space of the HTTP protocol versions; the declaration order is the entry
// order of the enum class and therefore the display order of the frontend select.
var httpVersions = []HTTPVersion{HTTPVersion10, HTTPVersion11, HTTPVersion2, HTTPVersion3}

// init 注册枚举类 httpMethod 与 httpVersion（取值空间固定，条目在注册时快照）。
// init registers the enum classes httpMethod and httpVersion (fixed value spaces, their entries
// snapshotted at registration time).
func init() {
	enum.Register(enum.Type{Name: "httpMethod", Entries: enum.EntriesOf(httpMethods)})
	enum.Register(enum.Type{Name: "httpVersion", Entries: enum.EntriesOf(httpVersions)})
}
