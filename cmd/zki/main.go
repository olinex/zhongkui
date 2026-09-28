// Package main 提供 zhongkui 的主程序入口：委托 internal/cmd 构建并执行根命令。
package main

import (
	"zhongkui.com/zhongkui/internal/cmd"
	_ "zhongkui.com/zhongkui/internal/pkg/validator"
)

// 进程执行入口函数
func main() {
	cmd.Execute()
}
