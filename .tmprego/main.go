package main

import (
"fmt"

"zhongkui.com/zhongkui/internal/tagengine"
)

func main() {
for _, src := range []string{"脚本文件", "package zki\n\ntags contains 脚本文件 if {\n}"} {
errs, ok := tagengine.InspectRego(src)
fmt.Printf("src=%q ok=%v errs=%+v\n", src, ok, errs)
}
}
