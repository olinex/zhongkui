package pcap

import (
	"context"

	"github.com/pterm/pterm"
	n "zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// TransFromPacketToModel 将数据包项转换为网络模型的数据转换函数。
// 从 PcapItem 中提取信息，使用 PacketModelBuilder 构建网络数据包模型。
func TransFromPacketToModel(ctx context.Context, i any) (any, error) {
	item, ok := i.(*PcapItem)
	if !ok {
		return nil, errors.Newf("Expect pcap.PcapItem, got %T", i)
	}
	builder := n.NewPacketModelBuilder(item.Packet, item.SessionID, item.Source)
	model := builder.BuildAsPacketModel()
	if model == nil {
		// 调试：当模型构建失败时，打印层堆栈以定位原因
		// builder.Stack 和 builder.Level 由 ExtractLayerInfos 填充
		pterm.Debug.Printfln(
			"[nil model] level=%d stack_len=%d",
			builder.Level, len(builder.Stack),
		)
		for idx, info := range builder.Stack {
			pterm.Debug.Printfln(
				"[nil model]   stack[%d] level=%d proto=%s headers=%v",
				idx, info.Level, info.Proto, info.Headers != nil,
			)
		}
	}
	return model, nil
}
