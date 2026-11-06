package docs

import (
	// 标准库
	"go/build"
	"os"
	"path/filepath"
	"slices"
	"strings"

	// 第三方
	"github.com/princjef/gomarkdoc"
	"github.com/princjef/gomarkdoc/lang"
	"github.com/princjef/gomarkdoc/logger"
	"golang.org/x/tools/go/packages"

	// 项目内部

	"zhongkuisec.com/zhongkui/internal/pkg/base"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// narrativeTemplate 是叙述档模板：仅输出包注释正文，不含任何符号小节。
const narrativeTemplate = `{{- if len .Doc.Blocks -}}
{{- template "doc" .Doc -}}
{{- end -}}`

// narrativeTextTemplate 是叙述档专用的 text 模板：对纯文本 span（text）原样输出 .Text。
// gomarkdoc 内置 text 模板会对 text 调用 escape，把 Markdown 特殊字符转义；而叙述档的包注释
// 本身就是手写 Markdown，需保留 `##` 标题、`行内代码`、`*` 强调等语义。因此这里关闭纯文本转义，
// 仅保留对 link 类 span 的 link + escape（链接文本仍需转义为合法 Markdown 链接）。
// 该覆盖仅在叙述档启用；参考档沿用内置 text 模板，其对符号名与字面量的转义是正确行为。
const narrativeTextTemplate = `{{- range . -}}
{{- if eq .Kind "text" -}}
{{- .Text -}}
{{- else if eq .Kind "rawText" -}}
{{- .Text -}}
{{- else if eq .Kind "autolink" -}}
{{- .Text -}}
{{- else if eq .Kind "link" -}}
{{- link (escape .Text) .URL -}}
{{- end -}}
{{- end -}}`

// docsBuildContext 固定文档生成使用的构建上下文为 Linux/amd64：文档描述生产部署形态，
// 且避免 macOS 开发机与 Linux CI 因构建约束不同而生成出不一致的参考页。
var docsBuildContext = func() build.Context {
	ctx := build.Default
	ctx.GOOS = "linux"
	ctx.GOARCH = "amd64"
	ctx.CgoEnabled = true
	return ctx
}()

// packagesEnv 返回固定为 Linux/amd64 的 go/packages 环境，与 docsBuildContext 保持同一平台。
func packagesEnv() []string {
	return append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=1")
}

// sourceRepo 是参考页源码链接使用的仓库地址与默认分支。显式指定而不依赖本地 git 元数据，
// 避免浅克隆缺少 origin/HEAD 或默认分支不同导致生成结果随环境漂移。
const (
	sourceRepoRemote        = "https://gitee.com/olinex/zhongkui"
	sourceRepoDefaultBranch = "master"
)

// referenceTemplate 是参考档模板：仅输出索引与各符号小节，不含包注释正文。
const referenceTemplate = `{{- if or (len .Consts) (len .Vars) (len .Funcs) (len .Types) -}}
{{- header (add .Level 1) "Index" -}}
{{- spacer -}}
{{- template "index" . -}}

{{- if len .Consts -}}
{{- spacer -}}
{{- header (add .Level 1) "Constants" -}}
{{- spacer -}}
{{- range (iter .Consts) -}}
{{- template "value" .Entry -}}
{{- if (not .Last) -}}{{- spacer -}}{{- end -}}
{{- end -}}
{{- end -}}

{{- if len .Vars -}}
{{- spacer -}}
{{- header (add .Level 1) "Variables" -}}
{{- spacer -}}
{{- range (iter .Vars) -}}
{{- template "value" .Entry -}}
{{- if (not .Last) -}}{{- spacer -}}{{- end -}}
{{- end -}}
{{- end -}}

{{- if len .Funcs -}}
{{- spacer -}}
{{- range (iter .Funcs) -}}
{{- template "func" .Entry -}}
{{- if (not .Last) -}}{{- spacer -}}{{- end -}}
{{- end -}}
{{- end -}}

{{- if len .Types -}}
{{- spacer -}}
{{- range (iter .Types) -}}
{{- template "type" .Entry -}}
{{- if (not .Last) -}}{{- spacer -}}{{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}`

// RenderPackage 按导入路径加载单个包并按选定档位渲染 Markdown。
// narrative 为 true 时按叙述档渲染（仅包注释）；为 false 时按参考档渲染（仅符号小节）。
// projectRoot 为项目根目录，用于解析导入路径对应的包目录。
//
// 若目录不存在（真正的包缺失）则返回包装后的错误；若目录存在但不含 Go 源码
// （如仅作为子包命名空间的目录），则视为无需渲染，返回空字符串。
func RenderPackage(projectRoot, importPath string, narrative bool) (string, error) {
	dir, err := packageDir(projectRoot, importPath)
	if err != nil {
		// 回退：按编译期模块前缀推导目录。模块前缀取自构建信息，无法获取时不做文件系统猜测，
		// 直接返回原始错误，避免构造出误导性路径。
		modulePath := base.ModulePath()
		if modulePath == "" {
			return "", errors.WithStack(err)
		}
		fallback := filepath.Join(projectRoot, strings.TrimPrefix(importPath, modulePath+"/"))
		if info, statErr := os.Stat(fallback); statErr != nil || !info.IsDir() {
			return "", errors.WithStack(err)
		}
		dir = fallback
	}

	if !hasGoFiles(dir) {
		return "", nil
	}
	return renderDir(dir, narrative)
}

// hasGoFiles 报告目录下是否存在 Go 源文件。
func hasGoFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
			return true
		}
	}
	return false
}

