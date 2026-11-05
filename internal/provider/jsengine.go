package provider

import (
	// 标准库
	"context"

	// 第三方
	"github.com/samber/do/v2"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/jsengine"
)

// newJSEngineProvider 构建进程内唯一的 JS 运行时：构造后立即 Start，执行各包在 init 阶段
// 登记的注入项（例如 mermaid 的官方 bundle），使首次工具调用不再承担加载延迟。加载失败视为
// 启动期缺陷，由调用方（MustInvokeJSEngine）中止启动。引擎实现 Shutdowner，容器关闭时由
// samber/do 统一中断执行并释放 VM。
func newJSEngineProvider(ctx context.Context) do.Provider[*jsengine.Engine] {
	return func(_ do.Injector) (*jsengine.Engine, error) {
		engine := jsengine.New()
		if err := engine.Start(ctx); err != nil {
			return nil, errors.WithStack(err)
		}
		return engine, nil
	}
}

// InjectJSEngine 向依赖注入器注册 JS 运行时 provider。
func InjectJSEngine(ctx context.Context, i do.Injector) {
	do.Provide(i, newJSEngineProvider(ctx))
}

// InvokeJSEngine 从依赖注入器解析 JS 运行时，错误交由调用方处理。
func InvokeJSEngine(i do.Injector) (*jsengine.Engine, error) {
	return do.Invoke[*jsengine.Engine](i)
}

// MustInvokeJSEngine 从依赖注入器解析 JS 运行时；失败时 panic（仅用于启动关键依赖）。
func MustInvokeJSEngine(i do.Injector) *jsengine.Engine {
	return do.MustInvoke[*jsengine.Engine](i)
}
