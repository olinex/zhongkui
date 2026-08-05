//go:build huginn

// Package huginn 提供 Huginn Net FFI 的 Go cgo 绑定。
// 本文件在启用 huginn build tag 时编译，链接 Rust 侧 libhuginn_ffi 库，
// 完成 TCP 指纹（p0f 风格）与 Http 指纹识别；未启用该 tag 时由 stub.go 降级。
//
// 内存安全约定：
//  1. Rust 侧分析函数均为同步调用，cgo 会在调用期间固定 Go 堆内存，
//     raw 切片在调用全程保持有效；Rust 侧不会在函数返回后保留该指针。
//  2. Rust 侧返回的 C 字符串由 Rust 分配，本包先通过 C.GoString 拷贝到
//     Go 堆，再调用 huginn_free 释放，避免悬垂指针与内存泄漏。
package huginn

/*
#cgo LDFLAGS: -L${SRCDIR}/../../../third_party/huginn-net/target/release -lhuginn_ffi
#include <stdlib.h>
#include <stdint.h>

extern char* huginn_tcp_analyze_ipv4(const uint8_t* raw, size_t len);
extern char* huginn_tcp_analyze_ipv6(const uint8_t* raw, size_t len);
extern char* huginn_http_analyze_request(const uint8_t* raw, size_t len);
extern char* huginn_http_analyze_response(const uint8_t* raw, size_t len);
extern void huginn_free(char* ptr);
extern char* huginn_version(void);
*/
import "C"

import (
	"encoding/json"
	"unsafe"

	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// ErrHuginnUnavailable 未启用 huginn build tag 或 FFI 不可用时的哨兵错误。
// 本文件中 FFI 已可用，该哨兵由 stub.go 在降级路径下返回。
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
// 本文件中 cgo 绑定已编译并链接 libhuginn_ffi，恒返回 true。
func Available() bool {
	return true
}

// parseResult 解析 Rust 侧返回的 JSON 字符串为指纹结果。
// ptr 为 Rust 分配的 C 字符串，解析完成后通过 huginn_free 释放；
// ptr 为 nil 表示无可识别结果，返回 (nil, nil)。
func parseResult[T any](ptr *C.char) (*T, error) {
	if ptr == nil {
		return nil, nil
	}
	defer C.huginn_free(ptr)

	var result T
	if err := json.Unmarshal([]byte(C.GoString(ptr)), &result); err != nil {
		return nil, errors.WithStack(err)
	}
	return &result, nil
}

// AnalyzeTCPIPv4 分析原始 IPv4+TCP 头字节，返回 TCP 指纹。
// raw 需为完整的 IPv4 头与 TCP 头字节序列；无法识别时返回 (nil, nil)。
//
//go:nocheckptr
func AnalyzeTCPIPv4(raw []byte) (*TCPFingerprint, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	return parseResult[TCPFingerprint](C.huginn_tcp_analyze_ipv4((*C.uint8_t)(unsafe.Pointer(&raw[0])), C.size_t(len(raw))))
}

// AnalyzeTCPIPv6 分析原始 IPv6+TCP 头字节，返回 TCP 指纹。
// raw 需为完整的 IPv6 头与 TCP 头字节序列；无法识别时返回 (nil, nil)。
//
//go:nocheckptr
func AnalyzeTCPIPv6(raw []byte) (*TCPFingerprint, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	return parseResult[TCPFingerprint](C.huginn_tcp_analyze_ipv6((*C.uint8_t)(unsafe.Pointer(&raw[0])), C.size_t(len(raw))))
}

// AnalyzeHTTPRequest 分析原始 Http/1.x 请求头字节，返回 Http 指纹。
// 解析 {"browser","language","quality"} 字段，未出现的字段保持零值；
// 无法识别时返回 (nil, nil)。
//
//go:nocheckptr
func AnalyzeHTTPRequest(raw []byte) (*HTTPFingerprint, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	return parseResult[HTTPFingerprint](C.huginn_http_analyze_request((*C.uint8_t)(unsafe.Pointer(&raw[0])), C.size_t(len(raw))))
}

// AnalyzeHTTPResponse 分析原始 Http/1.x 响应头字节，返回 Http 指纹。
// 解析 {"server","quality"} 字段，未出现的字段保持零值；
// 无法识别时返回 (nil, nil)。
//
//go:nocheckptr
func AnalyzeHTTPResponse(raw []byte) (*HTTPFingerprint, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	return parseResult[HTTPFingerprint](C.huginn_http_analyze_response((*C.uint8_t)(unsafe.Pointer(&raw[0])), C.size_t(len(raw))))
}
