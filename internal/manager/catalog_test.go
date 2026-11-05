// 本文件是「原生接口知识目录」离线装配的守卫测试：
//   - 目录装配确定性（连续两次装配结果逐字段一致）
//   - 目录无陈旧条目（每条 (method, path) 都能在离线路由表中找到对应注册）
//   - 目录覆盖全部视图域（任一域缺失即失败并打印缺失前缀）
//
// 目录由 views 包进程内注册表承载，本测试所在包（manager）的测试二进制独立于 views 包，
// 故此处装配的目录仅含 registerViewRoutes 注册的条目，不受其它测试包影响。
package manager

import (
	// 标准库
	"reflect"
	"strings"
	"testing"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/manager/views"
)

// catalogEntryView 是目录条目的可比较投影（仅取用于守卫判定的四个字段），
// 避免直接深比较含反射类型的 Meta 字段。
type catalogEntryView struct {
	Method      string
	Path        string
	Description string
	RagEnabled  bool
}

// projectCatalog 把目录投影为可深度比较的条目切片。
func projectCatalog(catalog []views.ApiDescriptor) []catalogEntryView {
	out := make([]catalogEntryView, 0, len(catalog))
	for _, descriptor := range catalog {
		out = append(out, catalogEntryView{
			Method:      descriptor.Method,
			Path:        descriptor.Path,
			Description: descriptor.Description,
			RagEnabled:  descriptor.RagEnabled,
		})
	}
	return out
}

// domainPrefix 描述一个视图域的名称与其完整路径前缀。
type domainPrefix struct {
	Name   string
	Prefix string
}

// viewDomainPrefixes 是全部 10 个视图域的路径前缀（以各 views/*/register.go 实际注册为准）。
// llm 域挂在 /api/v1/admin/llm 之下，故其前缀比 admin 更具体。
func viewDomainPrefixes() []domainPrefix {
	return []domainPrefix{
		{Name: "admin", Prefix: "/api/v1/admin"},
		{Name: "traffic", Prefix: "/api/v1/traffic"},
		{Name: "tag", Prefix: "/api/v1/tag-event"},
		{Name: "datasource", Prefix: "/api/v1/datasource"},
		{Name: "plaintext", Prefix: "/api/v1/plaintext"},
		{Name: "user", Prefix: "/api/v1/user"},
		{Name: "notification", Prefix: "/api/v1/notification"},
		{Name: "etl", Prefix: "/api/v1/etl"},
		{Name: "ai", Prefix: "/api/v1/ai"},
		{Name: "llm", Prefix: "/api/v1/admin/llm"},
	}
}

// underPrefix 判断 path 是否等于 prefix 或位于其下（按路径段边界，避免 /user 误匹配 /users）。
func underPrefix(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

// TestCollectApiCatalogDeterministic 校验离线目录装配非空且确定性：
// 连续两次 CollectApiCatalog 在 method/path/description/ragEnabled 上逐字段一致。
func TestCollectApiCatalogDeterministic(t *testing.T) {
	first := CollectApiCatalog()
	if len(first) == 0 {
		t.Fatal("CollectApiCatalog() 返回空目录，期望至少一条接口声明")
	}

	second := CollectApiCatalog()
	if len(second) != len(first) {
		t.Fatalf("两次装配条目数不一致：first=%d second=%d", len(first), len(second))
	}
	if !reflect.DeepEqual(projectCatalog(first), projectCatalog(second)) {
		t.Fatalf("两次装配结果不一致：\nfirst=%+v\nsecond=%+v", projectCatalog(first), projectCatalog(second))
	}
}

// TestCollectApiCatalogNoStaleEntries 校验目录中不存在陈旧条目：
// 每条 (method, path) 都能在离线注册出的 gin 路由表中找到对应注册。
func TestCollectApiCatalogNoStaleEntries(t *testing.T) {
	engine, rc := newOfflineRegisterContext()
	registerViewRoutes(rc)

	type routeKey struct {
		method string
		path   string
	}
	registered := make(map[routeKey]struct{}, len(engine.Routes()))
	for _, route := range engine.Routes() {
		registered[routeKey{method: route.Method, path: route.Path}] = struct{}{}
	}

	for _, descriptor := range CollectApiCatalog() {
		if _, ok := registered[routeKey{method: descriptor.Method, path: descriptor.Path}]; !ok {
			t.Errorf("目录存在陈旧条目：(%s %s) 在离线路由表中无对应注册", descriptor.Method, descriptor.Path)
		}
	}
}

// TestCollectApiCatalogCoversAllDomains 校验目录覆盖全部 10 个视图域的路径前缀；
// 任一域无任何条目即失败，并打印缺失前缀，用于暴露域清单遗漏。
func TestCollectApiCatalogCoversAllDomains(t *testing.T) {
	catalog := CollectApiCatalog()

	var missing []string
	for _, domain := range viewDomainPrefixes() {
		hit := false
		for _, descriptor := range catalog {
			if underPrefix(descriptor.Path, domain.Prefix) {
				hit = true
				break
			}
		}
		if !hit {
			missing = append(missing, domain.Name+"("+domain.Prefix+")")
		}
	}
	if len(missing) > 0 {
		t.Fatalf("目录缺失以下视图域前缀的条目：%s", strings.Join(missing, ", "))
	}
}
