// 本文件是三个编码类内置工具（hash / encode_convert / uuid）的适配器级测试：只覆盖 Info schema、
// 经 InvokableRun 的错误路径（断言「工具名: 子串」）与每个工具的 JSON 线形状 smoke。算法与域级用例已
// 随实现下沉到 internal/pkg/utils/encoding_test.go。

package tools

import (
	// 标准库
	"context"
	"encoding/json"
	"strings"
	"testing"

	// 第三方
	"github.com/cloudwego/eino/schema"
)

// ============================================================================
// 哈希工具
// ============================================================================

// TestHash_Info 覆盖 Info：工具名为 hash；algorithm 为带四种取值的必填 string；text 必填 string。
func TestHash_Info(t *testing.T) {
	info, err := hashTool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info error = %v", err)
	}
	if info.Name != "hash" {
		t.Errorf("Info name = %q, want hash", info.Name)
	}
	js, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema error = %v", err)
	}
	algorithm, ok := js.Properties.Get("algorithm")
	if !ok || algorithm.Type != string(schema.String) || len(algorithm.Enum) != 4 {
		t.Errorf("algorithm property = %+v (present=%v), want a string with four algorithms", algorithm, ok)
	}
	if text, ok := js.Properties.Get("text"); !ok || text.Type != string(schema.String) {
		t.Errorf("text property = %+v (present=%v), want a string", text, ok)
	}
	for _, name := range []string{"algorithm", "text"} {
		if !encodingTestRequiredContains(js.Required, name) {
			t.Errorf("required = %v, want %s required", js.Required, name)
		}
	}
}

// TestHash_InvokableErrors 覆盖经 InvokableRun 的错误路径：非法算法与空文本，断言错误消息带工具名
// 前缀；非法入参 JSON 的错误不经工具名前缀。
func TestHash_InvokableErrors(t *testing.T) {
	cases := []struct {
		name     string
		args     string
		wantHint string
	}{
		{"invalid-algorithm", `{"algorithm":"md6","text":"hello"}`, "hash: invalid algorithm"},
		{"empty-algorithm", `{"algorithm":"","text":"hello"}`, "hash: invalid algorithm"},
		{"empty-text", `{"algorithm":"md5","text":""}`, "hash: text must be non-empty"},
		{"invalid-arguments-in", `not-json`, "invalid character"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := hashTool.InvokableRun(context.Background(), tc.args)
			if err == nil {
				t.Fatal("error = nil, want a rejection")
			}
			if !strings.Contains(err.Error(), tc.wantHint) {
				t.Errorf("error = %q, want it to mention %q", err.Error(), tc.wantHint)
			}
		})
	}
}

// TestHash_Smoke 锁定 hash 的出参 JSON 线形状与字段名（摘要值随机无关，只校验字段名与非空）。
func TestHash_Smoke(t *testing.T) {
	out, err := hashTool.InvokableRun(context.Background(), `{"algorithm":"sha256","text":"hello"}`)
	if err != nil {
		t.Fatalf("InvokableRun error = %v", err)
	}
	var decoded struct {
		Algorithm string `json:"algorithm"`
		Hex       string `json:"hex"`
		Base64    string `json:"base64"`
	}
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("result %q is not valid JSON: %v", out, err)
	}
	if decoded.Algorithm != "sha256" {
		t.Errorf("algorithm = %q, want sha256", decoded.Algorithm)
	}
	if len(decoded.Hex) != 64 {
		t.Errorf("hex length = %d, want 64", len(decoded.Hex))
	}
	if decoded.Base64 == "" {
		t.Errorf("base64 is empty, want the digest's base64 representation")
	}
}

// ============================================================================
// 编码转换工具
// ============================================================================

// TestEncodeConvert_Info 覆盖 Info：工具名为 encode_convert；operation / scheme / text 均为必填
// string，operation 与 scheme 带枚举取值。
func TestEncodeConvert_Info(t *testing.T) {
	info, err := encodeConvertTool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info error = %v", err)
	}
	if info.Name != "encode_convert" {
		t.Errorf("Info name = %q, want encode_convert", info.Name)
	}
	js, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema error = %v", err)
	}
	for _, name := range []string{"operation", "scheme", "text"} {
		prop, ok := js.Properties.Get(name)
		if !ok || prop.Type != string(schema.String) {
			t.Errorf("property %q = %+v (present=%v), want a string", name, prop, ok)
		}
		if !encodingTestRequiredContains(js.Required, name) {
			t.Errorf("required = %v, want %s required", js.Required, name)
		}
	}
}

