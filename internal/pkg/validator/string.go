// Package validator 提供基于 go-playground/validator 的自定义校验规则（字符串、时间等），
// 供各接口结构体的 binding 标签按名引用。
package validator

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"net/url"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/dlclark/regexp2"
	"github.com/go-playground/validator/v10"
	"zhongkuisec.com/zhongkui/internal/pkg/codes"
	"zhongkuisec.com/zhongkui/internal/pkg/enum"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// 自定义校验标签名常量：各标签在注册表中按此名绑定到对应的验证函数。
const (
	// TagStrongPassword 强密码标签
	TagStrongPassword = "strongPassword"
	// TagWindowsFileName Windows 文件名标签
	TagWindowsFileName = "windowsFileName"
	// TagUnixFileName Unix 文件名标签
	TagUnixFileName = "unixFileName"
	// TagFileName 文件名标签（按当前操作系统选择规则）
	TagFileName = "fileName"
	// TagStrictFileName 严格文件名标签
	TagStrictFileName = "strictFileName"
	// TagDirpath 目录路径标签
	TagDirpath = "dirpath"
	// TagAlphanumSpecial 字母数字加指定特殊字符标签
	TagAlphanumSpecial = "alphanumspecial"
	// TagEnum 枚举取值标签（标签参数为枚举注册表的类名）
	TagEnum = "enum"
	// TagHTTPURL 绝对 HTTP/HTTPS URL 标签
	TagHTTPURL = "httpUrl"
	// TagJSONObject 字符串化 JSON 对象标签
	TagJSONObject = "jsonObject"
	// TagTrimmed 已去除首尾空白标签
	TagTrimmed = "trimmed"
)

// JSONType* 是 JSON 值类型名常量：供 validator 的 jsonObject 等校验能力与调用方复用，
// 集中在此声明以避免调用方就地书写类型名字面量。
const (
	// JSONTypeString JSON 字符串类型
	JSONTypeString = "string"
	// JSONTypeInt JSON 整数类型
	JSONTypeInt = "int"
	// JSONTypeBool JSON 布尔类型
	JSONTypeBool = "bool"
	// JSONTypeObject JSON 对象类型
	JSONTypeObject = "object"
	// JSONTypeArray JSON 数组类型
	JSONTypeArray = "array"
)

// alphanumSpecialRegexps 按特殊字符参数缓存编译后的校验正则。
var alphanumSpecialRegexps sync.Map

// windowsReservedNames Windows 保留设备名集合（不区分大小写），用作文件名时非法。
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

// 预编译的校验正则：分别用于强密码、Windows/Unix 非法字符、十六进制、UUID、年季/年周与 slug。
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

// IsValidStrongPassword 判断字符串是否符合强密码要求（至少包含一位数字、一个大写字母、
// 一个小写字母和一个特殊字符，长度不少于 6）。
func IsValidStrongPassword(value string) bool {
	// 强密码必须至少包含一位数字、一个大写字母、一个小写字母和一个特殊字符。
	if ok, err := strongPasswordRegex.MatchString(value); err == nil && ok {
		return true
	}
	return false
}

// strongPasswordCharsets 定义生成强密码的字符集分组，
// 与 strongPasswordRegex 允许的字符集保持一致，
// 保证生成的随机密码总能通过强密码校验。
var strongPasswordCharsets = []string{
	"abcdefghijklmnopqrstuvwxyz",
	"ABCDEFGHIJKLMNOPQRSTUVWXYZ",
	"0123456789",
	"@$!%*?&#",
}

// GenerateStrongPassword 生成指定长度且符合强密码要求的随机密码。
//
// 该函数从 strongPasswordCharsets 的每个字符集分组中至少选取一个字符，
// 保证密码同时包含小写字母、大写字母、数字与特殊字符，
// 其余字符从全字符集中随机选取，最后通过 Fisher-Yates 洗牌打乱顺序，
// 避免出现可预测的前缀模式。随机数源为 crypto/rand，
// 生成的密码具备密码学安全性，且始终能通过 IsValidStrongPassword 校验。
//
// 参数：
//   - length: 期望生成的密码长度，必须不小于 6（strongPasswordRegex 允许的最小长度）。
//
// 返回值：
//   - string: 符合强密码要求的随机密码，长度为 length。
//   - error: 如果 length 小于 6 或随机数生成失败，则返回错误。
func GenerateStrongPassword(length int) (string, error) {
	if length < 6 {
		return "", errors.Newf("validator: strong password length must be at least 6, got %d", length)
	}
	all := strings.Join(strongPasswordCharsets, "")
	password := make([]byte, 0, length)
	// 每类字符集至少选取一个字符，确保满足强密码要求
	for _, charset := range strongPasswordCharsets {
		idx, err := randomIndex(len(charset))
		if err != nil {
			return "", err
		}
		password = append(password, charset[idx])
	}
	// 填充剩余字符
	for len(password) < length {
		idx, err := randomIndex(len(all))
		if err != nil {
			return "", err
		}
		password = append(password, all[idx])
	}
	// Fisher-Yates 洗牌，打乱固定前缀顺序
	for i := len(password) - 1; i > 0; i-- {
		j, err := randomIndex(i + 1)
		if err != nil {
			return "", err
		}
		password[i], password[j] = password[j], password[i]
	}
	return string(password), nil
}

