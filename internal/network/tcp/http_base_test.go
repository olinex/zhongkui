package tcp

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	_ "zhongkui.com/zhongkui/internal/network/detectors"
)

func TestHttpBaseReassembly(t *testing.T) {
	Convey("Test http_base reassembly vs tshark", t, func() {
		Convey("Should match tshark output", func(c C) {
			verifyReassemblyAgainstTshark(t, c, "testdata/http_base.pcap", "testdata/http_base")
		})
	})
}

func BenchmarkHttpCapFile(b *testing.B) {
	for i := 0; i < b.N; i++ {
		runRouter("testdata/http_base.pcap")
	}
}
