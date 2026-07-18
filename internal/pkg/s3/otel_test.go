package s3

import (
	"net/http"
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestParseS3Path(t *testing.T) {
	convey.Convey("Test parseS3Path", t, func() {
		convey.Convey("should parse standard /bucket/key path", func() {
			bucket, objectKey := parseS3Path("/my-bucket/my-object-key.json")
			convey.So(bucket, convey.ShouldEqual, "my-bucket")
			convey.So(objectKey, convey.ShouldEqual, "my-object-key.json")
		})

		convey.Convey("should parse path with deep nested object key", func() {
			bucket, objectKey := parseS3Path("/my-bucket/a/b/c/d/e/file.txt")
			convey.So(bucket, convey.ShouldEqual, "my-bucket")
			convey.So(objectKey, convey.ShouldEqual, "a/b/c/d/e/file.txt")
		})

		convey.Convey("should return empty strings for root path '/'", func() {
			bucket, objectKey := parseS3Path("/")
			convey.So(bucket, convey.ShouldEqual, "")
			convey.So(objectKey, convey.ShouldEqual, "")
		})

		convey.Convey("should return empty strings for empty path", func() {
			bucket, objectKey := parseS3Path("")
			convey.So(bucket, convey.ShouldEqual, "")
			convey.So(objectKey, convey.ShouldEqual, "")
		})

		convey.Convey("should return bucket only for path without object key", func() {
			bucket, objectKey := parseS3Path("/my-bucket")
			convey.So(bucket, convey.ShouldEqual, "my-bucket")
			convey.So(objectKey, convey.ShouldEqual, "")
		})

		convey.Convey("should parse path without leading slash", func() {
			bucket, objectKey := parseS3Path("my-bucket/my-key")
			convey.So(bucket, convey.ShouldEqual, "my-bucket")
			convey.So(objectKey, convey.ShouldEqual, "my-key")
		})

		convey.Convey("should parse path with only leading slash and bucket", func() {
			bucket, objectKey := parseS3Path("/single-bucket-name")
			convey.So(bucket, convey.ShouldEqual, "single-bucket-name")
			convey.So(objectKey, convey.ShouldEqual, "")
		})

		convey.Convey("should handle path with encoded characters", func() {
			bucket, objectKey := parseS3Path("/my-bucket/path%20with%20spaces/file.txt")
			convey.So(bucket, convey.ShouldEqual, "my-bucket")
			convey.So(objectKey, convey.ShouldEqual, "path%20with%20spaces/file.txt")
		})

		convey.Convey("should parse path with object key containing bucket-like prefix", func() {
			bucket, objectKey := parseS3Path("/real-bucket/prefix/subpath")
			convey.So(bucket, convey.ShouldEqual, "real-bucket")
			convey.So(objectKey, convey.ShouldEqual, "prefix/subpath")
		})

		convey.Convey("should handle bucket name with hyphens and dots", func() {
			bucket, objectKey := parseS3Path("/my-bucket.example.com/key")
			convey.So(bucket, convey.ShouldEqual, "my-bucket.example.com")
			convey.So(objectKey, convey.ShouldEqual, "key")
		})
	})
}

func TestNewOtelRoundTripper(t *testing.T) {
	convey.Convey("Test NewOtelRoundTripper", t, func() {
		convey.Convey("should create otelRoundTripper with given transport", func() {
			next := http.DefaultTransport
			rt := NewOtelRoundTripper(next)
			convey.So(rt, convey.ShouldNotBeNil)
			convey.So(rt, convey.ShouldImplement, (*http.RoundTripper)(nil))
		})

		convey.Convey("should create otelRoundTripper with nil transport", func() {
			rt := NewOtelRoundTripper(nil)
			convey.So(rt, convey.ShouldNotBeNil)
			convey.So(rt, convey.ShouldImplement, (*http.RoundTripper)(nil))
		})

		convey.Convey("should return concrete *otelRoundTripper type", func() {
			rt := NewOtelRoundTripper(http.DefaultTransport)
			_, ok := rt.(*otelRoundTripper)
			convey.So(ok, convey.ShouldBeTrue)
		})
	})
}

func TestOtelRoundTripper_RoundTrip(t *testing.T) {
	convey.Convey("Test otelRoundTripper.RoundTrip", t, func() {
		convey.Convey("should forward request to underlying transport", func() {
			// 使用一个简单的 mock transport 来验证请求被转发
			transportCalled := false
			mockTransport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				transportCalled = true
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
				}, nil
			})

			rt := NewOtelRoundTripper(mockTransport)
			req, err := http.NewRequest("GET", "http://my-bucket.s3.amazonaws.com/my-key", nil)
			convey.So(err, convey.ShouldBeNil)

			resp, err := rt.RoundTrip(req)
			convey.So(err, convey.ShouldBeNil)
			convey.So(resp, convey.ShouldNotBeNil)
			convey.So(transportCalled, convey.ShouldBeTrue)
			convey.So(resp.StatusCode, convey.ShouldEqual, http.StatusOK)
		})

		convey.Convey("should propagate errors from underlying transport", func() {
			mockTransport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				return nil, http.ErrServerClosed
			})

			rt := NewOtelRoundTripper(mockTransport)
			req, err := http.NewRequest("GET", "http://localhost:9000/bucket/key", nil)
			convey.So(err, convey.ShouldBeNil)

			resp, err := rt.RoundTrip(req)
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(resp, convey.ShouldBeNil)
		})

		convey.Convey("should handle PUT request with object key in path", func() {
			transportCalled := false
			mockTransport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				transportCalled = true
				convey.So(req.Method, convey.ShouldEqual, "PUT")
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
				}, nil
			})

			rt := NewOtelRoundTripper(mockTransport)
			req, err := http.NewRequest("PUT", "http://s3.example.com/test-bucket/upload/test.json", nil)
			convey.So(err, convey.ShouldBeNil)

			resp, err := rt.RoundTrip(req)
			convey.So(err, convey.ShouldBeNil)
			convey.So(resp, convey.ShouldNotBeNil)
			convey.So(transportCalled, convey.ShouldBeTrue)
		})

		convey.Convey("should handle 4xx responses without error propagation", func() {
			mockTransport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Header:     make(http.Header),
				}, nil
			})

			rt := NewOtelRoundTripper(mockTransport)
			req, err := http.NewRequest("GET", "http://s3.example.com/bucket/nonexistent", nil)
			convey.So(err, convey.ShouldBeNil)

			resp, err := rt.RoundTrip(req)
			convey.So(err, convey.ShouldBeNil)
			convey.So(resp, convey.ShouldNotBeNil)
			convey.So(resp.StatusCode, convey.ShouldEqual, http.StatusNotFound)
		})

		convey.Convey("should handle 5xx responses without error propagation", func() {
			mockTransport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Header:     make(http.Header),
				}, nil
			})

			rt := NewOtelRoundTripper(mockTransport)
			req, err := http.NewRequest("GET", "http://s3.example.com/bucket/key", nil)
			convey.So(err, convey.ShouldBeNil)

			resp, err := rt.RoundTrip(req)
			convey.So(err, convey.ShouldBeNil)
			convey.So(resp, convey.ShouldNotBeNil)
			convey.So(resp.StatusCode, convey.ShouldEqual, http.StatusInternalServerError)
		})

		convey.Convey("should handle path with no bucket (root path)", func() {
			transportCalled := false
			mockTransport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				transportCalled = true
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
				}, nil
			})

			rt := NewOtelRoundTripper(mockTransport)
			req, err := http.NewRequest("GET", "http://s3.amazonaws.com/", nil)
			convey.So(err, convey.ShouldBeNil)

			resp, err := rt.RoundTrip(req)
			convey.So(err, convey.ShouldBeNil)
			convey.So(resp, convey.ShouldNotBeNil)
			convey.So(transportCalled, convey.ShouldBeTrue)
		})

		convey.Convey("should preserve original request headers", func() {
			var capturedReq *http.Request
			mockTransport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				capturedReq = req
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
				}, nil
			})

			rt := NewOtelRoundTripper(mockTransport)
			req, err := http.NewRequest("GET", "http://s3.example.com/bucket/key", nil)
			convey.So(err, convey.ShouldBeNil)
			req.Header.Set("X-Custom-Header", "custom-value")

			_, err = rt.RoundTrip(req)
			convey.So(err, convey.ShouldBeNil)
			convey.So(capturedReq.Header.Get("X-Custom-Header"), convey.ShouldEqual, "custom-value")
		})
	})
}

// roundTripperFunc 是将一个函数适配为 http.RoundTripper 接口的适配器类型。
// 用于在单元测试中创建轻量级的 mock transport，无需定义完整结构体。
type roundTripperFunc func(*http.Request) (*http.Response, error)

// RoundTrip 实现 http.RoundTripper 接口。
// 直接委托给内部的函数实现，实现零开销的函数到接口适配。
func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
