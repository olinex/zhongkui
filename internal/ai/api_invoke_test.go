// 本文件覆盖 api_invoke 挂起前校验：能力白名单、只读属性、请求体与路径参数规范化。用例只依赖
// 本地能力夹具，不触达数据库、网络或模型。

package ai

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"zhongkuisec.com/zhongkui/internal/ai/capability"
)

// apiInvokeTestCapabilities 是 api_invoke 与 Agent 消息组装测试共用的能力夹具：覆盖无路径参数
// 与带路径参数两类只读能力。
func apiInvokeTestCapabilities() []capability.Capability {
	return []capability.Capability{
		{
			ID:          "traffic.list",
			Domain:      "traffic",
			DomainLabel: "流量",
			Intent:      "查询 HTTP 流量列表",
			Method:      "POST",
			Path:        "/api/v1/traffic/http-transport/list",
			Entities:    []string{"http_transports"},
			Access:      capability.AccessRead,
			Invokable:   true,
		},
		{
			ID:          "tag.event.detail",
			Domain:      "tag",
			DomainLabel: "标签与事件",
			Intent:      "查看单条标签事件详情",
			Method:      "POST",
			Path:        "/api/v1/tag-event/record/:id/get",
			Entities:    []string{"tag_event_records"},
			Access:      capability.AccessRead,
			Invokable:   true,
		},
	}
}

// withTestCapabilities 返回注入测试能力夹具的 Agent 选项，供消息组装与工具链测试复用。
func withTestCapabilities() AgentOption {
	return WithCapabilities(apiInvokeTestCapabilities)
}

// TestPrepareApiInvokeCall_Canonical 校验只读能力被解析为规范入参，路径参数被替换。
func TestPrepareApiInvokeCall_Canonical(t *testing.T) {
	got, errText := prepareApiInvokeCall(`{"capability":"traffic.list","params":{"page":1}}`, apiInvokeTestCapabilities())
	if errText != "" {
		t.Fatalf("prepareApiInvokeCall error = %q", errText)
	}
	var parsed apiInvokeCanonicalArgs
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("canonical arguments are not JSON: %v", err)
	}
	if parsed.Capability != "traffic.list" || parsed.Method != "POST" ||
		parsed.Path != "/api/v1/traffic/http-transport/list" || string(parsed.Params) != `{"page":1}` {
		t.Errorf("canonical = %+v, want the whitelisted endpoint and params", parsed)
	}

	got, errText = prepareApiInvokeCall(`{"capability":"tag.event.detail","pathParams":{"id":"abc-123"}}`, apiInvokeTestCapabilities())
	if errText != "" {
		t.Fatalf("prepareApiInvokeCall(path params) error = %q", errText)
	}
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("canonical arguments are not JSON: %v", err)
	}
	if parsed.Path != "/api/v1/tag-event/record/abc-123/get" || string(parsed.Params) != `{}` {
		t.Errorf("canonical = %+v, want the resolved path and empty params", parsed)
	}
}

// TestApiInvokeToolInfo 校验 client schema 已登记 api_invoke，且只暴露 capability / params / pathParams。
func TestApiInvokeToolInfo(t *testing.T) {
	info, ok := clientToolInfo(apiInvokeToolName)
	if !ok {
		t.Fatalf("clientToolInfo(%q) 未登记", apiInvokeToolName)
	}
	if !isClientTool(apiInvokeToolName) || isQuestionnaireTool(apiInvokeToolName) {
		t.Errorf("api_invoke 必须是需审批的 client 工具，且不属于问卷类")
	}
	js, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema error = %v", err)
	}
	for _, name := range []string{"capability", "params", "pathParams"} {
		if _, ok := js.Properties.Get(name); !ok {
			t.Errorf("schema 缺少属性 %q", name)
		}
	}
	for _, forbidden := range []string{"url", "method", "path"} {
		if _, ok := js.Properties.Get(forbidden); ok {
			t.Errorf("schema 不应暴露 %q", forbidden)
		}
	}
}

// TestApiInvokePathPlaceholders 校验占位符提取保持出现顺序，且忽略普通段与空占位符。
func TestApiInvokePathPlaceholders(t *testing.T) {
	cases := []struct {
		name     string
		template string
		want     []string
	}{
		{"no placeholder", "/api/v1/traffic/http-transport/list", nil},
		{"middle", "/api/v1/tag-event/record/:id/get", []string{"id"}},
		{"trailing", "/api/v1/users/:id", []string{"id"}},
		{"multiple", "/api/v1/groups/:group/users/:id", []string{"group", "id"}},
		{"empty name", "/:id/:", []string{"id"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := apiInvokePathPlaceholders(c.template)
			if !slices.Equal(got, c.want) {
				t.Errorf("placeholders = %v, want %v", got, c.want)
			}
		})
	}
}

