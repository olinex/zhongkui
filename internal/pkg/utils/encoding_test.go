// 本文件覆盖 internal/pkg/utils 中三个编码类纯计算能力的算法与域级用例：Hash（摘要）、
// EncodeConvert（编码转换）与 GenerateUUID（UUID 生成）。全部用例直接调用 utils 导出函数，不经过任何
// 适配器或运行期上下文；错误断言针对 utils 抛出的无工具名前缀的错误。

package utils

import (
	// 标准库
	"strings"
	"testing"
)

// encodingUtilAssertUUIDVersion 断言 id 是标准 36 字符 UUID 且版本位（第 15 个字符）等于 want。
func encodingUtilAssertUUIDVersion(t *testing.T, id string, want byte) {
	t.Helper()
	if len(id) != 36 {
		t.Fatalf("uuid %q length = %d, want 36", id, len(id))
	}
	for _, dash := range []int{8, 13, 18, 23} {
		if id[dash] != '-' {
			t.Errorf("uuid %q: position %d = %q, want '-'", id, dash, id[dash])
		}
	}
	if id[14] != want {
		t.Errorf("uuid %q version char = %q, want %q", id, id[14], string(want))
	}
}

// ============================================================================
// Hash
// ============================================================================

// TestHash_SHA256 覆盖正常路径：sha256("hello") 的十六进制为公开已知常量，base64 表示可由 hex
// 还原，且出参字段名与取值正确。
func TestHash_SHA256(t *testing.T) {
	got, err := Hash("hello", "sha256")
	if err != nil {
		t.Fatalf("Hash error = %v", err)
	}
	const wantHex = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if got.Algorithm != "sha256" {
		t.Errorf("algorithm = %q, want sha256", got.Algorithm)
	}
	if got.Hex != wantHex {
		t.Errorf("hex = %q, want %q", got.Hex, wantHex)
	}
	if got.Base64 == "" {
		t.Errorf("base64 is empty, want the digest's base64 representation")
	}
	if got.Base64 == got.Hex {
		t.Errorf("base64 = %q equals hex, want a distinct base64 representation", got.Base64)
	}
}

// TestHash_AllAlgorithms 覆盖四种算法均可调用，且摘要十六进制长度符合各自规格。
func TestHash_AllAlgorithms(t *testing.T) {
	wantHexLen := map[string]int{"md5": 32, "sha1": 40, "sha256": 64, "sha512": 128}
	for algorithm, length := range wantHexLen {
		t.Run(algorithm, func(t *testing.T) {
			got, err := Hash("hello", algorithm)
			if err != nil {
				t.Fatalf("Hash error = %v", err)
			}
			if len(got.Hex) != length {
				t.Errorf("hex length = %d, want %d", len(got.Hex), length)
			}
		})
	}
}

// TestHash_InvalidAlgorithm 覆盖非法算法被拒且不产出结果。
func TestHash_InvalidAlgorithm(t *testing.T) {
	for _, algorithm := range []string{"md6", ""} {
		if _, err := Hash("hello", algorithm); err == nil {
			t.Fatalf("algorithm %q: error = nil, want an English rejection", algorithm)
		}
	}
}

// TestHash_EmptyText 覆盖空文本被拒。
func TestHash_EmptyText(t *testing.T) {
	if _, err := Hash("", "md5"); err == nil {
		t.Fatal("error = nil, want a rejection for empty text")
	}
}

// ============================================================================
// EncodeConvert
// ============================================================================

// TestEncodeConvert_Base64Encode 覆盖正常路径：base64("hello")="aGVsbG8="，且字段名与取值正确。
func TestEncodeConvert_Base64Encode(t *testing.T) {
	got, err := EncodeConvert("base64", "encode", "hello")
	if err != nil {
		t.Fatalf("EncodeConvert error = %v", err)
	}
	if got.Operation != "encode" || got.Scheme != "base64" {
		t.Errorf("result = %+v, want encode/base64", got)
	}
	if got.Result != "aGVsbG8=" {
		t.Errorf("result = %q, want %q", got.Result, "aGVsbG8=")
	}
}

// TestEncodeConvert_RoundTrip 覆盖各方案的编码后解码可还原原文。
func TestEncodeConvert_RoundTrip(t *testing.T) {
	const text = "hello world+中文?x=1"
	for _, scheme := range EncodeSchemeNames {
		t.Run(scheme, func(t *testing.T) {
			enc, err := EncodeConvert(scheme, "encode", text)
			if err != nil {
				t.Fatalf("encode error = %v", err)
			}
			dec, err := EncodeConvert(scheme, "decode", enc.Result)
			if err != nil {
				t.Fatalf("decode error = %v", err)
			}
			if dec.Result != text {
				t.Errorf("round trip = %q, want %q", dec.Result, text)
			}
		})
	}
}

