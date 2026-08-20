package validator

import (
	"fmt"
	"runtime"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

// TestIsValidStrongPassword tests the password validation function
func TestIsValidStrongPassword(t *testing.T) {
	Convey("Test strong password validation", t, func() {
		Convey("Valid strong passwords", func() {
			// Test passwords that meet all requirements
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
			// Test passwords that fail different requirements
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
			// Password with various special characters
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

// TestGenerateStrongPassword tests the strong password generation function
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

// TestIsValidWindowsFileName tests Windows filename validation
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
				"file\x00name.txt", // Null character
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
			// Create a filename longer than 255 characters
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

// TestIsValidUnixFileName tests Unix filename validation
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
				".",                // Forward slash
				"..",               // Null character
				"file\x00name.txt", // Tab should be handled by control character check in IsStrictValidFileName
			}

			for _, name := range invalidNames {
				Convey(fmt.Sprintf("Filename '%s' should be invalid on Unix", name), func() {
					So(IsValidUnixFileName(name), ShouldBeFalse)
				})
			}
		})

		Convey("Invalid Unix filenames - too long", func() {
			// Create a filename longer than 255 characters
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

// TestIsValidFileName tests cross-platform filename validation
func TestIsValidFileName(t *testing.T) {
	Convey("Test cross-platform filename validation", t, func() {
		Convey("Basic invalid filenames", func() {
			invalidNames := []string{
				"", ".", "..",
				"file\x00name.txt",               // Invalid UTF-8
				string([]byte{0xff, 0xfe, 0xfd}), // Invalid UTF-8
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
			// These filenames are valid on Unix but invalid on Windows
			unixOnlyValid := []string{
				"file:name.txt",
				"CON.txt", // Not reserved when lowercase
				"aux.log", // Not reserved when lowercase
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

// TestIsNull tests null value detection
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

// TestIsBool tests boolean value detection
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

// TestIsInt tests integer value detection
func TestIsInt(t *testing.T) {
	Convey("Test integer value detection", t, func() {
		Convey("Valid integers", func() {
			validInts := []string{
				"0",
				"1",
				"-1",
				"123",
				"-456",
				"2147483647",           // Max int32
				"-2147483648",          // Min int32
				"9223372036854775807",  // Max int64
				"-9223372036854775808", // Min int64
				"+123",                 // With plus sign
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
				"+",                    // Just plus sign
				"-",                    // Just minus sign
				"9223372036854775808",  // Overflow int64
				"-9223372036854775809", // Underflow int64
			}

			for _, value := range invalidInts {
				Convey(fmt.Sprintf("Value '%s' should not be recognized as integer", value), func() {
					So(IsInt(value), ShouldBeFalse)
				})
			}
		})
	})
}

// TestIsFloat tests float value detection
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
				"123.", // Trailing decimal point
				".0",   // Leading decimal point
				"+1.5", // With plus sign
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

// TestHashValidation tests hash validation functions
func TestHashValidation(t *testing.T) {
	Convey("MD5 validation", t, func() {
		Convey("Valid MD5 hashes", func() {
			validMD5s := []string{
				"d41d8cd98f00b204e9800998ecf8427e", // Empty string MD5
				"5d41402abc4b2a76b9719d911017c592", // "hello" MD5
				"098f6bcd4621d373cade4e832627b4f6", // "test" MD5
				"0123456789ABCDEF0123456789ABCDEF", // Uppercase
				"aBcDeF1234567890aBcDeF1234567890", // Mixed case
			}

			for _, value := range validMD5s {
				Convey(fmt.Sprintf("MD5 '%s' should be valid", value), func() {
					So(IsMD5(value), ShouldBeTrue)
				})
			}
		})

		Convey("Invalid MD5 hashes", func() {
			invalidMD5s := []string{
				"",                                  // Empty
				"d41d8cd98f00b204e9800998ecf8427",   // 31 chars
				"d41d8cd98f00b204e9800998ecf8427ee", // 33 chars
				"g41d8cd98f00b204e9800998ecf8427e",  // Invalid hex char
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
				"da39a3ee5e6b4b0d3255bfef95601890afd80709", // Empty string SHA1
				"aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d", // "hello" SHA1
				"a94a8fe5ccb19ba61c4c0873d391e987982fbbd3", // "test" SHA1
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
				"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", // Empty string SHA256
				"2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", // "hello" SHA256
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

// TestUUIDValidation tests UUID validation
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
				"550e8400e29b41d4a716446655440000",      // Without dashes
				"550e8400-e29b-41d4-a716-44665544",      // Too short
				"550e8400-e29b-41d4-a716-4466554400000", // Too long
				"550e8400-e29b-41d4-a716-44665544g000",  // Invalid hex char
				"550e8400-e29b-41d4-a716-44665544-0000", // Extra dash
				"g50e8400-e29b-41d4-a716-446655440000",  // Invalid hex
			}

			for _, value := range invalidUUIDs {
				Convey(fmt.Sprintf("UUID '%s' should be invalid", value), func() {
					So(IsUUID(value), ShouldBeFalse)
				})
			}
		})
	})
}

// TestBase64Validation tests Base64 validation
func TestBase64Validation(t *testing.T) {
	Convey("Test Base64 validation", t, func() {
		Convey("Valid Base64 strings", func() {
			validBase64s := []string{
				"aGVsbG8=",                 // "hello"
				"dGVzdA==",                 // "test"
				"SGVsbG8gV29ybGQh",         // "Hello World!"
				"eyJ0ZXN0IjoidmFsdWUifQ==", // {"test":"value"}
				"AA==",                     // Single byte
				"QQ==",                     // 'A'
				"YWJjZGVmZ2g=",             // "abcdefgh"
				"",                         // Empty string is valid base64
			}

			for _, value := range validBase64s {
				Convey(fmt.Sprintf("Base64 '%s' should be valid", value), func() {
					So(IsBase64(value), ShouldBeTrue)
				})
			}
		})

		Convey("Invalid Base64 strings", func() {
			invalidBase64s := []string{
				"!!!",                  // Invalid characters
				"YWJjZGVmZ2g",          // Missing padding
				"YWJjZGRmZ2g===",       // Too much padding
				"YWJjZGRmZ2g=aGVsbG8=", // Invalid characters in the middle
			}

			for _, value := range invalidBase64s {
				Convey(fmt.Sprintf("Base64 '%s' should be invalid", value), func() {
					So(IsBase64(value), ShouldBeFalse)
				})
			}
		})
	})
}

// TestBase32Validation tests Base32 validation
func TestBase32Validation(t *testing.T) {
	Convey("Test Base32 validation", t, func() {
		Convey("Valid Base32 strings", func() {
			validBase32s := []string{
				"NBSWY3DP", // "hello"
				"ORSXG5A=", // "test"
				"",         // Empty string is valid base32
			}

			for _, value := range validBase32s {
				Convey(fmt.Sprintf("Base32 '%s' should be valid", value), func() {
					So(IsBase32(value), ShouldBeTrue)
				})
			}
		})

		Convey("Invalid Base32 strings", func() {
			invalidBase32s := []string{
				"123",    // Numbers only
				"!!!",    // Invalid characters
				"abcdef", // Lowercase letters
			}

			for _, value := range invalidBase32s {
				Convey(fmt.Sprintf("Base32 '%s' should be invalid", value), func() {
					So(IsBase32(value), ShouldBeFalse)
				})
			}
		})
	})
}

// TestDateValidation tests date validation functions
func TestDateValidation(t *testing.T) {
	Convey("Test date validation functions", t, func() {
		Convey("Valid dates", func() {
			validDates := []string{
				"2023-01-01", // Simple date format
				"2023-12-31", // Simple date format
				"2020-02-29", // Leap year
				"2023-01",    // Year-month only
				"01-31-2023", // US format
				"12-25-2023", // US format
				"31.12.2023", // European format
				"01.01.2023", // European format
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
				"2023-13-01", // Invalid month
				"2023-01-32", // Invalid day
				"2023-04-31", // April has 30 days
				"2023-02-29", // Non-leap year Feb 29
				"13-01-2023", // US format invalid month
				"01-32-2023", // US format invalid day
				"31.13.2023", // European format invalid month
				"32.01.2023", // European format invalid day
				"2023-1-1",   // Single digit month/day
				"23-01-01",   // 2-digit year
			}

			for _, date := range invalidDates {
				Convey(fmt.Sprintf("Date '%s' should be invalid", date), func() {
					So(IsDate(date), ShouldBeFalse)
				})
			}
		})
	})
}

// TestYearQuarterValidation tests year-quarter validation
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
				"2023-q1", // Lowercase
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
				"23-Q1",   // 2-digit year
				"2023-Q",  // Missing quarter
				"2023-1",  // Missing Q
				"Q1-2023", // Wrong format
			}

			for _, quarter := range invalidQuarters {
				Convey(fmt.Sprintf("Year-quarter '%s' should be invalid", quarter), func() {
					So(IsYearQuarter(quarter), ShouldBeFalse)
				})
			}
		})
	})
}

// TestYearWeekValidation tests year-week validation
func TestYearWeekValidation(t *testing.T) {
	Convey("Test year-week validation", t, func() {
		Convey("Valid year-weeks", func() {
			validWeeks := []string{
				"2023-W01",
				"2023-W52",
				"2023-W53",
				"2020-W53", // Year with 53 weeks
				"1999-W01",
				"2000-W52",
				"2023-w01", // Lowercase
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
				"2023-W00", // Week 0 doesn't exist
				"2023-W54", // Week 54 doesn't exist
				"23-W01",   // 2-digit year
				"2023-W",   // Missing week number
				"2023-01",  // Missing W
				"W01-2023", // Wrong format
			}

			for _, week := range invalidWeeks {
				Convey(fmt.Sprintf("Year-week '%s' should be invalid", week), func() {
					So(IsYearWeek(week), ShouldBeFalse)
				})
			}
		})
	})
}

// TestDatetimeValidation tests datetime validation
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
				"2023-13-01-12-30-45", // Invalid month
				"2023-01-32-12-30-45", // Invalid day
				"2023-01-01-24-30-45", // Invalid hour
				"2023-01-01-12-60-45", // Invalid minute
				"2023-01-01-12-30-60", // Invalid second
				"2023-01-01-12-30",    // Missing seconds
				"2023-01-01-12",       // Missing minutes and seconds
				"23-01-01-12-30-45",   // 2-digit year
			}

			for _, datetime := range invalidDatetimes {
				Convey(fmt.Sprintf("Datetime '%s' should be invalid", datetime), func() {
					So(IsDatetime(datetime), ShouldBeFalse)
				})
			}
		})
	})
}

// TestSlugValidation tests slug validation
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
				"-hello",       // Starts with hyphen
				"hello-",       // Ends with hyphen
				"hello--world", // Double hyphen
				"hello world",  // Contains space
				"Hello-World",  // Contains uppercase
				"hello.world",  // Contains period
				"hello@world",  // Contains special char
			}

			for _, slug := range invalidSlugs {
				Convey(fmt.Sprintf("Slug '%s' should be invalid", slug), func() {
					So(IsSlug(slug), ShouldBeFalse)
				})
			}
		})
	})
}
