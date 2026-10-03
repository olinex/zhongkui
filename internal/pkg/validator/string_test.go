// Package validator 提供通用的字段与取值校验：字符串（密码、文件名、哈希、编码等）、
// 时间与月份等格式校验，以及供结构体绑定标签引用的通用字段级校验器。
package validator

import (
	"fmt"
	"runtime"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"zhongkui.com/zhongkui/internal/pkg/enum"
)

// TestIsValidStrongPassword 测试密码校验函数
func TestIsValidStrongPassword(t *testing.T) {
	Convey("Test strong password validation", t, func() {
		Convey("Valid strong passwords", func() {
			// 满足全部要求的密码
			validPasswords := []string{
				"Password123!",
				"MySecure@Pass1",
				"Test#Password1",
				"SecurePass$2023",
				"ComplexPwd1&",
				"Admin@123456",
				"UserPass1*",
				"Strong!Pass2",
				"Pass@1",
			}

			for _, password := range validPasswords {
				Convey(fmt.Sprintf("Password '%s' should be valid", password), func() {
					So(IsValidStrongPassword(password), ShouldBeTrue)
				})
			}
		})

		Convey("Invalid weak passwords", func() {
			// 未满足各项要求的密码
			invalidPasswords := []struct {
				password string
				reason   string
			}{
				{"", "empty password"},
				{"password", "no uppercase, digit, or special char"},
				{"PASSWORD", "no lowercase, digit, or special char"},
				{"123456", "no letters or special char"},
				{"Password", "no digit or special char"},
				{"Password123", "no special character"},
				{"password123!", "no uppercase letter"},
				{"PASSWORD123!", "no lowercase letter"},
				{"Password 123!", "contains space"},
				{"中文Password1!", "contains non-ASCII characters"},
			}

			for _, tc := range invalidPasswords {
				Convey(fmt.Sprintf("Password '%s' should be invalid: %s", tc.password, tc.reason), func() {
					So(IsValidStrongPassword(tc.password), ShouldBeFalse)
				})
			}
		})

		Convey("Edge cases", func() {
			// 含各种特殊字符的密码
			specialChars := []string{"@", "$", "!", "%", "*", "?", "&"}
			for _, char := range specialChars {
				Convey(fmt.Sprintf("Password with '%s' should be valid", char), func() {
					password := "Password123" + char
					So(IsValidStrongPassword(password), ShouldBeTrue)
				})
			}
		})
	})
}

// TestGenerateStrongPassword 测试强密码生成函数
func TestGenerateStrongPassword(t *testing.T) {
	Convey("Test strong password generation", t, func() {
		Convey("Valid lengths", func() {
			lengths := []int{6, 8, 16, 32, 64}
			for _, length := range lengths {
				Convey(fmt.Sprintf("Length %d should generate a valid strong password", length), func() {
					password, err := GenerateStrongPassword(length)
					So(err, ShouldBeNil)
					So(len(password), ShouldEqual, length)
					So(IsValidStrongPassword(password), ShouldBeTrue)
				})
			}
		})

		Convey("Invalid lengths", func() {
			invalidLengths := []int{-1, 0, 4, 5}
			for _, length := range invalidLengths {
				Convey(fmt.Sprintf("Length %d should return an error", length), func() {
					password, err := GenerateStrongPassword(length)
					So(err, ShouldNotBeNil)
					So(password, ShouldEqual, "")
				})
			}
		})

		Convey("Generated passwords should differ", func() {
			p1, err1 := GenerateStrongPassword(16)
			p2, err2 := GenerateStrongPassword(16)
			So(err1, ShouldBeNil)
			So(err2, ShouldBeNil)
			So(p1, ShouldNotEqual, p2)
		})
	})
}

