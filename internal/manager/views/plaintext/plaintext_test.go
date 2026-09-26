package plaintext

import (
	"reflect"
	"strings"
	"testing"

	"zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/pkg/crypto"
)

// TestCryptoFieldTagPayload 验证 PacketIPv4TCP / PacketIPv6TCP 内嵌的基础包结构
// （PacketIPv4 / PacketIPv6）与自身均声明 Payload 字段时的字段解析（crypto.FieldTag）：
//   - 单段路径 "payload" 必须解析到最外层（直接声明）字段（offset=38278 / 50359）；
//   - 多段路径（如 "PacketIPv6.Payload"）必须按显式路径解析到内层嵌入字段（offset=31496）。
func TestCryptoFieldTagPayload(t *testing.T) {
	cases := []struct {
		name   string
		model  any
		path   string
		offset int
	}{
		{"ipv4-tcp outer", &network.PacketIPv4TCP{}, "payload", 38278},
		{"ipv4-tcp inner", &network.PacketIPv4TCP{}, "PacketIPv4.Payload", 16429},
		{"ipv6-tcp outer", &network.PacketIPv6TCP{}, "payload", 50359},
		{"ipv6-tcp inner", &network.PacketIPv6TCP{}, "PacketIPv6.Payload", 31496},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec, err := crypto.FieldTag(tc.model, tc.path, "json")
			if err != nil {
				t.Fatalf("crypto.FieldTag(%q) failed: %v", tc.path, err)
			}
			if spec.Offset != tc.offset {
				t.Fatalf("%s offset mismatch: got %d, want %d", tc.path, spec.Offset, tc.offset)
			}
		})
	}
}

// TestPlaintextModelsSecureFieldsResolvableByJSONName 校验所有已注册明文模型的不变量：
// 每个可见的加密字段（secure tag）都必须带 json 名，且该 json 名能被 crypto.FieldTag
// 解析回同一字段（比对 offset）。明文接口按 json 名解析字段（见 CryptoPlaintextGet），
// 缺少 json 标签的字段会在运行时才暴露（reflection: field %q not found），故在此静态拦截。
// 注意：仅校验可见字段；内嵌结构中被外层同名遮蔽的字段不参与单段路径解析（见 crypto.FieldTag）。
func TestPlaintextModelsSecureFieldsResolvableByJSONName(t *testing.T) {
	for name, model := range plaintextModels {
		for _, f := range reflect.VisibleFields(derefStructType(model, t)) {
			secure := f.Tag.Get(crypto.SecurityTagKey)
			if secure == "" {
				continue
			}
			jsonName := strings.Split(f.Tag.Get("json"), ",")[0]
			if jsonName == "" {
				t.Errorf("%s: secure field %s has no json name (must be resolvable by the json name the plaintext API receives)", name, f.Name)
				continue
			}
			spec, err := crypto.FieldTag(model, jsonName, "json")
			if err != nil {
				t.Errorf("%s: secure field %s (json %q) is not resolvable: %v", name, f.Name, jsonName, err)
				continue
			}
			want, err := crypto.ParseCryptoTag(secure)
			if err != nil {
				t.Errorf("%s: secure tag of %s is malformed: %v", name, f.Name, err)
				continue
			}
			if spec.Offset != want.Offset {
				t.Errorf("%s: json name %q resolved to offset %d, want %d (%s)", name, jsonName, spec.Offset, want.Offset, f.Name)
			}
		}
	}
}

// derefStructType 返回模型（结构体或其指针）的结构体反射类型，非结构体直接失败
func derefStructType(model any, t *testing.T) reflect.Type {
	t.Helper()
	rt := reflect.TypeOf(model)
	for rt != nil && rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
	}
	if rt == nil || rt.Kind() != reflect.Struct {
		t.Fatalf("model %T is not a struct", model)
	}
	return rt
}