// randomIndex 使用密码学安全随机数生成器返回 [0, max) 范围内的随机索引。
func randomIndex(max int) (int, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0, errors.WithStack(err)
	}
	return int(n.Int64()), nil
}

// IsValidWindowsFileName 判断字符串是否为合法的 Windows 文件名（排除保留设备名、
// 非法字符、以空格或点结尾以及超长的情况）。
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

// IsValidUnixFileName 判断字符串是否为合法的 Unix 文件名（排除非法字符并限制长度）。
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

// IsValidFileName 判断字符串是否为合法文件名：按当前操作系统选择 Windows 或 Unix 规则。
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

// IsStrictValidFileName 在 IsValidFileName 基础上施加更严格的规则：不含控制字符、
// 不以连字符开头且不含空格。
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

// IsValidDirPath 判断字符串是否为合法目录路径（非空且不含 NUL 字符）。
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

// IsNull 判断字符串是否为 null / nil（不区分大小写）。
func IsNull(value string) bool {
	lower := strings.ToLower(value)
	return lower == "null" || lower == "nil"
}

// IsBool 判断字符串是否为布尔字面量 true / false（不区分大小写）。
func IsBool(value string) bool {
	lower := strings.ToLower(value)
	return lower == "true" || lower == "false"
}

// IsInt 判断字符串是否可解析为整数。
func IsInt(value string) bool {
	if _, err := strconv.ParseInt(value, 10, 64); err != nil {
		return false
	}
	return true
}

// IsFloat 判断字符串是否可解析为浮点数。
func IsFloat(value string) bool {
	if _, err := strconv.ParseFloat(value, 64); err != nil {
		return false
	}
	return true
}

// IsMD5 判断字符串是否为 32 位十六进制 MD5 摘要。
func IsMD5(value string) bool {
	if ok, err := hexRegex.MatchString(value); err == nil && ok && len(value) == 32 {
		return true
	}
	return false
}

// IsSHA1 判断字符串是否为 40 位十六进制 SHA1 摘要。
func IsSHA1(value string) bool {
	if ok, err := hexRegex.MatchString(value); err == nil && ok && len(value) == 40 {
		return true
	}
	return false
}

// IsSHA256 判断字符串是否为 64 位十六进制 SHA256 摘要。
func IsSHA256(value string) bool {
	if ok, err := hexRegex.MatchString(value); err == nil && ok && len(value) == 64 {
		return true
	}
	return false
}

// IsHex 判断字符串是否为十六进制串。
func IsHex(value string) bool {
	if ok, err := hexRegex.MatchString(value); err == nil && ok {
		return true
	}
	return false
}

// IsUUID 判断字符串是否符合标准 UUID 格式。
func IsUUID(value string) bool {
	if ok, err := uuidRegex.MatchString(value); err == nil && ok {
		return true
	}
	return false
}

// IsBase32 判断字符串是否为合法的 Base32 编码。
func IsBase32(value string) bool {
	if _, err := base32.StdEncoding.DecodeString(value); err != nil {
		return false
	}
	return true
}

// IsBase64 判断字符串是否为合法的 Base64 编码。
func IsBase64(value string) bool {
	if _, err := base64.StdEncoding.DecodeString(value); err != nil {
		return false
	}
	return true
}

// IsDate 判断字符串是否为支持的日期格式（YYYY-MM-DD、YYYY-MM、MM-DD-YYYY、DD.MM.YYYY）。
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

// IsYearQuarter 判断字符串是否为年季格式（YYYY-Q[1-4]）。
func IsYearQuarter(value string) bool {
	if ok, err := yearQuarterRegex.MatchString(value); err == nil && ok {
		return true
	}
	return false
}

