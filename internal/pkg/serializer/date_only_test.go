package serializer

import (
	"encoding/json"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDateOnly_BasicMethods(t *testing.T) {
	Convey("Test DateOnly basic methods", t, func() {
		Convey("String method", func() {
			date := Date(2023, 12, 25)
			So(date.String(), ShouldEqual, "2023-12-25")
		})

		Convey("Format method", func() {
			date := Date(2023, 12, 25)
			So(date.Format("2006/01/02"), ShouldEqual, "2023/12/25")
		})

		Convey("IsZero method", func() {
			date := Date(2023, 12, 25)
			So(date.IsZero(), ShouldBeFalse)
			zeroDate := DateOnly{}
			So(zeroDate.IsZero(), ShouldBeTrue)
		})

		Convey("AddDays method", func() {
			date := Date(2023, 12, 25)
			nextDay := date.AddDays(1)
			So(nextDay.String(), ShouldEqual, "2023-12-26")
		})

		Convey("Sub method", func() {
			date := Date(2023, 12, 25)
			nextDay := date.AddDays(1)
			diff := nextDay.Sub(date)
			So(diff, ShouldEqual, 1)
		})

		Convey("Year, Month, Day methods", func() {
			date := Date(2023, 12, 25)
			So(date.Year(), ShouldEqual, 2023)
			So(date.Month(), ShouldEqual, time.December)
			So(date.Day(), ShouldEqual, 25)
		})

		Convey("Weekday method", func() {
			date := Date(2023, 12, 25)
			So(date.Weekday(), ShouldEqual, time.Monday)
		})
	})
}

func TestDateOnly_JSON(t *testing.T) {
	Convey("Test DateOnly JSON serialization", t, func() {
		Convey("JSON Marshal", func() {
			date := Date(2023, 12, 25)
			jsonData, err := json.Marshal(date)
			So(err, ShouldBeNil)
			So(string(jsonData), ShouldEqual, "\"2023-12-25\"")
		})

		Convey("JSON Marshal zero value", func() {
			zeroDate := DateOnly{}
			jsonData, err := json.Marshal(zeroDate)
			So(err, ShouldBeNil)
			So(string(jsonData), ShouldEqual, "null")
		})

		Convey("JSON Unmarshal", func() {
			var unmarshaledDate DateOnly
			err := json.Unmarshal([]byte("\"2023-12-25\""), &unmarshaledDate)
			So(err, ShouldBeNil)
			So(unmarshaledDate.String(), ShouldEqual, "2023-12-25")
		})

		Convey("Unmarshal null", func() {
			var unmarshaledDate DateOnly
			err := json.Unmarshal([]byte("null"), &unmarshaledDate)
			So(err, ShouldBeNil)
		})

		Convey("Unmarshal empty string", func() {
			var unmarshaledDate DateOnly
			err := json.Unmarshal([]byte("\"\""), &unmarshaledDate)
			So(err, ShouldBeNil)
		})
	})
}

func TestDateOnly_Database(t *testing.T) {
	Convey("Test DateOnly database methods", t, func() {
		Convey("Value method", func() {
			date := Date(2023, 12, 25)
			value, err := date.Value()
			So(err, ShouldBeNil)
			So(value, ShouldEqual, "2023-12-25")
		})

		Convey("Value method with zero value", func() {
			zeroDate := DateOnly{}
			value, err := zeroDate.Value()
			So(err, ShouldBeNil)
			So(value, ShouldBeNil)
		})

		Convey("Scan from time.Time", func() {
			timeValue := time.Date(2023, 12, 25, 10, 30, 0, 0, time.UTC)
			var scannedDate DateOnly
			err := scannedDate.Scan(timeValue)
			So(err, ShouldBeNil)
			So(scannedDate.String(), ShouldEqual, "2023-12-25")
		})

		Convey("Scan from string", func() {
			var scannedDate DateOnly
			err := scannedDate.Scan("2023-12-25")
			So(err, ShouldBeNil)
			So(scannedDate.String(), ShouldEqual, "2023-12-25")
		})

		Convey("Scan from nil", func() {
			var scannedDate DateOnly
			err := scannedDate.Scan(nil)
			So(err, ShouldBeNil)
		})

		Convey("Scan unsupported type", func() {
			var scannedDate DateOnly
			err := scannedDate.Scan(123)
			So(err, ShouldNotBeNil)
		})
	})
}

func TestDateOnly_Parse(t *testing.T) {
	Convey("Test DateOnly Parse", t, func() {
		Convey("Parse ISO format", func() {
			date, err := ParseDateOnly("2023-12-25")
			So(err, ShouldBeNil)
			So(date.String(), ShouldEqual, "2023-12-25")
		})

		Convey("Parse short ISO format", func() {
			date, err := ParseDateOnly("2023-1-2")
			So(err, ShouldBeNil)
			So(date.String(), ShouldEqual, "2023-01-02")
		})

		Convey("Parse slash format", func() {
			date, err := ParseDateOnly("2023/12/25")
			So(err, ShouldBeNil)
			So(date.String(), ShouldEqual, "2023-12-25")
		})

		Convey("Parse short slash format", func() {
			date, err := ParseDateOnly("2023/1/2")
			So(err, ShouldBeNil)
			So(date.String(), ShouldEqual, "2023-01-02")
		})

		Convey("Parse US format", func() {
			date, err := ParseDateOnly("12/25/2023")
			So(err, ShouldBeNil)
			So(date.String(), ShouldEqual, "2023-12-25")
		})

		Convey("Parse short US format", func() {
			date, err := ParseDateOnly("1/2/2023")
			So(err, ShouldBeNil)
			So(date.String(), ShouldEqual, "2023-01-02")
		})

		Convey("Parse compact format", func() {
			date, err := ParseDateOnly("20231225")
			So(err, ShouldBeNil)
			So(date.String(), ShouldEqual, "2023-12-25")
		})

		Convey("Parse invalid format", func() {
			_, err := ParseDateOnly("invalid")
			So(err, ShouldNotBeNil)
		})
	})
}

func TestNowDateOnly(t *testing.T) {
	Convey("Test NowDateOnly", t, func() {
		Convey("Returns current date", func() {
			now := NowDateOnly()
			So(now.IsZero(), ShouldBeFalse)
			today := time.Now()
			expectedDate := Date(today.Year(), today.Month(), today.Day())
			So(now.String(), ShouldEqual, expectedDate.String())
		})
	})
}
