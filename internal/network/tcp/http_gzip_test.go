package tcp

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	_ "zhongkuisec.com/zhongkui/internal/network/detectors"
)

// TestHttpGzipReassembly 校验 http_gzip 抓包的流重组结果与 tshark 输出一致。
func TestHttpGzipReassembly(t *testing.T) {
	Convey("Test http_gzip reassembly vs tshark", t, func() {
		Convey("Should match tshark output", func(c C) {
			verifyReassemblyAgainstTshark(t, c, "testdata/http_gzip.pcap", "testdata/http_gzip")
		})
	})
}
