// Package cliinput provides a generic struct-based data extraction pattern
// that combines viper (CLI flags), pterm (interactive prompts), and
// validator (parameter validation).
//
// Define struct fields with the following tags:
//
//	viper:"key"    - viper config key to read first (also marks the field for extraction)
//	prompt:"text"  - interactive prompt text, used if viper has no value
//	mask:"true"    - mask input with '*', for sensitive fields like passwords
//	confirm:"true" - prompt twice and verify both inputs match (for password confirmation)
//	binding:"..."  - standard validator binding tags for validation
//
// Example:
//
//	type RegisterInput struct {
//	    Username string `viper:"username" prompt:"please enter username" binding:"required,min=6"`
//	    Password string `viper:"password" prompt:"please enter password" mask:"true" confirm:"true" binding:"required,min=8"`
//	}
//
//	func run(v *viper.Viper) error {
//	    var in RegisterInput
//	    if err := cliinput.Extract(v, &in); err != nil {
//	        return err
//	    }
//	    // in.Username and in.Password are now populated and validated
//	}
package cliinput

import (
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/pterm/pterm"
	"github.com/spf13/viper"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/validator"
)

// Tag names used in struct field tags.
const (
	TagViper   = "viper"
	TagPrompt  = "prompt"
	TagMask    = "mask"
	TagConfirm = "confirm"
)

// Extract fills a struct pointer from viper first, then falls back to
// pterm interactive input for any fields whose values remain empty.
// Each field is validated immediately after extraction.
//
// Supported field types: string, bool.
// Embedded structs are traversed recursively.
//
// For string fields: if viper returns a non-empty string, it is used directly.
// For bool fields: if viper has the key set, the bool value is used.
//
// Fields without a "viper" struct tag are skipped entirely.
func Extract(v *viper.Viper, target any, prefix string) error {
	val := reflect.ValueOf(target)
	if val.Kind() != reflect.Pointer || val.Elem().Kind() != reflect.Struct {
		return errors.Newf(`cliinput: target must be a pointer to a struct, got %T`, target)
	}
	return extractStruct(v, target, val.Elem(), prefix)
}

func extractStruct(v *viper.Viper, topLevel any, structVal reflect.Value, prefix string) error {
	structType := structVal.Type()

	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		fieldVal := structVal.Field(i)

		// Recurse into struct fields (both anonymous/embedded and named nested structs)
		// that serve as containers (i.e., don't have their own viper tag).
		if field.Type.Kind() == reflect.Struct && field.Tag.Get(TagViper) == "" {
			subPrefix := prefix + field.Name + "."
			if err := extractStruct(v, topLevel, fieldVal, subPrefix); err != nil {
				return err
			}
			continue
		}

		if !fieldVal.CanSet() {
			continue
		}

		viperKey := field.Tag.Get(TagViper)
		if viperKey == "" {
			continue
		}

		promptText := field.Tag.Get(TagPrompt)
		needMask := field.Tag.Get(TagMask) == "true"
		needConfirm := field.Tag.Get(TagConfirm) == "true"

		fieldPath := prefix + field.Name

		switch fieldVal.Kind() {
		case reflect.String:
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
			if field.Type.Elem().Kind() == reflect.String {
				if err := extractStringSlice(v, viperKey, promptText, fieldPath, fieldVal); err != nil {
					return err
				}
			} else {
				continue
			}

		default:
			// time.Duration has Kind() == Int64, handle by type comparison
			if field.Type == reflect.TypeFor[time.Duration]() {
				if err := extractDuration(v, viperKey, promptText, fieldPath, fieldVal); err != nil {
					return err
				}
			} else {
				// Unsupported field type, skip
				continue
			}
		}

		// Validate the field immediately after setting its value
		if validator.Validate != nil {
			if err := validator.Validate.StructPartial(topLevel, fieldPath); err != nil {
				return errors.Newf("cliinput: invalid %s: %w", fieldPath, err)
			}
		}
	}

	return nil
}

func promptOnce(promptText string, mask bool) (string, error) {
	if mask {
		return pterm.DefaultInteractiveTextInput.WithMask("*").Show(promptText)
	}
	return pterm.DefaultInteractiveTextInput.Show(promptText)
}

func promptWithConfirm(promptText string, mask bool) (string, error) {
	for {
		value, err := promptOnce(promptText, mask)
		if err != nil {
			return "", err
		}

		confirmValue, err := promptOnce(promptText+" (confirm)", mask)
		if err != nil {
			return "", err
		}

		if value == confirmValue {
			return value, nil
		}
		pterm.Warning.Println("values do not match, please try again!")
	}
}

// extractInt handles signed integer fields (int, int8, int16, int32, int64).
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

// extractUint handles unsigned integer fields (uint, uint8, uint16, uint32, uint64).
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

// extractFloat handles float fields (float32, float64).
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

// extractDuration handles time.Duration fields.
func extractDuration(v *viper.Viper, key, promptText, fieldPath string, fieldVal reflect.Value) error {
	if v.IsSet(key) {
		fieldVal.SetInt(int64(v.GetDuration(key)))
		return nil
	}
	if promptText == "" {
		return nil
	}
	raw, err := promptOnce(promptText, false)
	if err != nil {
		return errors.Newf("cliinput: cannot read %s: %w", fieldPath, err)
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return errors.Newf("cliinput: cannot parse %s as duration (e.g. 10s, 5m): %w", fieldPath, err)
	}
	fieldVal.SetInt(int64(d))
	return nil
}

// extractStringSlice handles []string fields.
func extractStringSlice(v *viper.Viper, key, promptText, fieldPath string, fieldVal reflect.Value) error {
	val := v.GetStringSlice(key)
	if len(val) > 0 {
		fieldVal.Set(reflect.ValueOf(val))
		return nil
	}
	if promptText == "" {
		return nil
	}
	raw, err := promptOnce(promptText+" (comma-separated)", false)
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
