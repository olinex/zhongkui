package tcp

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	_ "zhongkui.com/zhongkui/internal/network/detectors"
)

// TestHttpBaseReassembly 校验 http_base 抓包的流重组结果与 tshark 输出一致。
func TestHttpBaseReassembly(t *testing.T) {
	Convey("Test http_base reassembly vs tshark", t, func() {
		Convey("Should match tshark output", func(c C) {
			verifyReassemblyAgainstTshark(t, c, "testdata/http_base.pcap", "testdata/http_base")
		})
	})
}

// BenchmarkHttpCapFile 对 http_base 抓包文件的流重组流程进行基准测试。
func BenchmarkHttpCapFile(b *testing.B) {
	for i := 0; i < b.N; i++ {
		runRouter("testdata/http_base.pcap")
	}
}
