// Package licensecheck 提供第三方依赖许可证门禁：检查链接进 zki 二进制的 Go 模块与 Web 生产依赖，
// 拒绝强 copyleft、非商业、未知许可证，并输出可定位到具体组件的违规清单。
package licensecheck

import (
	// 标准库
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// DefaultGoPackage 是被检查的 Go 入口包，其依赖闭包即随二进制分发的模块集合。
const DefaultGoPackage = "./cmd/zki"

// DefaultWebDir 是 Web 管理端目录，包含 package.json 与 pnpm-lock.yaml。
const DefaultWebDir = "web"

// Options 控制许可证检查的范围与严格程度。
type Options struct {
	// ProjectRoot 是项目根目录。
	ProjectRoot string
	// GoPackage 是被检查的 Go 入口包。
	GoPackage string
	// WebDir 是 Web 管理端目录（相对 ProjectRoot）。
	WebDir string
	// RequireWeb 为 true 时，缺少 pnpm 或无法读取 Web 依赖即视为失败。
	RequireWeb bool
}

// Violation 描述一个不合规或无法判定的依赖。
type Violation struct {
	// Component 是模块路径或 npm 包名。
	Component string
	// Version 是依赖版本。
	Version string
	// License 是识别出的许可证或标记。
	License string
	// Reason 是判定原因。
	Reason string
}

// Report 是一次许可证检查的结果。
type Report struct {
	// GoModules 是检查的 Go 运行时模块数。
	GoModules int
	// WebPackages 是检查的 Web 生产依赖包数。
	WebPackages int
	// WebSkipped 表示 Web 检查因缺少 pnpm 被跳过。
	WebSkipped bool
	// Violations 是不合规依赖清单，按组件与版本排序。
	Violations []Violation
}

// AllowedLicenses 是允许进入运行时依赖的 SPDX 许可证集合。
// 新增许可证类型必须经过人工核对后显式加入本集合。
var AllowedLicenses = map[string]bool{
	"MIT":          true,
	"Apache-2.0":   true,
	"BSD-2-Clause": true,
	"BSD-3-Clause": true,
	"ISC":          true,
	"MPL-2.0":      true,
	"EPL-2.0":      true,
	"0BSD":         true,
	"Unlicense":    true,
	"Python-2.0":   true,
	"CC0-1.0":      true,
	"Zlib":         true,
	"BSL-1.0":      true,
	"OFL-1.1":      true,
}

// deniedFragments 是直接拒绝的许可证片段：强 copyleft、源码可用限制与非商业条款。
var deniedFragments = []string{
	"GPL",
	"SSPL",
	"BUSL",
	"COMMONS CLAUSE",
	"NONCOMMERCIAL",
	"NON-COMMERCIAL",
	"CC-BY-NC",
	"UNLICENSED",
	"NOASSERTION",
	"UNKNOWN",
}

// knownUnknown 记录 pnpm 元数据缺失但已人工核对许可证的包，键为 name@version。
var knownUnknown = map[string]string{
	"khroma@2.1.0": "MIT",
}

// knownMissingLicense 记录模块目录未携带 LICENSE 但已人工核对上游许可证的组件，键为 module@version。
// 版本升级后必须重新核对；当前两项均属于 cloudwego/eino-ext 单仓库子目录模块，
// 仓库根 LICENSE 为 Apache-2.0，子目录 README 亦标注 Apache-2.0。
var knownMissingLicense = map[string]string{
	"github.com/cloudwego/eino-ext/components/model/openai@v0.1.13": "Apache-2.0",
	"github.com/cloudwego/eino-ext/libs/acl/openai@v0.1.17":         "Apache-2.0",
}

// goModule 是 go list -m -json 输出的模块信息子集。
type goModule struct {
	// Path 是模块路径。
	Path string
	// Version 是模块版本。
	Version string
	// Dir 是模块在本地缓存中的目录。
	Dir string
	// Main 表示当前主模块。
	Main bool
	// Replace 是替换目标模块。
	Replace *goModule
}

// pnpmPackage 是 pnpm licenses list 输出中的单个包条目。
type pnpmPackage struct {
	// Name 是包名。
	Name string `json:"name"`
	// Versions 是该包出现的版本列表。
	Versions []string `json:"versions"`
	// License 是包声明的许可证。
	License string `json:"license"`
}

// Check 执行完整检查：先检查 Go 运行时模块，再检查 Web 生产依赖。
// Web 检查缺少 pnpm 时，RequireWeb 决定失败还是跳过。
func Check(ctx context.Context, opts Options) (Report, error) {
	root := opts.ProjectRoot
	if root == "" {
		root = "."
	}
	goPkg := opts.GoPackage
	if goPkg == "" {
		goPkg = DefaultGoPackage
	}
	webDir := opts.WebDir
	if webDir == "" {
		webDir = DefaultWebDir
	}

	report := Report{}
	goViolations, goCount, err := checkGo(ctx, root, goPkg)
	if err != nil {
		return Report{}, err
	}
	report.GoModules = goCount
	report.Violations = append(report.Violations, goViolations...)

	webViolations, webCount, skipped, err := checkWeb(ctx, root, webDir, opts.RequireWeb)
	if err != nil {
		return Report{}, err
	}
	report.WebPackages = webCount
	report.WebSkipped = skipped
	report.Violations = append(report.Violations, webViolations...)

	sort.Slice(report.Violations, func(i, j int) bool {
		if report.Violations[i].Component != report.Violations[j].Component {
			return report.Violations[i].Component < report.Violations[j].Component
		}
		return report.Violations[i].Version < report.Violations[j].Version
	})
	return report, nil
}

// checkGo 检查链接进指定入口包的 Go 模块许可证，返回违规清单与模块数。
func checkGo(ctx context.Context, root, goPkg string) ([]Violation, int, error) {
	// 先解析依赖闭包，确保运行时模块已下载到本地缓存。
	runtimePaths, err := goRuntimeModules(ctx, root, goPkg)
	if err != nil {
		return nil, 0, err
	}
	modules, err := goModuleInfo(ctx, root)
	if err != nil {
		return nil, 0, err
	}

	var violations []Violation
	count := 0
	for _, p := range runtimePaths {
		m, ok := modules[p]
		if !ok || m.Main {
			continue
		}
		count++
		dir, version := moduleDir(m)
		if dir == "" {
			violations = append(violations, Violation{
				Component: p, Version: version, License: "NO_MODULE_DIR",
				Reason: "模块未下载到本地缓存，无法读取许可证",
			})
			continue
		}
		lic := moduleLicense(dir)
		if lic == "NO_LICENSE_FILE" {
			if resolved, ok := knownMissingLicense[p+"@"+version]; ok {
				lic = resolved
			} else {
				violations = append(violations, Violation{
					Component: p, Version: version, License: lic,
					Reason: "模块目录未携带 LICENSE/COPYING/NOTICE，需人工核对上游许可证",
				})
				continue
			}
		}
		if reason := rejectReason(lic); reason != "" {
			violations = append(violations, Violation{
				Component: p, Version: version, License: lic, Reason: reason,
			})
		}
	}
	return violations, count, nil
}

// goRuntimeModules 返回链接进指定包的模块路径集合。
func goRuntimeModules(ctx context.Context, root, goPkg string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "go", "list", "-deps", "-f", "{{with .Module}}{{.Path}}{{end}}", goPkg)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list -deps %s: %w", goPkg, err)
	}
	seen := make(map[string]bool)
	var paths []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || seen[line] {
			continue
		}
		seen[line] = true
		paths = append(paths, line)
	}
	sort.Strings(paths)
	return paths, nil
}

