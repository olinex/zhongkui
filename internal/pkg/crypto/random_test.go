package crypto

import (
	"encoding/base64"
	"regexp"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGenerateRandomString(t *testing.T) {
	Convey("Test GenerateRandomString function", t, func() {
		Convey("Should generate string of specified length", func() {
			length := 16
			result, err := GenerateRandomString(length)
			So(err, ShouldBeNil)
			So(len(result), ShouldEqual, length)
		})

		Convey("Should generate different strings each time", func() {
			result1, err := GenerateRandomString(16)
			So(err, ShouldBeNil)
			result2, err := GenerateRandomString(16)
			So(err, ShouldBeNil)
			So(result1, ShouldNotEqual, result2)
		})

		Convey("Should only contain valid charset characters", func() {
			result, err := GenerateRandomString(100)
			So(err, ShouldBeNil)
			validCharset := regexp.MustCompile(`^[a-zA-Z0-9]+$`)
			So(validCharset.MatchString(result), ShouldBeTrue)
		})

		Convey("Should handle length of 0", func() {
			result, err := GenerateRandomString(0)
			So(err, ShouldBeNil)
			So(result, ShouldEqual, "")
		})

		Convey("Should handle length of 1", func() {
			result, err := GenerateRandomString(1)
			So(err, ShouldBeNil)
			So(len(result), ShouldEqual, 1)
			validCharset := regexp.MustCompile(`^[a-zA-Z0-9]$`)
			So(validCharset.MatchString(result), ShouldBeTrue)
		})

		Convey("Should handle large length", func() {
			result, err := GenerateRandomString(1000)
			So(err, ShouldBeNil)
			So(len(result), ShouldEqual, 1000)
		})

		Convey("Should handle small length", func() {
			result, err := GenerateRandomString(5)
			So(err, ShouldBeNil)
			So(len(result), ShouldEqual, 5)
			validCharset := regexp.MustCompile(`^[a-zA-Z0-9]{5}$`)
			So(validCharset.MatchString(result), ShouldBeTrue)
		})

		Convey("Should contain both uppercase and lowercase letters", func() {
			hasUpper := false
			hasLower := false
			hasDigit := false

			for i := 0; i < 100; i++ {
				result, err := GenerateRandomString(100)
				So(err, ShouldBeNil)

				for _, c := range result {
					if c >= 'A' && c <= 'Z' {
						hasUpper = true
					}
					if c >= 'a' && c <= 'z' {
						hasLower = true
					}
					if c >= '0' && c <= '9' {
						hasDigit = true
					}
				}
			}

			So(hasUpper, ShouldBeTrue)
			So(hasLower, ShouldBeTrue)
			So(hasDigit, ShouldBeTrue)
		})

		Convey("Should handle negative length", func() {
			result, err := GenerateRandomString(-1)
			So(err, ShouldBeNil)
			So(result, ShouldEqual, "")
		})
	})
}

func TestGenerateRandomStringConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	results := make([]string, 100)
	errs := make([]error, 100)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		idx := i
		go func() {
			defer wg.Done()
			s, err := GenerateRandomString(16)
			errs[idx] = err
			results[idx] = s
		}()
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: unexpected error: %v", i, err)
		}
	}

	uniqueResults := make(map[string]struct{})
	for i, r := range results {
		if len(r) != 16 {
			t.Errorf("goroutine %d: expected length 16, got %d", i, len(r))
		}
		uniqueResults[r] = struct{}{}
	}
	if len(uniqueResults) != 100 {
		t.Errorf("expected 100 unique results, got %d", len(uniqueResults))
	}
}

