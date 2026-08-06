package tcp

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	_ "zhongkui.com/zhongkui/internal/network/detectors"
)

func TestRustfsReassembly(t *testing.T) {
	Convey("Test rustfs HTTP reassembly vs tshark", t, func() {
		Convey("Should match tshark output", func(c C) {
			verifyReassemblyAgainstTshark(t, c, "testdata/rustfs.pcapng", "testdata/rustfs")
		})
	})
}
