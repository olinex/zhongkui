// Package cliinput 提供基于结构体的通用数据提取模式，
// 结合 viper（CLI 参数）、pterm（交互式提示）与 validator（参数校验）。
//
// 结构体字段支持以下 tag：
//
//	viper:"key"    - 优先读取的 viper 配置键（同时标记该字段需要提取）
//	prompt:"text"  - 交互式提示文本，viper 无值时使用
//	mask:"true"    - 以 '*' 遮蔽输入，适用于密码等敏感字段
//	confirm:"true" - 提示两次并校验两次输入一致（用于密码确认）
//	binding:"..."  - 标准的 validator binding 校验 tag
//
// 示例：
//
//	type RegisterInput struct {
//	    Username string `viper:"username" prompt:"please enter username" binding:"required,min=6"`
//	    Password string `viper:"password" prompt:"please enter password" mask:"true" confirm:"true" binding:"required,min=8"`
//	}
//
//	func run(v *viper.Viper, validate *validator.Validate) error {
//	    var in RegisterInput
//	    if err := cliinput.Extract(v, validate, &in); err != nil {
//	        return err
//	    }
//	    // in.Username 与 in.Password 此时已填充并通过校验
//	}
package cliinput

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	uv "github.com/go-playground/validator/v10"
	"github.com/pterm/pterm"
	"github.com/spf13/viper"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/pkg/translate"
)

// 结构体字段 tag 使用的 tag 名称。
const (
	TagViper      = "viper"
	TagPrompt     = "prompt"
	TagMask       = "mask"
	TagConfirm    = "confirm"
	CliI18nPrefix = "cli.input"
)

// translatePrompt 翻译交互提示文本，统一走 internal/pkg/translate 的 T 函数。
// 中文文案维护在 locales/zh-Hans.toml，英文在 locales/en.toml。
func translatePrompt(key string) string {
	return translate.T(key, nil)
}

// Extract 从 viper 配置中填充结构体指针，对于 viper 中值为空的字段，
// 回退到 pterm 交互式输入。每个字段在提取后立即进行验证。
//
// 支持的字段类型：string、bool、int/int8/int16/int32/int64、
// uint/uint8/uint16/uint32/uint64、float32/float64、time.Duration、[]string。
// 嵌入结构体和嵌套命名结构体会递归遍历。
//
// 参数说明：
//   - v: viper 实例，用于读取 CLI flag / 配置文件中的值
//   - validate: 验证器实例（binding tag），用于字段赋值后的即时校验
//   - target: 指向目标结构体的指针，结构体字段需带 viper/prompt 等 tag
//   - prefix: 字段路径前缀，用于嵌套结构体的验证路径拼接；
//     顶层调用时传空字符串即可
//
// 提取规则：
//   - string: viper 返回非空字符串则直接使用，否则触发交互提示
//   - bool: viper 中已设置该 key 则使用其值，否则触发交互确认
//   - 数值类型: viper 中已设置则使用，否则交互输入并解析
//   - []string: viper 中已有值则使用，否则交互输入逗号分隔列表
//   - time.Duration: viper 中已设置则使用，否则交互输入并解析（如 "10s", "5m"）
//
// 没有 "viper" struct tag 的字段会被完全跳过。
// 每个字段在赋值后会通过 StructPartial 进行即时校验，校验失败则立即返回错误。
func Extract(v *viper.Viper, validate *uv.Validate, target any, prefix string) error {
	val := reflect.ValueOf(target)
	// 校验 target 必须是指向结构体的指针
	if val.Kind() != reflect.Pointer || val.Elem().Kind() != reflect.Struct {
		return errors.Newf(`cliinput: target must be a pointer to a struct, got %T`, target)
	}
	return extractStruct(v, validate, target, val.Elem(), prefix)
}