// goModuleInfo 返回 go list -m -json all 的模块信息，键为模块路径。
func goModuleInfo(ctx context.Context, root string) (map[string]goModule, error) {
	cmd := exec.CommandContext(ctx, "go", "list", "-m", "-json", "all")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list -m all: %w", err)
	}
	modules := make(map[string]goModule)
	decoder := json.NewDecoder(bytes.NewReader(out))
	for {
		var m goModule
		if err := decoder.Decode(&m); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("解析 go list -m 输出: %w", err)
		}
		modules[m.Path] = m
	}
	return modules, nil
}

// moduleDir 返回模块实际使用的目录与版本，优先使用 Replace 目标。
func moduleDir(m goModule) (string, string) {
	if m.Replace != nil && m.Replace.Dir != "" {
		return m.Replace.Dir, m.Version
	}
	return m.Dir, m.Version
}

// moduleLicense 读取模块目录中的许可证文件并识别 SPDX 标识。
func moduleLicense(dir string) string {
	files := licenseFiles(dir)
	if len(files) == 0 {
		return "NO_LICENSE_FILE"
	}
	var text strings.Builder
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		text.Write(data)
		text.WriteString("\n")
	}
	return detectLicense(text.String())
}

// licenseFiles 返回目录下大小写不敏感的许可证类文件名。
func licenseFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		if strings.HasPrefix(name, "license") || strings.HasPrefix(name, "licence") ||
			strings.HasPrefix(name, "copying") || strings.HasPrefix(name, "notice") {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	sort.Strings(files)
	return files
}

