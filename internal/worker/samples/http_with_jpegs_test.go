package samples

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	_ "zhongkui.com/zhongkui/internal/network/detectors"
)

func TestHttpWithJpegsReassembly(t *testing.T) {
	Convey("Test http_with_jpegs reassembly vs tshark", t, func() {
		Convey("Should match tshark output", func(c C) {
			verifyReassemblyAgainstTshark(t, c, "http_with_jpegs.pcap", "http_with_jpegs")
		})
	})
}
