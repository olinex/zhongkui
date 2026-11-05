package validator

import (
	"errors"
	"testing"

	uv "github.com/go-playground/validator/v10"

	apperrors "zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// checkTestInput 是 Check 的测试结构体：只使用内置 tag，字段名由 json 标签对外呈现。
type checkTestInput struct {
	// Name 姓名，必填
	Name string `json:"name" binding:"required"`
	// Age 年龄，最小为 1
	Age int `json:"age" binding:"min=1"`
}

func TestCheck(t *testing.T) {
	if err := Check(checkTestInput{Name: "alice", Age: 18}); err != nil {
		t.Fatalf("Check(valid) = %v, want nil", err)
	}

	err := Check(checkTestInput{})
	if err == nil {
		t.Fatal("Check(invalid) = nil, want error")
	}
	var validationErrors uv.ValidationErrors
	if !errors.As(err, &validationErrors) {
		t.Fatalf("Check error type = %T, want validator.ValidationErrors", err)
	}
	if got := Describe(err); got != "name: required; age: min=1" {
		t.Fatalf("Describe(Check error) = %q, want %q", got, "name: required; age: min=1")
	}
}

func TestCheckPartial(t *testing.T) {
	cases := []struct {
		name   string
		input  checkTestInput
		fields []string
		want   string
	}{
		{"only name field", checkTestInput{Age: 0}, []string{"Name"}, "name: required"},
		{"only age field", checkTestInput{Name: "alice"}, []string{"Age"}, "age: min=1"},
		{"valid fields", checkTestInput{Name: "alice", Age: 18}, []string{"Name", "Age"}, ""},
		{"skip empty fields", checkTestInput{Name: "alice"}, []string{"Name"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckPartial(tc.input, tc.fields...)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("CheckPartial() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("CheckPartial() = nil, want %q", tc.want)
			}
			if got := Describe(err); got != tc.want {
				t.Fatalf("Describe(CheckPartial error) = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDescribe(t *testing.T) {
	if got := Describe(nil); got != "" {
		t.Fatalf("Describe(nil) = %q, want empty", got)
	}

	validationErr := Check(checkTestInput{})
	wrapped := apperrors.WithStack(validationErr)
	if got := Describe(wrapped); got != "name: required; age: min=1" {
		t.Fatalf("Describe(wrapped validation error) = %q, want %q", got, "name: required; age: min=1")
	}

	plainErr := errors.New("boom")
	if got := Describe(plainErr); got != "boom" {
		t.Fatalf("Describe(non-validation error) = %q, want %q", got, "boom")
	}
}
