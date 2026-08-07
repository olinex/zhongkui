package etl

import "context"

// Registry 泛型命名注册表，统一任务 Handler 与常驻 Processor 的注册机制。
// 提供两个操作：
//   - Register：按名称注册单元，由全局注册表持有；
//   - Snapshot：返回注入配置参数后的副本，供运行时（队列 Server / 运行时编排）消费，
//     不影响全局注册表。
//
// 两类单元的差异由调用方通过 wrap 函数表达：如何将同名配置参数注入上下文并包装单元。
type Registry[T any] struct {
	units map[string]T
}

// newRegistry 创建空注册表。
func newRegistry[T any]() *Registry[T] {
	return &Registry[T]{units: make(map[string]T)}
}

// Register 按名称注册单元，同名重复注册时后者覆盖前者。
func (r *Registry[T]) Register(name string, unit T) {
	r.units[name] = unit
}

// Snapshot 返回带参数注入的注册表副本。
// 对 params 中命名的单元，通过 wrap 生成包装后的单元；未命名的单元原样返回。
// 返回的副本与全局注册表相互独立，不污染已注册单元。
func (r *Registry[T]) Snapshot(params map[string]map[string]any, wrap func(name string, unit T, param map[string]any) T) map[string]T {
	snapshot := make(map[string]T, len(r.units))
	for name, unit := range r.units {
		if param, ok := params[name]; ok {
			snapshot[name] = wrap(name, unit, param)
		} else {
			snapshot[name] = unit
		}
	}
	return snapshot
}

// withParam 将配置参数注入上下文，供 GetHandlerParam / GetProcParam 读取。
// key 由调用方指定（ContextHandlerParamName / ContextProcParamName）。
func withParam(ctx context.Context, key string, param map[string]any) context.Context {
	return context.WithValue(ctx, key, param)
}
