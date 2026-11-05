// 本文件提供 cron 表达式合法性校验：纯函数 IsValidCronSpec 供任意调用方复用，ValidateCronSpec
// 将其接入结构体 binding 标签 `cron`。校验与分发器计算触发点使用同一个 robfig/cron 解析器，
// 保证「接口接受」与「调度可执行」口径一致。
package validator

import (
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/robfig/cron/v3"
)

// TagCron 是 cron 表达式合法性校验的 binding 标签名。
const TagCron = "cron"

// IsValidCronSpec 判断 cron 表达式是否合法：去除首尾空白后必须非空，且能被 robfig/cron 的
// 标准解析器接受（支持 5 段标准 cron 与 @daily 等描述符，不支持带秒的 6 段表达式）。
// 该函数不依赖校验器实例，是纯函数。
func IsValidCronSpec(expr string) bool {
	spec := strings.TrimSpace(expr)
	if spec == "" {
		return false
	}
	_, err := cron.ParseStandard(spec)
	return err == nil
}

// ValidateCronSpec 是 `cron` 标签的校验函数：仅接受字符串字段，非法或非字符串均返回 false。
func ValidateCronSpec(fl validator.FieldLevel) bool {
	if fl.Field().Kind() != reflect.String {
		return false
	}
	return IsValidCronSpec(fl.Field().String())
}
