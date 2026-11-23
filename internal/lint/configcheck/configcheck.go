// Package configcheck 校验部署配置样例与 `zki create default-config` 权威产物之间的漂移。
//
// 检查口径为配置键集合与标量类型，不比对具体取值：二进制、Compose、Helm 三种部署
// 形态的主机名、路径与口令占位符本就不同，只有字段集合必须与代码结构体保持一致。
package configcheck

import (
	"os"
	"regexp"
	"sort"
	"strings"

	toml "github.com/pelletier/go-toml/v2"

	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// Kind 表示配置键在 TOML 中的值类型。
type Kind string

const (
	// KindTable 表示该路径是 TOML 表（包含子键）。
	KindTable Kind = "table"
	// KindString 表示字符串值。
	KindString Kind = "string"
	// KindInt 表示整数值。
	KindInt Kind = "int"
	// KindFloat 表示浮点值。
	KindFloat Kind = "float"
	// KindBool 表示布尔值。
	KindBool Kind = "bool"
	// KindArray 表示数组值。
	KindArray Kind = "array"
	// KindOther 表示未归类的值（如日期时间）。
	KindOther Kind = "other"
)

// TypeMismatch 描述同一路径在权威产物与目标产物中的类型差异。
type TypeMismatch struct {
	// Path 是配置键路径。
	Path string
	// Authority 是权威产物中的类型。
	Authority Kind
	// Target 是目标产物中的类型。
	Target Kind
}

// Diff 描述目标产物相对权威产物的键差异。
type Diff struct {
	// Missing 是权威产物中存在、目标产物缺失的键。
	Missing []string
	// Extra 是目标产物中存在、权威产物已不存在的键。
	Extra []string
	// TypeMismatch 是两边都存在但类型不同的键。
	TypeMismatch []TypeMismatch
}

// Empty 判断是否存在差异。
func (d Diff) Empty() bool {
	return len(d.Missing) == 0 && len(d.Extra) == 0 && len(d.TypeMismatch) == 0
}

// TOMLTarget 是一个待比对产物。
type TOMLTarget struct {
	// Label 是产物名称，用于输出。
	Label string
	// Path 是产物文件路径。
	Path string
}

// EnvFile 是一份需要扫描 ZKI_* 覆盖键的文本文件。
type EnvFile struct {
	// Label 是文件名称，用于输出。
	Label string
	// Path 是文件路径。
	Path string
}

// EnvIssue 描述一个无法映射到权威配置键的环境变量覆盖。
type EnvIssue struct {
	// Label 是文件名称，用于输出。
	Label string
	// Path 是文件路径。
	Path string
	// Name 是未命中权威配置键的环境变量名。
	Name string
}

var (
	// envYAMLKey 匹配 YAML 映射键形式的覆盖，如 `ZKI_S3_BUCKET: "capture"`。
	envYAMLKey = regexp.MustCompile(`(?m)^[ \t]*(ZKI_[A-Z0-9_]+)[ \t]*:`)
	// envNameKey 匹配 Kubernetes 环境变量列表项，如 `- name: ZKI_S3_BUCKET`。
	envNameKey = regexp.MustCompile(`(?m)^[ \t]*-[ \t]*name:[ \t]*(ZKI_[A-Z0-9_]+)[ \t]*$`)
)

// LoadTOMLKeys 解析 TOML 文件并展开为「路径 → 类型」集合。
//
// 表节点自身也会记录为 KindTable，空表（如 `payload = {}`）与带引号的键名
// 均按 TOML 语义处理，确保与权威产物可直接比对。
func LoadTOMLKeys(path string) (map[string]Kind, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrapf(err, "read %s", path)
	}

	var doc map[string]any
	if err := toml.Unmarshal(data, &doc); err != nil {
		return nil, errors.Wrapf(err, "parse TOML %s", path)
	}

	keys := make(map[string]Kind, 512)
	flattenKeys(doc, "", keys)
	return keys, nil
}