// extractStruct 递归遍历结构体字段，依次从 viper 或交互式提示中提取值。
// topLevel 参数用于传递给 validator.StructPartial 进行部分字段校验。
// prefix 用于构建嵌套字段的验证路径（如 "Nested.Field"）。
func extractStruct(v *viper.Viper, validate *uv.Validate, topLevel any, structVal reflect.Value, prefix string) error {
	structType := structVal.Type()

	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		fieldVal := structVal.Field(i)

		// 递归处理嵌套结构体（包括匿名嵌入和命名嵌套）。
		// 仅当该结构体字段自身没有 viper tag 时才视为容器进行递归，
		// 否则按普通字段处理（实际上结构体类型在 switch 中会落入 default 被跳过）。
		if field.Type.Kind() == reflect.Struct && field.Tag.Get(TagViper) == "" {
			subPrefix := prefix + field.Name + "."
			if err := extractStruct(v, validate, topLevel, fieldVal, subPrefix); err != nil {
				return err
			}
			continue
		}

		// 跳过不可设置的字段（如未导出的私有字段）
		if !fieldVal.CanSet() {
			continue
		}

		viperKey := field.Tag.Get(TagViper)
		if viperKey == "" {
			continue
		}

		promptText := field.Tag.Get(TagPrompt)
		if promptText != "" {
			// 交互提示文本按字段级 key 翻译（中文文案维护在 zh-Hans.toml）
			promptText = translatePrompt(fmt.Sprintf(`%s.prompt.%s.%s`, CliI18nPrefix, structType.Name(), field.Name))
		}
		needMask := field.Tag.Get(TagMask) == "true"
		needConfirm := field.Tag.Get(TagConfirm) == "true"

		// fieldPath 用于验证器和错误消息中的字段定位
		fieldPath := prefix + field.Name

		// time.Duration / serializer.Duration / serializer.DurationMS 的底层 Kind 是 Int64，
		// 必须在 Kind-based switch 之前通过类型比较单独处理，否则会被 Int64 分支拦截
		if field.Type == reflect.TypeFor[time.Duration]() || field.Type == reflect.TypeFor[serializer.DurationNS]() || field.Type == reflect.TypeFor[serializer.DurationMS]() {
			if err := extractDuration(v, viperKey, promptText, fieldPath, fieldVal); err != nil {
				return err
			}
		} else {
			switch fieldVal.Kind() {
			case reflect.String:
				// 优先从 viper 读取，为空且配置了 prompt 则交互式输入
				value := v.GetString(viperKey)
				if value == "" && promptText != "" {
					var err error
					if needConfirm {
						value, err = promptWithConfirm(promptText, needMask)
					} else {
						value, err = promptOnce(promptText, needMask)
					}
					if err != nil {
						return errors.Newf("cliinput: cannot read %s: %w", fieldPath, err)
					}
				}
				fieldVal.SetString(value)

			case reflect.Bool:
				// bool 类型使用 IsSet 判断，因为 false 也是有效值
				if v.IsSet(viperKey) {
					fieldVal.SetBool(v.GetBool(viperKey))
				} else if promptText != "" {
					result, err := pterm.DefaultInteractiveConfirm.Show(promptText)
					if err != nil {
						return errors.Newf("cliinput: cannot read %s: %w", fieldPath, err)
					}
					fieldVal.SetBool(result)
				}

			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				if err := extractInt(v, viperKey, promptText, fieldPath, fieldVal); err != nil {
					return err
				}

			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				if err := extractUint(v, viperKey, promptText, fieldPath, fieldVal); err != nil {
					return err
				}

			case reflect.Float32, reflect.Float64:
				if err := extractFloat(v, viperKey, promptText, fieldPath, fieldVal); err != nil {
					return err
				}

			case reflect.Slice:
				// 目前仅支持 []string 类型
				if field.Type.Elem().Kind() == reflect.String {
					if err := extractStringSlice(v, viperKey, promptText, fieldPath, fieldVal); err != nil {
						return err
					}
				} else {
					continue
				}

			default:
				// 不支持的字段类型，跳过
				continue
			}
		}

		if err := validate.StructPartial(topLevel, fieldPath); err != nil {
			return errors.Newf("cliinput: invalid %s: %w", fieldPath, err)
		}
	}

	return nil
}

// promptOnce 执行一次交互式文本输入提示。
// 当 mask 为 true 时，用户输入会以 '*' 遮蔽显示，适用于密码等敏感字段。
func promptOnce(promptText string, mask bool) (string, error) {
	if mask {
		return pterm.DefaultInteractiveTextInput.WithMask("*").Show(promptText)
	}
	return pterm.DefaultInteractiveTextInput.Show(promptText)
}

// promptWithConfirm 执行带确认的交互式输入。
// 用户需输入两次，两次一致才返回；否则循环提示重新输入。
// 用于密码等需要确认的敏感字段（confirm:"true" tag）。
func promptWithConfirm(promptText string, mask bool) (string, error) {
	for {
		value, err := promptOnce(promptText, mask)
		if err != nil {
			return "", err
		}

		confirmValue, err := promptOnce(promptText+translatePrompt(CliI18nPrefix+".prompt.confirm-suffix"), mask)
		if err != nil {
			return "", err
		}

		if value == confirmValue {
			return value, nil
		}
		pterm.Warning.Println(translatePrompt(CliI18nPrefix + ".mismatch"))
	}
}

// extractInt 处理有符号整数字段（int, int8, int16, int32, int64）。
// 优先从 viper 读取；若未设置且提供了 promptText，则交互式输入并解析。
// 解析时使用字段实际位宽（如 int8 为 8 位）进行溢出检查。
func extractInt(v *viper.Viper, key, promptText, fieldPath string, fieldVal reflect.Value) error {
	if v.IsSet(key) {
		fieldVal.SetInt(v.GetInt64(key))
		return nil
	}
	if promptText == "" {
		return nil
	}
	raw, err := promptOnce(promptText, false)
	if err != nil {
		return errors.Newf("cliinput: cannot read %s: %w", fieldPath, err)
	}
	val, err := strconv.ParseInt(raw, 10, fieldVal.Type().Bits())
	if err != nil {
		return errors.Newf("cliinput: cannot parse %s as integer: %w", fieldPath, err)
	}
	fieldVal.SetInt(val)
	return nil
}

