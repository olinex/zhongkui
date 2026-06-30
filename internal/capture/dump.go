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
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

type notice struct {
	ts        time.Time
	secretKey string
}

func (c *capturer) dumpOne(ctx context.Context, ws wire.Wire, filePath string, secretKey string) (err error) {
	ctx, span := totel.StartDumpOneSpan(ctx, filePath)
	var packetCount int
	var byteCount int
	defer func() {
		if err != nil {
			totel.RecordDumpError(ctx)
		} else {
			totel.RecordSegmentDumped(ctx, packetCount, byteCount)
		}
		totel.EndSpan(span, &err)
	}()

	var file *os.File
	file, err = os.Create(filePath)
	if err != nil {
		return errors.WithStack(err)
	}
	defer file.Close()
	// 带缓冲区的写入器，实现批量写入文件
	buffer := bufio.NewWriterSize(file, int(c.uploadCfg.PacketBufferSize))
	defer buffer.Flush()
	// 流式压缩加密层，实现分块压缩后加密
	compressor, err := crypto.NewZstdCompressorWithLevel(c.uploadCfg.EncoderLevel)
	if err != nil {
		return errors.WithStack(err)
	}
	encryptor, err := crypto.NewAESGCMEncryptor([]byte(secretKey))
	if err != nil {
		return errors.WithStack(err)
	}
	pipeline, err := crypto.NewCompressEncryptPipeline(compressor, encryptor)
	if err != nil {
		return errors.WithStack(err)
	}
	wc, err := pipeline.CompressEncrypt(buffer)
	if err != nil {
		return errors.WithStack(err)
	}
	defer wc.Close()

	// 压缩加密后的写入器，实现批量写入文件
	writer := pcapgo.NewWriter(wc)
	if err := writer.WriteFileHeader(c.captureCfg.SnapLen, c.linkType); err != nil {
		return errors.WithStack(err)
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
		packetCount += 1
		byteCount += len(packet.Data)
		if uint32(byteCount) > uint32(c.uploadCfg.PacketBufferSize) || uint(packetCount) > c.uploadCfg.PacketCount {
			break
		}
	}
	return nil
}

func (c *capturer) dump(ctx context.Context, ws wire.Wire, notifyChan chan<- notice) (err error) {
	ctx, span := totel.StartDumpSpan(ctx)
	defer totel.EndSpan(span, &err)
	for {
		ts := time.Now()
		filePath := c.filePath(ts)
		secretKey, e := crypto.GenerateRandomBase64String(32)
		if e != nil {
			err = e
			return errors.WithStack(e)
		}
		n := notice{
			ts:        ts,
			secretKey: secretKey,
		}
		select {
		case <-ctx.Done():
			return nil
		default:
			err = c.dumpOne(ctx, ws, filePath, secretKey)
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