// TestIsValidWindowsFileName 测试 Windows 文件名校验
func TestIsValidWindowsFileName(t *testing.T) {
	Convey("Test Windows filename validation", t, func() {
		Convey("Valid Windows filenames", func() {
			validNames := []string{
				"file.txt",
				"document.docx",
				"my-file_name.pdf",
				"File.txt",
				"data.csv",
				"README.md",
				"program.exe",
				"archive.tar.gz",
				"image (1).jpg",
				"file with spaces.txt",
				"unicode_文件.txt",
			}

			for _, name := range validNames {
				Convey(fmt.Sprintf("Filename '%s' should be valid on Windows", name), func() {
					So(IsValidWindowsFileName(name), ShouldBeTrue)
				})
			}
		})

		Convey("Invalid Windows filenames - reserved names", func() {
			reservedNames := []string{
				"CON", "PRN", "AUX", "NUL",
				"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
				"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9",
			}

			for _, name := range reservedNames {
				Convey(fmt.Sprintf("Filename '%s' should be invalid (reserved name)", name), func() {
					So(IsValidWindowsFileName(name), ShouldBeFalse)
				})
			}
		})

		Convey("Invalid Windows filenames - illegal characters", func() {
			illegalChars := []string{
				"",
				"file<name.txt",
				"file>name.txt",
				"file:name.txt",
				"file\"name.txt",
				"file/name.txt",
				"file\\name.txt",
				"file|name.txt",
				"file?name.txt",
				"file*name.txt",
				"file\x00name.txt", // 空字符
			}

			for _, name := range illegalChars {
				Convey(fmt.Sprintf("Filename '%s' should be invalid (illegal characters)", name), func() {
					So(IsValidWindowsFileName(name), ShouldBeFalse)
				})
			}
		})

		Convey("Invalid Windows filenames - trailing space or dot", func() {
			invalidNames := []string{
				"file.txt ",
				"file.docx.",
				"document.pdf  ",
				"archive.tar.gz.",
				"file.txt .",
			}

			for _, name := range invalidNames {
				Convey(fmt.Sprintf("Filename '%s' should be invalid (trailing space or dot)", name), func() {
					So(IsValidWindowsFileName(name), ShouldBeFalse)
				})
			}
		})

		Convey("Invalid Windows filenames - too long", func() {
			// 构造一个超过 255 字符的文件名
			longName := string(make([]byte, 256))
			for i := range longName {
				longName = longName[:i] + "a" + longName[i+1:]
			}
			Convey("Filename longer than 255 characters should be invalid", func() {
				So(IsValidWindowsFileName(longName), ShouldBeFalse)
			})
		})
	})
}

// TestIsValidUnixFileName 测试 Unix 文件名校验
func TestIsValidUnixFileName(t *testing.T) {
	Convey("Test Unix filename validation", t, func() {
		Convey("Valid Unix filenames", func() {
			validNames := []string{
				"file.txt",
				"document.docx",
				"my-file_name.pdf",
				".hidden_file",
				"config.json",
				"README.md",
				"program",
				"archive.tar.gz",
				"image (1).jpg",
				"file with spaces.txt",
				"unicode_文件.txt",
				"file with \"quotes\".txt",
				"file:with:colons.txt",
				"file*asterisk.txt",
				"file?question.txt",
				"file<bracket>.txt",
			}

			for _, name := range validNames {
				Convey(fmt.Sprintf("Filename '%s' should be valid on Unix", name), func() {
					So(IsValidUnixFileName(name), ShouldBeTrue)
				})
			}
		})

		Convey("Invalid Unix filenames", func() {
			invalidNames := []string{
				"",
				".",                // 正斜杠
				"..",               // 空字符
				"file\x00name.txt", // 制表符应由 IsStrictValidFileName 中的控制字符检查处理
			}

			for _, name := range invalidNames {
				Convey(fmt.Sprintf("Filename '%s' should be invalid on Unix", name), func() {
					So(IsValidUnixFileName(name), ShouldBeFalse)
				})
			}
		})

		Convey("Invalid Unix filenames - too long", func() {
			// 构造一个超过 255 字符的文件名
			longName := string(make([]byte, 256))
			for i := range longName {
				longName = longName[:i] + "a" + longName[i+1:]
			}
			Convey("Filename longer than 255 characters should be invalid", func() {
				So(IsValidUnixFileName(longName), ShouldBeFalse)
			})
		})
	})
}