// TestEncodeConvert_InvokableErrors 覆盖经 InvokableRun 的错误路径：非法操作方向、非法方案、空文本与
// 非法 decode 输入，断言错误消息带工具名前缀。
func TestEncodeConvert_InvokableErrors(t *testing.T) {
	cases := []struct {
		name     string
		args     string
		wantHint string
	}{
		{"invalid-operation", `{"operation":"enc","scheme":"base64","text":"hello"}`, "encode_convert: invalid operation"},
		{"missing-operation", `{"operation":"","scheme":"base64","text":"hello"}`, "encode_convert: invalid operation"},
		{"invalid-scheme", `{"operation":"encode","scheme":"rot13","text":"hello"}`, "encode_convert: invalid scheme"},
		{"empty-text", `{"operation":"encode","scheme":"base64","text":""}`, "encode_convert: text must be non-empty"},
		{"invalid-decode", `{"operation":"decode","scheme":"base64","text":"not base64!!"}`, "encode_convert: invalid base64 input for decode"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := encodeConvertTool.InvokableRun(context.Background(), tc.args)
			if err == nil {
				t.Fatal("error = nil, want a rejection")
			}
			if !strings.Contains(err.Error(), tc.wantHint) {
				t.Errorf("error = %q, want it to mention %q", err.Error(), tc.wantHint)
			}
		})
	}
}

// TestEncodeConvert_Smoke 锁定 encode_convert 的出参 JSON 线形状与字段名。
func TestEncodeConvert_Smoke(t *testing.T) {
	out, err := encodeConvertTool.InvokableRun(context.Background(),
		`{"operation":"encode","scheme":"base64","text":"hello"}`)
	if err != nil {
		t.Fatalf("InvokableRun error = %v", err)
	}
	if want := `{"operation":"encode","scheme":"base64","result":"aGVsbG8="}`; out != want {
		t.Errorf("result = %q, want %q", out, want)
	}
}

// ============================================================================
// UUID 工具
// ============================================================================

// TestUUID_Info 覆盖 Info：工具名为 uuid；version 为可选带枚举 string；count 为可选 integer。
func TestUUID_Info(t *testing.T) {
	info, err := uuidTool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info error = %v", err)
	}
	if info.Name != "uuid" {
		t.Errorf("Info name = %q, want uuid", info.Name)
	}
	js, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema error = %v", err)
	}
	if version, ok := js.Properties.Get("version"); !ok || version.Type != string(schema.String) {
		t.Errorf("version property = %+v (present=%v), want a string", version, ok)
	}
	if count, ok := js.Properties.Get("count"); !ok || count.Type != string(schema.Integer) {
		t.Errorf("count property = %+v (present=%v), want an integer", count, ok)
	}
}

// TestUUID_InvokableErrors 覆盖经 InvokableRun 的错误路径：非法版本与数量越界，断言错误消息带工具名
// 前缀。
func TestUUID_InvokableErrors(t *testing.T) {
	cases := []struct {
		name     string
		args     string
		wantHint string
	}{
		{"invalid-version", `{"version":"1"}`, "uuid: invalid version"},
		{"bad-version", `{"version":"x"}`, "uuid: invalid version"},
		{"count-zero", `{"count":0}`, "uuid: count 0 out of range"},
		{"count-over", `{"count":101}`, "uuid: count 101 out of range"},
		{"count-negative", `{"count":-3}`, "uuid: count -3 out of range"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := uuidTool.InvokableRun(context.Background(), tc.args)
			if err == nil {
				t.Fatal("error = nil, want a rejection")
			}
			if !strings.Contains(err.Error(), tc.wantHint) {
				t.Errorf("error = %q, want it to mention %q", err.Error(), tc.wantHint)
			}
		})
	}
}

// TestUUID_Smoke 锁定 uuid 的出参 JSON 线形状与字段名（UUID 值随机无关，只校验字段名、版本与数量）。
func TestUUID_Smoke(t *testing.T) {
	out, err := uuidTool.InvokableRun(context.Background(), `{"version":"4","count":2}`)
	if err != nil {
		t.Fatalf("InvokableRun error = %v", err)
	}
	var decoded struct {
		Version string   `json:"version"`
		UUIDs   []string `json:"uuids"`
	}
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("result %q is not valid JSON: %v", out, err)
	}
	if decoded.Version != "4" {
		t.Errorf("version = %q, want 4", decoded.Version)
	}
	if len(decoded.UUIDs) != 2 {
		t.Fatalf("uuids length = %d, want 2", len(decoded.UUIDs))
	}
	for _, id := range decoded.UUIDs {
		if len(id) != 36 {
			t.Errorf("uuid %q length = %d, want 36", id, len(id))
		}
	}
}

// encodingTestRequiredContains 判断 JSON schema 的必填列表中是否含给定属性名，供各 Info 用例复用。
func encodingTestRequiredContains(required []string, name string) bool {
	for _, r := range required {
		if r == name {
			return true
		}
	}
	return false
}

// TestEncodingTools_BuildResolves 覆盖 init 期登记生效：Build 可按名解析三个工具并返回可用的 eino 工具。
func TestEncodingTools_BuildResolves(t *testing.T) {
	for _, name := range []string{"hash", "encode_convert", "uuid"} {
		built, err := Build(context.Background(), name, Deps{})
		if err != nil {
			t.Fatalf("Build(%s) error = %v", name, err)
		}
		info, err := built.Info(context.Background())
		if err != nil {
			t.Fatalf("built tool %s Info error = %v", name, err)
		}
		if info.Name != name {
			t.Errorf("built tool name = %q, want %q", info.Name, name)
		}
	}
}
