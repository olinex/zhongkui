package views

import (
	"testing"

	"zhongkui.com/zhongkui/internal/models/network"
)

// TestExtractCryptoFieldTagPayload 验证 PacketIPv4TCP / PacketIPv6TCP 内嵌的基础包结构
// （PacketIPv4 / PacketIPv6）与自身均声明 Payload 字段时的字段解析：
//   - 单段路径 "payload" 必须解析到最外层（直接声明）字段（offset=38278 / 50359）；
//   - 多段路径（如 "PacketIPv6.Payload"）必须按显式路径解析到内层嵌入字段（offset=31496）。
func TestExtractCryptoFieldTagPayload(t *testing.T) {
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
			spec, err := extractCryptoFieldTag(tc.model, tc.path)
			if err != nil {
				t.Fatalf("extractCryptoFieldTag(%q) failed: %v", tc.path, err)
			}
			if spec.Offset != tc.offset {
				t.Fatalf("%s offset mismatch: got %d, want %d", tc.path, spec.Offset, tc.offset)
			}
		})
	}
}
