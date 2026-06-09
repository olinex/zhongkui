package capture

import (
	"bufio"
	"context"
	"os"
	"time"

	"github.com/gopacket/gopacket/pcapgo"
	"zhongkui.com/zhongkui/internal/capture/wire"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

func (c *capturer) dumpOne(ctx context.Context, ws wire.Wire, pcapPath string) error {
	var file *os.File
	file, err := os.Create(pcapPath)
	if err != nil {
		return errors.WithStack(err)
	}
	defer file.Close()
	// 带缓冲区的写入器，实现批量写入文件
	buffer := bufio.NewWriterSize(file, int(c.uploadCfg.PacketBufferSize))
	writer := pcapgo.NewWriter(buffer)
	if err := writer.WriteFileHeader(c.captureCfg.SnapLen, c.linkType); err != nil {
		return errors.WithStack(err)
	}
	defer buffer.Flush()
	c.logger.Infof(
		"header {snap length: %d, link type: %s} is write to pcap file %s",
		c.captureCfg.SnapLen,
		c.linkType,
		pcapPath,
	)
	timer := time.NewTimer(c.uploadCfg.IntervalTimeout)

	count := 0
	size := 0
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			return nil
		default:
		}
		packet := ws.ZeroCopyReadPacketData()
		if packet == nil {
			break
		}
		if packet.Err != nil {
			if packet.HasNext() {
				continue
			}
			return errors.WithStack(packet.Err)
		}
		if err := writer.WritePacket(packet.CaptureInfo, packet.Data); err != nil {
			return errors.WithStack(err)
		}
		count += 1
		size += len(packet.Data)
		if uint32(size) > uint32(c.uploadCfg.PacketBufferSize) || uint(count) > c.uploadCfg.PacketCount {
			break
		}
	}
	return nil
}

func (c *capturer) dump(ctx context.Context, ws wire.Wire, notifyChan chan<- time.Time) error {
	for {
		ts := time.Now()
		select {
		case <-ctx.Done():
			return nil
		default:
			filePath := c.pcapPath(ts)
			err := c.dumpOne(ctx, ws, filePath)
			if err != nil {
				return errors.WithStack(err)
			}
			select {
			case <-ctx.Done():
				return nil
			case notifyChan <- ts:
			}
			c.logger.Infof("dump pcap file %s success", filePath)
		}
	}
}
