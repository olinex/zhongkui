// Package main 提供许可证门禁入口：检查运行时依赖的许可证，发现违规时以非零码退出。
//
// 用法：
//
//	go run cmd/licensecheck/main.go
//	LICENSE_CHECK_REQUIRE_WEB=1 go run cmd/licensecheck/main.go
package main

import (
	// 标准库
	"context"
	"flag"
	"fmt"
	"os"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/lint/licensecheck"
)

// 进程执行入口函数
func main() {
	root := flag.String("root", ".", "项目根目录")
	requireWeb := flag.Bool("require-web", os.Getenv("LICENSE_CHECK_REQUIRE_WEB") == "1",
		"未找到 pnpm 时是否失败（CI 应设为 true）")
	flag.Parse()

	report, err := licensecheck.Check(context.Background(), licensecheck.Options{
		ProjectRoot: *root,
		RequireWeb:  *requireWeb,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "licensecheck:", err)
		os.Exit(1)
	}

	fmt.Printf("许可证检查：Go 模块 %d，Web 包 %d\n", report.GoModules, report.WebPackages)
	if report.WebSkipped {
		fmt.Println("注意：未找到 pnpm，已跳过 Web 检查；CI 应设置 LICENSE_CHECK_REQUIRE_WEB=1")
	}
	if len(report.Violations) == 0 {
		fmt.Println("许可证检查通过")
		return
	}

	fmt.Fprintf(os.Stderr, "发现 %d 个不合规依赖：\n", len(report.Violations))
	for _, v := range report.Violations {
		fmt.Fprintf(os.Stderr, "  %s@%s  %s  %s\n", v.Component, v.Version, v.License, v.Reason)
	}
	os.Exit(1)
}
