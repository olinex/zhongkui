package tcp

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	_ "zhongkui.com/zhongkui/internal/network/detectors"
)

func TestHttpGzipReassembly(t *testing.T) {
	Convey("Test http_gzip reassembly vs tshark", t, func() {
		Convey("Should match tshark output", func(c C) {
			verifyReassemblyAgainstTshark(t, c, "testdata/http_gzip.pcap", "testdata/http_gzip")
		})
	})
}
