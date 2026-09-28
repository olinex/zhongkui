package tcp

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	_ "zhongkui.com/zhongkui/internal/network/detectors"
)

// TestHttpWithJpegsReassembly 校验 http_with_jpegs 抓包的流重组结果与 tshark 输出一致。
func TestHttpWithJpegsReassembly(t *testing.T) {
	Convey("Test http_with_jpegs reassembly vs tshark", t, func() {
		Convey("Should match tshark output", func(c C) {
			verifyReassemblyAgainstTshark(t, c, "testdata/http_with_jpegs.pcap", "testdata/http_with_jpegs")
		})
	})
}
