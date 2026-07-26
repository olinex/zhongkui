package reporter

import (
	"context"
	"io"
	"time"

	"github.com/hibiken/asynq"
	"github.com/samber/do/v2"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
	g "google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/etcd"
	"zhongkui.com/zhongkui/internal/pkg/grpc"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/pipeline"
	reporter_v1 "zhongkui.com/zhongkui/internal/pkg/protobuf/reporter/v1"
	"zhongkui.com/zhongkui/internal/provider"
	"zhongkui.com/zhongkui/internal/reporter/totel"
	"zhongkui.com/zhongkui/internal/worker/handlers/pcap"
)

// ReporterService 实现了 reporter_v1.ReporterServiceServer 接口
// 该服务提供两个主要功能：
//  1. Watch - 流式监听资源（如抓包器配置）的变化
//  2. Feedback - 接收客户端的反馈信息，并触发后续处理流程
//
// 结构体字段：
//   - UnimplementedReporterServiceServer: 嵌入未实现的服务基类，确保向前兼容
//   - client: etcd 客户端，用于存储和监听配置信息
//   - pClient: 管道客户端，用于提交异步处理任务
//   - logger: 结构化的日志记录器
type ReporterService struct {
	reporter_v1.UnimplementedReporterServiceServer
	grpc    *grpc.Server
	client  *etcd.EtcdClient
	pClient *pipeline.Client
	logger  *logging.Logger
}

func NewReportService(i do.Injector) *ReporterService {
	totel.InitMetrics()
	etcd := provider.MustInvokeEtcdClient(i)
	pClient := provider.MustInvokePipelineClient(i)
	logger := provider.MustInvokeGrpcLogger(i)
	grpc := provider.MustInvokeReporterGrpcServer(i)
	srv := &ReporterService{
		grpc:    grpc,
		client:  etcd,
		pClient: pClient,
		logger:  logger,
	}
	reporter_v1.RegisterReporterServiceServer(grpc, srv)
	return srv
}

// newWatchResponseError 创建一个表示错误的 WatchResponse
// 该函数用于将错误信息封装为 gRPC 流式响应，发送给客户端
//
// 参数：
//   - err: 原始错误对象，包含具体的错误信息
//   - errorType: 错误类型枚举，标识错误的分类
//   - globalVersion: 全局版本号，表示 etcd 集群的当前版本
//
// 返回值：
//   - *reporter_v1.WatchResponse: 包含错误信息的响应对象
//
// 响应结构：
//   - Timestamp: 当前时间戳（毫秒），标识错误发生的时间
//   - Content: 响应内容，此处为 Error 类型
//   - Error.Type: 错误类型，帮助客户端识别错误类别
//   - Error.Detail: 错误详情，包含具体的错误信息
//   - GlobalVersion: 全局版本号，用于客户端同步状态
func newWatchResponseError(err error, errorType reporter_v1.ErrorType, globalVersion int64) *reporter_v1.WatchResponse {
	return &reporter_v1.WatchResponse{
		Timestamp: time.Now().UnixMilli(),
		Content: &reporter_v1.WatchResponse_Error{
			Error: &reporter_v1.Error{
				Type:   errorType,
				Detail: err.Error(),
			},
		},
		GlobalVersion: globalVersion,
	}
}

