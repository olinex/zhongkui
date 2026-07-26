package pipeline

import (
	"context"

	"github.com/creasty/defaults"
	"github.com/mitchellh/mapstructure"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// 从上下文中获取自定义配置。
func getParam[T any](ctx context.Context, name string) (*T, error) {
	raw, ok := ctx.Value(name).(map[string]any)
	if !ok {
		return nil, errors.Newf(`param %s not found in context`, name)
	}
	var target T
	cfg := mapstructure.DecoderConfig{
		Result:           &target,
		WeaklyTypedInput: true,
		DecodeHook:       mapstructure.StringToTimeDurationHookFunc(),
	}
	if err := defaults.Set(&target); err != nil {
		return nil, errors.WithStack(err)
	} else if decoder, err := mapstructure.NewDecoder(&cfg); err != nil {
		return nil, errors.WithStack(err)
	} else if err := decoder.Decode(raw); err != nil {
		return nil, errors.WithStack(err)
	}
	return &target, nil
}
