package version

import (
	"encoding/json"
	"os/exec"
	"strings"
)

const (
	defaultLabel = "(not set)"
	unknownLabel = "unknown"
)

// 编译时通过 -ldflags 注入的版本信息
var (
	// 语义化版本号，例如 v1.0.0，用于 release 构建
	SemVer = ""
	// Git commit hash，用于开发和 CI 构建
	Commit = ""
	// 构建时间，ISO8601 格式
	BuildTime = ""
	// 谁构建的
	Builder = ""
)

// versionInfo 是最终要输出的版本结构
type versionInfo struct {
	SemVer    string `json:"semver,omitempty"`
	Commit    string `json:"commit,omitempty"`
	BuildTime string `json:"build_time,omitempty"`
	Builder   string `json:"builder,omitempty"`
}

// Version 返回单行版本号：优先语义化版本，其次 commit hash
func (v versionInfo) Version() string {
	if v.SemVer != "" {
		return v.SemVer
	}
	if v.Commit != "" {
		return v.Commit
	}
	return unknownLabel
}

// Verbose 返回多行详细信息
func (v versionInfo) Verbose() string {
	var b strings.Builder
	// semVer
	var semVer = defaultLabel
	if v.SemVer != "" {
		semVer = v.SemVer
	}
	if _, err := b.WriteString("Version:\t\t" + semVer); err != nil {
		panic(err)
	}
	// commit
	var commit = defaultLabel
	if v.Commit != "" {
		commit = v.Commit
	}
	if _, err := b.WriteString("\nCommit:\t\t\t" + commit); err != nil {
		panic(err)
	}
	// buildTime
	var buildTime = defaultLabel
	if v.BuildTime != "" {
		buildTime = v.BuildTime
	}
	if _, err := b.WriteString("\nBuild Time:\t\t" + buildTime); err != nil {
		panic(err)
	}
	// builder
	var builder = defaultLabel
	if v.Builder != "" {
		builder = v.Builder
	}
	if _, err := b.WriteString("\nBuilder:\t\t" + builder); err != nil {
		panic(err)
	}
	return b.String()
}

// JSON 返回 JSON 格式的版本信息
func (v versionInfo) JSON() string {
	data, _ := json.MarshalIndent(v, "", "  ")
	return string(data)
}

// gitCommit 尝试通过执行 git rev-parse 获取当前 commit hash
func gitCommit() string {
	out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Get 返回包含所有版本信息的 versionInfo
func Get() versionInfo {
	info := versionInfo{
		SemVer:    SemVer,
		Commit:    Commit,
		BuildTime: BuildTime,
		Builder:   Builder,
	}
	// ldflags 未注入时，回退到 git 命令获取 commit
	if info.Commit == "" {
		info.Commit = gitCommit()
	}
	return info
}

// Short 返回精简版本字符串
func Short() string {
	return Get().Version()
}

// PrintVersion 以格式化方式输出版本信息
func PrintVersion(verbose bool) string {
	info := Get()
	if verbose {
		return info.Verbose()
	}
	return info.Version()
}
