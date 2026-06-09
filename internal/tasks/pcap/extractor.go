package pcap

import (
	"context"
	"encoding/json"
	"io"
	"os"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/pcap"
	"github.com/klauspost/compress/zstd"
	"github.com/minio/minio-go/v7"
	"github.com/reactivex/rxgo/v2"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/pipeline"
	reporter_v1 "zhongkui.com/zhongkui/internal/pkg/protobuf/reporter/v1"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/tasks/workers"
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
	in := workers.GetFxIn(ctx)
	objectKey := obj.GetObjectKey()
	rc, err := in.CaptureS3.GetObject(ctx, objectKey, minio.GetObjectOptions{Checksum: true})
	if err != nil {
		return errors.WithStack(err)
	}
	defer rc.Close()
	decoder, err := zstd.NewReader(rc)
	if err != nil {
		return errors.WithStack(err)
	}
	defer decoder.Close()
	pipeOut, pipeIn, err := os.Pipe()
	if err != nil {
		return errors.WithStack(err)
	}
	defer pipeOut.Close()
	innerCtx, innerCause := context.WithCancelCause(ctx)
	go func() {
		defer pipeIn.Close()
		if _, err := io.Copy(pipeIn, decoder); err != nil {
			innerCause(errors.WithStack(err))
			return
		}
	}()
	handle, err := pcap.OpenOfflineFile(pipeOut)
	if err != nil {
		return errors.WithStack(err)
	}
	defer handle.Close()
	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
	for packet := range packetSource.Packets() {
		select {
		case <-innerCtx.Done():
			return context.Cause(innerCtx)
		default:
		}
		sessionID, err := serializer.HexUUIDv4FromString(obj.SessionID)
		if err != nil {
			return errors.WithStack(err)
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
}
