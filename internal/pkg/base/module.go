// 本文件提供项目模块导入路径的运行时获取：该值由构建期写入二进制，在进程生命周期内恒定，
// 故惰性求值并缓存。以 `go run <文件>` 方式构建时构建信息不含主模块路径，此时返回空字符串；
// 此处刻意不做文件系统猜测（如向上查找 go.mod）——那样只在源码环境下成立，二进制分发后
// 当前目录未必属于本项目，命中无关 go.mod 反而会给出误导性的模块路径。
package base

import (
	"runtime/debug"
	"sync"
)

// modulePath 缓存模块路径的求值结果：值为进程级常量，无需重复求值。
var modulePath = sync.OnceValue(func() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	return info.Main.Path
})

// ModulePath 返回构建期写入二进制的本项目模块导入路径（如 zhongkui.com/zhongkui）。
// 构建信息不可用时返回空字符串，由调用方自行决定降级或报错，本包不提供文件系统回退。
func ModulePath() string {
	return modulePath()
}