// TestIsValidFileName 测试跨平台文件名校验
func TestIsValidFileName(t *testing.T) {
	Convey("Test cross-platform filename validation", t, func() {
		Convey("Basic invalid filenames", func() {
			invalidNames := []string{
				"", ".", "..",
				"file\x00name.txt",               // 非法 UTF-8
				string([]byte{0xff, 0xfe, 0xfd}), // 非法 UTF-8
			}

			for _, name := range invalidNames {
				Convey(fmt.Sprintf("Filename '%v' should be invalid", name), func() {
					So(IsValidFileName(name), ShouldBeFalse)
				})
			}
		})

		Convey("Valid filenames across platforms", func() {
			validNames := []string{
				"file.txt",
				"document.docx",
				"my-file_name.pdf",
				"README.md",
				"program",
				"unicode_test.txt",
			}

			for _, name := range validNames {
				Convey(fmt.Sprintf("Filename '%s' should be valid", name), func() {
					So(IsValidFileName(name), ShouldBeTrue)
				})
			}
		})

		Convey("Platform-specific behavior", func() {
			// 这些文件名在 Unix 上合法、在 Windows 上非法
			unixOnlyValid := []string{
				"file:name.txt",
				"CON.txt", // 小写时不视为保留名
				"aux.log", // 小写时不视为保留名
			}

			for _, name := range unixOnlyValid {
				Convey(fmt.Sprintf("Platform-specific validation for '%s'", name), func() {
					result := IsValidFileName(name)
					if runtime.GOOS == "windows" {
						So(result, ShouldBeFalse)
					} else {
						So(result, ShouldBeTrue)
					}
				})
			}
		})
	})
}

// TestIsNull 测试空值识别
func TestIsNull(t *testing.T) {
	Convey("Test null value detection", t, func() {
		Convey("Valid null representations", func() {
			nullValues := []string{
				"null",
				"NULL",
				"Null",
				"nUlL",
				"nil",
				"NIL",
				"Nil",
				"nIl",
			}

			for _, value := range nullValues {
				Convey(fmt.Sprintf("Value '%s' should be recognized as null", value), func() {
					So(IsNull(value), ShouldBeTrue)
				})
			}
		})

		Convey("Non-null values", func() {
			nonNullValues := []string{
				"",
				"nulll",
				"nill",
				"undefined",
				"0",
				"false",
				"none",
				"null ",
				" null",
			}

			for _, value := range nonNullValues {
				Convey(fmt.Sprintf("Value '%s' should not be recognized as null", value), func() {
					So(IsNull(value), ShouldBeFalse)
				})
			}
		})
	})
}

// TestIsBool 测试布尔值识别
func TestIsBool(t *testing.T) {
	Convey("Test boolean value detection", t, func() {
		Convey("Valid boolean values", func() {
			boolValues := []string{
				"true",
				"TRUE",
				"True",
				"tRuE",
				"false",
				"FALSE",
				"False",
				"fAlSe",
			}

			for _, value := range boolValues {
				Convey(fmt.Sprintf("Value '%s' should be recognized as boolean", value), func() {
					So(IsBool(value), ShouldBeTrue)
				})
			}
		})

		Convey("Invalid boolean values", func() {
			nonBoolValues := []string{
				"",
				"t",
				"f",
				"1",
				"0",
				"yes",
				"no",
				"on",
				"off",
				"truee",
				"falsee",
				" true",
				"false ",
			}

			for _, value := range nonBoolValues {
				Convey(fmt.Sprintf("Value '%s' should not be recognized as boolean", value), func() {
					So(IsBool(value), ShouldBeFalse)
				})
			}
		})
	})
}

// TestIsInt 测试整数值识别
func TestIsInt(t *testing.T) {
	Convey("Test integer value detection", t, func() {
		Convey("Valid integers", func() {
			validInts := []string{
				"0",
				"1",
				"-1",
				"123",
				"-456",
				"2147483647",           // int32 最大值
				"-2147483648",          // int32 最小值
				"9223372036854775807",  // int64 最大值
				"-9223372036854775808", // int64 最小值
				"+123",                 // 带正号
			}

			for _, value := range validInts {
				Convey(fmt.Sprintf("Value '%s' should be recognized as integer", value), func() {
					So(IsInt(value), ShouldBeTrue)
				})
			}
		})

		Convey("Invalid integers", func() {
			invalidInts := []string{
				"",
				"12.3",
				"12a",
				"1.0",
				"0x10",
				"1e5",
				"123 ",
				" 123",
				"12-3",
				"+",                    // 仅正号
				"-",                    // 仅负号
				"9223372036854775808",  // int64 溢出
				"-9223372036854775809", // int64 下溢
			}

			for _, value := range invalidInts {
				Convey(fmt.Sprintf("Value '%s' should not be recognized as integer", value), func() {
					So(IsInt(value), ShouldBeFalse)
				})
			}
		})
	})
}

