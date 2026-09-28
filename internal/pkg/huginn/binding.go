// Package huginn 提供 Huginn Net 的 Go 绑定。
// 本文件为纯 Go 实现：将原始 IP/TCP 包或 HTTP/1.x 头字节送入观测值提取
// （observe_tcp.go / observe_http.go），在全局 p0f 签名库（database.go）中
// 匹配并渲染为与原 cgo 绑定完全一致的 JSON 契约。
//
// 契约对齐上游 ffi（ffi/src/lib.rs）：
//   - 空输入、无法解析、TCP 无握手包或签名库不可用时返回 (nil, nil)（对应 NULL）；
//   - TCP 命中握手包即返回结果（OS 未匹配时 os 为空串、os_quality 为 0.0，
//     sig 与 mtu 始终填充；mtu 缺省 1500）；
//   - HTTP 解析成功即返回结果（浏览器 / 服务端未匹配时字段为空串、quality 为 0.0）。
package huginn

import "github.com/creasty/defaults"

// 第三方

// TCPFingerprint TCP 指纹识别结果（p0f 风格）。
type TCPFingerprint struct {
	// OS 匹配到的操作系统名称，未命中时为空串
	OS string `json:"os"`
	// OSQuality 操作系统匹配质量分
	OSQuality float64 `json:"os_quality"`
	// Sig 匹配所用观测值签名
	Sig string `json:"sig"`
	// MTU 未从客户端 SYN 提取到 MTU 时的兜底值（对应 FFI DEFAULT_MTU）
	MTU uint16 `json:"mtu" default:"1500"`
}

// HTTPFingerprint Http 指纹识别结果。
type HTTPFingerprint struct {
	// Browser 匹配到的浏览器名称，未命中时为空串
	Browser string `json:"browser"`
	// Server 匹配到的服务端名称，未命中时为空串
	Server string `json:"server"`
	// Language 匹配到的语言标识
	Language string `json:"language"`
	// Quality 匹配质量分
	Quality float64 `json:"quality"`
}

// matchTCP 在全局签名库中匹配 TCP 观测值，返回 OS 猜测串与质量分。
// 签名库不可用或未命中时返回 ("", 0)。
func matchTCP(collection *tcpCollection, obs *tcpObservation) (string, float64) {
	if collection == nil {
		return "", 0
	}
	lbl, _, score := collection.findBestMatch(obs)
	if lbl == nil {
		return "", 0
	}
	return formatGuess(lbl.name, lbl.class, lbl.flavor), score
}

// matchHTTP 在全局签名库中匹配 HTTP 观测值，返回浏览器 / 服务端猜测串与质量分。
// 签名库不可用或未命中时返回 ("", 0)。
func matchHTTP(collection *httpCollection, obs *httpObservation) (string, float64) {
	if collection == nil {
		return "", 0
	}
	lbl, _, score := collection.findBestMatch(obs)
	if lbl == nil {
		return "", 0
	}
	return formatGuess(lbl.name, lbl.class, lbl.flavor), score
}

// analyzeTCP 分析原始 IPv4 / IPv6 数据包并匹配签名库。
// 返回 nil 表示无法识别（空输入、解析失败或非握手包）。
func analyzeTCP(raw []byte, v6 bool) *TCPFingerprint {
	if len(raw) == 0 {
		return nil
	}
	var (
		pkg *tcpObservedPackage
		err error
	)
	if v6 {
		pkg, err = analyzeTCPIPv6(raw)
	} else {
		pkg, err = analyzeTCPIPv4(raw)
	}
	if err != nil {
		return nil
	}

	db, err := globalDatabase()
	if err != nil {
		return nil
	}

	var (
		collection *tcpCollection
		obs        *tcpObservation
	)
	if pkg.tcpRequest != nil {
		collection, obs = db.tcpRequest, pkg.tcpRequest
	} else if pkg.tcpResponse != nil {
		collection, obs = db.tcpResponse, pkg.tcpResponse
	} else {
		return nil
	}

	os, quality := matchTCP(collection, obs)
	fp := &TCPFingerprint{OS: os, OSQuality: quality, Sig: obs.String()}
	// MTU 缺省值由 default 标签声明，未提取到时由 defaults.Set 填充
	if err := defaults.Set(fp); err != nil {
		return nil
	}
	if pkg.mtu != nil {
		fp.MTU = *pkg.mtu
	}
	return fp
}

// AnalyzeTCPIPv4 分析原始 IPv4+TCP 头字节，返回 TCP 指纹。
// raw 需为完整的 IPv4 头与 TCP 头字节序列；无法识别时返回 (nil, nil)。
func AnalyzeTCPIPv4(raw []byte) (*TCPFingerprint, error) {
	return analyzeTCP(raw, false), nil
}

// AnalyzeTCPIPv6 分析原始 IPv6+TCP 头字节，返回 TCP 指纹。
// raw 需为完整的 IPv6 头与 TCP 头字节序列；无法识别时返回 (nil, nil)。
func AnalyzeTCPIPv6(raw []byte) (*TCPFingerprint, error) {
	return analyzeTCP(raw, true), nil
}

// analyzeHTTP 分析原始 HTTP/1.x 头字节并在签名库中匹配。
// 返回 nil 表示无法解析（空输入、数据不完整或签名库不可用）。
// 解析成功时即使未命中签名也返回结果（对应 FFI 行为）。
func analyzeHTTP(raw []byte, response bool) *HTTPFingerprint {
	if len(raw) == 0 {
		return nil
	}
	db, err := globalDatabase()
	if err != nil {
		return nil
	}

	fp := &HTTPFingerprint{}
	if response {
		obs, err := parseHTTPResponse(raw)
		if err != nil {
			return nil
		}
		server, quality := matchHTTP(db.httpResponse, &obs.matching)
		fp.Server = server
		fp.Quality = quality
	} else {
		obs, err := parseHTTPRequest(raw)
		if err != nil {
			return nil
		}
		browser, quality := matchHTTP(db.httpRequest, &obs.matching)
		fp.Browser = browser
		fp.Language = obs.lang
		fp.Quality = quality
	}
	return fp
}

// AnalyzeHTTPRequest 分析原始 Http/1.x 请求头字节，返回 Http 指纹。
// 无法识别时返回 (nil, nil)。
func AnalyzeHTTPRequest(raw []byte) (*HTTPFingerprint, error) {
	return analyzeHTTP(raw, false), nil
}

// AnalyzeHTTPResponse 分析原始 Http/1.x 响应头字节，返回 Http 指纹。
// 无法识别时返回 (nil, nil)。
func AnalyzeHTTPResponse(raw []byte) (*HTTPFingerprint, error) {
	return analyzeHTTP(raw, true), nil
}
