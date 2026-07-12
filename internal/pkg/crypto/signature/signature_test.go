package signature

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestHMACSHA256Sign(t *testing.T) {
	Convey("Test HMACSHA256Sign function", t, func() {
		Convey("Should generate consistent signature for same input", func() {
			secret := "mysecretkey"
			value := "testvalue"

			sig1, err := HMACSHA256Sign(secret, value)
			So(err, ShouldBeNil)

			sig2, err := HMACSHA256Sign(secret, value)
			So(err, ShouldBeNil)

			So(sig1, ShouldEqual, sig2)
		})

		Convey("Should generate different signatures for different secrets", func() {
			value := "testvalue"

			sig1, err := HMACSHA256Sign("secret1", value)
			So(err, ShouldBeNil)

			sig2, err := HMACSHA256Sign("secret2", value)
			So(err, ShouldBeNil)

			So(sig1, ShouldNotEqual, sig2)
		})

		Convey("Should generate different signatures for different values", func() {
			secret := "mysecretkey"

			sig1, err := HMACSHA256Sign(secret, "value1")
			So(err, ShouldBeNil)

			sig2, err := HMACSHA256Sign(secret, "value2")
			So(err, ShouldBeNil)

			So(sig1, ShouldNotEqual, sig2)
		})

		Convey("Should generate base64 encoded signature", func() {
			sig, err := HMACSHA256Sign("secret", "value")
			So(err, ShouldBeNil)

			_, err = base64.StdEncoding.DecodeString(sig)
			So(err, ShouldBeNil)
		})

		Convey("Should handle empty secret", func() {
			sig, err := HMACSHA256Sign("", "value")
			So(err, ShouldBeNil)
			So(sig, ShouldNotBeEmpty)

			expectedSig := computeHMACSHA256("", "value")
			So(sig, ShouldEqual, expectedSig)
		})

		Convey("Should handle empty value", func() {
			sig, err := HMACSHA256Sign("secret", "")
			So(err, ShouldBeNil)
			So(sig, ShouldNotBeEmpty)

			expectedSig := computeHMACSHA256("secret", "")
			So(sig, ShouldEqual, expectedSig)
		})

		Convey("Should handle both empty secret and value", func() {
			sig, err := HMACSHA256Sign("", "")
			So(err, ShouldBeNil)
			So(sig, ShouldNotBeEmpty)
		})

		Convey("Should handle long secret and value", func() {
			longSecret := string(make([]byte, 1000))
			longValue := string(make([]byte, 1000))

			sig, err := HMACSHA256Sign(longSecret, longValue)
			So(err, ShouldBeNil)
			So(sig, ShouldNotBeEmpty)

			expectedSig := computeHMACSHA256(longSecret, longValue)
			So(sig, ShouldEqual, expectedSig)
		})

		Convey("Should handle special characters", func() {
			secret := "secret!@#$%^&*()"
			value := "value!@#$%^&*()"

			sig, err := HMACSHA256Sign(secret, value)
			So(err, ShouldBeNil)
			So(sig, ShouldNotBeEmpty)

			expectedSig := computeHMACSHA256(secret, value)
			So(sig, ShouldEqual, expectedSig)
		})

		Convey("Should handle unicode characters", func() {
			secret := "密钥"
			value := "值"

			sig, err := HMACSHA256Sign(secret, value)
			So(err, ShouldBeNil)
			So(sig, ShouldNotBeEmpty)

			expectedSig := computeHMACSHA256(secret, value)
			So(sig, ShouldEqual, expectedSig)
		})

		Convey("签名结果应为合法的 Base64 字符串", func() {
			sig, err := HMACSHA256Sign("secret", "value")
			So(err, ShouldBeNil)
			decoded, err := base64.StdEncoding.DecodeString(sig)
			So(err, ShouldBeNil)
			So(len(decoded), ShouldEqual, sha256.Size)
		})

		Convey("大小写敏感", func() {
			sig1, err := HMACSHA256Sign("secret", "Value")
			So(err, ShouldBeNil)
			sig2, err := HMACSHA256Sign("secret", "value")
			So(err, ShouldBeNil)
			So(sig1, ShouldNotEqual, sig2)
		})

		Convey("空格差异应产生不同签名", func() {
			sig1, err := HMACSHA256Sign("secret", "value")
			So(err, ShouldBeNil)
			sig2, err := HMACSHA256Sign("secret", " value")
			So(err, ShouldBeNil)
			sig3, err := HMACSHA256Sign("secret", "value ")
			So(err, ShouldBeNil)
			So(sig1, ShouldNotEqual, sig2)
			So(sig1, ShouldNotEqual, sig3)
		})
	})
}

