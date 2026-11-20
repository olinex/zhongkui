package serializer

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"gorm.io/gorm/schema"
)

// S3CryptoTestModel 用于 GORM schema 解析，获取带 s3crypto 标签的 Field。
type S3CryptoTestModel struct {
	// Body 带 s3crypto 标签的对象键字段，用于验证 GORM 序列化器解析出的字段标签。
	Body S3Crypto `gorm:"serializer:s3crypto" s3crypto:"offset=44001,length=32"`
}

// getS3CryptoField 从 S3CryptoTestModel 中解析出 schema.Field。
func getS3CryptoField() *schema.Field {
	s, err := schema.Parse(&S3CryptoTestModel{}, &sync.Map{}, schema.NamingStrategy{})
	So(err, ShouldBeNil)
	field := s.FieldsByDBName["body"]
	So(field, ShouldNotBeNil)
	return field
}

func TestS3CryptoSerializer_Scan(t *testing.T) {
	Convey("Test S3CryptoSerializer.Scan", t, func() {
		ss := S3CryptoSerializer{}
		field := getS3CryptoField()
		ctx := context.Background()

		Convey("string dbValue is wrapped", func() {
			model := &S3CryptoTestModel{}
			dst := reflect.ValueOf(model).Elem()
			So(ss.Scan(ctx, field, dst, "body/request/abc.zkcn"), ShouldBeNil)
			So(model.Body.Key(), ShouldEqual, "body/request/abc.zkcn")
		})

		Convey("[]byte dbValue is wrapped (Postgres VARCHAR)", func() {
			model := &S3CryptoTestModel{}
			dst := reflect.ValueOf(model).Elem()
			So(ss.Scan(ctx, field, dst, []byte("body/response/def.zkcn")), ShouldBeNil)
			So(model.Body.Key(), ShouldEqual, "body/response/def.zkcn")
		})

		Convey("nil dbValue resets to zero value", func() {
			model := &S3CryptoTestModel{Body: NewS3Crypto("old")}
			dst := reflect.ValueOf(model).Elem()
			So(ss.Scan(ctx, field, dst, nil), ShouldBeNil)
			So(model.Body.Key(), ShouldEqual, "")
		})

		Convey("invalid dbValue type fails", func() {
			model := &S3CryptoTestModel{}
			dst := reflect.ValueOf(model).Elem()
			err := ss.Scan(ctx, field, dst, 12345)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "s3crypto: expected []byte or string")
		})
	})
}

func TestS3CryptoSerializer_Value(t *testing.T) {
	Convey("Test S3CryptoSerializer.Value", t, func() {
		ss := S3CryptoSerializer{}
		field := getS3CryptoField()
		ctx := context.Background()

		Convey("valid S3Crypto returns object key", func() {
			model := &S3CryptoTestModel{Body: NewS3Crypto("body/request/abc.zkcn")}
			dst := reflect.ValueOf(model).Elem()
			result, err := ss.Value(ctx, field, dst, model.Body)
			So(err, ShouldBeNil)
			So(result, ShouldEqual, "body/request/abc.zkcn")
		})

		Convey("zero S3Crypto returns empty string (not nil)", func() {
			model := &S3CryptoTestModel{}
			dst := reflect.ValueOf(model).Elem()
			result, err := ss.Value(ctx, field, dst, model.Body)
			So(err, ShouldBeNil)
			So(result, ShouldEqual, "")
		})

		Convey("nil fieldValue returns empty string", func() {
			model := &S3CryptoTestModel{}
			dst := reflect.ValueOf(model).Elem()
			result, err := ss.Value(ctx, field, dst, nil)
			So(err, ShouldBeNil)
			So(result, ShouldEqual, "")
		})

		Convey("non-S3Crypto fieldValue fails", func() {
			model := &S3CryptoTestModel{}
			dst := reflect.ValueOf(model).Elem()
			_, err := ss.Value(ctx, field, dst, "plain-string")
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "s3crypto: field value must be serializer.S3Crypto")
		})
	})
}