// TestEncodeConvert_Base64URL 覆盖 base64url 使用 URL 安全字母表（不含 + / =）。
func TestEncodeConvert_Base64URL(t *testing.T) {
	got, err := EncodeConvert("base64url", "encode", "\u00fb\u00ff")
	if err != nil {
		t.Fatalf("EncodeConvert error = %v", err)
	}
	if strings.ContainsAny(got.Result, "+/") {
		t.Errorf("result = %q, want URL-safe alphabet without + or /", got.Result)
	}
}

// TestEncodeConvert_InvalidDecode 覆盖非法 decode 输入（非法 base64 / hex）被拒。
func TestEncodeConvert_InvalidDecode(t *testing.T) {
	cases := []struct {
		scheme string
		text   string
	}{
		{"base64", "not base64!!"},
		{"base64url", "%%%"},
		{"hex", "zzzz"},
		{"hex", "abc"},
	}
	for _, tc := range cases {
		if _, err := EncodeConvert(tc.scheme, "decode", tc.text); err == nil {
			t.Errorf("scheme %s text %q: error = nil, want an English rejection", tc.scheme, tc.text)
		}
	}
}

// TestEncodeConvert_InvalidOperationAndScheme 覆盖非法操作方向与非法方案被拒。
func TestEncodeConvert_InvalidOperationAndScheme(t *testing.T) {
	cases := []struct {
		scheme    string
		operation string
	}{
		{"base64", "enc"},
		{"rot13", "encode"},
		{"base64", ""},
	}
	for _, tc := range cases {
		if _, err := EncodeConvert(tc.scheme, tc.operation, "hello"); err == nil {
			t.Errorf("scheme %s operation %q: error = nil, want an English rejection", tc.scheme, tc.operation)
		}
	}
}

// ============================================================================
// GenerateUUID
// ============================================================================

// TestGenerateUUID_Defaults 覆盖缺省版本：空版本取 DefaultUUIDVersion，生成个数与格式正确。
func TestGenerateUUID_Defaults(t *testing.T) {
	got, err := GenerateUUID("", 1)
	if err != nil {
		t.Fatalf("GenerateUUID error = %v", err)
	}
	if got.Version != DefaultUUIDVersion {
		t.Errorf("version = %q, want the default %q", got.Version, DefaultUUIDVersion)
	}
	if len(got.UUIDs) != 1 {
		t.Fatalf("uuids length = %d, want 1", len(got.UUIDs))
	}
	encodingUtilAssertUUIDVersion(t, got.UUIDs[0], '4')
}

// TestGenerateUUID_VersionsAndCount 覆盖 v4 / v7 与自定义数量：数量一致、版本位正确、且互不重复。
func TestGenerateUUID_VersionsAndCount(t *testing.T) {
	for _, version := range []string{"4", "7"} {
		t.Run(version, func(t *testing.T) {
			got, err := GenerateUUID(version, 5)
			if err != nil {
				t.Fatalf("GenerateUUID error = %v", err)
			}
			if got.Version != version {
				t.Errorf("version = %q, want %q", got.Version, version)
			}
			if len(got.UUIDs) != 5 {
				t.Fatalf("uuids length = %d, want 5", len(got.UUIDs))
			}
			seen := map[string]bool{}
			for _, id := range got.UUIDs {
				encodingUtilAssertUUIDVersion(t, id, version[0])
				if seen[id] {
					t.Errorf("duplicate uuid %q in a batch", id)
				}
				seen[id] = true
			}
		})
	}
}

// TestGenerateUUID_InvalidVersionAndCount 覆盖非法版本与数量越界（0 与 101）被拒。
func TestGenerateUUID_InvalidVersionAndCount(t *testing.T) {
	cases := []struct {
		version string
		count   int
	}{
		{"1", 1},
		{"x", 1},
		{"4", 0},
		{"4", 101},
		{"4", -3},
	}
	for _, tc := range cases {
		if _, err := GenerateUUID(tc.version, tc.count); err == nil {
			t.Errorf("version %q count %d: error = nil, want an English rejection", tc.version, tc.count)
		}
	}
}
