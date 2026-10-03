package health

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"zhongkuisec.com/zhongkui/internal/pkg/etcd"
	"zhongkuisec.com/zhongkui/internal/pkg/status"
)

// TestKeyOf 验证服务注册键按前缀、服务名与实例 ID 拼接正确。
func TestKeyOf(t *testing.T) {
	server := NewServer(&Config{ReadyTimeout: time.Second}, status.New(), nil)
	cfg := RegistryConfig{KeyPrefix: "services/"}
	r := newRegistrar(nil, server, cfg)

	want := etcd.WithPrefix("services/") + "web/host-123"
	if got := r.keyOf("web", "host-123"); got != want {
		t.Fatalf("keyOf()=%q, want %q", got, want)
	}
}

// TestServiceInfoJSON 验证 ServiceInfo 的 JSON 编解码字段名与取值正确。
func TestServiceInfoJSON(t *testing.T) {
	info := ServiceInfo{
		ServiceName: "web",
		InstanceID:  "host-1",
		Live:        true,
		Ready:       true,
		Started:     true,
	}

	raw, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(raw)
	for _, key := range []string{"serviceName", "instanceId", "live", "ready", "started"} {
		if !strings.Contains(s, key) {
			t.Fatalf("json %s missing key %q", s, key)
		}
	}

	var back ServiceInfo
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.ServiceName != info.ServiceName {
		t.Errorf("ServiceName=%q, want %q", back.ServiceName, info.ServiceName)
	}
	if back.InstanceID != info.InstanceID {
		t.Errorf("InstanceID=%q, want %q", back.InstanceID, info.InstanceID)
	}
	if back.Live != info.Live {
		t.Errorf("Live=%v, want %v", back.Live, info.Live)
	}
	if back.Ready != info.Ready {
		t.Errorf("Ready=%v, want %v", back.Ready, info.Ready)
	}
	if back.Started != info.Started {
		t.Errorf("Started=%v, want %v", back.Started, info.Started)
	}
}
