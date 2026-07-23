package pcap

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/pcap"
	"github.com/minio/minio-go/v7"
	"github.com/reactivex/rxgo/v2"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/crypto/combine"
	"zhongkui.com/zhongkui/internal/pkg/errgroupx"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/pipeline"
	reporter_v1 "zhongkui.com/zhongkui/internal/pkg/protobuf/reporter/v1"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/provider"
	"zhongkui.com/zhongkui/internal/tasks/totel"
)

// PcapItem 表示从 PCAP 文件中提取的数据包项。
// 包含会话 ID、数据源信息和 gopacket 包。
type PcapItem struct {
	SessionID serializer.HexUUID      // 会话唯一标识符
	Source    reporter_v1.ResourceRaw // 数据源信息
	Packet    gopacket.Packet         // gopacket 数据包对象
}

// ExtractPacketFromPcap 从 PCAP 文件中提取数据包的提取函数。
// 从 S3 获取 PCAP 文件，解压后解析数据包并通过 channel 输出。
func ExtractPacketFromPcap(ctx context.Context, task *pipeline.Task, channel chan<- rxgo.Item) error {
	var obj PcapS3Object
	if err := json.Unmarshal(task.Payload(), &obj); err != nil {
		return errors.WithStack(err)
	}
	taskType := task.Type()
	ctx, span := totel.StartExtractSpan(ctx, taskType)
	var packetCount int64
	var err error
	defer func() {
		totel.RecordPacketsExtracted(ctx, taskType, int(packetCount))
		totel.EndSpan(span, &err)
		if err != nil {
			totel.RecordTaskError(ctx, taskType)
		}
	}()

	scope := pipeline.GetScope(ctx)
	s3, e := provider.InvokeS3(scope)
	if e != nil {
		err = e
		return errors.WithStack(e)
	}
	objectKey := obj.GetObjectKey()

	// 下载 S3 对象的原始加密数据流
	rc, e := s3.GetObject(ctx, objectKey, minio.GetObjectOptions{})
	if e != nil {
		err = e
		return errors.WithStack(e)
	}
	defer rc.Close()

	// 包装下载流为 ETag 校验 Reader，边读取边计算 MD5，读取完毕后自动与服务端 ETag 比对
	verifiedReader, e := s3.VerifyDownloadIntegrity(ctx, objectKey, rc)
	if e != nil {
		err = e
		return errors.WithStack(e)
	}

	compressor, e := combine.NewZstdCompressor()
	if e != nil {
		err = e
		return errors.WithStack(e)
	}
	encryptor, e := combine.NewAESGCMCryptor([]byte(obj.SecretKey))
	if e != nil {
		err = e
		return errors.WithStack(e)
	}
	cryptoPipeline, e := combine.NewCompressEncryptPipeline(compressor, encryptor)
	if e != nil {
		err = e
		return errors.WithStack(e)
	}
	drc, e := cryptoPipeline.DecryptDecompress(verifiedReader)
	if e != nil {
		err = e
		return errors.WithStack(e)
	}
	defer drc.Close()
	// 解压后的数据通过管道传递给 PCAP 解析器
	pipeOut, pipeIn, e := os.Pipe()
	if e != nil {
		err = e
		return errors.WithStack(e)
	}
	defer pipeOut.Close()
	wg, ctx := errgroupx.WithContext(ctx)
	wg.GoNamed("pipe-in", func() error {
		defer pipeIn.Close()
		var src io.Reader = drc
		if base.Env.Debug {
			debugDir := filepath.Join("tmp")
			if e := os.MkdirAll(debugDir, 0755); e != nil {
				return errors.WithStack(e)
			}
			debugFile, e := os.Create(filepath.Join(debugDir, fmt.Sprintf("%s_%s.pcap", obj.SessionID, obj.Timestamp.Format("20060102_150405"))))
			if e != nil {
				return errors.WithStack(e)
			}
			defer debugFile.Close()
			src = io.TeeReader(drc, debugFile)
		}
		if _, e := io.Copy(pipeIn, src); e != nil {
			return errors.WithStack(e)
		}
		return nil
	})
	wg.GoNamed("pipe-out", func() error {
		defer pipeOut.Close()
		handle, e := pcap.OpenOfflineFile(pipeOut)
		if e != nil {
			return errors.WithStack(e)
		}
		defer handle.Close()
		packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
		for packet := range packetSource.Packets() {
			if packet == nil {
				continue
			}
			// 过滤空数据包：解压管道末尾可能产生尾部填充字节，
			// gopacket pcap reader 会将其解析为 data 为空的伪数据包
			if len(packet.Data()) == 0 {
				continue
			}
			select {
			case <-ctx.Done():
				return nil
			default:
			}
			sessionID, e := serializer.HexUUIDv4FromString(obj.SessionID)
			if e != nil {
				return errors.WithStack(e)
			}
			atomic.AddInt64(&packetCount, 1)
			if stats := GetPcapStats(ctx); stats != nil {
				stats.RecordPacket(len(packet.Data()))
			}
			channel <- rxgo.Of(&PcapItem{
				SessionID: sessionID,
				Source: reporter_v1.ResourceRaw{
					Namespace: obj.Namespace,
					Name:      obj.Name,
					Version:   obj.Version,
				},
				Packet: packet,
			})
		}
		return nil
	})
	err = wg.Wait()
	return err
}
