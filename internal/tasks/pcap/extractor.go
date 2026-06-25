package pcap

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/pcap"
	"github.com/minio/minio-go/v7"
	"github.com/reactivex/rxgo/v2"
	"golang.org/x/sync/errgroup"
	_ "zhongkui.com/zhongkui/internal/network/detectors"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/crypto"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/pipeline"
	reporter_v1 "zhongkui.com/zhongkui/internal/pkg/protobuf/reporter/v1"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/provider"
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
	scope := pipeline.GetScope(ctx)
	s3, err := provider.InvokeS3(scope)
	if err != nil {
		return errors.WithStack(err)
	}
	objectKey := obj.GetObjectKey()
	rc, err := s3.GetObject(ctx, objectKey, minio.GetObjectOptions{Checksum: true})
	if err != nil {
		return errors.WithStack(err)
	}
	defer rc.Close()
	compressor, err := crypto.NewZstdCompressor()
	if err != nil {
		return errors.WithStack(err)
	}
	encryptor, err := crypto.NewAESGCMEncryptor([]byte(obj.SecretKey))
	if err != nil {
		return errors.WithStack(err)
	}
	pipeline, err := crypto.NewCompressEncryptPipeline(compressor, encryptor)
	if err != nil {
		return errors.WithStack(err)
	}
	drc, err := pipeline.DecryptDecompress(rc)
	if err != nil {
		return errors.WithStack(err)
	}
	defer drc.Close()
	// 解压后的数据通过管道传递给 PCAP 解析器
	pipeOut, pipeIn, err := os.Pipe()
	if err != nil {
		return errors.WithStack(err)
	}
	defer pipeOut.Close()
	wg, ctx := errgroup.WithContext(ctx)
	wg.Go(func() error {
		defer pipeIn.Close()
		var src io.Reader = drc
		if base.Env.Debug {
			debugDir := filepath.Join("tmp")
			if err = os.MkdirAll(debugDir, 0755); err != nil {
				return errors.WithStack(err)
			}
			debugFile, err := os.Create(filepath.Join(debugDir, fmt.Sprintf("%s_%s.pcap", obj.SessionID, obj.Timestamp.Format("20060102_150405"))))
			if err != nil {
				return errors.WithStack(err)
			}
			defer debugFile.Close()
			src = io.TeeReader(drc, debugFile)
		}
		if _, err = io.Copy(pipeIn, src); err != nil {
			return errors.WithStack(err)
		}
		return nil
	})
	wg.Go(func() error {
		defer pipeOut.Close()
		handle, err := pcap.OpenOfflineFile(pipeOut)
		if err != nil {
			return errors.WithStack(err)
		}
		defer handle.Close()
		packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
		for packet := range packetSource.Packets() {
			if packet == nil {
				continue
			}
			select {
			case <-ctx.Done():
				return nil
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
	})
	return wg.Wait()
}
