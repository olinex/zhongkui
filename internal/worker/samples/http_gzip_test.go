package samples

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	_ "zhongkui.com/zhongkui/internal/network/detectors"
)

func TestHttpGzipReassembly(t *testing.T) {
	Convey("Test http_gzip reassembly vs tshark", t, func() {
		Convey("Should match tshark output", func(c C) {
			verifyReassemblyAgainstTshark(t, c, "http_gzip.pcap", "http_gzip")
		})
	})
}
