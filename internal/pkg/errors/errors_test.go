package errors

import (
	"fmt"
	"io"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"zhongkui.com/zhongkui/internal/pkg/base"
)

func TestNew(t *testing.T) {
	convey.Convey("Test New function", t, func() {
		convey.Convey("simple error message", func() {
			err := New("something went wrong")
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Error(), convey.ShouldEqual, "something went wrong")
		})

		convey.Convey("empty error message", func() {
			err := New("")
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Error(), convey.ShouldEqual, "")
		})

		convey.Convey("error with special characters", func() {
			err := New("error: failed to parse @#$%")
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Error(), convey.ShouldEqual, "error: failed to parse @#$%")
		})

		convey.Convey("error with unicode characters", func() {
			err := New("错误: 发生问题")
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Error(), convey.ShouldEqual, "错误: 发生问题")
		})

		convey.Convey("error with newlines", func() {
			err := New("line1\nline2\r\nline3")
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Error(), convey.ShouldEqual, "line1\nline2\r\nline3")
		})

		convey.Convey("error with tabs", func() {
			err := New("col1\tcol2\tcol3")
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Error(), convey.ShouldEqual, "col1\tcol2\tcol3")
		})

		convey.Convey("error implements error interface", func() {
			err := New("test error")
			convey.So(err, convey.ShouldImplement, (*error)(nil))
		})

		convey.Convey("different errors with different messages are not equal", func() {
			err1 := New("error 1")
			err2 := New("error 2")
			convey.So(err1, convey.ShouldNotEqual, err2)
		})
	})
}

func TestNewf(t *testing.T) {
	convey.Convey("Test Newf function", t, func() {
		convey.Convey("format with string", func() {
			err := Newf("failed to open file: %s", "config.txt")
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Error(), convey.ShouldEqual, "failed to open file: config.txt")
		})

		convey.Convey("format with integer", func() {
			err := Newf("invalid count: %d", 42)
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Error(), convey.ShouldEqual, "invalid count: 42")
		})

		convey.Convey("format with multiple args", func() {
			err := Newf("%s:%d %s", "error", 500, "internal server error")
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Error(), convey.ShouldEqual, "error:500 internal server error")
		})

		convey.Convey("format with float", func() {
			err := Newf("value: %.2f", 3.14159)
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Error(), convey.ShouldEqual, "value: 3.14")
		})

		convey.Convey("format with boolean", func() {
			err := Newf("flag enabled: %t", true)
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Error(), convey.ShouldEqual, "flag enabled: true")
		})

		convey.Convey("format without args", func() {
			err := Newf("just a message")
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Error(), convey.ShouldEqual, "just a message")
		})

		convey.Convey("format with percent sign", func() {
			err := Newf("progress: 100%%")
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Error(), convey.ShouldEqual, "progress: 100%")
		})

		convey.Convey("format with struct", func() {
			type Person struct {
				Name string
				Age  int
			}
			p := Person{Name: "John", Age: 30}
			err := Newf("person: %+v", p)
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Error(), convey.ShouldContainSubstring, "John")
			convey.So(err.Error(), convey.ShouldContainSubstring, "30")
		})

		convey.Convey("format implements error interface", func() {
			err := Newf("formatted error %d", 1)
			convey.So(err, convey.ShouldImplement, (*error)(nil))
		})
	})
}

func TestWithStack(t *testing.T) {
	original := base.ERROR_STACK_ENV_VAR
	base.ERROR_STACK_ENV_VAR = false
	defer func() {
		base.ERROR_STACK_ENV_VAR = original
	}()

	convey.Convey("Test WithStack function", t, func() {
		convey.Convey("nil error", func() {
			got := WithStack(nil)
			convey.So(got, convey.ShouldBeNil)
		})

		convey.Convey("simple error", func() {
			err := New("test error")
			got := WithStack(err)
			convey.So(got.Error(), convey.ShouldEqual, "test error")
		})

		convey.Convey("wrapped error returns original error", func() {
			err := fmt.Errorf("outer: %w", New("inner"))
			got := WithStack(err)
			convey.So(got.Error(), convey.ShouldEqual, "outer: inner")
		})

		convey.Convey("nil error preserves nil when stack disabled", func() {
			got := WithStack(nil)
			convey.So(got, convey.ShouldBeNil)
		})

		convey.Convey("standard error with stack disabled", func() {
			stdErr := fmt.Errorf("standard error")
			got := WithStack(stdErr)
			convey.So(got, convey.ShouldEqual, stdErr)
		})

		convey.Convey("empty error message", func() {
			err := New("")
			got := WithStack(err)
			convey.So(got.Error(), convey.ShouldEqual, "")
		})
	})
}

