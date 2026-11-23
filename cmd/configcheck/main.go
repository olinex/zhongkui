// Package main 提供部署配置漂移检查入口。
//
// 权威源由 `zki create default-config` 生成，本命令只负责比对：
//
//	go run cmd/configcheck/main.go \
//	  --authority /tmp/authority.toml \
//	  --toml binary=deploys/binary/config.example.toml \
//	  --env compose-env=deploys/compose/docker-compose.yml
//
// 退出码：0 全部通过；1 存在漂移；2 参数或读取/解析错误。
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"zhongkuisec.com/zhongkui/internal/lint/configcheck"
)

func main() {
	fs := flag.NewFlagSet("configcheck", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	authorityPath := fs.String("authority", "", "权威配置 TOML 路径（zki create default-config 生成）")

	var targets []configcheck.TOMLTarget
	fs.Func("toml", "待比对产物，格式 label=path，可重复", func(value string) error {
		label, path, err := splitLabelPath(value)
		if err != nil {
			return err
		}
		targets = append(targets, configcheck.TOMLTarget{Label: label, Path: path})
		return nil
	})

	var envFiles []configcheck.EnvFile
	fs.Func("env", "扫描 ZKI_* 覆盖键的文本文件，格式 label=path，可重复", func(value string) error {
		label, path, err := splitLabelPath(value)
		if err != nil {
			return err
		}
		envFiles = append(envFiles, configcheck.EnvFile{Label: label, Path: path})
		return nil
	})

	// ZKI_ENVIRONMENT 是运行时环境标识（写入 OTel），ZKI_API_UPSTREAM 由 Web
	// 容器 nginx 使用；两者都不是后端配置字段，不参与覆盖键校验。
	allowEnv := []string{"ZKI_ENVIRONMENT", "ZKI_API_UPSTREAM"}
	fs.Func("allow-env", "放行的非配置环境变量名，可重复", func(value string) error {
		value = strings.TrimSpace(value)
		if value == "" {
			return fmt.Errorf("--allow-env must not be empty")
		}
		allowEnv = append(allowEnv, value)
		return nil
	})

	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "configcheck: 存在未解析参数 %v\n", fs.Args())
		os.Exit(2)
	}
	if *authorityPath == "" {
		fmt.Fprintln(os.Stderr, "configcheck: 缺少 --authority")
		os.Exit(2)
	}
	if len(targets) == 0 && len(envFiles) == 0 {
		fmt.Fprintln(os.Stderr, "configcheck: 至少需要一个 --toml 或 --env")
		os.Exit(2)
	}

	authority, err := configcheck.LoadTOMLKeys(*authorityPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "configcheck:", err)
		os.Exit(2)
	}

	drifted := 0
	for _, target := range targets {
		keys, err := configcheck.LoadTOMLKeys(target.Path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "configcheck:", err)
			os.Exit(2)
		}

		diff := configcheck.Compare(authority, keys)
		if diff.Empty() {
			fmt.Printf("[ OK ] %s（%s）\n", target.Label, target.Path)
			continue
		}
		drifted++
		printDiff(target, diff)
	}

	issues, err := configcheck.CheckEnvOverrides(authority, envFiles, allowEnv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "configcheck:", err)
		os.Exit(2)
	}

	envIssueLabels := make(map[string]struct{}, len(envFiles))
	for _, issue := range issues {
		envIssueLabels[issue.Label] = struct{}{}
	}
	for _, file := range envFiles {
		if _, ok := envIssueLabels[file.Label]; ok {
			continue
		}
		fmt.Printf("[ OK ] %s（%s）\n", file.Label, file.Path)
	}
	printed := make(map[string]struct{}, len(envIssueLabels))
	for _, issue := range issues {
		if _, ok := printed[issue.Label]; !ok {
			printed[issue.Label] = struct{}{}
			drifted++
			fmt.Printf("[FAIL] %s（%s）\n", issue.Label, issue.Path)
		}
		fmt.Printf("  未知环境变量覆盖: %s\n", issue.Name)
	}
	if drifted > 0 {
		fmt.Fprintf(os.Stderr, "配置漂移检查未通过：%d 个产物存在差异\n", drifted)
		os.Exit(1)
	}
	fmt.Println("配置漂移检查通过")
}

// splitLabelPath 解析 label=path 形式的参数。
func splitLabelPath(value string) (string, string, error) {
	label, path, ok := strings.Cut(value, "=")
	label = strings.TrimSpace(label)
	path = strings.TrimSpace(path)
	if !ok || label == "" || path == "" {
		return "", "", fmt.Errorf("argument %q is not a valid label=path", value)
	}
	return label, path, nil
}

// printDiff 输出单个产物的差异明细。
func printDiff(target configcheck.TOMLTarget, diff configcheck.Diff) {
	fmt.Printf("[FAIL] %s（%s）\n", target.Label, target.Path)
	if len(diff.Missing) > 0 {
		fmt.Printf("  缺少 %d 项:\n", len(diff.Missing))
		for _, path := range diff.Missing {
			fmt.Printf("    %s\n", path)
		}
	}
	if len(diff.Extra) > 0 {
		fmt.Printf("  多余 %d 项:\n", len(diff.Extra))
		for _, path := range diff.Extra {
			fmt.Printf("    %s\n", path)
		}
	}
	if len(diff.TypeMismatch) > 0 {
		fmt.Printf("  类型不一致 %d 项:\n", len(diff.TypeMismatch))
		for _, item := range diff.TypeMismatch {
			fmt.Printf("    %s（权威 %s，产物 %s）\n", item.Path, item.Authority, item.Target)
		}
	}
}
