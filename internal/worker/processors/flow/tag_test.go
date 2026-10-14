// Package flow 实现基于 TCP 流的分布式抓包处理。
// 包含从 ClickHouse 轮询数据、TCP 流重组、批量输出到 ClickHouse 的完整流水线。
package flow

import (
	"context"
	"testing"
	"time"

	"github.com/gopacket/gopacket/layers"
	. "github.com/smartystreets/goconvey/convey"

	"zhongkuisec.com/zhongkui/internal/models/network"
	"zhongkuisec.com/zhongkui/internal/models/tag"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
	"zhongkuisec.com/zhongkui/internal/pkg/testdb"
	"zhongkuisec.com/zhongkui/internal/tagengine"
)

// setupTagStage 创建共享已迁移的 PostgreSQL 测试库与基于 tagengine 的标签阶段。
func setupTagStage(t *testing.T) (*tagStage, *database.DB) {
	t.Helper()
	logger, err := logging.NewDefaultLogger()
	if err != nil {
		t.Fatal(err)
	}
	db := testdb.Core(t)
	engine := tagengine.NewEngine(db, logger, nil)
	return &tagStage{engine: engine, logger: logger}, db
}

// insertHttpTransportRule 插入一条针对 HttpTransport 目标模型的启用规则。
func insertHttpTransportRule(t *testing.T, db *database.DB, name, rego string) {
	t.Helper()
	model, err := tagengine.TypeKey(&network.HttpTransport{})
	if err != nil {
		t.Fatal(err)
	}
	rule := &tag.TagRule{
		NamespaceName: "default",
		Name:          name,
		Model:         model,
		Rego:          rego,
		Enabled:       true,
	}
	if err := db.Create(rule).Error; err != nil {
		t.Fatal(err)
	}
}

// runStage 将 items 逐项送入 tagStage 并收集全部输出，验证阶段级行为。
func runStage(ctx context.Context, s *tagStage, items ...any) ([]any, error) {
	in := make(chan any, len(items))
	out := make(chan any, len(items))
	for _, item := range items {
		in <- item
	}
	close(in)
	if err := s.Process(ctx, in, out); err != nil {
		return nil, err
	}
	got := make([]any, 0, len(items))
	for item := range out {
		got = append(got, item)
	}
	return got, nil
}

func TestTagStage_Recognize(t *testing.T) {
	Convey("Test TagStage Recognize", t, func() {
		stage, db := setupTagStage(t)
		insertHttpTransportRule(t, db, "http-get", `package zki

tags contains "web_request" if {
    input.method == "GET"
}`)

		Convey("命中规则时写入 HttpTransport.Tags", func() {
			transport := &network.HttpTransport{}
			transport.Namespace = "default"
			transport.Method = "GET"
			got, err := runStage(context.Background(), stage, transport)
			So(err, ShouldBeNil)
			So(got, ShouldHaveLength, 1)
			So(transport.Tags, ShouldResemble, []string{"web_request"})
		})

		Convey("未命中规则时标签为空且不透传错误", func() {
			transport := &network.HttpTransport{}
			transport.Namespace = "default"
			transport.Method = "POST"
			_, err := runStage(context.Background(), stage, transport)
			So(err, ShouldBeNil)
			So(transport.Tags, ShouldBeEmpty)
		})
	})
}

func TestTagStage_Passthrough(t *testing.T) {
	Convey("Test TagStage Passthrough", t, func() {
		stage, _ := setupTagStage(t)

		Convey("非 HttpTransport 产物原样透传", func() {
			stack := &network.NetworkStack{}
			got, err := runStage(context.Background(), stage, stack)
			So(err, ShouldBeNil)
			So(got, ShouldHaveLength, 1)
			So(got[0], ShouldEqual, stack)
		})
	})
}

func TestTagStage_NoRules(t *testing.T) {
	Convey("Test TagStage NoRules", t, func() {
		stage, _ := setupTagStage(t)

		Convey("无规则分组时不报错且标签为空", func() {
			transport := &network.HttpTransport{}
			transport.Namespace = "default"
			transport.Method = "GET"
			_, err := runStage(context.Background(), stage, transport)
			So(err, ShouldBeNil)
			So(transport.Tags, ShouldBeEmpty)
		})
	})
}