func TestIs(t *testing.T) {
	errNotFound := New("not found")
	errInvalid := New("invalid input")
	errIO := io.EOF
	errCustom := New("custom error")

	convey.Convey("Test Is function", t, func() {
		convey.Convey("nil error with nil target", func() {
			got := Is(nil, nil)
			convey.So(got, convey.ShouldBeTrue)
		})

		convey.Convey("exact match", func() {
			got := Is(errNotFound, errNotFound)
			convey.So(got, convey.ShouldBeTrue)
		})

		convey.Convey("wrapped error match", func() {
			err := fmt.Errorf("context: %w", errNotFound)
			got := Is(err, errNotFound)
			convey.So(got, convey.ShouldBeTrue)
		})

		convey.Convey("wrapped error chain match", func() {
			err := fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", errInvalid))
			got := Is(err, errInvalid)
			convey.So(got, convey.ShouldBeTrue)
		})

		convey.Convey("no match", func() {
			got := Is(errNotFound, errInvalid)
			convey.So(got, convey.ShouldBeFalse)
		})

		convey.Convey("io.EOF match", func() {
			got := Is(errIO, io.EOF)
			convey.So(got, convey.ShouldBeTrue)
		})

		convey.Convey("nil error with non-nil target", func() {
			got := Is(nil, errNotFound)
			convey.So(got, convey.ShouldBeFalse)
		})

		convey.Convey("non-nil error with nil target", func() {
			got := Is(errNotFound, nil)
			convey.So(got, convey.ShouldBeFalse)
		})

		convey.Convey("sentinel error match", func() {
			sentinel := io.ErrUnexpectedEOF
			err := fmt.Errorf("wrapped: %w", sentinel)
			got := Is(err, sentinel)
			convey.So(got, convey.ShouldBeTrue)
		})

		convey.Convey("multiple levels of wrapping", func() {
			level1 := fmt.Errorf("level1: %w", errCustom)
			level2 := fmt.Errorf("level2: %w", level1)
			level3 := fmt.Errorf("level3: %w", level2)
			got := Is(level3, errCustom)
			convey.So(got, convey.ShouldBeTrue)
		})

		convey.Convey("searching for different error in chain", func() {
			level1 := fmt.Errorf("level1: %w", errNotFound)
			level2 := fmt.Errorf("level2: %w", level1)
			got := Is(level2, errInvalid)
			convey.So(got, convey.ShouldBeFalse)
		})

		convey.Convey("with stack error enabled", func() {
			original := base.ERROR_STACK_ENV_VAR
			base.ERROR_STACK_ENV_VAR = true
			defer func() {
				base.ERROR_STACK_ENV_VAR = original
			}()

			errWithStack := New("stack error")
			errWrapped := fmt.Errorf("wrapped: %w", errWithStack)
			got := Is(errWrapped, errWithStack)
			convey.So(got, convey.ShouldBeTrue)
		})
	})
}

func TestStackError(t *testing.T) {
	convey.Convey("Test StackError", t, func() {
		convey.Convey("StackError implements error interface", func() {
			err := New("test error")
			convey.So(err.Error(), convey.ShouldEqual, "test error")
		})
	})
}