func TestNewHMACSHA256Signaturer(t *testing.T) {
	Convey("Test NewHMACSHA256Signaturer function", t, func() {
		Convey("Should create Signaturer with correct prefix", func() {
			signaturer := NewHMACSHA256Signaturer("mysecret")
			So(signaturer, ShouldNotBeNil)
		})

		Convey("Should sign value with correct prefix", func() {
			signaturer := NewHMACSHA256Signaturer("mysecret")
			signed, err := signaturer.Sign("testvalue")
			So(err, ShouldBeNil)
			So(signed, ShouldStartWith, PREFIX_HMACSHA256+":")
		})

		Convey("Should verify valid signature", func() {
			signaturer := NewHMACSHA256Signaturer("mysecret")
			signed, err := signaturer.Sign("testvalue")
			So(err, ShouldBeNil)

			valid, err := signaturer.Check(signed, "testvalue")
			So(err, ShouldBeNil)
			So(valid, ShouldBeTrue)
		})

		Convey("Should reject invalid signature for wrong value", func() {
			signaturer := NewHMACSHA256Signaturer("mysecret")
			signed, err := signaturer.Sign("testvalue")
			So(err, ShouldBeNil)

			valid, err := signaturer.Check(signed, "wrongvalue")
			So(err, ShouldBeNil)
			So(valid, ShouldBeFalse)
		})

		Convey("Should reject invalid prefix", func() {
			signaturer := NewHMACSHA256Signaturer("mysecret")

			invalidSigned := "invalid-prefix:signature"
			valid, err := signaturer.Check(invalidSigned, "value")
			So(err, ShouldBeNil)
			So(valid, ShouldBeFalse)
		})

		Convey("Should reject malformed signature (no colon)", func() {
			signaturer := NewHMACSHA256Signaturer("mysecret")

			invalidSigned := "no-colon-signature"
			valid, err := signaturer.Check(invalidSigned, "value")
			So(err, ShouldBeNil)
			So(valid, ShouldBeFalse)
		})

		Convey("Should reject malformed signature (too many parts)", func() {
			signaturer := NewHMACSHA256Signaturer("mysecret")

			invalidSigned := "hmac-sha256:part1:part2"
			valid, err := signaturer.Check(invalidSigned, "value")
			So(err, ShouldBeNil)
			So(valid, ShouldBeFalse)
		})

		Convey("Should reject empty signature", func() {
			signaturer := NewHMACSHA256Signaturer("mysecret")

			valid, err := signaturer.Check("", "value")
			So(err, ShouldBeNil)
			So(valid, ShouldBeFalse)
		})

		Convey("Should handle empty value", func() {
			signaturer := NewHMACSHA256Signaturer("mysecret")

			signed, err := signaturer.Sign("")
			So(err, ShouldBeNil)
			So(signed, ShouldNotBeEmpty)

			valid, err := signaturer.Check(signed, "")
			So(err, ShouldBeNil)
			So(valid, ShouldBeTrue)
		})

		Convey("Should handle different secrets for signing and verification", func() {
			signaturer1 := NewHMACSHA256Signaturer("secret1")
			signaturer2 := NewHMACSHA256Signaturer("secret2")

			signed, err := signaturer1.Sign("testvalue")
			So(err, ShouldBeNil)

			valid, err := signaturer2.Check(signed, "testvalue")
			So(err, ShouldBeNil)
			So(valid, ShouldBeFalse)
		})

		Convey("Should generate deterministic signatures", func() {
			signaturer := NewHMACSHA256Signaturer("mysecret")

			signed1, err := signaturer.Sign("testvalue")
			So(err, ShouldBeNil)

			signed2, err := signaturer.Sign("testvalue")
			So(err, ShouldBeNil)

			So(signed1, ShouldEqual, signed2)
		})

		Convey("Should handle long values", func() {
			signaturer := NewHMACSHA256Signaturer("mysecret")
			longValue := string(make([]byte, 10000))

			signed, err := signaturer.Sign(longValue)
			So(err, ShouldBeNil)

			valid, err := signaturer.Check(signed, longValue)
			So(err, ShouldBeNil)
			So(valid, ShouldBeTrue)
		})

		Convey("Should handle special characters in value", func() {
			signaturer := NewHMACSHA256Signaturer("mysecret")
			specialValue := "test!@#$%^&*()_+-=[]{}|;':\",./<>?"

			signed, err := signaturer.Sign(specialValue)
			So(err, ShouldBeNil)

			valid, err := signaturer.Check(signed, specialValue)
			So(err, ShouldBeNil)
			So(valid, ShouldBeTrue)
		})

		Convey("签名值中冒号仅作为分隔符使用", func() {
			signaturer := NewHMACSHA256Signaturer("mysecret")
			valueWithColon := "key:value"
			signed, err := signaturer.Sign(valueWithColon)
			So(err, ShouldBeNil)

			parts := splitSignature(signed)
			So(len(parts), ShouldEqual, 2)
			So(parts[0], ShouldEqual, PREFIX_HMACSHA256)

			valid, err := signaturer.Check(signed, valueWithColon)
			So(err, ShouldBeNil)
			So(valid, ShouldBeTrue)
		})

		Convey("校验时签名值部分被篡改应返回 false", func() {
			signaturer := NewHMACSHA256Signaturer("mysecret")
			signed, err := signaturer.Sign("testvalue")
			So(err, ShouldBeNil)

			parts := splitSignature(signed)
			So(len(parts), ShouldEqual, 2)

			tampered := fmt.Sprintf("%s:%s", parts[0], parts[1]+"tampered")
			valid, err := signaturer.Check(tampered, "testvalue")
			So(err, ShouldBeNil)
			So(valid, ShouldBeFalse)
		})

		Convey("空密钥创建的签名器仍可正常工作", func() {
			signaturer := NewHMACSHA256Signaturer("")
			signed, err := signaturer.Sign("value")
			So(err, ShouldBeNil)

			valid, err := signaturer.Check(signed, "value")
			So(err, ShouldBeNil)
			So(valid, ShouldBeTrue)
		})
	})
}

