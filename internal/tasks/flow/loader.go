package flow

import (
	"context"

	"zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/pipeline"
	"zhongkui.com/zhongkui/internal/provider"
)

func LoadHttpTransportToTable(ctx context.Context, items []any) error {
	scope := pipeline.GetScope(ctx)
	rawDB, err := provider.InvokeRawDatabase(scope)
	if err != nil {
		return errors.WithStack(err)
	}
	models := make([]*network.HttpTransport, 0, len(items))
	for _, item := range items {
		models = append(models, item.(*network.HttpTransport))
	}
	if err := rawDB.WithContext(ctx).Create(models).Error; err != nil {
		return errors.WithStack(err)
	}
	return nil
}
