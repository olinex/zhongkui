package docs

import (
	// 标准库
	"fmt"
	"path"
	"sort"
	"strings"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/manager"
	"zhongkuisec.com/zhongkui/internal/manager/views"
)

// apiDomainLabels 是接口业务域到中文名的映射；未登记的业务域直接使用路径段。
var apiDomainLabels = map[string]string{
	"admin":        "管理",
	"traffic":      "流量",
	"tag":          "标签",
	"datasource":   "数据源",
	"plaintext":    "明文",
	"user":         "用户",
	"notification": "通知",
	"etl":          "数据管道",
	"ai":           "AI 助手",
	"llm":          "模型与工具",
	"features":     "功能开关",
}

// apiIndexItem 是接口索引中的一条记录，用于生成分组索引页。
type apiIndexItem struct {
	// Domain 是业务域标识。
	Domain string
	// Method 是 HTTP 方法。
	Method string
	// Path 是完整接口路径。
	Path string
	// Description 是接口用途。
	Description string
	// Output 是接口页相对 docs/ 的路径。
	Output string
}

// apiReferenceProjections 生成接口参考投影：按业务域分组的索引页，以及每个接口一页。
// 接口页正文复用 RAG 知识文档的渲染器，保证人类文档与模型上下文同源。
func apiReferenceProjections() ([]Projection, error) {
	catalog := manager.CollectApiCatalog()
	sort.Slice(catalog, func(i, j int) bool {
		if catalog[i].Path != catalog[j].Path {
			return catalog[i].Path < catalog[j].Path
		}
		return catalog[i].Method < catalog[j].Method
	})

	projections := make([]Projection, 0, len(catalog)+1)
	items := make([]apiIndexItem, 0, len(catalog))
	used := make(map[string]bool, len(catalog))
	for _, descriptor := range catalog {
		domain := apiDomain(descriptor.Path)
		slug := uniqueApiSlug(used, descriptor.Method, descriptor.Path)
		output := path.Join(GeneratedRoot, "reference", "api", domain, slug+".md")
		doc := views.RenderApiDoc(descriptor)
		projections = append(projections, Projection{
			Title:       descriptor.Method + " " + descriptor.Path,
			Summary:     descriptor.Description,
			Roles:       []Role{RoleDev},
			Scene:       "需要调用该接口、构造请求参数或排查返回结构时阅读。",
			Output:      output,
			SourceLabel: "internal/manager/views/catalog.go",
			Content:     doc.Content,
		})
		items = append(items, apiIndexItem{
			Domain:      domain,
			Method:      descriptor.Method,
			Path:        descriptor.Path,
			Description: descriptor.Description,
			Output:      output,
		})
	}

	indexOutput := path.Join(GeneratedRoot, "reference", "api", "index.md")
	projections = append(projections, Projection{
		Title:       "接口参考索引",
		Summary:     "按业务域分组列出全部原生接口，每个接口一页。",
		Roles:       []Role{RoleDev, RoleSecOps},
		Scene:       "需要查找某个业务域有哪些接口、或从接口名定位具体页面时阅读。",
		Output:      indexOutput,
		SourceLabel: "internal/manager/views/catalog.go",
		Content:     renderApiIndex(items),
	})
	return projections, nil
}

// apiDomain 从接口路径中提取业务域：/api/v1/{domain}/...。
func apiDomain(apiPath string) string {
	trimmed := strings.TrimPrefix(apiPath, "/api/v1/")
	parts := strings.Split(trimmed, "/")
	if len(parts) == 0 || parts[0] == "" {
		return "root"
	}
	return parts[0]
}

// apiDomainLabel 返回业务域中文名，未登记时回退为业务域标识。
func apiDomainLabel(domain string) string {
	if label, ok := apiDomainLabels[domain]; ok {
		return label
	}
	return domain
}

// uniqueApiSlug 生成接口页文件名（方法 + 路径），并在同域冲突时追加序号。
func uniqueApiSlug(used map[string]bool, method, apiPath string) string {
	base := strings.ToLower(method) + "-" + strings.TrimPrefix(apiPath, "/api/v1/")
	replacer := strings.NewReplacer("/", "-", ":", "", "{", "", "}", "", "_", "-", "..", "-")
	slug := replacer.Replace(base)
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "root"
	}
	candidate := slug
	for i := 2; used[candidate]; i++ {
		candidate = fmt.Sprintf("%s-%d", slug, i)
	}
	used[candidate] = true
	return candidate
}

// renderApiIndex 生成按业务域分组的接口索引页正文。
func renderApiIndex(items []apiIndexItem) string {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Domain != items[j].Domain {
			return items[i].Domain < items[j].Domain
		}
		if items[i].Path != items[j].Path {
			return items[i].Path < items[j].Path
		}
		return items[i].Method < items[j].Method
	})

	var b strings.Builder
	b.WriteString("## 按业务域\n\n")
	current := ""
	for _, item := range items {
		if item.Domain != current {
			current = item.Domain
			b.WriteString("### ")
			b.WriteString(apiDomainLabel(current))
			b.WriteString("\n\n")
		}
		link := strings.TrimPrefix(item.Output, path.Join(GeneratedRoot, "reference", "api")+"/")
		b.WriteString("- [`")
		b.WriteString(item.Method)
		b.WriteString(" ")
		b.WriteString(item.Path)
		b.WriteString("`](./")
		b.WriteString(link)
		b.WriteString(") — ")
		b.WriteString(item.Description)
		b.WriteString("\n")
	}
	return b.String()
}