func TestNewSignaturerWithCustomFunc(t *testing.T) {
	Convey("Test newSignaturer with custom SignFunc", t, func() {
		Convey("应支持自定义签名函数", func() {
			customPrefix := "custom-algo"
			customFunc := func(secret, value string) (string, error) {
				return secret + "-" + value, nil
			}

			signaturer := newSignaturer(customPrefix, "mysecret", customFunc)
			So(signaturer, ShouldNotBeNil)

			signed, err := signaturer.Sign("testvalue")
			So(err, ShouldBeNil)
			So(signed, ShouldEqual, "custom-algo:mysecret-testvalue")

			valid, err := signaturer.Check(signed, "testvalue")
			So(err, ShouldBeNil)
			So(valid, ShouldBeTrue)
		})

		Convey("自定义签名函数返回错误时 Sign 应传播错误", func() {
			expectedErr := errors.New("sign failed")
			errorFunc := func(secret, value string) (string, error) {
				return "", expectedErr
			}

			signaturer := newSignaturer("err-prefix", "secret", errorFunc)
			_, err := signaturer.Sign("value")
			So(err, ShouldNotBeNil)
		})

		Convey("自定义签名函数返回错误时 Check 应传播错误", func() {
			expectedErr := errors.New("check sign failed")
			callCount := 0
			errorFunc := func(secret, value string) (string, error) {
				callCount++
				if callCount > 1 {
					return "", expectedErr
				}
				return "valid-sig", nil
			}

			signaturer := newSignaturer("err-prefix", "secret", errorFunc)
			signed, err := signaturer.Sign("value")
			So(err, ShouldBeNil)

			_, err = signaturer.Check(signed, "value")
			So(err, ShouldNotBeNil)
		})

		Convey("自定义前缀与签名中的前缀不匹配时 Check 应返回 false", func() {
			customFunc := func(secret, value string) (string, error) {
				return "sig", nil
			}

			signaturer := newSignaturer("algo-a", "secret", customFunc)
			valid, err := signaturer.Check("algo-b:sig", "value")
			So(err, ShouldBeNil)
			So(valid, ShouldBeFalse)
		})
	})
}