func TestGenerateRandomBase64String(t *testing.T) {
	Convey("Test GenerateRandomBase64String function", t, func() {
		Convey("Should generate URL-safe base64 string without padding", func() {
			result, err := GenerateRandomBase64String(16)
			So(err, ShouldBeNil)
			So(result, ShouldNotBeEmpty)

			noPadding := regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
			So(noPadding.MatchString(result), ShouldBeTrue)
		})

		Convey("Should generate different strings each time", func() {
			result1, err := GenerateRandomBase64String(16)
			So(err, ShouldBeNil)
			result2, err := GenerateRandomBase64String(16)
			So(err, ShouldBeNil)
			So(result1, ShouldNotEqual, result2)
		})

		Convey("Should handle length of 0", func() {
			result, err := GenerateRandomBase64String(0)
			So(err, ShouldBeNil)
			So(result, ShouldEqual, "")
		})

		Convey("Should handle length of 4", func() {
			result, err := GenerateRandomBase64String(4)
			So(err, ShouldBeNil)
			So(result, ShouldNotBeEmpty)
			noPadding := regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
			So(noPadding.MatchString(result), ShouldBeTrue)
		})

		Convey("Should generate output of specified length", func() {
			inputLength := 16
			result, err := GenerateRandomBase64String(inputLength)
			So(err, ShouldBeNil)
			So(len(result), ShouldEqual, inputLength)
		})

		Convey("Should handle large length", func() {
			result, err := GenerateRandomBase64String(1000)
			So(err, ShouldBeNil)
			So(len(result), ShouldEqual, 1000)
			noPadding := regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
			So(noPadding.MatchString(result), ShouldBeTrue)
		})

		Convey("Should not contain padding characters", func() {
			result, err := GenerateRandomBase64String(100)
			So(err, ShouldBeNil)
			So(result, ShouldNotContainSubstring, "=")
		})

		Convey("Should not contain + or / (standard base64)", func() {
			for i := 0; i < 100; i++ {
				result, err := GenerateRandomBase64String(100)
				So(err, ShouldBeNil)
				So(result, ShouldNotContainSubstring, "+")
				So(result, ShouldNotContainSubstring, "/")
			}
		})

		Convey("Should contain - and _ (URL-safe base64)", func() {
			hasDash := false
			hasUnderscore := false

			for i := 0; i < 1000; i++ {
				result, err := GenerateRandomBase64String(100)
				So(err, ShouldBeNil)

				for _, c := range result {
					if c == '-' {
						hasDash = true
					}
					if c == '_' {
						hasUnderscore = true
					}
				}

				if hasDash && hasUnderscore {
					break
				}
			}

			So(hasDash, ShouldBeTrue)
			So(hasUnderscore, ShouldBeTrue)
		})

		Convey("Should handle length not divisible by 4", func() {
			result, err := GenerateRandomBase64String(7)
			So(err, ShouldBeNil)
			So(len(result), ShouldBeLessThanOrEqualTo, 7)
			So(len(result), ShouldBeGreaterThan, 0)

			noPadding := regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
			So(noPadding.MatchString(result), ShouldBeTrue)
		})

		Convey("Should produce valid base64 decodable output", func() {
			result, err := GenerateRandomBase64String(16)
			So(err, ShouldBeNil)

			decoded, err := base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(result)
			So(err, ShouldBeNil)
			So(len(decoded), ShouldBeGreaterThan, 0)
		})

		Convey("Should handle negative length", func() {
			result, err := GenerateRandomBase64String(-1)
			So(err, ShouldBeNil)
			So(result, ShouldEqual, "")
		})

		Convey("Should match expected length for multiples of 4", func() {
			for _, length := range []int{4, 8, 12, 20, 40} {
				result, err := GenerateRandomBase64String(length)
				So(err, ShouldBeNil)
				So(len(result), ShouldEqual, length)
			}
		})
	})
}

func TestGenerateRandomBase64StringConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	results := make([]string, 100)
	errs := make([]error, 100)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		idx := i
		go func() {
			defer wg.Done()
			s, err := GenerateRandomBase64String(16)
			errs[idx] = err
			results[idx] = s
		}()
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: unexpected error: %v", i, err)
		}
	}

	uniqueResults := make(map[string]struct{})
	for i, r := range results {
		if len(r) != 16 {
			t.Errorf("goroutine %d: expected length 16, got %d", i, len(r))
		}
		uniqueResults[r] = struct{}{}
	}
	if len(uniqueResults) != 100 {
		t.Errorf("expected 100 unique results, got %d", len(uniqueResults))
	}
}
