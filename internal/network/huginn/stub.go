//go:build !huginn

// Package huginn 提供 Huginn Net FFI 的 Go cgo 绑定。
// 本文件在未启用 huginn build tag 时编译，提供与 binding.go 完全一致的
// 导出类型与函数签名作为空实现降级，保证未集成 Rust 侧库时项目仍可正常构建运行。
package huginn

import "zhongkui.com/zhongkui/internal/pkg/errors"

// ErrHuginnUnavailable 未启用 huginn build tag 或 FFI 不可用时的哨兵错误。
var ErrHuginnUnavailable = errors.New("huginn FFI unavailable, build with -tags huginn")

// TCPFingerprint TCP 指纹识别结果（p0f 风格）。
type TCPFingerprint struct {
	OS        string  `json:"os"`
	OSQuality float64 `json:"os_quality"`
	Sig       string  `json:"sig"`
	MTU       uint16  `json:"mtu"`
}

// HTTPFingerprint Http 指纹识别结果。
type HTTPFingerprint struct {
	Browser  string  `json:"browser"`
	Server   string  `json:"server"`
	Language string  `json:"language"`
	Quality  float64 `json:"quality"`
}

// Available 返回 FFI 是否可用。
// 未启用 huginn build tag 时 FFI 不可用，恒返回 false。
func Available() bool {
	return false
}

// AnalyzeTCPIPv4 分析原始 IPv4+TCP 头字节，返回 TCP 指纹。
// FFI 不可用，恒返回 (nil, ErrHuginnUnavailable)。
func AnalyzeTCPIPv4(raw []byte) (*TCPFingerprint, error) {
	return nil, ErrHuginnUnavailable
}

// AnalyzeTCPIPv6 分析原始 IPv6+TCP 头字节，返回 TCP 指纹。
// FFI 不可用，恒返回 (nil, ErrHuginnUnavailable)。
func AnalyzeTCPIPv6(raw []byte) (*TCPFingerprint, error) {
	return nil, ErrHuginnUnavailable
}

// AnalyzeHTTPRequest 分析原始 Http/1.x 请求头字节，返回 Http 指纹。
// FFI 不可用，恒返回 (nil, ErrHuginnUnavailable)。
func AnalyzeHTTPRequest(raw []byte) (*HTTPFingerprint, error) {
	return nil, ErrHuginnUnavailable
}

// AnalyzeHTTPResponse 分析原始 Http/1.x 响应头字节，返回 Http 指纹。
// FFI 不可用，恒返回 (nil, ErrHuginnUnavailable)。
func AnalyzeHTTPResponse(raw []byte) (*HTTPFingerprint, error) {
	return nil, ErrHuginnUnavailable
}