func TestSignaturerConcurrent(t *testing.T) {
	Convey("Test Signaturer concurrent safety", t, func() {
		Convey("并发签名和校验应保持一致性", func() {
			signaturer := NewHMACSHA256Signaturer("concurrent-secret")
			var wg sync.WaitGroup
			errs := make([]error, 100)
			results := make([]string, 100)

			for i := 0; i < 100; i++ {
				wg.Add(1)
				idx := i
				go func() {
					defer wg.Done()
					signed, err := signaturer.Sign("concurrent-value")
					errs[idx] = err
					results[idx] = signed
				}()
			}

			wg.Wait()

			for i, err := range errs {
				So(err, ShouldBeNil)
				_ = i
			}

			for _, signed := range results {
				valid, err := signaturer.Check(signed, "concurrent-value")
				So(err, ShouldBeNil)
				So(valid, ShouldBeTrue)
			}
		})

		Convey("并发签名结果应具有确定性", func() {
			signaturer := NewHMACSHA256Signaturer("concurrent-secret")
			var wg sync.WaitGroup
			results := make([]string, 50)

			for i := 0; i < 50; i++ {
				wg.Add(1)
				idx := i
				go func() {
					defer wg.Done()
					signed, err := signaturer.Sign("same-value")
					if err == nil {
						results[idx] = signed
					}
				}()
			}

			wg.Wait()

			for i := 1; i < len(results); i++ {
				So(results[i], ShouldEqual, results[0])
			}
		})
	})
}

func TestCheckEdgeCases(t *testing.T) {
	Convey("Test Check method edge cases", t, func() {
		signaturer := NewHMACSHA256Signaturer("test-secret")

		Convey("仅前缀无签名值时应返回 false", func() {
			valid, err := signaturer.Check("hmac-sha256:", "value")
			So(err, ShouldBeNil)
			So(valid, ShouldBeFalse)
		})

		Convey("仅冒号时应返回 false", func() {
			valid, err := signaturer.Check(":", "value")
			So(err, ShouldBeNil)
			So(valid, ShouldBeFalse)
		})

		Convey("前缀为空但包含签名值时应返回 false", func() {
			signed, err := signaturer.Sign("value")
			So(err, ShouldBeNil)
			parts := splitSignature(signed)
			So(len(parts), ShouldEqual, 2)

			invalidSigned := ":" + parts[1]
			valid, err := signaturer.Check(invalidSigned, "value")
			So(err, ShouldBeNil)
			So(valid, ShouldBeFalse)
		})

		Convey("签名值正确但原始值包含冒号时应正常校验", func() {
			value := "key:subkey:detail"
			signed, err := signaturer.Sign(value)
			So(err, ShouldBeNil)

			valid, err := signaturer.Check(signed, value)
			So(err, ShouldBeNil)
			So(valid, ShouldBeTrue)
		})

		Convey("unicode 值的签名与校验", func() {
			value := "你好世界🔐🔑"
			signed, err := signaturer.Sign(value)
			So(err, ShouldBeNil)

			valid, err := signaturer.Check(signed, value)
			So(err, ShouldBeNil)
			So(valid, ShouldBeTrue)
		})
	})
}

func TestSignAndCheckIntegration(t *testing.T) {
	Convey("Test Sign and Check integration flow", t, func() {
		testCases := []struct {
			secret string
			value  string
		}{
			{"simple-secret", "simple-value"},
			{"secret-with-特殊字符", "value-with-特殊字符"},
			{"", "empty-secret"},
			{"empty-value", ""},
			{"both-empty", ""},
			{"long" + string(make([]byte, 500)), "also-long" + string(make([]byte, 500))},
		}

		for _, tc := range testCases {
			signaturer := NewHMACSHA256Signaturer(tc.secret)
			signed, err := signaturer.Sign(tc.value)
			So(err, ShouldBeNil)
			So(signed, ShouldNotBeEmpty)

			valid, err := signaturer.Check(signed, tc.value)
			So(err, ShouldBeNil)
			So(valid, ShouldBeTrue)
		}
	})
}

func computeHMACSHA256(secret, value string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(value))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func splitSignature(signed string) []string {
	var result []string
	idx := 0
	for i, c := range signed {
		if c == ':' && idx == 0 {
			result = append(result, signed[:i])
			result = append(result, signed[i+1:])
			break
		}
	}
	return result
}