// flattenKeys 递归展开配置表，把每个路径写入 out。
func flattenKeys(table map[string]any, prefix string, out map[string]Kind) {
	for key, value := range table {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}

		switch typed := value.(type) {
		case map[string]any:
			out[path] = KindTable
			flattenKeys(typed, path, out)
		case []any:
			out[path] = KindArray
		case string:
			out[path] = KindString
		case int64:
			out[path] = KindInt
		case float64:
			out[path] = KindFloat
		case bool:
			out[path] = KindBool
		default:
			out[path] = KindOther
		}
	}
}

// Compare 计算 target 相对 authority 的缺失键、多余键与类型差异。
func Compare(authority, target map[string]Kind) Diff {
	var diff Diff

	for path, authorityKind := range authority {
		targetKind, ok := target[path]
		if !ok {
			diff.Missing = append(diff.Missing, path)
			continue
		}
		if targetKind != authorityKind {
			diff.TypeMismatch = append(diff.TypeMismatch, TypeMismatch{
				Path:      path,
				Authority: authorityKind,
				Target:    targetKind,
			})
		}
	}

	for path := range target {
		if _, ok := authority[path]; !ok {
			diff.Extra = append(diff.Extra, path)
		}
	}

	sort.Strings(diff.Missing)
	sort.Strings(diff.Extra)
	sort.Slice(diff.TypeMismatch, func(i, j int) bool {
		return diff.TypeMismatch[i].Path < diff.TypeMismatch[j].Path
	})
	return diff
}

// CheckEnvOverrides 校验 ZKI_* 覆盖键均能映射到权威配置键。
//
// 归一化规则：环境变量去掉 ZKI_ 前缀、转小写并删除下划线；配置键转小写并删除
// 点与下划线。allowlist 用于放行非配置字段（如运行时标识 ZKI_ENVIRONMENT）。
func CheckEnvOverrides(authority map[string]Kind, files []EnvFile, allowlist []string) ([]EnvIssue, error) {
	normalized, err := normalizeAuthorityKeys(authority)
	if err != nil {
		return nil, err
	}

	allowed := make(map[string]struct{}, len(allowlist))
	for _, name := range allowlist {
		allowed[name] = struct{}{}
	}

	var issues []EnvIssue
	for _, file := range files {
		data, err := os.ReadFile(file.Path)
		if err != nil {
			return nil, errors.Wrapf(err, "read %s", file.Path)
		}

		names := make(map[string]struct{})
		for _, match := range envYAMLKey.FindAllStringSubmatch(string(data), -1) {
			names[match[1]] = struct{}{}
		}
		for _, match := range envNameKey.FindAllStringSubmatch(string(data), -1) {
			names[match[1]] = struct{}{}
		}

		for name := range names {
			if _, ok := allowed[name]; ok {
				continue
			}
			if _, ok := normalized[normalizeEnvName(name)]; ok {
				continue
			}
			issues = append(issues, EnvIssue{Label: file.Label, Path: file.Path, Name: name})
		}
	}

	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Label != issues[j].Label {
			return issues[i].Label < issues[j].Label
		}
		return issues[i].Name < issues[j].Name
	})
	return issues, nil
}

// normalizeAuthorityKeys 建立「归一化键 → 原始键」映射，并拒绝会产生歧义的碰撞。
func normalizeAuthorityKeys(authority map[string]Kind) (map[string]string, error) {
	normalized := make(map[string]string, len(authority))
	for path := range authority {
		key := normalizeAuthorityKey(path)
		if existing, ok := normalized[key]; ok && existing != path {
			return nil, errors.Newf("normalized config key collision: %s and %s", existing, path)
		}
		normalized[key] = path
	}
	return normalized, nil
}

// normalizeAuthorityKey 把配置路径归一化为可与环境变量名比较的形式。
func normalizeAuthorityKey(path string) string {
	replacer := strings.NewReplacer(".", "", "_", "")
	return strings.ToLower(replacer.Replace(path))
}

// normalizeEnvName 把 ZKI_* 环境变量名归一化为可与配置键比较的形式。
func normalizeEnvName(name string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimPrefix(name, "ZKI_"), "_", ""))
}
