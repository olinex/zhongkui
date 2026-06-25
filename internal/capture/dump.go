package capture

import (
	"bufio"
	"context"
	"os"
	"time"

	"github.com/gopacket/gopacket/pcapgo"
	"zhongkui.com/zhongkui/internal/capture/wire"
	"zhongkui.com/zhongkui/internal/pkg/crypto"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

type notice struct {
	ts        time.Time
	secretKey string
}

func (c *capturer) dumpOne(ctx context.Context, ws wire.Wire, filePath string, secretKey string) error {
	var file *os.File
	file, err := os.Create(filePath)
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
	c.logger.Infof(
		"header {snap length: %d, link type: %s} is write to pcap file %s",
		c.captureCfg.SnapLen,
		c.linkType,
		filePath,
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

func (c *capturer) dump(ctx context.Context, ws wire.Wire, notifyChan chan<- notice) error {
	for {
		ts := time.Now()
		filePath := c.filePath(ts)
		secretKey, err := crypto.GenerateRandomBase64String(32)
		if err != nil {
			return errors.WithStack(err)
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
			c.logger.Infof("dump pcap file %s success", filePath)
		}
	}
}
