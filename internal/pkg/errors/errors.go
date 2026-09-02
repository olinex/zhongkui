package errors

// Package errors 提供了增强的错误处理功能，支持堆栈跟踪。
// 该包根据环境变量配置来决定是否启用堆栈跟踪功能。
import (
	e "errors"
	"fmt"

	"github.com/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/base"
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

// WithStack 返回错误的根本原因，并为第三方库或标准库的错误添加堆栈信息。
// 如果错误已经是 StackError 类型，则直接返回原错误。
// 这个函数通常用于包装来自外部库的错误，以添加上下文调用栈。
//
// 示例:
//
//	result, err := someExternalLib.DoSomething()
//	if err != nil {
//	    return WithStack(err)
//	}
func WithStack(err error) error {
	if base.Env.ErrorStack && err != nil {
		switch err.(type) {
		case stackError:
			return err
		default:
			return errors.WithStack(err)
		}
	}
	return err
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

func Join(errs ...error) error {
	return e.Join(errs...)
}