// buildMockHttpTransport 构造一条模拟的 HTTP 流量记录：
// curl 客户端访问 api.example.com 的 /admin/users 管理接口，返回 nginx 服务端。
func buildMockHttpTransport(t *testing.T) *network.HttpTransport {
	t.Helper()
	clientIP, err := serializer.NewIPAddr("192.168.1.100")
	if err != nil {
		t.Fatal(err)
	}
	serverIP, err := serializer.NewIPAddr("10.0.0.8")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	transport := &network.HttpTransport{}
	transport.Namespace = "default"
	transport.Capture = "capture-01"
	transport.Version = "1.0"
	transport.SessionID = serializer.NewHexUUIDv4()
	transport.Timestamp = now
	transport.CreatedAt = now
	transport.ClientIP = *clientIP
	transport.ClientPort = 54321
	transport.ServerIP = *serverIP
	transport.ServerPort = 443
	transport.IPVersion = layers.IPProtocolIPv4
	transport.HttpVersion = "HTTP/1.1"
	transport.URL = "https://api.example.com/admin/users?page=1"
	transport.Scheme = "https"
	transport.Host = "api.example.com"
	transport.Path = "/admin/users"
	transport.Method = "GET"
	transport.ReqHdrHost = "api.example.com"
	transport.ReqHdrUserAgent = "curl/8.4.0"
	transport.ReqHdrAccept = "*/*"
	transport.RspHdrServer = "nginx/1.24.0"
	transport.RspHdrContentType = "application/json"
	transport.RspBodyLength = 2048
	return transport
}

// TestTagStage_RealisticScenario 模拟真实流量与多条标签规则，演示 tagStage 的识别效果。
// tagStage 的组合关系为业务逻辑写死（目标模型 = HttpTransport），同一目标模型下
// 的全部启用规则会合并评估，命中规则产出的标签均写入 Tags 字段。
func TestTagStage_RealisticScenario(t *testing.T) {
	Convey("Test TagStage Realistic Scenario", t, func() {
		stage, db := setupTagStage(t)
		insertHttpTransportRule(t, db, "curl-automation", `package zki

tags contains "automation" if {
    contains(input.reqHdrUserAgent, "curl")
}`)
		insertHttpTransportRule(t, db, "method-get", `package zki

tags contains "web_request" if {
    input.method == "GET"
}`)
		insertHttpTransportRule(t, db, "api-host", `package zki

tags contains "api_endpoint" if {
    input.host == "api.example.com"
}`)
		insertHttpTransportRule(t, db, "admin-path", `package zki

tags contains "sensitive_path" if {
    startswith(input.path, "/admin")
}`)
		insertHttpTransportRule(t, db, "nginx-server", `package zki

tags contains "nginx" if {
    contains(input.rspHdrServer, "nginx")
}`)

		// 记录一：curl 访问 /admin 管理接口（命中全部 5 条规则）
		curlTransport := buildMockHttpTransport(t)
		// 记录二：浏览器 POST 上传普通路径（仅命中 api-host 与 nginx-server 规则）
		browserTransport := &network.HttpTransport{}
		browserTransport.Namespace = "default"
		browserTransport.Method = "POST"
		browserTransport.URL = "https://api.example.com/v1/upload"
		browserTransport.Host = "api.example.com"
		browserTransport.Path = "/v1/upload"
		browserTransport.ReqHdrUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
		browserTransport.RspHdrServer = "nginx/1.24.0"

		// 打印规则评估可见的输入文档关键字段，便于核对规则与 JSON 字段映射
		input, err := tagengine.BuildInput(curlTransport)
		So(err, ShouldBeNil)
		t.Logf("recognition input (key fields): method=%v host=%v path=%v user_agent=%v rsp_hdr_server=%v",
			input["method"], input["host"], input["path"], input["reqHdrUserAgent"], input["rspHdrServer"])

		got, err := runStage(context.Background(), stage, curlTransport, browserTransport)
		So(err, ShouldBeNil)
		So(got, ShouldHaveLength, 2)

		t.Logf("curl 记录识别结果 tags=%v", curlTransport.Tags)
		t.Logf("浏览器记录识别结果 tags=%v", browserTransport.Tags)

		Convey("curl 记录命中全部规则", func() {
			So(curlTransport.Tags, ShouldResemble, []string{"api_endpoint", "automation", "nginx", "sensitive_path", "web_request"})
		})

		Convey("浏览器记录仅命中 api-host 与 nginx-server 规则", func() {
			So(browserTransport.Tags, ShouldResemble, []string{"api_endpoint", "nginx"})
		})
	})
}
