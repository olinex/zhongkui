// Package main 提供注释检查器入口：以 standard go/analysis 驱动运行 commentlint analyzer。
//
// 用法与 go vet 一致（诊断输出到 stderr，发现缺口时退出码 3）：
//
//	go run cmd/commentlint/main.go ./...
package main

import (
	"golang.org/x/tools/go/analysis/singlechecker"

	"zhongkuisec.com/zhongkui/internal/lint/commentlint"
)

// 进程执行入口函数
func main() {
	singlechecker.Main(commentlint.Analyzer)
}