// Watch 实现 gRPC 流式监听接口，用于监听资源（如抓包器配置）的变化
// 该方法提供了完整的资源状态监听功能，包括：
//  1. 初始状态获取：客户端连接时立即返回当前资源状态
//  2. 实时变更监听：持续监听资源的创建、修改和删除事件
//  3. 心跳机制：定期发送心跳包，保持连接活跃并检测连接状态
//  4. 错误处理：将各种错误封装为统一的错误响应格式
//
// 参数：
//   - req: WatchRequest - 客户端发起的监听请求
//   - GlobalVersion: 客户端已知的全局版本号，用于增量同步
//   - Resource: 要监听的资源标识
//   - HeartbeatIntervalSeconds: 心跳间隔秒数
//   - stream: g.ServerStreamingServer[WatchResponse] - 服务器端流式响应接口
//
// 返回值：
//   - error: 监听过程中发生的错误，nil 表示正常结束
//
// 工作流程：
//  1. 从请求中提取资源键和全局版本号
//  2. 获取资源的当前状态作为初始响应
//  3. 启动 etcd Watch 监听资源变更
//  4. 启动定时器发送心跳包
//  5. 循环处理：上下文取消、资源变更事件、心跳事件
//  6. 将事件转换为统一的响应格式发送给客户端
func (srv *ReporterService) Watch(req *reporter_v1.WatchRequest, stream g.ServerStreamingServer[reporter_v1.WatchResponse]) (err error) {
	// 获取请求上下文，用于控制整个监听过程的生命周期
	ctx := stream.Context()
	key := core.ReporterConfigKey(req.Resource)
	ctx, span := totel.StartWatchSpan(ctx, key)
	defer totel.EndSpan(span, &err)
	totel.RecordWatchConnection(ctx)

	// 提取客户端已知的全局版本号，用于增量同步
	globalVersion := req.GlobalVersion

	// 首次获取初始值 - 客户端连接时立即获取当前资源状态
	rsp, err := srv.client.Get(
		ctx,
		key,
		clientv3.WithRev(globalVersion), // 使用指定的版本号，支持从特定版本开始同步
	)
	if err != nil {
		// 获取初始状态失败，返回错误响应
		return stream.Send(newWatchResponseError(errors.WithStack(err), reporter_v1.ErrorType_UNKNOWN, globalVersion))
	}

	// 检查资源是否存在
	if len(rsp.Kvs) == 0 {
		// 资源不存在，返回资源未找到错误
		return stream.Send(newWatchResponseError(errors.New("resource not found"), reporter_v1.ErrorType_UNKNOWN, globalVersion))
	}

	// 反序列化配置数据
	config, err := core.UnmarshalAsReporterConfig(rsp.Kvs[0].Value)
	if err != nil {
		// 反序列化失败，返回错误响应
		return stream.Send(newWatchResponseError(errors.WithStack(err), reporter_v1.ErrorType_UNKNOWN, globalVersion))
	}

	// 更新全局版本号为 etcd 响应的最新版本
	globalVersion = rsp.Header.Revision

	// 发送初始状态响应，表示资源已存在（CREATE 事件）
	err = stream.Send(&reporter_v1.WatchResponse{
		Timestamp: time.Now().UnixMilli(),
		Content: &reporter_v1.WatchResponse_Event{
			Event: &reporter_v1.Event{
				Type:        reporter_v1.EventType_CREATED,
				PrvSnapshot: nil, // 初始状态没有前一个快照
				CurSnapshot: &reporter_v1.Snapshot{
					Config:        config,
					ConfigVersion: rsp.Kvs[0].ModRevision,
				},
			},
		},
		GlobalVersion: globalVersion,
	})
	if err != nil {
		// 发送初始响应失败
		return errors.WithStack(err)
	}

	// 创建心跳定时器，定期发送心跳包保持连接活跃
	heartbeater := time.NewTicker(time.Second * time.Duration(req.HeartbeatIntervalSeconds))
	defer heartbeater.Stop() // 确保函数退出时停止定时器

	// 启动 etcd Watch 监听，从当前版本+1开始监听变更
	watchChan := srv.client.Watch(
		ctx,
		key,
		clientv3.WithPrevKV(),             // 包含变更前的键值对，用于 MODIFIED 事件
		clientv3.WithRev(globalVersion+1), // 从下一个版本开始监听，避免重复接收当前版本
	)

	// 主事件循环，处理三种类型的事件：上下文取消、资源变更、心跳
	for {
		select {
		case <-ctx.Done():
			// 上下文被取消（如客户端断开连接、超时等），正常结束监听
			return nil

		case rsp, open := <-watchChan:
			// 接收到资源变更事件
			if !open {
				// Watch 通道意外关闭，返回错误
				return errors.New("watch channel was closed before context done")
			}

			// 更新全局版本号为响应的最新版本
			globalVersion := rsp.Header.Revision

			// 处理所有变更事件（一次响应可能包含多个事件）
			for _, event := range rsp.Events {
				switch event.Type {
				case clientv3.EventTypeDelete:
					// 资源删除事件
					totel.RecordWatchEvent(ctx, "DELETE")
					config, err := core.UnmarshalAsReporterConfig(event.PrevKv.Value)
					if err != nil {
						// 反序列化删除前的配置失败
						return stream.Send(newWatchResponseError(errors.WithStack(err), reporter_v1.ErrorType_UNKNOWN, globalVersion))
					}

					// 发送 DELETE 事件响应
					err = stream.Send(&reporter_v1.WatchResponse{
						Timestamp: time.Now().UnixMilli(),
						Content: &reporter_v1.WatchResponse_Event{
							Event: &reporter_v1.Event{
								Type: reporter_v1.EventType_DELETED,
								PrvSnapshot: &reporter_v1.Snapshot{
									Config:        config,
									ConfigVersion: event.PrevKv.ModRevision,
								},
								CurSnapshot: nil, // 删除后没有当前快照
							},
						},
						GlobalVersion: globalVersion,
					})
					if err != nil {
						return errors.WithStack(err)
					}

				case clientv3.EventTypePut:
					// 资源创建或修改事件
					config, err := core.UnmarshalAsReporterConfig(event.Kv.Value)
					if err != nil {
						// 反序列化当前配置失败
						return stream.Send(newWatchResponseError(errors.WithStack(err), reporter_v1.ErrorType_UNKNOWN, globalVersion))
					}

					if event.IsCreate() {
						// 资源创建事件
						totel.RecordWatchEvent(ctx, "CREATE")
						stream.Send(&reporter_v1.WatchResponse{
							Timestamp: time.Now().UnixMilli(),
							Content: &reporter_v1.WatchResponse_Event{
								Event: &reporter_v1.Event{
									Type:        reporter_v1.EventType_CREATED,
									PrvSnapshot: nil, // 创建事件没有前一个快照
									CurSnapshot: &reporter_v1.Snapshot{
										Config:        config,
										ConfigVersion: event.Kv.CreateRevision,
									},
								},
							},
							GlobalVersion: globalVersion,
						})
					} else if event.IsModify() {
						// 资源修改事件
						totel.RecordWatchEvent(ctx, "MODIFY")
						prvConfig, err := core.UnmarshalAsReporterConfig(event.PrevKv.Value)
						if err != nil {
							// 反序列化修改前的配置失败
							return stream.Send(newWatchResponseError(errors.WithStack(err), reporter_v1.ErrorType_UNKNOWN, globalVersion))
						}

						curConfig, err := core.UnmarshalAsReporterConfig(event.Kv.Value)
						if err != nil {
							// 反序列化修改后的配置失败
							return stream.Send(newWatchResponseError(errors.WithStack(err), reporter_v1.ErrorType_UNKNOWN, globalVersion))
						}

						// 发送 MODIFIED 事件响应，包含修改前后的快照
						err = stream.Send(&reporter_v1.WatchResponse{
							Timestamp: time.Now().UnixMilli(),
							Content: &reporter_v1.WatchResponse_Event{
								Event: &reporter_v1.Event{
									Type: reporter_v1.EventType_MODIFIED,
									PrvSnapshot: &reporter_v1.Snapshot{
										Config:        prvConfig,
										ConfigVersion: event.PrevKv.ModRevision,
									},
									CurSnapshot: &reporter_v1.Snapshot{
										Config:        curConfig,
										ConfigVersion: event.Kv.ModRevision,
									},
								},
							},
							GlobalVersion: globalVersion,
						})
						if err != nil {
							return errors.WithStack(err)
						}
					}
				}
			}

		case <-heartbeater.C:
			// 心跳定时器触发，发送心跳包
			err := stream.Send(&reporter_v1.WatchResponse{
				Timestamp: time.Now().UnixMilli(),
				Content: &reporter_v1.WatchResponse_Heartbeat{
					Heartbeat: &reporter_v1.Heartbeat{},
				},
				GlobalVersion: globalVersion,
			})
			if err != nil {
				// 发送心跳失败，可能连接已断开
				return errors.WithStack(err)
			}
		}
	}
}

