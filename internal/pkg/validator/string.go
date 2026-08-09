package validator

import (
	"encoding/base32"
	"encoding/base64"
	"fmt"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/dlclark/regexp2"
	"github.com/go-playground/validator/v10"
	"zhongkui.com/zhongkui/internal/pkg/codes"
)

const (
	TagStrongPassword  = "strongPassword"
	TagWindowsFileName = "windowsFileName"
	TagUnixFileName    = "unixFileName"
	TagFileName        = "fileName"
	TagStrictFileName  = "strictFileName"
	TagDirpath         = "dirpath"
	TagAlphanumSpecial = "alphanumspecial"
)

// alphanumSpecialRegexps 按特殊字符参数缓存编译后的校验正则。
var alphanumSpecialRegexps sync.Map

var (
	windowsReservedNames = map[string]any{
		"CON":  nil,
		"PRN":  nil,
		"AUX":  nil,
		"NUL":  nil,
		"COM1": nil,
		"COM2": nil,
		"COM3": nil,
		"COM4": nil,
		"COM5": nil,
		"COM6": nil,
		"COM7": nil,
		"COM8": nil,
		"COM9": nil,
		"LPT1": nil,
		"LPT2": nil,
		"LPT3": nil,
		"LPT4": nil,
		"LPT5": nil,
		"LPT6": nil,
		"LPT7": nil,
		"LPT8": nil,
		"LPT9": nil,
	}
)

