package flow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/network/tcp"
	"zhongkui.com/zhongkui/internal/pkg/crypto"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/pipeline"
	"zhongkui.com/zhongkui/internal/pkg/s3"
	"zhongkui.com/zhongkui/internal/provider"
	"zhongkui.com/zhongkui/internal/tasks/workers"
)

func objectKey(name, version string) string {
	return fmt.Sprintf(`temp/%s/%s.%s`, name, version, crypto.FILE_EXT)
}

type IPv4TCPRouter = tcp.Router[*network.PacketIPv4TCP]

type IPv6TCPRouter = tcp.Router[*network.PacketIPv6TCP]

type IPv4FlowVersionTaskWorker = workers.VersionTaskWorker[IPv4TCPRouter]

type IPv6FlowVersionTaskWorker = workers.VersionTaskWorker[IPv6TCPRouter]

type FlowVersionTaskWrapper[T *tcp.Router[P], P tcp.TCPPacket] struct {
	worker pipeline.TaskWorker
}

func (f *FlowVersionTaskWrapper[T, P]) Next(ctx context.Context, name, lastVersion string) (nextVersion string, err error) {
	param, err := pipeline.GetTaskParam[Param](ctx)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return time.Now().Truncate(param.WindowRange).Format("20060102150405"), nil
}

func (f *FlowVersionTaskWrapper[T, P]) Load(ctx context.Context, name, lastVersion string) (T, error) {
	scope := pipeline.GetScope(ctx)
	param, err := pipeline.GetTaskParam[Param](ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	client, err := provider.InvokeS3(scope)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	temp := s3.NewZkcn(
		client,
		objectKey(name, lastVersion),
		"",
		0,
	)
	rc, err := temp.Load(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if rc == nil {
		router := tcp.NewRouter[P](
			param.MaxReassemblerSize,
			param.MaxOutOrderPacketSize,
			param.NoticeTimeout,
		)
		return &router, nil
	} else {
		defer rc.Close()
		data, err := io.ReadAll(rc)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		router := new(tcp.Router[P]) // T = *tcp.Router[P]，需先创建实例再反序列化
		err = json.Unmarshal(data, router)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return router, nil
	}
}

func (f *FlowVersionTaskWrapper[T, P]) Store(ctx context.Context, name, nextVersion string, data T) error {
	scope := pipeline.GetScope(ctx)
	client, err := provider.InvokeS3(scope)
	if err != nil {
		return errors.WithStack(err)
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return errors.WithStack(err)
	}
	temp := s3.NewZkcn(
		client,
		objectKey(name, nextVersion),
		"",
		0,
	)
	_, err = temp.Store(ctx, bytes.NewReader(raw))
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (f *FlowVersionTaskWrapper[T, P]) Clear(ctx context.Context, name, lastVersion string) error {
	scope := pipeline.GetScope(ctx)
	client, err := provider.InvokeS3(scope)
	if err != nil {
		return errors.WithStack(err)
	}
	temp := s3.NewZkcn(
		client,
		objectKey(name, lastVersion),
		"",
		0,
	)
	err = temp.Clear(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (f *FlowVersionTaskWrapper[T, P]) Do(ctx context.Context, task *pipeline.Task) error {
	return f.worker.Do(ctx, task)
}

func newIPv4FlowTaskWorker() pipeline.TaskWorker {
	return workers.NewBatchTaskWorker(
		10000,
		time.Minute,
		ExtractIPv4HttpTransportFromTable,
		[]pipeline.TransformFunc{
			TransAppendTags,
		},
		[]pipeline.LoadFunc{
			LoadHttpTransportToTable,
		},
		nil,
		nil,
	)
}

func newIPv6FlowTaskWorker() pipeline.TaskWorker {
	return workers.NewBatchTaskWorker(
		10000,
		time.Minute,
		ExtractIPv6HttpTransportFromTable,
		[]pipeline.TransformFunc{
			TransAppendTags,
		},
		[]pipeline.LoadFunc{
			LoadHttpTransportToTable,
		},
		nil,
		nil,
	)
}

func NewIPv4FlowVersionTaskWorker() *IPv4FlowVersionTaskWorker {
	worker := newIPv4FlowTaskWorker()
	wrapper := &FlowVersionTaskWrapper[*IPv4TCPRouter, *network.PacketIPv4TCP]{worker: worker}
	return workers.NewVersionTaskWorker(
		wrapper,
	)
}

func NewIPv6FlowVersionTaskWorker() *IPv6FlowVersionTaskWorker {
	worker := newIPv6FlowTaskWorker()
	wrapper := &FlowVersionTaskWrapper[*IPv6TCPRouter, *network.PacketIPv6TCP]{worker: worker}
	return workers.NewVersionTaskWorker(
		wrapper,
	)
}