// TestIsFloat 测试浮点值识别
func TestIsFloat(t *testing.T) {
	Convey("Test float value detection", t, func() {
		Convey("Valid floats", func() {
			validFloats := []string{
				"0.0",
				"1.0",
				"-1.5",
				"123.456",
				"-456.789",
				"1e5",
				"1E-10",
				"-2.5e3",
				".5",
				"0.5",
				"-0.5",
				"123.", // 带尾随小数点的数字
				".0",   // 前导小数点
				"+1.5", // 带正号
			}

			for _, value := range validFloats {
				Convey(fmt.Sprintf("Value '%s' should be recognized as float", value), func() {
					So(IsFloat(value), ShouldBeTrue)
				})
			}
		})

		Convey("Invalid floats", func() {
			invalidFloats := []string{
				"",
				"abc",
				"1.2.3",
				"e10",
				"1e",
				".",
				"1.2.3.4",
				"++1.5",
				"1.5e10.5",
				"1.2.3e4",
			}

			for _, value := range invalidFloats {
				Convey(fmt.Sprintf("Value '%s' should not be recognized as float", value), func() {
					So(IsFloat(value), ShouldBeFalse)
				})
			}
		})
	})
}

// TestHashValidation 测试哈希校验函数
func TestHashValidation(t *testing.T) {
	Convey("MD5 validation", t, func() {
		Convey("Valid MD5 hashes", func() {
			validMD5s := []string{
				"d41d8cd98f00b204e9800998ecf8427e", // 空字符串的 MD5
				"5d41402abc4b2a76b9719d911017c592", // 对应 "hello" 的 MD5
				"098f6bcd4621d373cade4e832627b4f6", // 对应 "test" 的 MD5
				"0123456789ABCDEF0123456789ABCDEF", // 大写
				"aBcDeF1234567890aBcDeF1234567890", // 大小写混合
			}

			for _, value := range validMD5s {
				Convey(fmt.Sprintf("MD5 '%s' should be valid", value), func() {
					So(IsMD5(value), ShouldBeTrue)
				})
			}
		})

		Convey("Invalid MD5 hashes", func() {
			invalidMD5s := []string{
				"",                                  // 空字符串
				"d41d8cd98f00b204e9800998ecf8427",   // 31 个字符
				"d41d8cd98f00b204e9800998ecf8427ee", // 33 个字符
				"g41d8cd98f00b204e9800998ecf8427e",  // 非法十六进制字符
				"not_a_hash",
			}

			for _, value := range invalidMD5s {
				Convey(fmt.Sprintf("MD5 '%s' should be invalid", value), func() {
					So(IsMD5(value), ShouldBeFalse)
				})
			}
		})
	})

	Convey("SHA1 validation", t, func() {
		Convey("Valid SHA1 hashes", func() {
			validSHA1s := []string{
				"da39a3ee5e6b4b0d3255bfef95601890afd80709", // 空字符串的 SHA1
				"aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d", // 对应 "hello" 的 SHA1
				"a94a8fe5ccb19ba61c4c0873d391e987982fbbd3", // 对应 "test" 的 SHA1
			}

			for _, value := range validSHA1s {
				Convey(fmt.Sprintf("SHA1 '%s' should be valid", value), func() {
					So(IsSHA1(value), ShouldBeTrue)
				})
			}
		})

		Convey("Invalid SHA1 hashes", func() {
			Convey("Empty string should be invalid SHA1", func() {
				So(IsSHA1(""), ShouldBeFalse)
			})
			Convey("39-character string should be invalid SHA1", func() {
				So(IsSHA1("da39a3ee5e6b4b0d3255bfef95601890afd8070"), ShouldBeFalse)
			})
		})
	})

	Convey("SHA256 validation", t, func() {
		Convey("Valid SHA256 hashes", func() {
			validSHA256s := []string{
				"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", // 空字符串的 SHA256
				"2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", // 对应 "hello" 的 SHA256
			}

			for _, value := range validSHA256s {
				Convey(fmt.Sprintf("SHA256 '%s' should be valid", value), func() {
					So(IsSHA256(value), ShouldBeTrue)
				})
			}
		})

		Convey("Invalid SHA256 hashes", func() {
			Convey("Empty string should be invalid SHA256", func() {
				So(IsSHA256(""), ShouldBeFalse)
			})
			Convey("63-character string should be invalid SHA256", func() {
				So(IsSHA256("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b85"), ShouldBeFalse)
			})
		})
	})

	Convey("Generic hex validation", t, func() {
		Convey("Valid hex strings", func() {
			validHexs := []string{
				"123abcABC",
				"deadbeef",
				"DEADBEEF",
				"0123456789abcdefABCDEF",
			}

			for _, value := range validHexs {
				Convey(fmt.Sprintf("Hex '%s' should be valid", value), func() {
					So(IsHex(value), ShouldBeTrue)
				})
			}
		})

		Convey("Invalid hex strings", func() {
			invalidHexs := []string{
				"",
				"g",
				"x",
				"123g",
				"xyz",
				"12 34",
				"12-34",
			}

			for _, value := range invalidHexs {
				Convey(fmt.Sprintf("Hex '%s' should be invalid", value), func() {
					So(IsHex(value), ShouldBeFalse)
				})
			}
		})
	})
}