// Feedback 实现 gRPC 客户端流式接口，用于接收客户端的反馈信息
// 该方法处理客户端发送的状态报告，并触发相应的后续处理流程：
//  1. 状态存储：将客户端状态存储到 etcd 中，支持带租约的自动过期
//  2. 租约管理：为客户端状态创建和管理 etcd 租约，确保状态自动清理
//  3. 任务触发：当客户端报告数据上传完成时，触发异步处理任务
//
// 参数：
//   - stream: g.ClientStreamingServer[FeedbackRequest, emptypb.Empty] - 客户端流式请求接口
//
// 返回值：
//   - error: 处理过程中发生的错误，nil 表示正常结束
//
// 工作流程：
//  1. 循环接收客户端发送的 FeedbackRequest
//  2. 为第一个请求创建 etcd 租约，后续请求续租
//  3. 将请求数据序列化并存储到 etcd
//  4. 如果请求标记为已上传，创建异步处理任务
//  5. 客户端发送完成后，返回空的响应
func (srv *ReporterService) Feedback(stream g.ClientStreamingServer[reporter_v1.FeedbackRequest, emptypb.Empty]) (err error) {
	// 确保函数退出时发送并关闭流，返回空的响应
	defer stream.SendAndClose(&emptypb.Empty{})

	// 获取请求上下文
	ctx := stream.Context()
	ctx, span := totel.StartFeedbackSpan(ctx)
	defer totel.EndSpan(span, &err)

	// 租约对象，用于管理状态数据的生命周期
	var lease *clientv3.LeaseGrantResponse

	// 循环接收客户端流式请求
	for {
		// 接收客户端发送的反馈请求
		request, err := stream.Recv()
		if err == io.EOF {
			// 客户端发送完成，正常结束
			return nil
		}
		if err != nil {
			// 接收请求时发生错误
			return errors.WithStack(err)
		}

		// 记录接收到的反馈请求
		totel.RecordFeedbackRequest(ctx)

		// 租约管理：为第一个请求创建租约，后续请求续租
		if lease == nil {
			// 首次请求，创建 etcd 租约
			// 租约过期时间由客户端指定，确保状态数据自动清理
			lease, err = srv.client.Grant(ctx, request.TimeoutSeconds)
			if err != nil {
				return errors.WithStack(err)
			}
		} else {
			// 后续请求，续租以保持状态数据有效
			_, err := srv.client.KeepAliveOnce(ctx, lease.ID)
			if err != nil {
				return errors.WithStack(err)
			}
		}

		// 序列化请求数据为 Protocol Buffers 格式
		value, err := proto.Marshal(request)
		if err != nil {
			return errors.WithStack(err)
		}

		// 将状态数据存储到 etcd，附带租约确保自动过期
		_, err = srv.client.Put(
			ctx,
			core.ReporterStatusDetailKey(request.Resource, request.SessionId),
			string(value),
			clientv3.WithLease(lease.ID), // 附加租约，数据在租约过期后自动删除
		)
		if err != nil {
			return errors.WithStack(err)
		}

		// 检查是否上传完成标记
		if !request.Uploaded {
			// 数据未上传完成，继续接收下一个请求
			continue
		}

		// 数据已上传完成，创建异步处理任务
		task, err := pcap.NewPcapS3ObjectTask(&pcap.PcapS3Object{
			SessionID: request.SessionId,
			Timestamp: time.UnixMilli(request.Timestamp),
			Namespace: request.Resource.Namespace,
			Name:      request.Resource.Name,
			Version:   request.Resource.Version,
			SecretKey: request.SecretKey,
		})
		if err != nil {
			return errors.WithStack(err)
		}

		// TODO: 任务的超时设置，后续作为动态配置，存储至 etcd
		// 提交异步处理任务到工作流管道
		// Retention(time.Hour): 任务在队列中保留1小时，确保被处理
		_, err = srv.pClient.EnqueueContext(ctx, task, asynq.Retention(time.Hour))
		if err != nil {
			return errors.WithStack(err)
		}
		totel.RecordFeedbackTaskEnqueued(ctx)
	}
}

func (srv *ReporterService) Run(ctx context.Context) (err error) {
	ctx, span := totel.StartRunSpan(ctx)
	defer totel.EndSpan(span, &err)

	// 在 goroutine 中启动 gRPC 服务
	errCh := make(chan error, 1)
	go func() {
		srv.logger.Ctx(ctx).Info("grpc server listening on")
		if err := srv.grpc.Serve(); err != nil {
			errCh <- errors.WithStack(err)
		}
	}()

	// 等待 context 取消或错误
	select {
	case <-ctx.Done():
		// 收到信号，开始优雅关闭
		srv.logger.Ctx(ctx).Info("grpc received shutdown signal")

		// 设置优雅停止超时定时器
		// 5秒后如果仍未优雅停止，强制停止服务器
		timer := time.AfterFunc(time.Second*5, func() {
			srv.logger.Ctx(ctx).Warn("server graceful stop timeout")
			srv.grpc.Stop() // 强制停止
		})
		defer timer.Stop()

		// 尝试优雅停止，等待正在处理的请求完成
		srv.grpc.GracefulStop()
		return nil
	case err := <-errCh:
		// 服务启动失败
		srv.logger.Fatal("failed to start grpc server", zap.Error(err))
		return err
	}
}
