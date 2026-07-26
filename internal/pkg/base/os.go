package base

import "os"

// Hostname 返回本机主机名，获取失败则返回 "unknown"。
func Hostname() string {
	if name, err := os.Hostname(); err == nil {
		return name
	}
	return "unknown"
}
