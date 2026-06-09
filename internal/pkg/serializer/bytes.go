package serializer

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"

	"zhongkui.com/zhongkui/internal/pkg/errors"
)

const (
	byteSegmentsMagicNumber uint64 = 0x5d6946ead8466e02
)

type ByteSegments struct {
	Segments  [][]byte `json:"segments"`
	MaxLength uint32   `json:"max_length"`
}

func NewByteSegments(maxLength uint32) (*ByteSegments, error) {
	return &ByteSegments{
		Segments:  make([][]byte, 0),
		MaxLength: maxLength,
	}, nil
}

func (b *ByteSegments) Length() uint32 {
	return uint32(len(b.Segments))
}

func (b *ByteSegments) Append(segment []byte) error {
	if uint64(len(segment)) > uint64(b.MaxLength) {
		return errors.Newf(`Segment length %d out of range [0, %d)`, len(segment), b.MaxLength)
	}
	b.Segments = append(b.Segments, segment)
	return nil
}

func (b *ByteSegments) GetSegment(index int) []byte {
	if index < 0 || index >= len(b.Segments) {
		return nil
	}
	return b.Segments[index]
}

func (b *ByteSegments) ConcatSegments() (*bytes.Buffer, error) {
	buffer := bytes.NewBuffer(nil)
	for _, segment := range b.Segments {
		if written, err := buffer.Write(segment); err != nil {
			return buffer, errors.WithStack(err)
		} else if written != len(segment) {
			return buffer, errors.Newf(
				`Cannot write all segment bytes to buffer, expect write %d bytes, actual %d`,
				written,
				len(segment),
			)
		}
	}
	return buffer, nil
}

func (b *ByteSegments) Marshal() (*bytes.Buffer, error) {
	buffer := bytes.NewBuffer(nil)
	if err := binary.Write(buffer, binary.BigEndian, byteSegmentsMagicNumber); err != nil {
		return buffer, errors.WithStack(err)
	}
	if err := binary.Write(buffer, binary.BigEndian, b.MaxLength); err != nil {
		return buffer, errors.WithStack(err)
	}
	if err := binary.Write(buffer, binary.BigEndian, uint32(len(b.Segments))); err != nil {
		return buffer, errors.WithStack(err)
	}
	for _, segment := range b.Segments {
		length := len(segment)
		if err := binary.Write(buffer, binary.BigEndian, uint32(length)); err != nil {
			return buffer, errors.WithStack(err)
		}
		if written, err := buffer.Write(segment); err != nil {
			return buffer, errors.WithStack(err)
		} else if written != length {
			return buffer, errors.Newf(
				`Cannot write all segment bytes to buffer, expect write %d bytes, actual %d`,
				written,
				len(segment),
			)
		}
	}
	return buffer, nil
}

func (b *ByteSegments) Unmarshal(raw []byte) error {
	var offset uint64 = 0
	magicNumber := binary.BigEndian.Uint64(raw[offset:])
	if magicNumber != byteSegmentsMagicNumber {
		return errors.Newf(`Bad magic number, except %X, actual %X`, byteSegmentsMagicNumber, magicNumber)
	}
	offset += 8
	maxLength := uint64(binary.BigEndian.Uint32(raw[offset:]))
	offset += 4
	count := binary.BigEndian.Uint32(raw[offset:])
	offset += 4
	segments := make([][]byte, 0, count)
	for i := 0; uint32(i) < count; i++ {
		length := uint64(binary.BigEndian.Uint32(raw[offset:]))
		if length > maxLength {
			return errors.Newf(`Bad length %d receive from data[%d:%d]`, length, offset, offset+4)
		}
		offset += 4
		segments = append(segments, raw[offset:offset+length])
		offset += length
	}
	b.MaxLength = uint32(maxLength)
	b.Segments = segments
	return nil
}

func (b ByteSegments) MarshalJSON() ([]byte, error) {
	buffer, err := b.Marshal()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	bytes := buffer.Bytes()
	return json.Marshal(base64.RawStdEncoding.EncodeToString(bytes))
}

func (b *ByteSegments) UnmarshalJSON(data []byte) error {
	var hexStr string
	err := json.Unmarshal(data, &hexStr)
	if err != nil {
		return errors.WithStack(err)
	}
	bytes, err := base64.RawStdEncoding.DecodeString(hexStr)
	if err != nil {
		return errors.WithStack(err)
	}
	if b == nil {
		tmp, err := NewByteSegments(0)
		if err != nil {
			return errors.WithStack(err)
		}
		b = tmp
	}
	return errors.WithStack(b.Unmarshal(bytes))
}
