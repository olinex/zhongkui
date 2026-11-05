package bench

import (
	"os"
	"testing"
)

// TestMain 运行基准包测试，并在全部测试结束后打印性能结果表。
func TestMain(m *testing.M) {
	code := m.Run()
	PrintResultsTable()
	os.Exit(code)
}
