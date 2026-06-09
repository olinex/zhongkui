package pcap

import (
	"context"

	n "zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// TransFromPacketToModel 将数据包项转换为网络模型的数据转换函数。
// 从 PcapItem 中提取信息，使用 PacketModelBuilder 构建网络数据包模型。
func TransFromPacketToModel(ctx context.Context, i any) (any, error) {
	item, ok := i.(*PcapItem)
	if !ok {
		return nil, errors.Newf("Expect gopacket.Packet, got %T", i)
	}
	builder := n.NewPacketModelBuilder(item.Packet, item.SessionID, item.Source)
	return builder.BuildAsPacketModel(), nil
}