// IsYearWeek 判断字符串是否为年周格式（YYYY-W[01-53]）。
func IsYearWeek(value string) bool {
	if ok, err := yearWeekyRegex.MatchString(value); err == nil && ok {
		return true
	}
	return false
}

// IsDatetime 判断字符串是否为支持的日期时间格式（以 - 或 _ 分隔的多种变体）。
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

// IsCodesISO31661 判断字符串是否为合法的 ISO 3166-1 国家/地区代码（alpha-2/alpha-3/名称）。
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

// IsCodesISO6392 判断字符串是否为合法的 ISO 639-2 语言代码（alpha-2/alpha-3/名称）。
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

// IsCodesISO4217 判断字符串是否为合法的 ISO 4217 货币代码（alpha-3/名称）。
func IsCodesISO4217(value string) bool {
	checker := codes.GetISO4217Codes()
	if len(value) == 3 {
		return checker.ConvertFromAlpha3(value) != nil
	} else {
		return checker.ConvertFromName(value) != nil
	}
}

// IsCodeISO15924 判断字符串是否为合法的 ISO 15924 文字代码（alpha-4/名称）。
func IsCodeISO15924(value string) bool {
	checker := codes.GetISO15924Codes()
	if len(value) == 4 {
		return checker.ConvertFromAlpha4(value) != nil
	} else {
		return checker.ConvertFromName(value) != nil
	}
}

// IsCodesRFC1766 判断字符串是否为合法的 RFC 1766 语言标签。
func IsCodesRFC1766(value string) bool {
	return codes.GetRFC1766Codes().ConvertFromName(value) != nil
}

// IsCodesRFC4646 判断字符串是否为合法的 RFC 4646 语言标签。
func IsCodesRFC4646(value string) bool {
	return codes.GetRFC4646Codes().ConvertFromName(value) != nil
}

// IsCodesFilenameExtension 判断字符串是否为已知的文件扩展名。
func IsCodesFilenameExtension(value string) bool {
	return codes.GetFilenameExtensionCodes().ConvertFromName(value) != nil
}

// IsSlug 判断字符串是否为 slug 格式（小写字母数字，以 - 或 _ 连接）。
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

// ValidateAlphanumSpecial 是 alphanumspecial 标签的验证函数，标签参数为允许的特殊字符集合。
func ValidateAlphanumSpecial(fl validator.FieldLevel) bool {
	if v, ok := fl.Field().Interface().(string); !ok {
		return false
	} else {
		return IsValidAlphanumSpecial(v, fl.Param())
	}
}

// ValidateStrongPassword 是 strongPassword 标签的验证函数。
func ValidateStrongPassword(fl validator.FieldLevel) bool {
	if v, ok := fl.Field().Interface().(string); !ok {
		return false
	} else {
		return IsValidStrongPassword(v)
	}
}

// ValidateWindowsFileName 是 windowsFileName 标签的验证函数。
func ValidateWindowsFileName(fl validator.FieldLevel) bool {
	if v, ok := fl.Field().Interface().(string); !ok {
		return false
	} else {
		return IsValidWindowsFileName(v)
	}
}

// ValidateUnixFileName 是 unixFileName 标签的验证函数。
func ValidateUnixFileName(fl validator.FieldLevel) bool {
	if v, ok := fl.Field().Interface().(string); !ok {
		return false
	} else {
		return IsValidUnixFileName(v)
	}
}

// ValidateFileName 是 fileName 标签的验证函数。
func ValidateFileName(fl validator.FieldLevel) bool {
	if v, ok := fl.Field().Interface().(string); !ok {
		return false
	} else {
		return IsValidFileName(v)
	}
}

// ValidateStrictFileName 是 strictFileName 标签的验证函数。
func ValidateStrictFileName(fl validator.FieldLevel) bool {
	if v, ok := fl.Field().Interface().(string); !ok {
		return false
	} else {
		return IsStrictValidFileName(v)
	}
}

// ValidateDirPath 是 dirpath 标签的验证函数。
func ValidateDirPath(fl validator.FieldLevel) bool {
	if v, ok := fl.Field().Interface().(string); !ok {
		return false
	} else {
		return IsValidDirPath(v)
	}
}