// detectLicense 根据许可证文本识别 SPDX 标识，无法识别时返回 UNKNOWN。
func detectLicense(text string) string {
	t := strings.ToLower(text)
	switch {
	case strings.Contains(t, "apache license") && strings.Contains(t, "version 2.0"):
		return "Apache-2.0"
	case strings.Contains(t, "mozilla public license"):
		return "MPL-2.0"
	case strings.Contains(t, "eclipse public license") && strings.Contains(t, "2.0"):
		return "EPL-2.0"
	case strings.Contains(t, "python software foundation license"):
		return "Python-2.0"
	case strings.Contains(t, "gnu affero general public license"):
		return "AGPL-3.0"
	case strings.Contains(t, "gnu lesser general public license"):
		return "LGPL"
	case strings.Contains(t, "gnu general public license"):
		return "GPL"
	case strings.Contains(t, "permission is hereby granted, free of charge") || strings.Contains(t, "mit license"):
		return "MIT"
	case strings.Contains(t, "redistribution and use in source and binary forms"):
		if strings.Contains(t, "neither the name") {
			return "BSD-3-Clause"
		}
		return "BSD-2-Clause"
	case strings.Contains(t, "permission to use, copy, modify, and distribute") ||
		strings.Contains(t, "permission to use, copy, modify, and/or distribute"):
		return "ISC"
	case strings.Contains(t, "cc0 1.0") || strings.Contains(t, "creativecommons.org/publicdomain/zero"):
		return "CC0-1.0"
	case strings.Contains(t, "this is free and unencumbered software released into the public domain"):
		return "Unlicense"
	case strings.Contains(t, "universal permissive license"):
		return "UPL-1.0"
	case strings.Contains(t, "boost software license"):
		return "BSL-1.0"
	case strings.Contains(t, "open font license"):
		return "OFL-1.1"
	default:
		return "UNKNOWN"
	}
}

// checkWeb 检查 Web 生产依赖许可证，返回违规清单、包数与是否跳过。
func checkWeb(ctx context.Context, root, webDir string, requireWeb bool) ([]Violation, int, bool, error) {
	pnpmPath, err := exec.LookPath("pnpm")
	if err != nil {
		if requireWeb {
			return nil, 0, false, fmt.Errorf("未找到 pnpm，无法检查 Web 生产依赖；请安装 pnpm 或设置 LICENSE_CHECK_REQUIRE_WEB=0")
		}
		return nil, 0, true, nil
	}
	cmd := exec.CommandContext(ctx, pnpmPath, "licenses", "list", "--prod", "--json")
	cmd.Dir = filepath.Join(root, webDir)
	out, err := cmd.Output()
	if err != nil {
		return nil, 0, false, fmt.Errorf("pnpm licenses list: %w", err)
	}

	var groups map[string][]pnpmPackage
	if err := json.Unmarshal(out, &groups); err != nil {
		return nil, 0, false, fmt.Errorf("解析 pnpm licenses 输出: %w", err)
	}

	var violations []Violation
	count := 0
	for declared, packages := range groups {
		for _, pkg := range packages {
			for _, version := range pkg.Versions {
				count++
				lic := normalizeNpmLicense(pkg.Name, version, declared)
				if reason := rejectReason(lic); reason != "" {
					violations = append(violations, Violation{
						Component: pkg.Name, Version: version, License: lic, Reason: reason,
					})
				}
			}
		}
	}
	return violations, count, false, nil
}

// normalizeNpmLicense 归一化 npm 元数据中的许可证写法，并为已知缺失元数据的包补全取值。
func normalizeNpmLicense(name, version, declared string) string {
	switch strings.TrimSpace(declared) {
	case "MIT/X11":
		return "MIT"
	case "BSD":
		return "BSD-3-Clause"
	case "Unknown", "":
		if lic, ok := knownUnknown[name+"@"+version]; ok {
			return lic
		}
		return "UNKNOWN"
	default:
		return strings.TrimSpace(declared)
	}
}

// rejectReason 返回拒绝原因；许可证允许时返回空串。
func rejectReason(license string) string {
	license = strings.TrimSpace(license)
	if license == "" {
		return "许可证为空"
	}
	upper := strings.ToUpper(license)
	for _, fragment := range deniedFragments {
		if strings.Contains(upper, fragment) {
			return "许可证包含禁止条款：" + fragment
		}
	}
	if !licenseAllowed(license) {
		return "许可证不在允许清单中，需人工核对后加入 AllowedLicenses"
	}
	return ""
}

// licenseAllowed 判断许可证表达式是否允许：OR 任一分支允许即可，AND 要求全部分支允许。
func licenseAllowed(expr string) bool {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return false
	}
	for strings.HasPrefix(expr, "(") && strings.HasSuffix(expr, ")") {
		expr = strings.TrimSpace(expr[1 : len(expr)-1])
	}
	if parts := strings.Split(expr, " OR "); len(parts) > 1 {
		for _, part := range parts {
			if licenseAllowed(part) {
				return true
			}
		}
		return false
	}
	if parts := strings.Split(expr, " AND "); len(parts) > 1 {
		for _, part := range parts {
			if !licenseAllowed(part) {
				return false
			}
		}
		return true
	}
	// WITH 例外的许可义务以基础许可证为准；GPL 类已在拒绝片段中拦截。
	if idx := strings.Index(strings.ToUpper(expr), " WITH "); idx >= 0 {
		expr = strings.TrimSpace(expr[:idx])
	}
	return AllowedLicenses[expr]
}
