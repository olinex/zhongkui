package flow

import (
	"context"

	"zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/tasks/workers"
)

// LoadHttpTransportToTable 将 HTTP 传输层数据加载到数据库表中。
// 将接收到的 items 转换为 HttpTransport 切片并批量插入数据库。
func LoadHttpTransportToTable(ctx context.Context, items []any) error {
	in := workers.GetFxIn(ctx)
	httpTransports := make([]*network.HttpTransport, 0)
	for _, item := range items {
		httpTransports = append(httpTransports, item.(*network.HttpTransport))
	}
	if err := in.RawDB.WithContext(ctx).Create(httpTransports).Error; err != nil {
		return errors.WithStack(err)
	}
	return nil
}
