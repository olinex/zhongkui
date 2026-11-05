package pcap

import (
	"context"

	"zhongkuisec.com/zhongkui/internal/models/network"
	"zhongkuisec.com/zhongkui/internal/pkg/errgroupx"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/provider"
)

// LoadPacketModelToTable 将网络数据包模型加载到数据库表中。
// 根据不同的数据包类型（Ethernet、IPv4、IPv6、TCP、UDP），分别批量插入对应的数据表。
// 使用并发方式同时写入多种类型的包数据。
func LoadPacketModelToTable(ctx context.Context, items []any) error {
	scope := provider.GetScope(ctx)
	rawDB, err := provider.InvokeRawDatabase(scope)
	if err != nil {
		return errors.WithStack(err)
	}
	ethModels := make([]*network.PacketEthernet, 0)
	ipv4Models := make([]*network.PacketIPv4, 0)
	ipv6Models := make([]*network.PacketIPv6, 0)
	ipv4TCPModels := make([]*network.PacketIPv4TCP, 0)
	ipv6TCPModels := make([]*network.PacketIPv6TCP, 0)
	ipv4UDPModels := make([]*network.PacketIPv4UDP, 0)
	ipv6UDPModels := make([]*network.PacketIPv6UDP, 0)
	stats := GetPcapStats(ctx)
	for _, item := range items {
		if item == nil {
			continue
		}
		switch model := item.(type) {
		case *network.PacketEthernet:
			ethModels = append(ethModels, model)
			if stats != nil {
				stats.RecordEth()
			}
		case *network.PacketIPv4:
			ipv4Models = append(ipv4Models, model)
			if stats != nil {
				stats.RecordIPv4()
			}
		case *network.PacketIPv6:
			ipv6Models = append(ipv6Models, model)
			if stats != nil {
				stats.RecordIPv6()
			}
		case *network.PacketIPv4TCP:
			ipv4TCPModels = append(ipv4TCPModels, model)
			if stats != nil {
				stats.RecordIPv4TCP()
			}
		case *network.PacketIPv6TCP:
			ipv6TCPModels = append(ipv6TCPModels, model)
			if stats != nil {
				stats.RecordIPv6TCP()
			}
		case *network.PacketIPv4UDP:
			ipv4UDPModels = append(ipv4UDPModels, model)
			if stats != nil {
				stats.RecordIPv4UDP()
			}
		case *network.PacketIPv6UDP:
			ipv6UDPModels = append(ipv6UDPModels, model)
			if stats != nil {
				stats.RecordIPv6UDP()
			}
		default:
			return errors.Newf("Packet model type error, expect network packet model, got %T", item)
		}
	}
	wg, innerCtx := errgroupx.WithContext(ctx)
	if len(ethModels) > 0 {
		wg.GoNamed("eth", func() error {
			if err := rawDB.WithContext(innerCtx).Create(ethModels).Error; err != nil {
				return errors.WithStack(err)
			}
			return nil
		})
	}
	if len(ipv4Models) > 0 {
		wg.GoNamed("ipv4", func() error {
			if err := rawDB.WithContext(innerCtx).Create(ipv4Models).Error; err != nil {
				return errors.WithStack(err)
			}
			return nil
		})
	}
	if len(ipv6Models) > 0 {
		wg.GoNamed("ipv6", func() error {
			if err := rawDB.WithContext(innerCtx).Create(ipv6Models).Error; err != nil {
				return errors.WithStack(err)
			}
			return nil
		})
	}
	if len(ipv4TCPModels) > 0 {
		wg.GoNamed("ipv4-tcp", func() error {
			if err := rawDB.WithContext(innerCtx).Create(ipv4TCPModels).Error; err != nil {
				return errors.WithStack(err)
			}
			return nil
		})
	}
	if len(ipv6TCPModels) > 0 {
		wg.GoNamed("ipv6-tcp", func() error {
			if err := rawDB.WithContext(innerCtx).Create(ipv6TCPModels).Error; err != nil {
				return errors.WithStack(err)
			}
			return nil
		})
	}
	if len(ipv4UDPModels) > 0 {
		wg.GoNamed("ipv4-udp", func() error {
			if err := rawDB.WithContext(innerCtx).Create(ipv4UDPModels).Error; err != nil {
				return errors.WithStack(err)
			}
			return nil
		})
	}
	if len(ipv6UDPModels) > 0 {
		wg.GoNamed("ipv6-udp", func() error {
			if err := rawDB.WithContext(innerCtx).Create(ipv6UDPModels).Error; err != nil {
				return errors.WithStack(err)
			}
			return nil
		})
	}
	return wg.Wait()
}
