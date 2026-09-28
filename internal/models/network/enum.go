// 本文件把 HTTP 请求方法与协议版本注册为枚举类 httpMethod / httpVersion。两者的取值空间由本文件
// 唯一声明（HTTP 流量检索的 method / httpVersion 字段即取此处常量），类名须与 search.toml 字段绑定的
// 引用一致。本包是对应模型（HttpRequestBody.Method / HttpBase.HttpVersion）的归属包。
package network

import (
	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/enum"
)

// HTTPMethod 是 HTTP 请求方法的取值类型（HttpRequestBody.Method 字段）。
type HTTPMethod string

// HTTP 请求方法常量。
const (
	// HTTPMethodGet GET 方法
	HTTPMethodGet HTTPMethod = "GET"
	// HTTPMethodPost POST 方法
	HTTPMethodPost HTTPMethod = "POST"
	// HTTPMethodPut PUT 方法
	HTTPMethodPut HTTPMethod = "PUT"
	// HTTPMethodDelete DELETE 方法
	HTTPMethodDelete HTTPMethod = "DELETE"
	// HTTPMethodPatch PATCH 方法
	HTTPMethodPatch HTTPMethod = "PATCH"
	// HTTPMethodHead HEAD 方法
	HTTPMethodHead HTTPMethod = "HEAD"
	// HTTPMethodOptions OPTIONS 方法
	HTTPMethodOptions HTTPMethod = "OPTIONS"
	// HTTPMethodTrace TRACE 方法
	HTTPMethodTrace HTTPMethod = "TRACE"
	// HTTPMethodConnect CONNECT 方法
	HTTPMethodConnect HTTPMethod = "CONNECT"
)

// httpMethods 是 HTTP 请求方法的取值空间，声明顺序即枚举类条目顺序，也是前端下拉的展示顺序。
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
type HTTPVersion string

// HTTP 协议版本常量。
const (
	// HTTPVersion10 HTTP/1.0 版本
	HTTPVersion10 HTTPVersion = "HTTP/1.0"
	// HTTPVersion11 HTTP/1.1 版本
	HTTPVersion11 HTTPVersion = "HTTP/1.1"
	// HTTPVersion2 HTTP/2 版本
	HTTPVersion2 HTTPVersion = "HTTP/2"
	// HTTPVersion3 HTTP/3 版本
	HTTPVersion3 HTTPVersion = "HTTP/3"
)

// httpVersions 是 HTTP 协议版本的取值空间，声明顺序即枚举类条目顺序，也是前端下拉的展示顺序。
var httpVersions = []HTTPVersion{HTTPVersion10, HTTPVersion11, HTTPVersion2, HTTPVersion3}

// init 注册枚举类 httpMethod 与 httpVersion（取值空间固定，条目在注册时快照）。
func init() {
	enum.Register(enum.Type{Name: "httpMethod", Entries: enum.EntriesOf(httpMethods)})
	enum.Register(enum.Type{Name: "httpVersion", Entries: enum.EntriesOf(httpVersions)})
}
