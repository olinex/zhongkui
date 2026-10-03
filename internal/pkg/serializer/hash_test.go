package serializer

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"gorm.io/gorm/schema"
)

// HashTestModel 用于 GORM schema 解析，获取带 hmac-sha256 secure 标签的 Hash 字段。
type HashTestModel struct {
	// Digest 摘要字段，供 HashSerializer 的标签校验与 Scan/Value 往返验证。
	Digest Hash `gorm:"type:CHAR(64);serializer:hash" secure:"mode=\"hmac-sha256\",offset=12804,length=32"`
}

// getHashField 从 HashTestModel 中解析出 schema.Field。
func getHashField() *schema.Field {
	s, err := schema.Parse(&HashTestModel{}, &sync.Map{}, schema.NamingStrategy{})
	So(err, ShouldBeNil)
	field := s.FieldsByDBName["digest"]
	So(field, ShouldNotBeNil)
	return field
}

func TestHash_Accessors(t *testing.T) {
	Convey("Test Hash accessors", t, func() {
		Convey("zero value returns empty digest", func() {
			var h Hash
			So(h.Digest(), ShouldEqual, "")
			So(h.String(), ShouldEqual, "")
		})

		Convey("NewHashDigest keeps digest", func() {
			h := NewHashDigest("abc123")
			So(h.Digest(), ShouldEqual, "abc123")
			So(h.String(), ShouldEqual, "abc123")
		})
	})
}

func TestHash_JSON(t *testing.T) {
	Convey("Test Hash JSON roundtrip", t, func() {
		Convey("marshal outputs plain string", func() {
			data, err := json.Marshal(NewHashDigest("abc123"))
			So(err, ShouldBeNil)
			So(string(data), ShouldEqual, `"abc123"`)
		})

		Convey("zero value marshals to empty string", func() {
			data, err := json.Marshal(Hash{})
			So(err, ShouldBeNil)
			So(string(data), ShouldEqual, `""`)
		})

		Convey("unmarshal string restores digest", func() {
			var h Hash
			So(json.Unmarshal([]byte(`"deadbeef"`), &h), ShouldBeNil)
			So(h.Digest(), ShouldEqual, "deadbeef")
		})

		Convey("unmarshal null resets to zero value", func() {
			h := NewHashDigest("old")
			So(json.Unmarshal([]byte(`null`), &h), ShouldBeNil)
			So(h.Digest(), ShouldEqual, "")
		})

		Convey("unmarshal empty string resets to zero value", func() {
			h := NewHashDigest("old")
			So(json.Unmarshal([]byte(`""`), &h), ShouldBeNil)
			So(h.Digest(), ShouldEqual, "")
		})

		Convey("unmarshal non-string fails", func() {
			var h Hash
			So(json.Unmarshal([]byte(`123`), &h), ShouldNotBeNil)
		})
	})
}

func TestHashSerializer_validateTag(t *testing.T) {
	Convey("Test HashSerializer.validateTag", t, func() {
		hs := HashSerializer{}

		Convey("valid hmac-sha256 tag", func() {
			cTag, err := hs.validateTag(getHashField())
			So(err, ShouldBeNil)
			So(cTag.Mode, ShouldEqual, "hmac-sha256")
		})

		Convey("aes-gcm tag is rejected", func() {
			type AesModel struct {
				Digest Hash `gorm:"serializer:hash" secure:"mode=\"aes-gcm\",offset=12804,length=32"`
			}
			s, err := schema.Parse(&AesModel{}, &sync.Map{}, schema.NamingStrategy{})
			So(err, ShouldBeNil)
			field := s.FieldsByDBName["digest"]
			So(field, ShouldNotBeNil)

			_, err = hs.validateTag(field)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "hash: field mode must be")
		})

		Convey("invalid tag format is rejected", func() {
			type BadModel struct {
				Digest Hash `gorm:"serializer:hash" secure:"invalid_tag"`
			}
			s, err := schema.Parse(&BadModel{}, &sync.Map{}, schema.NamingStrategy{})
			So(err, ShouldBeNil)
			field := s.FieldsByDBName["digest"]
			So(field, ShouldNotBeNil)

			_, err = hs.validateTag(field)
			So(err, ShouldNotBeNil)
		})
	})
}

