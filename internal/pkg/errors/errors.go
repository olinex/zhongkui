// Package errors 提供带堆栈跟踪的增强错误处理；是否记录堆栈由环境配置决定。
package errors

import (
	e "errors"
	"fmt"

	"github.com/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/base"
)

type stackError interface {
	StackTrace() errors.StackTrace
}

// New 创建一个新的错误，根据环境变量决定是否包含堆栈信息。
// 如果 ERROR_STACK_ENV_VAR 为 true，则返回带堆栈的错误；
// 否则返回标准库的简单错误。
//
// 示例:
//
//	err := errors.New("something went wrong")
func New(msg string) error {
	if base.Env.ErrorStack {
		return errors.New(msg)
	}
	return e.New(msg)
}

// Newf 根据格式化字符串创建一个新的错误。
// 与 New 类似，根据环境变量决定是否包含堆栈信息。
//
// 示例:
//
//	err := errors.Newf("invalid value: %d", value)
func Newf(format string, args ...any) error {
	if base.Env.ErrorStack {
		return errors.New(fmt.Sprintf(format, args...))
	}
	return fmt.Errorf(format, args...)
}

// WithStack 为第三方库或标准库的错误添加堆栈信息；关闭堆栈时原样返回。
// 已带堆栈的错误不会被重复包装，nil 错误返回 nil。
// 这个函数通常用于包装来自外部库的错误，以添加上下文调用栈。
//
// 示例:
//
//	result, err := someExternalLib.DoSomething()
//	if err != nil {
//	    return WithStack(err)
//	}
func WithStack(err error) error {
	if err == nil {
		return nil
	}
	if !base.Env.ErrorStack {
		return err
	}
	if _, ok := err.(stackError); ok {
		return err
	}
	return errors.WithStack(err)
}

// Wrapf 为已有错误添加上下文并保留错误链；err 为 nil 时返回 nil。
// 关闭堆栈时仍保留可 errors.Is/As 的错误链，只是不附加调用栈。
//
// 示例:
//
//	return errors.Wrapf(err, "load config %q", path)
func Wrapf(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	if base.Env.ErrorStack {
		return errors.Wrapf(err, format, args...)
	}
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), err)
}

// Is 报告目标错误是否与给定的错误匹配。
// 如果 err 是 StackError 类型，会先解包内部的错误再进行比较。
// 支持错误链的递归检查。
//
// 示例:
//
//	if errors.Is(err, io.EOF) {
//	    // 处理 EOF 错误
//	}
func Is(err error, target error) bool {
	return errors.Is(err, target)
}

// As 在错误链中查找与 target 匹配的错误并赋值。
// 与 Is 同理，支持错误链的递归检查，用于按错误类型（而非哨兵值）分流业务错误码。
//
// 示例:
//
//	var mismatch *ai.EmbeddingDimensionMismatchError
//	if errors.As(err, &mismatch) {
//	    // 处理维度不兼容错误
//	}
func As(err error, target any) bool {
	return e.As(err, target)
}

// Join 把多个错误合并为一个错误，nil 错误会被忽略（全部为 nil 时返回 nil）。
func Join(errs ...error) error {
	return e.Join(errs...)
}
