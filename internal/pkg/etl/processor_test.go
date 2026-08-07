package etl

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// mockProcessor 用于测试的模拟处理器，记录 Start/Shutdown 调用状态。
type mockProcessor struct {
	startCalled    bool
	capturedCtx    context.Context
	capturedName   string
	shutdownCalled bool
	startErr       error
}

func (m *mockProcessor) Start(ctx context.Context, name string) error {
	m.startCalled = true
	m.capturedCtx = ctx
	m.capturedName = name
	return m.startErr
}

func (m *mockProcessor) Shutdown() {
	m.shutdownCalled = true
}

// 测试通过 RegisterProcessorFactory 注册处理器工厂，并验证注册表内容。
func TestRegisterProcessorFactory(t *testing.T) {
	Convey("注册处理器工厂", t, func() {
		Convey("注册单个工厂后应可通过 processorRegistry 访问", func() {
			processorRegistry = newRegistry[ProcessorFactory]()

			mp := &mockProcessor{}
			factory := func(ctx context.Context) (Processor, error) {
				return mp, nil
			}
			RegisterProcessorFactory("test_proc", factory)

			So(len(processorRegistry.units), ShouldEqual, 1)
			So(processorRegistry.units["test_proc"], ShouldNotBeNil)
		})

		Convey("注册多个不同名称的工厂，数量应正确", func() {
			processorRegistry = newRegistry[ProcessorFactory]()

			mp1 := &mockProcessor{}
			RegisterProcessorFactory("proc_a", func(ctx context.Context) (Processor, error) {
				return mp1, nil
			})
			mp2 := &mockProcessor{}
			RegisterProcessorFactory("proc_b", func(ctx context.Context) (Processor, error) {
				return mp2, nil
			})
			mp3 := &mockProcessor{}
			RegisterProcessorFactory("proc_c", func(ctx context.Context) (Processor, error) {
				return mp3, nil
			})

			So(len(processorRegistry.units), ShouldEqual, 3)
			So(processorRegistry.units["proc_a"], ShouldNotBeNil)
			So(processorRegistry.units["proc_b"], ShouldNotBeNil)
			So(processorRegistry.units["proc_c"], ShouldNotBeNil)
		})
	})
}

// 测试 ProcessorFactoryRegistry 在无参数注入时返回原始工厂。
func TestProcessorFactoryRegistry_NoParams(t *testing.T) {
	Convey("ProcessorFactoryRegistry 无参数场景", t, func() {
		Convey("注册 2 个工厂后，传入空 params map 应返回 2 个同名工厂，调用返回原始处理器", func() {
			processorRegistry = newRegistry[ProcessorFactory]()

			mp1 := &mockProcessor{}
			RegisterProcessorFactory("proc_a", func(ctx context.Context) (Processor, error) {
				return mp1, nil
			})
			mp2 := &mockProcessor{}
			RegisterProcessorFactory("proc_b", func(ctx context.Context) (Processor, error) {
				return mp2, nil
			})

			factories := ProcessorFactoryRegistry(map[string]map[string]any{})

			So(len(factories), ShouldEqual, 2)
			So(factories["proc_a"], ShouldNotBeNil)
			So(factories["proc_b"], ShouldNotBeNil)

			// 调用工厂，返回的应是未包装的原始处理器
			p1, err := factories["proc_a"](context.Background())
			So(err, ShouldBeNil)
			So(p1, ShouldEqual, mp1)

			p2, err := factories["proc_b"](context.Background())
			So(err, ShouldBeNil)
			So(p2, ShouldEqual, mp2)
		})
	})
}

// 测试 ProcessorFactoryRegistry 在带参数注入场景下，工厂能从上下文获取参数。
func TestProcessorFactoryRegistry_WithParams(t *testing.T) {
	Convey("ProcessorFactoryRegistry 带参数注入", t, func() {
		Convey("注册工厂后传入同名参数，工厂内可通过 GetProcParam 获取注入的参数", func() {
			processorRegistry = newRegistry[ProcessorFactory]()

			type myConfig struct {
				Key string `mapstructure:"key"`
			}

			var capturedConfig *myConfig
			innerProcessor := &mockProcessor{}

			RegisterProcessorFactory("my_proc", func(ctx context.Context) (Processor, error) {
				cfg, err := GetProcParam[myConfig](ctx)
				if err != nil {
					return nil, errors.WithStack(err)
				}
				capturedConfig = cfg
				return innerProcessor, nil
			})

			params := map[string]map[string]any{
				"my_proc": {
					"key": "hello_value",
				},
			}
			factories := ProcessorFactoryRegistry(params)

			So(len(factories), ShouldEqual, 1)
			So(factories["my_proc"], ShouldNotBeNil)

			proc, err := factories["my_proc"](context.Background())
			So(err, ShouldBeNil)
			So(capturedConfig, ShouldNotBeNil)
			So(capturedConfig.Key, ShouldEqual, "hello_value")

			// 返回的应是 ProcessorWrapper
			_, isWrapper := proc.(*ProcessorWrapper)
			So(isWrapper, ShouldBeTrue)
		})
	})
}

// 测试 ProcessorWrapper.Start 会将参数注入上下文并传递给内部处理器。
func TestProcessorWrapper_Start(t *testing.T) {
	Convey("ProcessorWrapper.Start 上下文注入", t, func() {
		Convey("包装处理器后调用 Start，内部处理器收到的上下文应包含 ContextProcParamName 参数", func() {
			inner := &mockProcessor{}
			params := map[string]any{
				"foo": "bar",
			}
			wrapper := &ProcessorWrapper{
				Processor: inner,
				params:    params,
			}

			err := wrapper.Start(context.Background(), "test_proc")
			So(err, ShouldBeNil)

			// 内部处理器应收到注入参数的上下文
			raw, ok := inner.capturedCtx.Value(ContextProcParamName).(map[string]any)
			So(ok, ShouldBeTrue)
			So(raw["foo"], ShouldEqual, "bar")
		})
	})
}

// 测试 ProcessorWrapper.Shutdown 会委托给内部处理器的 Shutdown。
func TestProcessorWrapper_Shutdown(t *testing.T) {
	Convey("ProcessorWrapper.Shutdown 委托", t, func() {
		Convey("调用 Shutdown 应委托到内部处理器的 Shutdown 方法", func() {
			inner := &mockProcessor{}
			wrapper := &ProcessorWrapper{
				Processor: inner,
			}

			// 调用的应是内部处理器的 Shutdown
			So(inner.shutdownCalled, ShouldBeFalse)
			wrapper.Shutdown()
			So(inner.shutdownCalled, ShouldBeTrue)
		})
	})
}
