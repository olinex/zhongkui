package jsengine

import (
	// 标准库
	"errors"
	"fmt"
)

var (
	// ErrClosed 表示引擎已经 Shutdown，不再接受脚本执行。
	ErrClosed = errors.New("jsengine: engine is closed")
	// ErrNotStarted 表示引擎尚未 Start。
	ErrNotStarted = errors.New("jsengine: engine is not started")
	// ErrNilCallback 表示 Do 收到空的执行函数。
	ErrNilCallback = errors.New("jsengine: nil callback")
)

// LoadError 记录某个注入项执行失败：用于把注册项名称带进启动错误，便于定位是哪段脚本坏了。
type LoadError struct {
	// Registrar 失败的注入项名称
	Registrar string
	// Cause 注入项返回的原始错误
	Cause error
}

// Error 实现 error。
func (e *LoadError) Error() string {
	return fmt.Sprintf("jsengine: registrar %q failed: %v", e.Registrar, e.Cause)
}

// Unwrap 支持 errors.Is / errors.As 追到原始错误。
func (e *LoadError) Unwrap() error {
	return e.Cause
}
