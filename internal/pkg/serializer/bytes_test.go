package serializer

import (
	"encoding/binary"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestByteSegments_Basic(t *testing.T) {
	Convey("Basic Append/Get/Length/Concat", t, func() {
		b, err := NewByteSegments(100)
		So(err, ShouldBeNil)

		err = b.Append([]byte("hello"))
		So(err, ShouldBeNil)
		err = b.Append([]byte("world"))
		So(err, ShouldBeNil)

		So(b.Length(), ShouldEqual, 2)
		So(b.GetSegment(0), ShouldResemble, []byte("hello"))

		buf, err := b.ConcatSegments()
		So(err, ShouldBeNil)
		So(buf.Bytes(), ShouldResemble, []byte("helloworld"))
	})
}

func TestByteSegments_AppendErrorsAndGet(t *testing.T) {
	Convey("Append too long and GetSegment out of range", t, func() {
		b, _ := NewByteSegments(2)
		err := b.Append([]byte{1, 2, 3})
		So(err, ShouldNotBeNil)

		// out of range
		b2, _ := NewByteSegments(10)
		b2.Append([]byte("x"))
		So(b2.GetSegment(-1), ShouldBeNil)
		So(b2.GetSegment(10), ShouldBeNil)
	})
}

func TestByteSegments_MarshalUnmarshalRoundtrip(t *testing.T) {
	Convey("Marshal and Unmarshal roundtrip", t, func() {
		b, _ := NewByteSegments(1024)
		_ = b.Append([]byte("a"))
		_ = b.Append([]byte(""))
		_ = b.Append([]byte("xyz"))

		buf, err := b.Marshal()
		So(err, ShouldBeNil)
		So(binary.BigEndian.Uint64(buf.Bytes()[:8]), ShouldEqual, byteSegmentsMagicNumber)

		c, _ := NewByteSegments(1)
		err = c.Unmarshal(buf.Bytes())
		So(err, ShouldBeNil)
		So(c.MaxLength, ShouldEqual, b.MaxLength)
		So(c.Length(), ShouldEqual, b.Length())
		for i := 0; i < int(b.Length()); i++ {
			So(c.GetSegment(i), ShouldResemble, b.GetSegment(i))
		}
	})
}

func TestByteSegments_UnmarshalErrors(t *testing.T) {
	Convey("Unmarshal with bad magic should return error", t, func() {
		b, _ := NewByteSegments(10)
		_ = b.Append([]byte("x"))
		buf, _ := b.Marshal()
		data := make([]byte, len(buf.Bytes()))
		copy(data, buf.Bytes())
		data[0] ^= 0xFF

		c, _ := NewByteSegments(10)
		err := c.Unmarshal(data)
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "Bad magic")
	})

	Convey("Unmarshal with segment length bigger than maxLength should return error", t, func() {
		b, _ := NewByteSegments(1024)
		_ = b.Append([]byte("hello"))
		buf, _ := b.Marshal()
		data := make([]byte, len(buf.Bytes()))
		copy(data, buf.Bytes())
		// set maxLength to 1
		binary.BigEndian.PutUint32(data[8:12], 1)

		c, _ := NewByteSegments(10)
		err := c.Unmarshal(data)
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "Bad length")
	})

	Convey("Unmarshal with too-short data should panic", t, func() {
		c, _ := NewByteSegments(1)
		So(func() { c.Unmarshal([]byte{}) }, ShouldPanic)
	})
}
