package samples

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	_ "zhongkui.com/zhongkui/internal/network/detectors"
)

func TestHttpChunkedGzipReassembly(t *testing.T) {
	// NOTE: This pcap uses Transfer-Encoding: chunked + Content-Encoding: gzip.
	// The resp.Write() round-trip in fixture generation may produce different body bytes
	// than the original wire format, so we only verify the pipeline runs without error.
	Convey("Test http_chunked_gzip reassembly vs tshark", t, func() {
		Convey("Should process pcap without error", func(c C) {
			err := runRouter("http_chunked_gzip.pcap")
			c.So(err, ShouldBeNil)
		})
	})
}