// extractUint 处理无符号整数字段（uint, uint8, uint16, uint32, uint64）。
// 优先从 viper 读取；若未设置且提供了 promptText，则交互式输入并解析。
// 解析时使用字段实际位宽（如 uint8 为 8 位）进行溢出检查。
func extractUint(v *viper.Viper, key, promptText, fieldPath string, fieldVal reflect.Value) error {
	if v.IsSet(key) {
		fieldVal.SetUint(v.GetUint64(key))
		return nil
	}
	if promptText == "" {
		return nil
	}
	raw, err := promptOnce(promptText, false)
	if err != nil {
		return errors.Newf("cliinput: cannot read %s: %w", fieldPath, err)
	}
	val, err := strconv.ParseUint(raw, 10, fieldVal.Type().Bits())
	if err != nil {
		return errors.Newf("cliinput: cannot parse %s as unsigned integer: %w", fieldPath, err)
	}
	fieldVal.SetUint(val)
	return nil
}

// extractFloat 处理浮点数字段（float32, float64）。
// 优先从 viper 读取；若未设置且提供了 promptText，则交互式输入并解析。
// 解析时使用字段实际位宽（float32 为 32 位，float64 为 64 位）进行精度控制。
func extractFloat(v *viper.Viper, key, promptText, fieldPath string, fieldVal reflect.Value) error {
	if v.IsSet(key) {
		fieldVal.SetFloat(v.GetFloat64(key))
		return nil
	}
	if promptText == "" {
		return nil
	}
	raw, err := promptOnce(promptText, false)
	if err != nil {
		return errors.Newf("cliinput: cannot read %s: %w", fieldPath, err)
	}
	val, err := strconv.ParseFloat(raw, fieldVal.Type().Bits())
	if err != nil {
		return errors.Newf("cliinput: cannot parse %s as float: %w", fieldPath, err)
	}
	fieldVal.SetFloat(val)
	return nil
}

// extractDuration 处理 time.Duration / serializer.Duration / serializer.DurationMS 类型字段。
// 优先从 viper 读取；若未设置且提供了 promptText，则交互式输入并解析。
// 支持标准 duration 格式，如 "10s"、"5m"、"1h30m" 等；
// DurationMS 字段以毫秒为单位，额外支持纯数字毫秒输入。
func extractDuration(v *viper.Viper, key, promptText, fieldPath string, fieldVal reflect.Value) error {
	isMS := fieldVal.Type() == reflect.TypeFor[serializer.DurationMS]()
	if v.IsSet(key) {
		if isMS {
			fieldVal.SetInt(v.GetInt64(key))
		} else {
			fieldVal.SetInt(int64(v.GetDuration(key)))
		}
		return nil
	}
	if promptText == "" {
		return nil
	}
	raw, err := promptOnce(promptText, false)
	if err != nil {
		return errors.Newf("cliinput: cannot read %s: %w", fieldPath, err)
	}
	if isMS {
		// DurationMS 复用 serializer 的解析：可读时长字符串（"5s"）与整数毫秒均可
		var d serializer.DurationMS
		if err := d.UnmarshalText([]byte(raw)); err != nil {
			return errors.Newf("cliinput: cannot parse %s as duration (e.g. 10s, 5m or milliseconds): %w", fieldPath, err)
		}
		fieldVal.SetInt(int64(d))
		return nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return errors.Newf("cliinput: cannot parse %s as duration (e.g. 10s, 5m): %w", fieldPath, err)
	}
	fieldVal.SetInt(int64(d))
	return nil
}

// extractStringSlice 处理 []string 类型字段。
// 优先从 viper 读取；若为空且提供了 promptText，则交互式输入逗号分隔列表。
// 输入会自动去除首尾空白并过滤空字符串。
func extractStringSlice(v *viper.Viper, key, promptText, fieldPath string, fieldVal reflect.Value) error {
	val := v.GetStringSlice(key)
	if len(val) > 0 {
		fieldVal.Set(reflect.ValueOf(val))
		return nil
	}
	if promptText == "" {
		return nil
	}
	raw, err := promptOnce(promptText+translatePrompt(CliI18nPrefix+".prompt.comma-separated-suffix"), false)
	if err != nil {
		return errors.Newf("cliinput: cannot read %s: %w", fieldPath, err)
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	fieldVal.Set(reflect.ValueOf(result))
	return nil
}