// TestUUIDValidation 测试 UUID 校验
func TestUUIDValidation(t *testing.T) {
	Convey("Test UUID validation", t, func() {
		Convey("Valid UUIDs", func() {
			validUUIDs := []string{
				"550e8400-e29b-41d4-a716-446655440000",
				"6ba7b810-9dad-11d1-80b4-00c04fd430c8",
				"6ba7b811-9dad-11d1-80b4-00c04fd430c8",
				"FFFFFFFF-FFFF-FFFF-FFFF-FFFFFFFFFFFF",
				"00000000-0000-0000-0000-000000000000",
			}

			for _, value := range validUUIDs {
				Convey(fmt.Sprintf("UUID '%s' should be valid", value), func() {
					So(IsUUID(value), ShouldBeTrue)
				})
			}
		})

		Convey("Invalid UUIDs", func() {
			invalidUUIDs := []string{
				"",
				"550e8400e29b41d4a716446655440000",      // 无连字符
				"550e8400-e29b-41d4-a716-44665544",      // 过短
				"550e8400-e29b-41d4-a716-4466554400000", // 过长
				"550e8400-e29b-41d4-a716-44665544g000",  // 非法十六进制字符
				"550e8400-e29b-41d4-a716-44665544-0000", // 多余的连字符
				"g50e8400-e29b-41d4-a716-446655440000",  // 非法十六进制
			}

			for _, value := range invalidUUIDs {
				Convey(fmt.Sprintf("UUID '%s' should be invalid", value), func() {
					So(IsUUID(value), ShouldBeFalse)
				})
			}
		})
	})
}

// TestBase64Validation 测试 Base64 校验
func TestBase64Validation(t *testing.T) {
	Convey("Test Base64 validation", t, func() {
		Convey("Valid Base64 strings", func() {
			validBase64s := []string{
				"aGVsbG8=",                 // 对应 "hello"
				"dGVzdA==",                 // 对应 "test"
				"SGVsbG8gV29ybGQh",         // 对应 "Hello World!"
				"eyJ0ZXN0IjoidmFsdWUifQ==", // 对应 {"test":"value"}
				"AA==",                     // 单字节
				"QQ==",                     // 对应 'A'
				"YWJjZGVmZ2g=",             // 对应 "abcdefgh"
				"",                         // 空字符串是合法的 base64
			}

			for _, value := range validBase64s {
				Convey(fmt.Sprintf("Base64 '%s' should be valid", value), func() {
					So(IsBase64(value), ShouldBeTrue)
				})
			}
		})

		Convey("Invalid Base64 strings", func() {
			invalidBase64s := []string{
				"!!!",                  // 非法字符
				"YWJjZGVmZ2g",          // 缺少填充
				"YWJjZGRmZ2g===",       // 填充过多
				"YWJjZGRmZ2g=aGVsbG8=", // 中间含非法字符
			}

			for _, value := range invalidBase64s {
				Convey(fmt.Sprintf("Base64 '%s' should be invalid", value), func() {
					So(IsBase64(value), ShouldBeFalse)
				})
			}
		})
	})
}

// TestBase32Validation 测试 Base32 校验
func TestBase32Validation(t *testing.T) {
	Convey("Test Base32 validation", t, func() {
		Convey("Valid Base32 strings", func() {
			validBase32s := []string{
				"NBSWY3DP", // 对应 "hello"
				"ORSXG5A=", // 对应 "test"
				"",         // 空字符串是合法的 base32
			}

			for _, value := range validBase32s {
				Convey(fmt.Sprintf("Base32 '%s' should be valid", value), func() {
					So(IsBase32(value), ShouldBeTrue)
				})
			}
		})

		Convey("Invalid Base32 strings", func() {
			invalidBase32s := []string{
				"123",    // 仅数字
				"!!!",    // 非法字符
				"abcdef", // 小写字母
			}

			for _, value := range invalidBase32s {
				Convey(fmt.Sprintf("Base32 '%s' should be invalid", value), func() {
					So(IsBase32(value), ShouldBeFalse)
				})
			}
		})
	})
}