func TestHashSerializer_Scan(t *testing.T) {
	Convey("Test HashSerializer.Scan", t, func() {
		hs := HashSerializer{}
		field := getHashField()
		ctx := context.Background()

		Convey("string dbValue is wrapped", func() {
			model := &HashTestModel{}
			dst := reflect.ValueOf(model).Elem()
			So(hs.Scan(ctx, field, dst, "abc123"), ShouldBeNil)
			So(model.Digest.Digest(), ShouldEqual, "abc123")
		})

		Convey("[]byte dbValue is wrapped (Postgres CHAR/BYTEA)", func() {
			model := &HashTestModel{}
			dst := reflect.ValueOf(model).Elem()
			So(hs.Scan(ctx, field, dst, []byte("deadbeef")), ShouldBeNil)
			So(model.Digest.Digest(), ShouldEqual, "deadbeef")
		})

		Convey("nil dbValue resets to zero value", func() {
			model := &HashTestModel{Digest: NewHashDigest("old")}
			dst := reflect.ValueOf(model).Elem()
			So(hs.Scan(ctx, field, dst, nil), ShouldBeNil)
			So(model.Digest.Digest(), ShouldEqual, "")
		})

		Convey("invalid dbValue type fails", func() {
			model := &HashTestModel{}
			dst := reflect.ValueOf(model).Elem()
			err := hs.Scan(ctx, field, dst, 12345)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "hash: expected []byte or string")
		})
	})
}

func TestHashSerializer_Value(t *testing.T) {
	Convey("Test HashSerializer.Value", t, func() {
		hs := HashSerializer{}
		field := getHashField()
		ctx := context.Background()

		Convey("valid Hash returns digest string", func() {
			model := &HashTestModel{Digest: NewHashDigest("abc123")}
			dst := reflect.ValueOf(model).Elem()
			result, err := hs.Value(ctx, field, dst, model.Digest)
			So(err, ShouldBeNil)
			So(result, ShouldEqual, "abc123")
		})

		Convey("empty Hash returns empty string", func() {
			model := &HashTestModel{}
			dst := reflect.ValueOf(model).Elem()
			result, err := hs.Value(ctx, field, dst, model.Digest)
			So(err, ShouldBeNil)
			So(result, ShouldEqual, "")
		})

		Convey("non-Hash fieldValue fails", func() {
			model := &HashTestModel{}
			dst := reflect.ValueOf(model).Elem()
			_, err := hs.Value(ctx, field, dst, "plain-string")
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "hash: field value must be serializer.Hash")
		})

		Convey("aes-gcm tag is rejected before value handling", func() {
			type AesModel struct {
				Digest Hash `gorm:"serializer:hash" secure:"mode=\"aes-gcm\",offset=12804,length=32"`
			}
			s, err := schema.Parse(&AesModel{}, &sync.Map{}, schema.NamingStrategy{})
			So(err, ShouldBeNil)
			aesField := s.FieldsByDBName["digest"]
			So(aesField, ShouldNotBeNil)

			model := &AesModel{Digest: NewHashDigest("abc123")}
			dst := reflect.ValueOf(model).Elem()
			_, err = hs.Value(ctx, aesField, dst, model.Digest)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "hash: field mode must be")
		})
	})
}

func TestHashSerializer_ScanValueRoundtrip(t *testing.T) {
	Convey("Test Hash Scan/Value roundtrip", t, func() {
		hs := HashSerializer{}
		field := getHashField()
		ctx := context.Background()

		Convey("digest survives write then read", func() {
			write := &HashTestModel{Digest: NewHashDigest("0123456789abcdef")}
			dstWrite := reflect.ValueOf(write).Elem()
			dbValue, err := hs.Value(ctx, field, dstWrite, write.Digest)
			So(err, ShouldBeNil)
			So(strings.TrimSpace(dbValue.(string)), ShouldEqual, "0123456789abcdef")

			read := &HashTestModel{}
			dstRead := reflect.ValueOf(read).Elem()
			So(hs.Scan(ctx, field, dstRead, dbValue), ShouldBeNil)
			So(read.Digest.Digest(), ShouldEqual, write.Digest.Digest())
		})
	})
}
