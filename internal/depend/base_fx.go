package depend

import (
	"os"

	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"zhongkui.com/zhongkui/internal/pkg/base"
)

func FxLogger() fx.Option {
	if base.DEBUG_ENV_VAR {
		return fx.WithLogger(func() fxevent.Logger {
			return &fxevent.ConsoleLogger{
				W: os.Stdout,
			}
		})
	}
	return fx.WithLogger(func() fxevent.Logger {
		return fxevent.NopLogger
	})
}