// IsValidEnum 判断 value 是否为指定枚举类（enum 注册表的类名）的合法取值。值空间唯一来源是
// internal/pkg/enum 注册表，调用方一律以 `binding:"enum=<类名>"` 引用，禁止就地书写枚举字面量
// 或 switch 分支。按条目的 Value 严格匹配，与「数据库字段存储 value、不可存 key」的约定一致：
// In 结构体的入参语义即即将落库的取值，接受 key 会让展示键被写进数据库。
// className 未注册属编程缺陷（标签参数写错），此时 panic 而非返回 false——静默地把所有入参判为
// 非法会让缺陷伪装成用户的输入问题。
func IsValidEnum(className, value string) bool {
	class, ok := enum.Get(className)
	if !ok {
		panic(fmt.Sprintf("validator: enum type %q is not registered, check the enum tag parameter", className))
	}
	if _, ok := class.EntryByValue(value); !ok {
		return false
	}
	return true
}

// IsValidHTTPURL 判断字符串是否为绝对的 http / https URL（scheme 与 host 均非空）。空字符串、
// 相对路径、其他 scheme（file / ftp / ws 等）一律不合法；字段可选时以 `omitempty` 放行空值。
func IsValidHTTPURL(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	return parsed.Host != ""
}

// IsValidJSONObject 判断字符串是否为「所有值均为字符串的 JSON 对象」文本，用于承载嵌套在字符串
// 字段里的结构化配置（如请求头）。空字符串不合法；字段可选时以 `omitempty` 放行空值。
// 字面量 null 反序列化到 map 不报错但得到 nil map，故须显式排除，否则「null」会被当成合法对象。
func IsValidJSONObject(value string) bool {
	var obj map[string]string
	if err := json.Unmarshal([]byte(value), &obj); err != nil {
		return false
	}
	return obj != nil
}

// IsValidJSONTypedValue 按声明的类型名校验一个已解码的 JSON 值：value 须来自 encoding/json 对
// 值的解码（数字为 float64、对象为 map[string]any、数组为 []any、布尔为 bool、字符串为 string）。
// 「动态 schema → 值」的校验无法经结构体 binding 标签表达，故在此提供通用能力，避免调用方就地
// 堆砌 if-else：string/int/bool/object/array 五类做基本类型判定，int 额外要求无小数部分；未知
// 类型名一律判为不合法（fail closed，目录侧已由 fail-fast 保证只登记已知类型）。
func IsValidJSONTypedValue(typeName string, value any) bool {
	switch typeName {
	case JSONTypeString:
		_, ok := value.(string)
		return ok
	case JSONTypeInt:
		n, ok := value.(float64)
		return ok && n == math.Trunc(n)
	case JSONTypeBool:
		_, ok := value.(bool)
		return ok
	case JSONTypeObject:
		obj, ok := value.(map[string]any)
		return ok && obj != nil
	case JSONTypeArray:
		_, ok := value.([]any)
		return ok
	default:
		return false
	}
}

// IsTrimmed 判断字符串首尾是否均无空白字符。空白由前端输入框负责去除（见 AGENTS.md 前端约定），
// 后端只做判定、不做静默裁剪——静默裁剪会让「前端已提交的原值」与「实际落库值」不一致，
// 排查时看不到用户真正输入了什么。空字符串视为已去空白（是否允许为空由 required 决定）。
func IsTrimmed(value string) bool {
	return strings.TrimSpace(value) == value
}

// stringFieldValue 取出字符串类字段的值，支持具名 string 类型（如 pkg/llm 的枚举取值类型），
// 使自定义标签（enum / httpUrl / jsonObject / trimmed）可直接施加于枚举字段；非 string 类的
// 字段返回 false。
func stringFieldValue(fl validator.FieldLevel) (string, bool) {
	f := fl.Field()
	if f.Kind() != reflect.String {
		return "", false
	}
	return f.String(), true
}

// ValidateEnum 是 enum 标签的验证函数，标签参数为 enum 注册表的类名，例如
// `binding:"enum=mcpTransport"`。
func ValidateEnum(fl validator.FieldLevel) bool {
	v, ok := stringFieldValue(fl)
	if !ok {
		return false
	}
	return IsValidEnum(fl.Param(), v)
}

// ValidateHTTPURL 是 httpUrl 标签的验证函数。
func ValidateHTTPURL(fl validator.FieldLevel) bool {
	v, ok := stringFieldValue(fl)
	if !ok {
		return false
	}
	return IsValidHTTPURL(v)
}

// ValidateJSONObject 是 jsonObject 标签的验证函数。
func ValidateJSONObject(fl validator.FieldLevel) bool {
	v, ok := stringFieldValue(fl)
	if !ok {
		return false
	}
	return IsValidJSONObject(v)
}

// ValidateTrimmed 是 trimmed 标签的验证函数。
func ValidateTrimmed(fl validator.FieldLevel) bool {
	v, ok := stringFieldValue(fl)
	if !ok {
		return false
	}
	return IsTrimmed(v)
}