var (
	strongPasswordRegex      = regexp2.MustCompile(`^(?=.*[a-z])(?=.*[A-Z])(?=.*\d)(?=.*[@$!%*?&#])[A-Za-z\d@$!%*?&#]{6,}$`, regexp2.DefaultUnmarshalOptions)
	windowsIllegalCharsRegex = regexp2.MustCompile(`^(.*[<>:"/\\|?*\x00-\x1F].*|.*[.\s])$`, regexp2.DefaultUnmarshalOptions)
	unixIllegalCharsRegex    = regexp2.MustCompile(`^(\.*|.*[/\x00].*)$`, regexp2.DefaultUnmarshalOptions)
	hexRegex                 = regexp2.MustCompile(`^[0-9a-fA-F]{1,}$`, regexp2.DefaultUnmarshalOptions)
	uuidRegex                = regexp2.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`, regexp2.DefaultUnmarshalOptions)

	// date
	// YYYY-Q[1-4]
	yearQuarterRegex = regexp2.MustCompile(`^\d{4}-[qQ][1-4]$`, regexp2.DefaultUnmarshalOptions)
	// YYYY-W[01-53]
	yearWeekyRegex = regexp2.MustCompile(`^\d{4}-[wW](?:0[1-9]|[1-4]\d|5[0-3])`, regexp2.DefaultUnmarshalOptions)

	// complex
	slugRegex = regexp2.MustCompile(`^\b[a-z0-9]+(?:[-_][a-z0-9]+)*\b$`, regexp2.DefaultUnmarshalOptions)
)

func IsValidStrongPassword(value string) bool {
	// The strong password must contain at least one digit, one uppercase letter, one lowercase letter, and one special character.
	if ok, err := strongPasswordRegex.MatchString(value); err == nil && ok {
		return true
	}
	return false
}

func IsValidWindowsFileName(value string) bool {
	if value == "" {
		return false
	}
	// 去除扩展名检查（不区分大小写）
	baseName := strings.ToUpper(strings.TrimSuffix(value, filepath.Ext(value)))
	if _, ok := windowsReservedNames[baseName]; ok {
		return false
	}

	// Windows非法字符
	if ok, err := windowsIllegalCharsRegex.MatchString(value); err != nil || ok {
		return false
	}

	// 不能以空格或点结尾
	if strings.HasSuffix(value, " ") || strings.HasSuffix(value, ".") {
		return false
	}

	// 长度限制（不包括路径）
	if len(value) > 255 {
		return false
	}

	return true
}

func IsValidUnixFileName(value string) bool {
	if value == "" {
		return false
	}
	if ok, err := unixIllegalCharsRegex.MatchString(value); err != nil || ok {
		return false
	}

	// 长度限制
	if len(value) > 255 {
		return false
	}

	return true
}

func IsValidFileName(value string) bool {
	// 基本检查
	if value == "" || value == "." || value == ".." {
		return false
	}

	// 检查UTF-8编码有效性
	if !utf8.ValidString(value) {
		return false
	}

	// 根据操作系统使用不同的验证规则
	switch runtime.GOOS {
	case "windows":
		return IsValidWindowsFileName(value)
	case "darwin", "linux": // macOS 和 Linux
		return IsValidUnixFileName(value)
	default:
		// 默认使用Unix风格的规则
		return IsValidUnixFileName(value)
	}
}

func IsStrictValidFileName(value string) bool {
	if !IsValidFileName(value) {
		return false
	}

	// 额外的严格规则
	// 1. 不能包含控制字符
	for _, r := range value {
		if r < 32 { // ASCII控制字符
			return false
		}
	}

	// 2. 不能以连字符开头（避免与命令行参数混淆）
	if strings.HasPrefix(value, "-") {
		return false
	}

	// 3. 不能包含空格（可选规则）
	if strings.Contains(value, " ") {
		return false
	}

	return true
}

func IsValidDirPath(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r == 0 {
			return false
		}
	}
	return true
}

func IsNull(value string) bool {
	lower := strings.ToLower(value)
	return lower == "null" || lower == "nil"
}

func IsBool(value string) bool {
	lower := strings.ToLower(value)
	return lower == "true" || lower == "false"
}

func IsInt(value string) bool {
	if _, err := strconv.ParseInt(value, 10, 64); err != nil {
		return false
	}
	return true
}

func IsFloat(value string) bool {
	if _, err := strconv.ParseFloat(value, 64); err != nil {
		return false
	}
	return true
}

func IsMD5(value string) bool {
	if ok, err := hexRegex.MatchString(value); err == nil && ok && len(value) == 32 {
		return true
	}
	return false
}

func IsSHA1(value string) bool {
	if ok, err := hexRegex.MatchString(value); err == nil && ok && len(value) == 40 {
		return true
	}
	return false
}

func IsSHA256(value string) bool {
	if ok, err := hexRegex.MatchString(value); err == nil && ok && len(value) == 64 {
		return true
	}
	return false
}

func IsHex(value string) bool {
	if ok, err := hexRegex.MatchString(value); err == nil && ok {
		return true
	}
	return false
}

func IsUUID(value string) bool {
	if ok, err := uuidRegex.MatchString(value); err == nil && ok {
		return true
	}
	return false
}

func IsBase32(value string) bool {
	if _, err := base32.StdEncoding.DecodeString(value); err != nil {
		return false
	}
	return true
}

func IsBase64(value string) bool {
	if _, err := base64.StdEncoding.DecodeString(value); err != nil {
		return false
	}
	return true
}

func IsDate(value string) bool {
	// YYYY-MM-DD
	if _, err := time.Parse(time.DateOnly, value); err == nil {
		return true
	}
	// YYYY-MM
	if _, err := time.Parse("2006-01", value); err == nil {
		return true
	}
	// MM-DD-YYYY
	if _, err := time.Parse("01-02-2006", value); err == nil {
		return true
	}
	// DD.MM.YYYY
	if _, err := time.Parse("02.01.2006", value); err == nil {
		return true
	}
	return false
}

func IsYearQuarter(value string) bool {
	if ok, err := yearQuarterRegex.MatchString(value); err == nil && ok {
		return true
	}
	return false
}

func IsYearWeek(value string) bool {
	if ok, err := yearWeekyRegex.MatchString(value); err == nil && ok {
		return true
	}
	return false
}

func IsDatetime(value string) bool {
	if _, err := time.Parse("2006-01-02_15-04-05", value); err == nil {
		return true
	}
	if _, err := time.Parse("2006-01-02-15-04-05", value); err == nil {
		return true
	}
	if _, err := time.Parse("2006_01_02-15_04_05", value); err == nil {
		return true
	}
	if _, err := time.Parse("2006_01_02_15_04_05", value); err == nil {
		return true
	}
	return false
}

func IsCodesISO31661(value string) bool {
	checker := codes.GetISO31661Codes()
	if len(value) == 2 {
		return checker.ConvertFromAlpha2(value) != nil
	} else if len(value) == 3 {
		return checker.ConvertFromAlpha3(value) != nil
	} else {
		return checker.ConvertFromName(value) != nil
	}
}

func IsCodesISO6392(value string) bool {
	checker := codes.GetISO6392Codes()
	if len(value) == 2 {
		return checker.ConvertFromAlpha2(value) != nil
	} else if len(value) == 3 {
		return checker.ConvertFromAlpha3(value) != nil
	} else {
		return checker.ConvertFromName(value) != nil
	}
}

func IsCodesISO4217(value string) bool {
	checker := codes.GetISO4217Codes()
	if len(value) == 3 {
		return checker.ConvertFromAlpha3(value) != nil
	} else {
		return checker.ConvertFromName(value) != nil
	}
}

func IsCodeISO15924(value string) bool {
	checker := codes.GetISO15924Codes()
	if len(value) == 4 {
		return checker.ConvertFromAlpha4(value) != nil
	} else {
		return checker.ConvertFromName(value) != nil
	}
}

func IsCodesRFC1766(value string) bool {
	return codes.GetRFC1766Codes().ConvertFromName(value) != nil
}

func IsCodesRFC4646(value string) bool {
	return codes.GetRFC4646Codes().ConvertFromName(value) != nil
}

func IsCodesFilenameExtension(value string) bool {
	return codes.GetFilenameExtensionCodes().ConvertFromName(value) != nil
}

func IsSlug(value string) bool {
	if ok, err := slugRegex.MatchString(value); err == nil && ok {
		return true
	}
	return false
}

// IsValidAlphanumSpecial 判断字符串是否仅由字母数字（含 Unicode）与
// 指定的特殊字符组成，且至少包含一个字母或数字（不允许仅有特殊符号）。
// specialChars 为允许的特殊字符集合（如 "-_"）。
func IsValidAlphanumSpecial(value, specialChars string) bool {
	if value == "" {
		return false
	}
	if re, ok := alphanumSpecialRegexps.Load(specialChars); ok {
		return matchAlphanumSpecial(re.(*regexp2.Regexp), value)
	}
	pattern := fmt.Sprintf(`^(?=.*[\p{L}\p{N}])[\p{L}\p{N}%s]+$`, regexp.QuoteMeta(specialChars))
	re, err := regexp2.Compile(pattern, regexp2.DefaultUnmarshalOptions)
	if err != nil {
		return false
	}
	loaded, _ := alphanumSpecialRegexps.LoadOrStore(specialChars, re)
	return matchAlphanumSpecial(loaded.(*regexp2.Regexp), value)
}

// matchAlphanumSpecial 用编译后的正则匹配字符串。
func matchAlphanumSpecial(re *regexp2.Regexp, value string) bool {
	if ok, err := re.MatchString(value); err == nil && ok {
		return true
	}
	return false
}

func validateAlphanumSpecial(fl validator.FieldLevel) bool {
	if v, ok := fl.Field().Interface().(string); !ok {
		return false
	} else {
		return IsValidAlphanumSpecial(v, fl.Param())
	}
}

func validateStrongPassword(fl validator.FieldLevel) bool {
	if v, ok := fl.Field().Interface().(string); !ok {
		return false
	} else {
		return IsValidStrongPassword(v)
	}
}

func validateWindowsFileName(fl validator.FieldLevel) bool {
	if v, ok := fl.Field().Interface().(string); !ok {
		return false
	} else {
		return IsValidWindowsFileName(v)
	}
}

func validateUnixFileName(fl validator.FieldLevel) bool {
	if v, ok := fl.Field().Interface().(string); !ok {
		return false
	} else {
		return IsValidUnixFileName(v)
	}
}

func validateFileName(fl validator.FieldLevel) bool {
	if v, ok := fl.Field().Interface().(string); !ok {
		return false
	} else {
		return IsValidFileName(v)
	}
}

func validateStrictFileName(fl validator.FieldLevel) bool {
	if v, ok := fl.Field().Interface().(string); !ok {
		return false
	} else {
		return IsStrictValidFileName(v)
	}
}

func validateDirPath(fl validator.FieldLevel) bool {
	if v, ok := fl.Field().Interface().(string); !ok {
		return false
	} else {
		return IsValidDirPath(v)
	}
}

func init() {
	validations[TagStrongPassword] = validateStrongPassword
	validations[TagWindowsFileName] = validateWindowsFileName
	validations[TagUnixFileName] = validateUnixFileName
	validations[TagFileName] = validateFileName
	validations[TagStrictFileName] = validateStrictFileName
	validations[TagDirpath] = validateDirPath
	validations[TagAlphanumSpecial] = validateAlphanumSpecial
}