// packageDir 借助 go/packages 解析导入路径对应的包目录。
func packageDir(projectRoot, importPath string) (string, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles,
		Dir:  projectRoot,
		Env:  packagesEnv(),
	}
	pkgs, err := packages.Load(cfg, importPath)
	if err != nil {
		return "", errors.WithStack(err)
	}
	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			return "", errors.WithStack(pkg.Errors[0])
		}
		if pkg.Dir != "" {
			return pkg.Dir, nil
		}
	}
	return "", errors.Newf("docs: cannot resolve directory of package %q", importPath)
}

// renderDir 加载指定目录的包并按选定档位渲染 Markdown。
func renderDir(dir string, narrative bool) (string, error) {
	// 保持 gomarkdoc 日志安静，避免污染 CLI 输出。
	log := logger.New(logger.ErrorLevel)

	buildPkg, err := docsBuildContext.ImportDir(dir, build.ImportComment)
	if err != nil {
		return "", errors.WithStack(err)
	}

	pkg, err := lang.NewPackageFromBuild(log, buildPkg,
		lang.PackageWithRepositoryOverrides(&lang.Repo{
			Remote:        sourceRepoRemote,
			DefaultBranch: sourceRepoDefaultBranch,
		}))
	if err != nil {
		return "", errors.WithStack(err)
	}

	// 参考档只覆盖 package 模板，沿用内置 text 模板（对符号名/字面量的转义是正确行为）；
	// 叙述档额外覆盖 text 模板以关闭纯文本转义，保留包注释里的 Markdown 语义。
	opts := []gomarkdoc.RendererOption{gomarkdoc.WithTemplateOverride("package", referenceTemplate)}
	if narrative {
		opts = []gomarkdoc.RendererOption{
			gomarkdoc.WithTemplateOverride("package", narrativeTemplate),
			gomarkdoc.WithTemplateOverride("text", narrativeTextTemplate),
		}
	}
	renderer, err := gomarkdoc.NewRenderer(opts...)
	if err != nil {
		return "", errors.WithStack(err)
	}

	out, err := renderer.Package(pkg)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return out, nil
}

// isTestdataPath 报告路径是否位于 testdata 目录内（应被排除）。
func isTestdataPath(p string) bool {
	return slices.Contains(strings.Split(filepath.ToSlash(p), "/"), "testdata")
}
