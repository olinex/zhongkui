package a

import "testing"

// mock 测试替身类型。
type mock struct{}

func (mock) Do() {} // want "导出方法 Do 缺少注释"

// want +2 "导出函数 ExportedHelper 缺少注释"

func ExportedHelper() {}

func TestThing(t *testing.T) {}

func BenchmarkThing(b *testing.B) {}

// 本文件无包注释：同包内任一文件带包注释即视为该包已有包注释（按目录聚合口径）。