// TestDateValidation 测试日期校验函数
func TestDateValidation(t *testing.T) {
	Convey("Test date validation functions", t, func() {
		Convey("Valid dates", func() {
			validDates := []string{
				"2023-01-01", // 简单日期格式
				"2023-12-31", // 简单日期格式
				"2020-02-29", // 闰年
				"2023-01",    // 仅年月
				"01-31-2023", // 美式格式
				"12-25-2023", // 美式格式
				"31.12.2023", // 欧式格式
				"01.01.2023", // 欧式格式
			}

			for _, date := range validDates {
				Convey(fmt.Sprintf("Date '%s' should be valid", date), func() {
					So(IsDate(date), ShouldBeTrue)
				})
			}
		})

		Convey("Invalid dates", func() {
			invalidDates := []string{
				"",
				"2023-13-01", // 非法月份
				"2023-01-32", // 非法日期
				"2023-04-31", // 4 月只有 30 天
				"2023-02-29", // 非闰年的 2 月 29 日
				"13-01-2023", // 美式格式的非法月份
				"01-32-2023", // 美式格式的非法日期
				"31.13.2023", // 欧式格式的非法月份
				"32.01.2023", // 欧式格式的非法日期
				"2023-1-1",   // 单位数的月/日
				"23-01-01",   // 2 位年份
			}

			for _, date := range invalidDates {
				Convey(fmt.Sprintf("Date '%s' should be invalid", date), func() {
					So(IsDate(date), ShouldBeFalse)
				})
			}
		})
	})
}

// TestYearQuarterValidation 测试年-季度校验
func TestYearQuarterValidation(t *testing.T) {
	Convey("Test year-quarter validation", t, func() {
		Convey("Valid year-quarters", func() {
			validQuarters := []string{
				"2023-Q1",
				"2023-Q2",
				"2023-Q3",
				"2023-Q4",
				"1999-Q1",
				"2000-Q4",
				"2100-Q2",
				"2023-q1", // 小写
			}

			for _, quarter := range validQuarters {
				Convey(fmt.Sprintf("Year-quarter '%s' should be valid", quarter), func() {
					So(IsYearQuarter(quarter), ShouldBeTrue)
				})
			}
		})

		Convey("Invalid year-quarters", func() {
			invalidQuarters := []string{
				"",
				"2023-Q0",
				"2023-Q5",
				"23-Q1",   // 2 位年份
				"2023-Q",  // 缺少季度
				"2023-1",  // 缺少 Q
				"Q1-2023", // 格式错误
			}

			for _, quarter := range invalidQuarters {
				Convey(fmt.Sprintf("Year-quarter '%s' should be invalid", quarter), func() {
					So(IsYearQuarter(quarter), ShouldBeFalse)
				})
			}
		})
	})
}

// TestYearWeekValidation 测试年-周校验
func TestYearWeekValidation(t *testing.T) {
	Convey("Test year-week validation", t, func() {
		Convey("Valid year-weeks", func() {
			validWeeks := []string{
				"2023-W01",
				"2023-W52",
				"2023-W53",
				"2020-W53", // 具有 53 周的年份
				"1999-W01",
				"2000-W52",
				"2023-w01", // 小写
			}

			for _, week := range validWeeks {
				Convey(fmt.Sprintf("Year-week '%s' should be valid", week), func() {
					So(IsYearWeek(week), ShouldBeTrue)
				})
			}
		})

		Convey("Invalid year-weeks", func() {
			invalidWeeks := []string{
				"",
				"2023-W00", // 第 0 周不存在
				"2023-W54", // 第 54 周不存在
				"23-W01",   // 2 位年份
				"2023-W",   // 缺少周数
				"2023-01",  // 缺少 W
				"W01-2023", // 格式错误
			}

			for _, week := range invalidWeeks {
				Convey(fmt.Sprintf("Year-week '%s' should be invalid", week), func() {
					So(IsYearWeek(week), ShouldBeFalse)
				})
			}
		})
	})
}

