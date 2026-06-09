package serializer

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLocation_ToSTD(t *testing.T) {
	Convey("Test Location ToSTD", t, func() {
		Convey("Valid timezone", func() {
			loc := Location("Asia/Shanghai")
			timeLoc, err := loc.ToSTD()
			So(err, ShouldBeNil)
			So(timeLoc, ShouldNotBeNil)
		})

		Convey("Invalid timezone", func() {
			loc := Location("Invalid/Timezone")
			timeLoc, err := loc.ToSTD()
			So(err, ShouldNotBeNil)
			So(timeLoc, ShouldBeNil)
		})
	})
}