// TestApiInvokeResolvePath 校验路径构建：无占位符原样返回，占位符按整段替换，多参数与
// 前缀重叠互不干扰，并提供明确的错误文本。
func TestApiInvokeResolvePath(t *testing.T) {
	const allowed = "A-z.0_9~"
	cases := []struct {
		name     string
		template string
		params   map[string]string
		want     string
		wantErr  string
	}{
		{"no placeholder", "/api/v1/traffic/http-transport/list", nil, "/api/v1/traffic/http-transport/list", ""},
		{"middle placeholder", "/api/v1/tag-event/record/:id/get", map[string]string{"id": "abc-123"}, "/api/v1/tag-event/record/abc-123/get", ""},
		{"trailing placeholder", "/api/v1/users/:id", map[string]string{"id": "abc-123"}, "/api/v1/users/abc-123", ""},
		{"multiple placeholders", "/api/v1/groups/:group/users/:id", map[string]string{"group": "g1", "id": "u2"}, "/api/v1/groups/g1/users/u2", ""},
		{"allowed charset", "/api/v1/users/:id", map[string]string{"id": allowed}, "/api/v1/users/" + allowed, ""},
		{"prefix overlap", "/api/v1/:id/:idx", map[string]string{"id": "1", "idx": "2"}, "/api/v1/1/2", ""},
		{"missing value", "/api/v1/groups/:group/:id", map[string]string{"group": "g1"}, "", "requires path parameters"},
		{"empty value", "/api/v1/users/:id", map[string]string{"id": ""}, "", "missing path parameter"},
		{"blank value", "/api/v1/users/:id", map[string]string{"id": "   "}, "", "missing path parameter"},
		{"unknown key", "/api/v1/users/:id", map[string]string{"id": "1", "extra": "2"}, "", "unknown path parameter"},
		{"extra key without placeholder", "/api/v1/traffic/list", map[string]string{"id": "1"}, "", "no path parameters"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, errText := apiInvokeResolvePath(c.template, c.params)
			if c.wantErr != "" {
				if errText == "" || !strings.Contains(errText, c.wantErr) {
					t.Errorf("error = %q, want containing %q", errText, c.wantErr)
				}
				if got != "" {
					t.Errorf("path = %q, want empty on error", got)
				}
				return
			}
			if errText != "" {
				t.Fatalf("unexpected error = %q", errText)
			}
			if got != c.want {
				t.Errorf("path = %q, want %q", got, c.want)
			}
		})
	}
}

// TestApiInvokeResolvePath_RejectsUnsafeValues 校验路径参数不得改变路径结构或注入查询串。
func TestApiInvokeResolvePath_RejectsUnsafeValues(t *testing.T) {
	for _, value := range []string{"../etc", "a/b", "a?b", "a#b", "a b", "a%b", "a:b", "中文"} {
		t.Run(value, func(t *testing.T) {
			got, errText := apiInvokeResolvePath("/api/v1/users/:id", map[string]string{"id": value})
			if errText == "" || !strings.Contains(errText, "not allowed") {
				t.Errorf("value %q: error = %q, want not allowed", value, errText)
			}
			if got != "" {
				t.Errorf("value %q: path = %q, want empty on error", value, got)
			}
		})
	}
}

// TestApiInvokeResolvePath_RejectsGinWildcards 校验不支持的 gin 通配符模板被显式拒绝，
// 避免占位符原样留在最终 URL 中。
func TestApiInvokeResolvePath_RejectsGinWildcards(t *testing.T) {
	cases := []struct {
		name     string
		template string
	}{
		{"catch-all", "/api/v1/files/*path"},
		{"escaped colon", `/api/v1/records/\:id/get`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path, errText := apiInvokeResolvePath(c.template, nil)
			if errText == "" || !strings.Contains(errText, "unsupported wildcard syntax") {
				t.Errorf("error = %q, want unsupported wildcard syntax", errText)
			}
			if path != "" {
				t.Errorf("path = %q, want empty", path)
			}
		})
	}
}

// TestPrepareApiInvokeCall_Rejections 校验越权、非法请求体与非法路径参数被拒绝并回灌英文错误。
func TestPrepareApiInvokeCall_Rejections(t *testing.T) {
	cases := []struct {
		name string
		args string
		want string
	}{
		{"invalid json", `not-json`, "valid JSON object"},
		{"empty capability", `{"capability":""}`, "non-empty capability"},
		{"unknown capability", `{"capability":"not.exist"}`, "not registered"},
		{"params not object", `{"capability":"traffic.list","params":[1,2]}`, "must be a JSON object"},
		{"missing path param", `{"capability":"tag.event.detail"}`, "requires path parameters"},
		{"unknown path param", `{"capability":"traffic.list","pathParams":{"id":"1"}}`, "no path parameters"},
		{"unsafe path param", `{"capability":"tag.event.detail","pathParams":{"id":"../etc"}}`, "not allowed"},
	}
	for _, c := range cases {
		_, errText := prepareApiInvokeCall(c.args, apiInvokeTestCapabilities())
		if errText == "" || !strings.Contains(errText, c.want) {
			t.Errorf("%s: error = %q, want containing %q", c.name, errText, c.want)
		}
	}
}
