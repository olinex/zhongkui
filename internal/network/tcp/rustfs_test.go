package tcp

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	_ "zhongkui.com/zhongkui/internal/network/detectors"
)

// TestRustfsReassembly 校验 rustfs 抓包的 HTTP 流重组结果与 tshark 输出一致。
func TestRustfsReassembly(t *testing.T) {
	Convey("Test rustfs HTTP reassembly vs tshark", t, func() {
		Convey("Should match tshark output", func(c C) {
			verifyReassemblyAgainstTshark(t, c, "testdata/rustfs.pcapng", "testdata/rustfs")
		})
	})
}
