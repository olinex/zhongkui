package capture

import (
	"bufio"
	"context"
	"os"
	"time"

	"github.com/gopacket/gopacket/pcapgo"
	"zhongkui.com/zhongkui/internal/capture/totel"
	"zhongkui.com/zhongkui/internal/capture/wire"
	"zhongkui.com/zhongkui/internal/pkg/crypto"
	"zhongkui.com/zhongkui/internal/pkg/crypto/combine"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

type notice struct {
	ts          time.Time
	cryptoKey   string
	packetCount int
	byteCount   int
}

func (c *capturer) dumpOne(ctx context.Context, ws wire.Wire, filePath string, cryptoKey []byte) (packetCount int, byteCount int, err error) {
	ctx, span := totel.StartDumpOneSpan(ctx, filePath)
	packetCount = 0
	byteCount = 0
	defer func() {
		if err != nil {
			totel.RecordDumpError(ctx)
		} else {
			totel.RecordSegmentDumped(ctx, packetCount, byteCount)
			if c.stats != nil {
				// 数据包数和字节数已在循环中通过 RecordPacket 实时统计，此处仅增加 segment 计数
				c.stats.RecordSegment()
			}
		}
		totel.EndSpan(span, &err)
	}()

	var file *os.File
	file, err = os.Create(filePath)
	if err != nil {
		return 0, 0, errors.WithStack(err)
	}
	defer file.Close()
	if c.trackFile != nil {
		c.trackFile(filePath)
	}
	// 带缓冲区的写入器，实现批量写入文件
	buffer := bufio.NewWriterSize(file, int(c.uploadCfg.PacketBufferSize))
	defer buffer.Flush()
	// 流式压缩加密层，实现分块压缩后加密
	pipeline, err := combine.NewZstdAESGCMPipelineWithLevel(cryptoKey, c.uploadCfg.EncoderLevel)
	if err != nil {
		return 0, 0, errors.WithStack(err)
	}
	wc, err := pipeline.CompressEncrypt(buffer)
	if err != nil {
		return 0, 0, errors.WithStack(err)
	}
	defer wc.Close()

	// 压缩加密后的写入器，实现批量写入文件
	writer := pcapgo.NewWriter(wc)
	if err := writer.WriteFileHeader(c.captureCfg.SnapLen, c.linkType); err != nil {
		return 0, 0, errors.WithStack(err)
	}
	c.logger.Ctx(ctx).Infof(
		"header {snap length: %d, link type: %s} is write to pcap file %s",
		c.captureCfg.SnapLen,
		c.linkType,
		filePath,
	)
	timer := time.NewTimer(c.uploadCfg.IntervalTimeout)

	packetCount = 0
	byteCount = 0
	for {
		select {
		case <-ctx.Done():
			return packetCount, byteCount, nil
		case <-timer.C:
			return packetCount, byteCount, nil
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
			return 0, 0, errors.WithStack(packet.Err)
		}
		if err = writer.WritePacket(packet.CaptureInfo, packet.Data); err != nil {
			return 0, 0, errors.WithStack(err)
		}
		packetCount += 1
		byteCount += len(packet.Data)
		if c.stats != nil {
			c.stats.RecordPacket(len(packet.Data))
		}
		if uint32(byteCount) > uint32(c.uploadCfg.PacketBufferSize) || uint(packetCount) > c.uploadCfg.PacketCount {
			break
		}
	}
	return packetCount, byteCount, nil
}

func (c *capturer) dump(ctx context.Context, ws wire.Wire, notifyChan chan<- notice) (err error) {
	ctx, span := totel.StartDumpSpan(ctx)
	defer totel.EndSpan(span, &err)
	for {
		ts := time.Now()
		filePath := c.filePath(ts)
		cryptoKey, e := crypto.GenerateRandomBase64String(32)
		if e != nil {
			err = e
			return errors.WithStack(e)
		}
		n := notice{
			ts:        ts,
			cryptoKey: cryptoKey,
		}
		select {
		case <-ctx.Done():
			return nil
		default:
			var packetCount, byteCount int
			packetCount, byteCount, err = c.dumpOne(ctx, ws, filePath, []byte(cryptoKey))
			n.packetCount = packetCount
			n.byteCount = byteCount
			if err != nil {
				return errors.WithStack(err)
			}
			select {
			case <-ctx.Done():
				return nil
			case notifyChan <- n:
			}
			c.logger.Ctx(ctx).Infof("dump pcap file %s success", filePath)
		}
	}
}