func TestWithStackEnabled(t *testing.T) {
	original := base.ERROR_STACK_ENV_VAR
	base.ERROR_STACK_ENV_VAR = true
	defer func() {
		base.ERROR_STACK_ENV_VAR = original
	}()

	convey.Convey("Test with stack enabled", t, func() {
		convey.Convey("New with stack enabled returns StackError", func() {
			err := New("test error")
			_, ok := err.(stackError)
			convey.So(ok, convey.ShouldBeTrue)
		})

		convey.Convey("Cause with stack enabled wraps error", func() {
			inner := New("inner error")
			result := WithStack(inner)
			_, ok := result.(stackError)
			convey.So(ok, convey.ShouldBeTrue)
		})

		convey.Convey("Newf with stack enabled returns StackError", func() {
			err := Newf("formatted: %s", "test")
			_, ok := err.(stackError)
			convey.So(ok, convey.ShouldBeTrue)
		})

		convey.Convey("nil error with stack enabled returns nil", func() {
			result := WithStack(nil)
			convey.So(result, convey.ShouldBeNil)
		})

		convey.Convey("standard error wrapped with stack enabled", func() {
			stdErr := fmt.Errorf("standard error")
			result := WithStack(stdErr)
			convey.So(result, convey.ShouldNotBeNil)
			_, ok := result.(stackError)
			convey.So(ok, convey.ShouldBeTrue)
		})

		convey.Convey("stack error already has stack returns same error", func() {
			inner := New("inner error")
			firstWrap := WithStack(inner)
			secondWrap := WithStack(firstWrap)
			convey.So(secondWrap, convey.ShouldEqual, firstWrap)
		})
	})
}

func TestWithStackDisabled(t *testing.T) {
	original := base.ERROR_STACK_ENV_VAR
	base.ERROR_STACK_ENV_VAR = false
	defer func() {
		base.ERROR_STACK_ENV_VAR = original
	}()

	convey.Convey("Test with stack disabled", t, func() {
		convey.Convey("New with stack disabled returns standard error", func() {
			err := New("test error")
			_, ok := err.(stackError)
			convey.So(ok, convey.ShouldBeFalse)
		})

		convey.Convey("Cause with stack disabled returns original error", func() {
			inner := New("inner error")
			result := WithStack(inner)
			convey.So(result, convey.ShouldEqual, inner)
		})

		convey.Convey("Newf with stack disabled returns standard error", func() {
			err := Newf("formatted: %s", "test")
			_, ok := err.(stackError)
			convey.So(ok, convey.ShouldBeFalse)
		})

		convey.Convey("nil error with stack disabled returns nil", func() {
			result := WithStack(nil)
			convey.So(result, convey.ShouldBeNil)
		})

		convey.Convey("standard error with stack disabled unchanged", func() {
			stdErr := fmt.Errorf("standard error")
			result := WithStack(stdErr)
			convey.So(result, convey.ShouldEqual, stdErr)
		})
	})
}

func TestComplexErrorChains(t *testing.T) {
	convey.Convey("Test complex error chains", t, func() {
		convey.Convey("deeply nested error chain", func() {
			root := New("root cause")
			level1 := fmt.Errorf("level1: %w", root)
			level2 := fmt.Errorf("level2: %w", level1)
			level3 := fmt.Errorf("level3: %w", level2)

			result := WithStack(level3)
			convey.So(Is(result, root), convey.ShouldBeTrue)
		})

		convey.Convey("multiple wrapping with different types", func() {
			errNotFound := New("not found")
			errPermission := New("permission denied")

			wrapped := fmt.Errorf("outer: %w", errNotFound)

			convey.So(Is(wrapped, errNotFound), convey.ShouldBeTrue)
			convey.So(Is(wrapped, errPermission), convey.ShouldBeFalse)
		})
	})
}

func BenchmarkNew(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = New("benchmark error")
	}
}

func BenchmarkNewf(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = Newf("benchmark error %d", i)
	}
}

func BenchmarkCause(b *testing.B) {
	err := New("test error")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = WithStack(err)
	}
}

func BenchmarkIs(b *testing.B) {
	err := New("test error")
	target := New("test error")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Is(err, target)
	}
}