func TestS3CryptoSerializer_Binding(t *testing.T) {
	Convey("Test S3CryptoSerializer tag binding", t, func() {
		ss := S3CryptoSerializer{}
		ctx := context.Background()

		Convey("wrong field type fails", func() {
			type BadModel struct {
				Body string `gorm:"serializer:s3crypto" s3crypto:"offset=44001,length=32"`
			}
			s, err := schema.Parse(&BadModel{}, &sync.Map{}, schema.NamingStrategy{})
			So(err, ShouldBeNil)
			field := s.FieldsByDBName["body"]
			So(field, ShouldNotBeNil)
			model := &BadModel{}
			dst := reflect.ValueOf(model).Elem()
			_, err = ss.Value(ctx, field, dst, model.Body)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "s3crypto: field Body must be serializer.S3Crypto")
		})

		Convey("missing s3crypto tag fails", func() {
			type BadModel struct {
				Body S3Crypto `gorm:"serializer:s3crypto"`
			}
			s, err := schema.Parse(&BadModel{}, &sync.Map{}, schema.NamingStrategy{})
			So(err, ShouldBeNil)
			field := s.FieldsByDBName["body"]
			So(field, ShouldNotBeNil)
			model := &BadModel{}
			dst := reflect.ValueOf(model).Elem()
			_, err = ss.Value(ctx, field, dst, model.Body)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, `must carry a "s3crypto" tag`)
		})

		Convey("malformed s3crypto tag fails", func() {
			type BadModel struct {
				Body S3Crypto `gorm:"serializer:s3crypto" s3crypto:"offset=bad,length=32"`
			}
			s, err := schema.Parse(&BadModel{}, &sync.Map{}, schema.NamingStrategy{})
			So(err, ShouldBeNil)
			field := s.FieldsByDBName["body"]
			So(field, ShouldNotBeNil)
			model := &BadModel{}
			dst := reflect.ValueOf(model).Elem()
			_, err = ss.Value(ctx, field, dst, model.Body)
			So(err, ShouldNotBeNil)
		})
	})
}

func TestS3CryptoSerializer_Roundtrip(t *testing.T) {
	Convey("Test S3Crypto Scan/Value roundtrip", t, func() {
		ss := S3CryptoSerializer{}
		field := getS3CryptoField()
		ctx := context.Background()

		Convey("object key survives write then read", func() {
			source := &S3CryptoTestModel{Body: NewS3Crypto("body/request/roundtrip.zkcn")}
			dst := reflect.ValueOf(source).Elem()
			raw, err := ss.Value(ctx, field, dst, source.Body)
			So(err, ShouldBeNil)

			target := &S3CryptoTestModel{}
			targetDst := reflect.ValueOf(target).Elem()
			So(ss.Scan(ctx, field, targetDst, raw), ShouldBeNil)
			So(target.Body.Key(), ShouldEqual, "body/request/roundtrip.zkcn")
		})
	})
}

func TestS3CryptoJSON(t *testing.T) {
	Convey("Test S3Crypto JSON", t, func() {
		Convey("MarshalJSON outputs object key string", func() {
			data, err := json.Marshal(NewS3Crypto("body/request/abc.zkcn"))
			So(err, ShouldBeNil)
			So(string(data), ShouldEqual, `"body/request/abc.zkcn"`)
		})

		Convey("zero value marshals to empty string", func() {
			data, err := json.Marshal(S3Crypto{})
			So(err, ShouldBeNil)
			So(string(data), ShouldEqual, `""`)
		})

		Convey("UnmarshalJSON accepts object key string", func() {
			var s S3Crypto
			So(json.Unmarshal([]byte(`"body/request/abc.zkcn"`), &s), ShouldBeNil)
			So(s.Key(), ShouldEqual, "body/request/abc.zkcn")
		})

		Convey("UnmarshalJSON treats null / {} as zero value", func() {
			var s S3Crypto
			So(json.Unmarshal([]byte(`null`), &s), ShouldBeNil)
			So(s.Key(), ShouldEqual, "")

			s = NewS3Crypto("old")
			So(json.Unmarshal([]byte(`{}`), &s), ShouldBeNil)
			So(s.Key(), ShouldEqual, "")
		})
	})
}
