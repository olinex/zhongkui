package commentlint

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// TestAnalyzer 校验检查口径：包注释按目录聚合（含被构建约束排除的文件）、导出声明与
// 结构体字段（含非导出字段）、接口方法、comment tag 与行尾注释豁免、生成文件整体跳过、
// 正文提及生成标记不算生成文件、测试入口函数豁免、panic 消息英文约束。
func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), Analyzer, "a", "b", "c", "d", "e", "f", "g")
}
