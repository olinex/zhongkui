package validator

import (
	"errors"
	"strings"

	"github.com/go-playground/validator/v10"
)

// Check 使用全局校验引擎校验结构体，是 HTTP 之外场景（worker / CLI / LLM 输出等）的统一校验入口。
// 校验失败时原样返回 validator.ValidationErrors，便于上层继续按类型断言分流。
func Check(v any) error {
	return Validate.Struct(v)
}

// CheckPartial 使用全局校验引擎只校验指定字段，是局部校验场景（如部分更新）的统一入口。
// 校验失败时原样返回 validator.ValidationErrors。
func CheckPartial(v any, fields ...string) error {
	return Validate.StructPartial(v, fields...)
}

// Describe 把校验错误渲染为稳定的英文摘要：按出现顺序输出「字段: tag」或「字段: tag=参数」，
// 多项以 "; " 连接。err 为 nil 时返回空串，非校验错误返回其原始消息；校验错误即使被包装也能识别。
func Describe(err error) string {
	if err == nil {
		return ""
	}
	var validationErrors validator.ValidationErrors
	if !errors.As(err, &validationErrors) {
		return err.Error()
	}
	parts := make([]string, 0, len(validationErrors))
	for _, fieldError := range validationErrors {
		part := fieldError.Field() + ": " + fieldError.Tag()
		if param := fieldError.Param(); param != "" {
			part += "=" + param
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, "; ")
}
