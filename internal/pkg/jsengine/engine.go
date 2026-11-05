// Package jsengine 是进程内唯一的 goja JavaScript 运行时组件：生命周期由依赖注入容器统一管理
// （启动时预热、关闭时释放），外部通过注册函数把嵌入式脚本与 Go 绑定注入同一个 VM。
//
// ## 职责边界
//
// 本包只负责「运行时宿主」：创建/预热 VM、串行化访问、按注册顺序执行注入项、关闭时中断执行。
// 具体脚本（例如官方 mermaid bundle）及其需要的浏览器桩由使用方在自己的包里通过 Register
// 注册，本包不感知任何业务语义——mermaid 只是众多注入项之一。
//
// ## 注册与启动
//
// 注册项通常在包 init 阶段登记（嵌入式脚本用 RegisterScript，需要 Go 绑定的用 Register），
// 引擎 Start 时按登记顺序执行一次。引擎启动后新增的注册项不会回头注入已启动的引擎，
// 因此注册应在启动路径之前完成（常规做法是 init）。
//
// ## 并发
//
// goja 的 Runtime 不是并发安全的：Engine 内所有 VM 访问经互斥锁串行，调用方通过 Do 获得
// 加锁的 VM 访问权；单次脚本调用（如 mermaid.parse 约 50ms）期间会阻塞其他调用方，这是
// 单 VM 方案的固有代价，换来的是极低的内存与启动开销。
package jsengine

import (
	// 标准库
	"context"
	"sync"

	// 第三方
	"github.com/dop251/goja"
)

// Runtime 是 goja 运行时的类型别名：注入项与 Do 回调统一使用 *Runtime 表达「一段可执行脚本的
// 宿主」，使业务包无需直接依赖 goja。
type Runtime = goja.Runtime

// Value 是 goja 值类型的别名，配合 IsUndefined 使用。
type Value = goja.Value

// IsUndefined 报告 value 是否为 undefined。
func IsUndefined(value Value) bool {
	return goja.IsUndefined(value)
}

// Registrar 是一个注入项：在引擎启动时对 VM 做一次初始化，可安装全局桩、绑定 Go 函数或
// 执行嵌入式脚本。Setup 返回错误会中止引擎启动（视为编程缺陷，由调用方决定是否 panic）。
type Registrar struct {
	// Name 注入项名称（唯一，用于错误定位与重复登记校验）
	Name string
	// Setup 在引擎启动时执行一次，返回错误即中止启动
	Setup func(vm *Runtime) error
}

var (
	// registryMu 保护注册表；注册通常发生在 init，但启动期仍可能与注册并发。
	registryMu sync.Mutex
	// registrars 是全局注入项登记表，按登记顺序执行。
	registrars []Registrar
)

// Register 登记一个注入项：名称空或重复即 panic（与内置工具注册表的 fail fast 一致）。
func Register(registrar Registrar) {
	if registrar.Name == "" {
		panic("jsengine: register a registrar with an empty name")
	}
	if registrar.Setup == nil {
		panic("jsengine: register registrar " + registrar.Name + " with a nil setup")
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	for _, existing := range registrars {
		if existing.Name == registrar.Name {
			panic("jsengine: duplicate registrar name: " + registrar.Name)
		}
	}
	registrars = append(registrars, registrar)
}

// RegisterScript 是注册「执行一段嵌入式脚本」的便捷入口：脚本在引擎启动时按登记顺序执行，
// 需要浏览器桩或 Go 绑定的场景请直接用 Register。
func RegisterScript(name string, source []byte) {
	Register(Registrar{
		Name: name,
		Setup: func(vm *Runtime) error {
			_, err := vm.RunString(string(source))
			return err
		},
	})
}

// registrarsSnapshot 返回当前注册项副本，避免启动过程中持有注册表锁执行脚本。
func registrarsSnapshot() []Registrar {
	registryMu.Lock()
	defer registryMu.Unlock()
	return append([]Registrar(nil), registrars...)
}

// Engine 是受管的 goja 运行时：一个进程一份（由 provider 构造并管理生命周期），所有调用串行。
type Engine struct {
	// mu 串行化 Start / Do / Shutdown，保护 vm 与状态字段。
	mu sync.Mutex
	// vm 是已启动的运行时；未启动或已关闭时为空。
	vm *Runtime
	// started 标记 Start 已完成。
	started bool
	// closed 标记 Shutdown 已执行，此后拒绝一切访问。
	closed bool
	// startErr 记录首次启动失败，后续 Do 直接返回该错误。
	startErr error
}

// New 构造一个未启动的引擎；调用方须执行 Start 后才可 Do。
func New() *Engine {
	return &Engine{}
}

// Start 创建 VM 并按注册顺序执行全部注入项；幂等（重复调用直接返回）。ctx 仅用于表达启动时机，
// 脚本执行本身不可取消（单次加载通常在数百毫秒内完成）。
func (e *Engine) Start(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return ErrClosed
	}
	if e.started {
		return e.startErr
	}
	if e.startErr != nil {
		return e.startErr
	}

	vm := goja.New()
	for _, registrar := range registrarsSnapshot() {
		if err := registrar.Setup(vm); err != nil {
			e.startErr = &LoadError{Registrar: registrar.Name, Cause: err}
			return e.startErr
		}
	}
	e.vm = vm
	e.started = true
	return nil
}

// Do 在互斥保护下把 VM 交给 fn 执行。引擎未启动或已关闭时返回错误；fn 返回的错误原样透传。
func (e *Engine) Do(fn func(vm *Runtime) error) error {
	if fn == nil {
		return ErrNilCallback
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return ErrClosed
	}
	if !e.started || e.vm == nil {
		if e.startErr != nil {
			return e.startErr
		}
		return ErrNotStarted
	}
	return fn(e.vm)
}

// Shutdown 实现 do.Shutdowner：中断正在执行的脚本并释放 VM 引用，之后 Do 一律返回 ErrClosed。
// 幂等；VM 对象由 GC 回收（goja 未提供 Close）。
func (e *Engine) Shutdown() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil
	}
	e.closed = true
	if e.vm != nil {
		e.vm.Interrupt("jsengine: shutdown")
		e.vm = nil
	}
	return nil
}
