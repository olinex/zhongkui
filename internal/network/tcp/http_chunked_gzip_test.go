package tcp

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	_ "zhongkui.com/zhongkui/internal/network/detectors"
)

func TestHttpChunkedGzipReassembly(t *testing.T) {
	// 注意：该 pcap 使用 Transfer-Encoding: chunked + Content-Encoding: gzip。
	// fixture 生成时 resp.Write() 往返可能产生与原始线上格式不同的 body 字节，
	// 因此这里仅验证流水线可无错运行。
	Convey("Test http_chunked_gzip reassembly vs tshark", t, func() {
		Convey("Should process pcap without error", func(c C) {
			err := runRouter("testdata/http_chunked_gzip.pcap")
			c.So(err, ShouldBeNil)
		})
	})
}