// TestDatetimeValidation 测试日期时间校验
func TestDatetimeValidation(t *testing.T) {
	Convey("Test datetime validation", t, func() {
		Convey("Valid datetimes", func() {
			validDatetimes := []string{
				"2023-01-01-12-30-45",
				"2023_01_01_12_30_45",
				"2023-01-01_12-30-45",
				"2023-12-31-23-59-59",
				"2020-02-29-00-00-00",
			}

			for _, datetime := range validDatetimes {
				Convey(fmt.Sprintf("Datetime '%s' should be valid", datetime), func() {
					So(IsDatetime(datetime), ShouldBeTrue)
				})
			}
		})

		Convey("Invalid datetimes", func() {
			invalidDatetimes := []string{
				"",
				"2023-13-01-12-30-45", // 非法月份
				"2023-01-32-12-30-45", // 非法日期
				"2023-01-01-24-30-45", // 非法小时
				"2023-01-01-12-60-45", // 非法分钟
				"2023-01-01-12-30-60", // 非法秒
				"2023-01-01-12-30",    // 缺少秒
				"2023-01-01-12",       // 缺少分和秒
				"23-01-01-12-30-45",   // 2 位年份
			}

			for _, datetime := range invalidDatetimes {
				Convey(fmt.Sprintf("Datetime '%s' should be invalid", datetime), func() {
					So(IsDatetime(datetime), ShouldBeFalse)
				})
			}
		})
	})
}

// TestSlugValidation 测试 slug 校验
func TestSlugValidation(t *testing.T) {
	Convey("Test slug validation", t, func() {
		Convey("Valid slugs", func() {
			validSlugs := []string{
				"hello",
				"world",
				"hello-world",
				"test-slug",
				"a-b-c-d",
				"123",
				"test-123",
				"a",
				"a-b",
				"very-long-slug-name-with-many-hyphens",
			}

			for _, slug := range validSlugs {
				Convey(fmt.Sprintf("Slug '%s' should be valid", slug), func() {
					So(IsSlug(slug), ShouldBeTrue)
				})
			}
		})

		Convey("Invalid slugs", func() {
			invalidSlugs := []string{
				"",
				"-hello",       // 以连字符开头
				"hello-",       // 以连字符结尾
				"hello--world", // 连续双连字符
				"hello world",  // 含空格
				"Hello-World",  // 含大写字母
				"hello.world",  // 含句点
				"hello@world",  // 含特殊字符
			}

			for _, slug := range invalidSlugs {
				Convey(fmt.Sprintf("Slug '%s' should be invalid", slug), func() {
					So(IsSlug(slug), ShouldBeFalse)
				})
			}
		})
	})
}

// namedKind / namedKindPayload 用于验证自定义标签（enum / trimmed）可直接作用于具名 string 类型，
// 即 pkg/llm 的枚举取值类型这类字段。
type namedKind string

type namedKindPayload struct {
	Kind namedKind `json:"kind" binding:"omitempty,trimmed,enum=validatorTestKind"`
}

