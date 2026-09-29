package b_test

import "testing"

func TestValue(t *testing.T) {}

// 本包（package b_test）无包注释：外部测试包与主包同目录，包注释由主包承载，
// 故按目录聚合口径不报告包注释缺口；Test 前缀函数豁免注释检查。
