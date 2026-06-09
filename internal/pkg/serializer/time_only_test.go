package serializer

import (
	"encoding/json"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTimeOnly_BasicMethods(t *testing.T) {
	Convey("Test TimeOnly basic methods", t, func() {
		Convey("String method", func() {
			timeObj := Time(14, 30, 45, 123456789)
			So(timeObj.String(), ShouldEqual, "14:30:45")
		})

		Convey("Format method", func() {
			timeObj := Time(14, 30, 45, 123456789)
			So(timeObj.Format("15:04"), ShouldEqual, "14:30")
		})

		Convey("IsZero method", func() {
			timeObj := Time(14, 30, 45, 123456789)
			So(timeObj.IsZero(), ShouldBeFalse)
			zeroTime := TimeOnly{}
			So(zeroTime.IsZero(), ShouldBeTrue)
		})

		Convey("Hour, Minute, Second, Nanosecond methods", func() {
			timeObj := Time(14, 30, 45, 123456789)
			So(timeObj.Hour(), ShouldEqual, 14)
			So(timeObj.Minute(), ShouldEqual, 30)
			So(timeObj.Second(), ShouldEqual, 45)
			So(timeObj.Nanosecond(), ShouldEqual, 123456789)
		})

		Convey("Add method", func() {
			timeObj := Time(14, 30, 45, 123456789)
			addedTime := timeObj.Add(30 * time.Minute)
			So(addedTime.String(), ShouldEqual, "15:00:45")
		})

		Convey("Sub method", func() {
			timeObj := Time(14, 30, 45, 123456789)
			addedTime := timeObj.Add(30 * time.Minute)
			diff := addedTime.Sub(timeObj)
			So(diff, ShouldEqual, 30*time.Minute)
		})

		Convey("Before method", func() {
			timeObj := Time(14, 30, 45, 123456789)
			addedTime := timeObj.Add(30 * time.Minute)
			So(timeObj.Before(addedTime), ShouldBeTrue)
			So(addedTime.Before(timeObj), ShouldBeFalse)
		})

		Convey("After method", func() {
			timeObj := Time(14, 30, 45, 123456789)
			addedTime := timeObj.Add(30 * time.Minute)
			So(addedTime.After(timeObj), ShouldBeTrue)
			So(timeObj.After(addedTime), ShouldBeFalse)
		})

		Convey("Equal method", func() {
			timeObj := Time(14, 30, 45, 123456789)
			addedTime := timeObj.Add(30 * time.Minute)
			So(timeObj.Equal(timeObj), ShouldBeTrue)
			So(timeObj.Equal(addedTime), ShouldBeFalse)
		})
	})
}

func TestTimeOnly_JSON(t *testing.T) {
	Convey("Test TimeOnly JSON serialization", t, func() {
		Convey("JSON Marshal", func() {
			timeObj := Time(14, 30, 45, 0)
			jsonData, err := json.Marshal(timeObj)
			So(err, ShouldBeNil)
			So(string(jsonData), ShouldEqual, "\"14:30:45\"")
		})

		Convey("JSON Marshal zero value", func() {
			zeroTime := TimeOnly{}
			jsonData, err := json.Marshal(zeroTime)
			So(err, ShouldBeNil)
			So(string(jsonData), ShouldEqual, "null")
		})

		Convey("JSON Unmarshal", func() {
			var unmarshaledTime TimeOnly
			err := json.Unmarshal([]byte("\"14:30:45\""), &unmarshaledTime)
			So(err, ShouldBeNil)
			So(unmarshaledTime.String(), ShouldEqual, "14:30:45")
		})

		Convey("Unmarshal null", func() {
			var unmarshaledTime TimeOnly
			err := json.Unmarshal([]byte("null"), &unmarshaledTime)
			So(err, ShouldBeNil)
		})

		Convey("Unmarshal empty string", func() {
			var unmarshaledTime TimeOnly
			err := json.Unmarshal([]byte("\"\""), &unmarshaledTime)
			So(err, ShouldBeNil)
		})
	})
}

func TestTimeOnly_Database(t *testing.T) {
	Convey("Test TimeOnly database methods", t, func() {
		Convey("Value method", func() {
			timeObj := Time(14, 30, 45, 0)
			value, err := timeObj.Value()
			So(err, ShouldBeNil)
			So(value, ShouldEqual, "14:30:45")
		})

		Convey("Value method with zero value", func() {
			zeroTime := TimeOnly{}
			value, err := zeroTime.Value()
			So(err, ShouldBeNil)
			So(value, ShouldBeNil)
		})

		Convey("Scan from time.Time", func() {
			timeValue := time.Date(2023, 12, 25, 14, 30, 45, 0, time.UTC)
			var scannedTime TimeOnly
			err := scannedTime.Scan(timeValue)
			So(err, ShouldBeNil)
			So(scannedTime.String(), ShouldEqual, "14:30:45")
		})

		Convey("Scan from string", func() {
			var scannedTime TimeOnly
			err := scannedTime.Scan("14:30:45")
			So(err, ShouldBeNil)
			So(scannedTime.String(), ShouldEqual, "14:30:45")
		})

		Convey("Scan from nil", func() {
			var scannedTime TimeOnly
			err := scannedTime.Scan(nil)
			So(err, ShouldBeNil)
		})

		Convey("Scan unsupported type", func() {
			var scannedTime TimeOnly
			err := scannedTime.Scan(123)
			So(err, ShouldNotBeNil)
		})
	})
}

func TestTimeOnly_Parse(t *testing.T) {
	Convey("Test TimeOnly Parse", t, func() {
		Convey("Parse full format", func() {
			timeObj, err := ParseTimeOnly("14:30:45")
			So(err, ShouldBeNil)
			So(timeObj.String(), ShouldEqual, "14:30:45")
		})

		Convey("Parse short format", func() {
			timeObj, err := ParseTimeOnly("14:30")
			So(err, ShouldBeNil)
			So(timeObj.String(), ShouldEqual, "14:30:00")
		})

		Convey("Parse 12-hour format with seconds", func() {
			timeObj, err := ParseTimeOnly("2:30:45 PM")
			So(err, ShouldBeNil)
			So(timeObj.String(), ShouldEqual, "14:30:45")
		})

		Convey("Parse 12-hour format without seconds", func() {
			timeObj, err := ParseTimeOnly("2:30 PM")
			So(err, ShouldBeNil)
			So(timeObj.String(), ShouldEqual, "14:30:00")
		})

		Convey("Parse compact format", func() {
			timeObj, err := ParseTimeOnly("143045")
			So(err, ShouldBeNil)
			So(timeObj.String(), ShouldEqual, "14:30:45")
		})

		Convey("Parse short compact format", func() {
			timeObj, err := ParseTimeOnly("1430")
			So(err, ShouldBeNil)
			So(timeObj.String(), ShouldEqual, "14:30:00")
		})

		Convey("Parse invalid format", func() {
			_, err := ParseTimeOnly("invalid")
			So(err, ShouldNotBeNil)
		})
	})
}

func TestNowTimeOnly(t *testing.T) {
	Convey("Test NowTimeOnly", t, func() {
		Convey("Returns current time", func() {
			now := NowTimeOnly()
			So(now.IsZero(), ShouldBeFalse)
			currentTime := time.Now()
			expectedTime := Time(currentTime.Hour(), currentTime.Minute(), currentTime.Second(), currentTime.Nanosecond())
			So(now.String(), ShouldEqual, expectedTime.String())
		})
	})
}