// TestGenericFieldValidations 覆盖 enum / httpUrl / jsonObject / trimmed 四个通用字段级校验：
// 它们供各视图层 In 结构体以绑定标签直接引用，取代各自就地书写的解析与校验方法。
func TestGenericFieldValidations(t *testing.T) {
	// 注册仅供测试使用的枚举类，避免与业务枚举类的注册冲突
	enum.Register(enum.Type{Name: "validatorTestKind", Entries: []enum.Entry{
		{Key: "alpha", Value: "alpha"},
		{Key: "beta", Value: "beta"},
	}})

	Convey("Test generic field validations", t, func() {
		Convey("enum accepts exactly the values of the registered class", func() {
			So(IsValidEnum("validatorTestKind", "alpha"), ShouldBeTrue)
			So(IsValidEnum("validatorTestKind", "beta"), ShouldBeTrue)
			// 按 value 严格匹配：类外取值、大小写变体与带空白的取值一律拒绝
			So(IsValidEnum("validatorTestKind", "gamma"), ShouldBeFalse)
			So(IsValidEnum("validatorTestKind", "Alpha"), ShouldBeFalse)
			So(IsValidEnum("validatorTestKind", " alpha"), ShouldBeFalse)
			So(IsValidEnum("validatorTestKind", ""), ShouldBeFalse)
		})

		Convey("enum panics on an unregistered class (a tag parameter typo)", func() {
			So(func() { IsValidEnum("validatorTestKindMissing", "alpha") }, ShouldPanic)
		})

		Convey("httpUrl accepts absolute http / https URLs only", func() {
			for _, valid := range []string{
				"http://example.com/mcp",
				"https://example.com",
				"https://example.com:8443/mcp?x=1",
			} {
				Convey(fmt.Sprintf("URL '%s' should be valid", valid), func() {
					So(IsValidHTTPURL(valid), ShouldBeTrue)
				})
			}
			for _, invalid := range []string{
				"",
				"example.com",
				"/mcp",
				"ftp://example.com",
				"ws://example.com",
				"http://",
			} {
				Convey(fmt.Sprintf("URL '%s' should be invalid", invalid), func() {
					So(IsValidHTTPURL(invalid), ShouldBeFalse)
				})
			}
		})

		Convey("jsonObject accepts object text whose values are all strings", func() {
			for _, valid := range []string{
				"{}",
				`{"Authorization":"Bearer x"}`,
				`{"A":"1","B":"2"}`,
			} {
				Convey(fmt.Sprintf("JSON '%s' should be valid", valid), func() {
					So(IsValidJSONObject(valid), ShouldBeTrue)
				})
			}
			for _, invalid := range []string{
				"",
				"null",
				"[]",
				`"text"`,
				`{"A":1}`,
				`{"A":{"B":"1"}}`,
				"{",
			} {
				Convey(fmt.Sprintf("JSON '%s' should be invalid", invalid), func() {
					So(IsValidJSONObject(invalid), ShouldBeFalse)
				})
			}
		})

		Convey("trimmed rejects leading or trailing whitespace", func() {
			for _, valid := range []string{"", "abc", "a b", "中文"} {
				Convey(fmt.Sprintf("value '%s' should be trimmed", valid), func() {
					So(IsTrimmed(valid), ShouldBeTrue)
				})
			}
			for _, invalid := range []string{" abc", "abc ", "\tabc", "abc\n", " "} {
				Convey(fmt.Sprintf("value '%q' should not be trimmed", invalid), func() {
					So(IsTrimmed(invalid), ShouldBeFalse)
				})
			}
		})

		Convey("custom tags apply to a named string type (enum value types)", func() {
			v := newTestValidate()
			// 合法取值通过；空值由 omitempty 放行
			So(v.Struct(namedKindPayload{Kind: "beta"}), ShouldBeNil)
			So(v.Struct(namedKindPayload{Kind: ""}), ShouldBeNil)
			// 带首尾空白被 trimmed 拒绝（只判定、不静默裁剪）
			So(v.Struct(namedKindPayload{Kind: " beta"}), ShouldNotBeNil)
			// 类外取值被 enum 拒绝
			So(v.Struct(namedKindPayload{Kind: "gamma"}), ShouldNotBeNil)
		})
	})
}

// TestIsValidJSONTypedValue 测试「按类型名校验已解码 JSON 值」的通用能力，覆盖五类基本类型、
// int 的整数要求与未知类型名的 fail-closed 行为。
func TestIsValidJSONTypedValue(t *testing.T) {
	Convey("Test dynamic JSON value type validation", t, func() {
		Convey("string", func() {
			So(IsValidJSONTypedValue(JSONTypeString, "abc"), ShouldBeTrue)
			So(IsValidJSONTypedValue(JSONTypeString, ""), ShouldBeTrue)
			So(IsValidJSONTypedValue(JSONTypeString, float64(1)), ShouldBeFalse)
		})

		Convey("int requires an integral value", func() {
			So(IsValidJSONTypedValue(JSONTypeInt, float64(3)), ShouldBeTrue)
			So(IsValidJSONTypedValue(JSONTypeInt, float64(3.5)), ShouldBeFalse)
			So(IsValidJSONTypedValue(JSONTypeInt, "3"), ShouldBeFalse)
		})

		Convey("bool", func() {
			So(IsValidJSONTypedValue(JSONTypeBool, true), ShouldBeTrue)
			So(IsValidJSONTypedValue(JSONTypeBool, float64(1)), ShouldBeFalse)
		})

		Convey("object", func() {
			So(IsValidJSONTypedValue(JSONTypeObject, map[string]any{"a": 1}), ShouldBeTrue)
			So(IsValidJSONTypedValue(JSONTypeObject, []any{}), ShouldBeFalse)
			So(IsValidJSONTypedValue(JSONTypeObject, map[string]any(nil)), ShouldBeFalse)
		})

		Convey("array", func() {
			So(IsValidJSONTypedValue(JSONTypeArray, []any{"a"}), ShouldBeTrue)
			So(IsValidJSONTypedValue(JSONTypeArray, map[string]any{}), ShouldBeFalse)
		})

		Convey("unknown type name fails closed", func() {
			So(IsValidJSONTypedValue("integer", float64(1)), ShouldBeFalse)
			So(IsValidJSONTypedValue("", "abc"), ShouldBeFalse)
		})
	})
}
