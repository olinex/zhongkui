package main

import (
	"zhongkui.com/zhongkui/internal/cmd"
	_ "zhongkui.com/zhongkui/internal/pkg/validator"
)

// 进程执行入口函数
func main() {
	cmd.Execute()
}
