package capture

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/cenkalti/backoff/v5"
	"golang.org/x/sync/errgroup"
	"zhongkui.com/zhongkui/internal/capture/wire"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	reporter_v1 "zhongkui.com/zhongkui/internal/pkg/protobuf/reporter/v1"
	"zhongkui.com/zhongkui/internal/pkg/s3"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

// CaptureClient 抓包客户端，负责管理与远程服务器的通信、监控配置变化、执行数据包捕获和上传
// 该结构体是整个抓包系统的核心组件，负责：
// 1. 与远程Reporter服务建立gRPC连接，监控配置变化
// 2. 根据配置启动/停止数据包捕获任务
// 3. 管理数据包的上传流程（导出、压缩、上传到S3）
// 4. 定期向服务器发送状态反馈，维持连接健康状态
// 5. 实现重试机制，确保在网络波动或服务暂时不可用时的可靠性
//
// 设计原则：
// - 事件驱动：通过通道监听配置变更事件
// - 资源隔离：每个捕获会话有独立的资源管理
// - 错误恢复：内置重试和错误处理机制
// - 状态同步：与服务器保持状态一致性
type CaptureClient struct {
	// cfg 抓包配置，包含资源定义、监控参数、上传设置和反馈策略
	// 配置通过Viper框架从配置文件加载，支持热更新和动态调整
	// 包含心跳间隔、重试次数、缓冲区大小等关键参数
	cfg *Config

	// logger 日志记录器，用于记录客户端运行过程中的关键事件和错误信息
	// 支持不同级别的日志记录（DEBUG、INFO、WARN、ERROR），便于问题排查和系统监控
	logger *logging.Logger

	// client gRPC客户端，用于与远程Reporter服务进行双向流式通信
	// 负责配置监控（Watch）、状态反馈（Feedback）等核心功能
	// 通过gRPC流实现实时配置更新和状态同步
	client reporter_v1.ReporterServiceClient

	// s3 S3客户端，用于与S3兼容的对象存储服务进行交互
	// 负责将压缩后的数据包文件上传到对象存储，并设置过期时间
	// 支持多区域、多桶配置，确保数据持久性和可用性
	s3 *s3.S3Client
}

// NewCaptureClient 创建新的抓包客户端实例，初始化所有必要组件和通道
// 该函数是CaptureClient的主要构造函数，负责：
// 1. 初始化客户端核心组件（日志、gRPC客户端、S3客户端、配置）
// 2. 创建状态管理器和通信通道
// 3. 设置合理的通道容量，避免在高负载场景下发生阻塞
//
// 参数：
//
//	logger - 日志记录器实例，用于记录客户端运行日志
//	client - gRPC客户端实例，用于与远程Reporter服务通信
//	s3 - S3客户端实例，用于上传数据包文件到对象存储
//	cfg - 抓包配置实例，包含所有运行参数和策略设置
//
// 返回值：
//
//	*CaptureClient - 初始化完成的抓包客户端实例，可以直接启动使用
//
// 设计说明：
// - notifyChan容量为1：确保最新的配置快照能够及时传递，避免旧配置堆积
// - uploadChan容量为100：适应高频率上传场景，避免上传通知丢失
// - mutex初始化为空：等待第一次配置更新时设置初始状态
//
// 使用示例：
//
//	logger := logging.NewLogger()
//	grpcClient := reporter_v1.NewReporterServiceClient(conn)
//	s3Client := s3.NewS3Client(config)
//	cfg := capture.LoadConfig()
//	client := capture.NewCaptureClient(logger, grpcClient, s3Client, cfg)
func NewCaptureClient(logger *logging.Logger, client reporter_v1.ReporterServiceClient, s3 *s3.S3Client, cfg *Config) *CaptureClient {
	return &CaptureClient{
		logger: logger,
		client: client,
		s3:     s3,
		cfg:    cfg,
	}
}

// runWithRetry 使用指数退避算法执行函数，支持重试机制，提高系统可靠性
// 该函数实现了健壮的错误恢复机制，适用于网络通信、远程服务调用等可能失败的操作
// 核心特性：
// 1. 指数退避：重试间隔随时间指数增长，避免对服务造成雪崩效应
// 2. 最大重试次数：防止无限重试，在合理次数后放弃并返回错误
// 3. 超时控制：设置最大运行时间，避免长时间阻塞
// 4. 上下文感知：支持外部取消操作，及时释放资源
// 5. 重试通知：记录重试日志，便于监控和问题排查
//
// 参数：
//
//	ctx - 上下文对象，用于控制超时和取消操作
//	maxRetries - 最大重试次数，0表示不重试，正整数表示最多重试次数
//	fn - 需要执行的函数，接受上下文参数，返回错误表示执行失败
//
// 返回值：
//
//	error - 如果所有重试都失败，返回最后一次的错误（包含堆栈信息）
//	        成功时返回nil
//
// 重试策略配置：
// - 初始间隔：1秒，首次重试等待时间
// - 最大间隔：2秒，重试间隔的上限，避免等待时间过长
// - 最大运行时间：30秒，整个重试过程的最长时间限制
// - 重试通知：每次重试时记录日志，包含重试次数和间隔时间
//
// 使用场景：
// 1. gRPC连接建立失败时的重试
// 2. 网络波动导致的数据包上传失败
// 3. 远程服务暂时不可用时的操作重试
// 4. 资源竞争导致的临时性失败
//
// 错误处理：
// - 使用errors.WithStack包装错误，保留完整的调用堆栈信息
// - 支持上下文取消，及时中断重试过程
// - 记录重试日志，便于监控系统运行状态
func (cc *CaptureClient) runWithRetry(ctx context.Context, maxRetries uint, fn func(context.Context) error) error {
	// 创建指数退避策略实例
	// 指数退避算法：每次重试间隔 = 初始间隔 * 2^(重试次数-1)
	// 但不超过最大间隔限制，避免等待时间过长
	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = 1 * time.Second // 首次重试等待1秒
	expBackoff.MaxInterval = 2 * time.Second     // 最大重试间隔2秒

	// 执行带重试的函数调用
	// backoff.Retry会自动处理重试逻辑，包括间隔计算和错误检查
	_, err := backoff.Retry(
		ctx,
		func() (any, error) {
			// 检查上下文是否已被取消
			// 如果上下文已取消，立即返回错误，避免不必要的重试
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
				// 上下文正常，继续执行
			}
			// 调用用户提供的函数
			// 如果函数返回错误，backoff.Retry会根据配置决定是否重试
			return nil, fn(ctx)
		},
		// 配置重试策略参数
		backoff.WithBackOff(expBackoff),            // 使用指数退避策略
		backoff.WithMaxTries(maxRetries),           // 设置最大重试次数
		backoff.WithMaxElapsedTime(30*time.Second), // 设置最大运行时间30秒
		backoff.WithNotify(func(err error, interval time.Duration) {
			// 重试通知回调函数
			// 每次重试时记录日志，便于监控和问题排查
			cc.logger.Infof("retry %d times, interval: %s", maxRetries, interval)
		}),
	)

	// 使用errors.WithStack包装错误，保留完整的调用堆栈信息
	// 便于在错误发生时快速定位问题根源
	return errors.WithStack(err)
}

// watch 通过gRPC流监控远程配置变化，处理心跳和配置创建/修改/删除事件
// 该函数是CaptureClient的核心监控组件，负责：
// 1. 建立与远程Reporter服务的gRPC Watch流连接
// 2. 处理心跳消息，维持连接健康状态
// 3. 响应配置变更事件（创建、修改、删除）
// 4. 维护本地状态与远程状态的版本一致性
// 5. 检测异常情况（心跳超时、版本冲突）并采取相应措施
//
// 工作流程：
//  1. 创建Watch请求，包含资源标识、时间戳、心跳间隔和当前全局版本
//  2. 建立gRPC流连接，开始接收服务器推送的消息
//  3. 循环处理接收到的消息：
//     a. 心跳消息：更新本地心跳时间，检查心跳间隔是否超时
//     b. 配置事件：根据事件类型（创建/修改/删除）更新本地配置
//     c. 错误消息：立即返回错误，中断监控流程
//  4. 持续监控直到上下文取消或发生不可恢复的错误
//
// 参数：
//
//	ctx - 上下文对象，用于控制监控流程的超时和取消
//
// 返回值：
//
//	error - 如果发生错误（连接失败、流错误、配置错误等），返回包含堆栈信息的错误
//	        正常退出时返回nil（通常由上下文取消触发）
//
// 心跳机制：
// - 服务器定期发送心跳消息，客户端记录接收时间
// - 检查心跳间隔是否超过容忍阈值，超时则触发panic（严重错误）
// - 首次心跳时初始化本地心跳时间，后续心跳进行间隔检查
//
// 版本控制：
// - 全局版本（GlobalVersion）：服务器维护的全局递增版本号
// - 配置版本（ConfigVersion）：特定配置的版本号
// - 本地版本必须小于等于远程版本，否则表示状态不一致（触发panic）
//
// 事件处理：
// 1. CREATED：新配置创建，更新本地配置和版本信息
// 2. MODIFIED：配置修改，验证版本连续性后更新配置
// 3. DELETED：配置删除，重置本地状态，等待新配置
//
// 错误处理：
// - 连接失败：记录警告日志，返回包含堆栈信息的错误
// - 流错误：记录警告日志，返回包含堆栈信息的错误
// - 心跳超时：触发panic，表示严重状态不一致
// - 版本冲突：触发panic，表示严重状态不一致
// - 配置错误：记录错误日志，返回包含堆栈信息的错误
//
// 并发安全：
// - 通过mutex保护共享状态（配置、版本信息）
// - 使用通道进行事件通知，避免数据竞争
// - 支持多goroutine并发处理不同资源
func (cc *CaptureClient) watch(ctx context.Context, mutex *captureMutex) error {
	// 记录监控启动日志，包含资源标识信息
	// 便于跟踪监控流程和问题排查
	cc.logger.Infof("watch resource %s", cc.cfg.Resource.String())

	// 计算允许的心跳间隔容忍阈值
	// 用于检测心跳是否超时，防止连接假死
	allowInterval := cc.cfg.Watch.TolerateIntervalTimeout()

	// 获取当前本地状态快照
	// 快照机制确保在读取状态时不会发生数据竞争
	snapshot := mutex.snapshot()

	// 创建Watch请求，建立与服务器的gRPC流连接
	// 请求包含资源标识、当前时间戳、心跳间隔和本地全局版本
	stream, err := cc.client.Watch(ctx, &reporter_v1.WatchRequest{
		Resource:                 cc.cfg.Resource.ToGRPC(),                      // 资源标识（转换为gRPC格式）
		Timestamp:                time.Now().UnixMilli(),                        // 当前时间戳（毫秒）
		HeartbeatIntervalSeconds: int32(cc.cfg.Watch.IntervalTimeout.Seconds()), // 心跳间隔（秒）
		GlobalVersion:            snapshot.globalVersion,                        // 本地全局版本
	})
	if err != nil {
		// 连接建立失败，记录警告日志并返回错误
		// 上层调用者可以通过重试机制恢复
		cc.logger.Warnf("failed to create watch stream: %v", err)
		return errors.WithStack(err)
	}

	// 主监控循环，持续处理服务器推送的消息
	// 直到上下文取消或发生不可恢复的错误
	for {
		// 检查上下文是否已被取消
		// 如果已取消，立即退出监控循环
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// 上下文正常，继续处理消息
		}

		// 接收服务器推送的消息
		// Recv()是阻塞调用，直到收到消息或流关闭
		resp, err := stream.Recv()
		if err != nil {
			// 流错误，记录警告日志并返回错误
			// 可能是网络中断、服务器重启或协议错误
			cc.logger.Warnf("watch stream error: %v", err)
			return errors.WithStack(err)
		}

		// 根据消息类型进行不同的处理
		// 使用类型开关（type switch）区分心跳、事件和错误
		switch content := resp.Content.(type) {
		case *reporter_v1.WatchResponse_Error:
			// 服务器返回的错误消息
			// 表示请求参数错误、权限不足或其他业务错误
			errDetail := errors.New(content.Error.Detail)
			return errDetail

		case *reporter_v1.WatchResponse_Heartbeat:
			// 心跳消息，用于维持连接健康状态
			// 获取当前本地状态快照（可能已被其他goroutine更新）
			snapshot = mutex.snapshot()

			// 解析心跳时间戳
			curHeartbeat := time.UnixMilli(resp.Timestamp)

			// 记录心跳接收日志（调试级别）
			// 便于监控连接状态和问题排查
			cc.logger.Debugf(
				"receive current heartbeat: %s, previous heartbeat: %s",
				curHeartbeat.Format(time.RFC3339),
				snapshot.prevHeartbeat.Format(time.RFC3339),
			)

			// 如果是首次心跳，直接更新本地心跳时间
			// 不进行间隔检查（因为没有前一次心跳时间）
			if snapshot.prevHeartbeat.IsZero() {
				mutex.heartbeat(curHeartbeat)
				continue
			}

			// 计算实际心跳间隔
			interval := curHeartbeat.Sub(snapshot.prevHeartbeat)

			// 检查心跳间隔是否超过容忍阈值
			// 如果超时，表示连接可能假死或服务器异常
			if interval > allowInterval {
				// 触发panic，表示严重状态不一致
				// 需要人工干预或系统重启
				panic(fmt.Sprintf(
					"heartbeat interval %s must less than or equal to allow heartbeat interval %s",
					interval,
					allowInterval,
				))
			}

			// 心跳正常，更新本地心跳时间
			mutex.heartbeat(curHeartbeat)

		case *reporter_v1.WatchResponse_Event:
			// 配置变更事件，包括创建、修改、删除
			// 获取当前本地状态快照
			snapshot = mutex.snapshot()

			// 检查版本一致性：本地全局版本必须小于等于远程全局版本
			// 否则表示状态不一致（可能错过了某些更新）
			if snapshot.globalVersion >= resp.GlobalVersion {
				// 触发panic，表示严重状态不一致
				// 需要人工干预或系统重启
				panic(fmt.Sprintf(
					"local global version %d must less than remote global version %d",
					mutex.globalVersion,
					resp.GlobalVersion,
				))
			}

			// 根据事件类型进行不同的处理
			switch content.Event.Type {
			case reporter_v1.EventType_CREATED:
				// 新配置创建事件
				// 更新本地配置、配置版本和全局版本
				mutex.update(
					content.Event.CurSnapshot.Config,
					content.Event.CurSnapshot.ConfigVersion,
					resp.GlobalVersion,
				)

				// 记录配置创建日志
				cc.logger.Infof(
					"config created, config version: %d, global version: %d",
					content.Event.CurSnapshot.ConfigVersion,
					resp.GlobalVersion,
				)

			case reporter_v1.EventType_MODIFIED:
				// 配置修改事件
				// 验证版本连续性：本地配置版本必须等于事件中的前一个配置版本
				if snapshot.configVersion != content.Event.PrvSnapshot.ConfigVersion {
					// 触发panic，表示版本不连续
					// 可能错过了某些配置更新
					panic(fmt.Sprintf(
						"config version not match, expect %d, got %d",
						snapshot.configVersion,
						content.Event.PrvSnapshot.ConfigVersion,
					))
				}

				// 版本连续，更新本地配置、配置版本和全局版本
				mutex.update(
					content.Event.CurSnapshot.Config,
					content.Event.CurSnapshot.ConfigVersion,
					resp.GlobalVersion,
				)

				// 记录配置修改日志
				cc.logger.Infof(
					"config modified, config version: %d, global version: %d",
					content.Event.CurSnapshot.ConfigVersion,
					resp.GlobalVersion,
				)

			case reporter_v1.EventType_DELETED:
				// 配置删除事件
				// 重置本地状态，等待新配置
				mutex.reset()

				// 记录配置删除日志
				cc.logger.Infof(
					"config deleted, config version: %d, global version: %d",
					content.Event.PrvSnapshot.ConfigVersion,
					resp.GlobalVersion,
				)
			}
		}
	}
}

// uploadPacketHandler 处理数据包捕获、上传的主要逻辑，打开网络设备并实时捕获数据包
func (cc *CaptureClient) uploadPacketHandler(
	ctx context.Context,
	cause context.CancelCauseFunc,
	sessionID serializer.HexUUID,
	uploadChan chan time.Time,
	snapshot *captureSnapshot,
) {
	ws, err := wire.NewWire(snapshot.config, cc.logger)
	if err != nil {
		cause(errors.WithStack(err))
		return
	}
	defer ws.Close()
	cc.logger.Infof(
		"open wire(%s) device %s, snaplen: %d, promisc: %v, timeout: %s",
		ws.Name(),
		snapshot.config.Device,
		snapshot.config.SnapLen,
		snapshot.config.Promisc,
		time.Duration(snapshot.config.Timeout)*time.Second,
	)
	err = ws.SetBPFFilter(snapshot.config.BpfExpr)
	if err != nil {
		cause(errors.WithStack(err))
		return
	}
	cc.logger.Infof("set bpf filter %s", snapshot.config.BpfExpr)
	linkType := ws.LinkType()
	cc.logger.Infof("link type %s", linkType)
	tmpCapturer := capturer{
		resource:   &cc.cfg.Resource,
		uploadCfg:  &cc.cfg.Upload,
		captureCfg: snapshot.config,
		logger:     cc.logger,
		s3:         cc.s3,
		sessionID:  sessionID,
		linkType:   linkType,
	}
	err = tmpCapturer.capture(ctx, ws, uploadChan)
	if err != nil {
		cause(errors.WithStack(err))
	}
}

// upload 监听配置变更，当收到新配置时启动数据包捕获处理器
func (cc *CaptureClient) upload(ctx context.Context, sessionID serializer.HexUUID, mutex *captureMutex, uploadChan chan time.Time) error {
	err := os.MkdirAll(cc.cfg.Upload.DumpDir, 0755)
	if err != nil {
		return errors.WithStack(err)
	}
	watchNotifyTicker := time.NewTicker(time.Second * 5)
	defer watchNotifyTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case snapshot, open := <-mutex.snapshots():
			if !open {
				return errors.New("notify channel was closed before context done")
			}
			if snapshot.config == nil || !snapshot.config.Enabled {
				continue
			}
			innerCtx, innerCause := context.WithCancelCause(ctx)
			cc.logger.Infof(
				"upload resource %s/%s, config version: %d",
				cc.cfg.Resource.String(),
				sessionID,
				snapshot.configVersion,
			)
			go cc.uploadPacketHandler(
				innerCtx,
				innerCause,
				sessionID,
				uploadChan,
				snapshot,
			)
			mutex.asRunning()
		WATCH_LOOP:
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-innerCtx.Done():
					break WATCH_LOOP
				case <-watchNotifyTicker.C:
					if len(mutex.snapshots()) > 0 {
						innerCause(nil)
						break WATCH_LOOP
					}
				}
			}
			mutex.asStopped()
			if err := context.Cause(innerCtx); err == nil || errors.Is(err, context.Canceled) {
				cc.logger.Infof("upload packet handler exit, session id: %s", sessionID)
			} else {
				cc.logger.Warnf("recv report inner ctx err: %v, upgrade error to main context", err)
				return errors.WithStack(err)
			}
		}
	}
}

// feedback 通过gRPC流定期向服务器发送反馈，包含抓包状态和上传信息
func (cc *CaptureClient) feedback(ctx context.Context, sessionID serializer.HexUUID, mutex *captureMutex, uploadChan <-chan time.Time) error {
	ticker := time.NewTicker(cc.cfg.Feedback.IntervalTimeout)
	defer ticker.Stop()

	stream, err := cc.client.Feedback(ctx)
	if err != nil {
		cc.logger.Warnf("failed to create feedback stream: %v", err)
		return errors.WithStack(err)
	}

	for {
		var timestamp time.Time
		var uploaded bool
		select {
		case <-ctx.Done():
			stream.CloseAndRecv()
			return ctx.Err()
		case timestamp = <-ticker.C:
			uploaded = false
		case timestamp = <-uploadChan:
			uploaded = true
		}

		snapshot := mutex.snapshot()
		err := stream.Send(&reporter_v1.FeedbackRequest{
			Timestamp: timestamp.UnixMilli(),
			Resource:  cc.cfg.Resource.ToGRPC(),
			SessionId: sessionID.String(),
			Status: &reporter_v1.Status{
				Type:          snapshot.status,
				ConfigVersion: snapshot.configVersion,
				GlobalVersion: snapshot.globalVersion,
			},
			TimeoutSeconds: int64(cc.cfg.Feedback.TolerateIntervalTimeout().Seconds()),
			Uploaded:       uploaded,
		})
		if err != nil {
			cc.logger.Warnf("feedback stream error: %v", err)
			stream.CloseAndRecv()
			return errors.WithStack(err)
		}
	}
}

func (cc *CaptureClient) Run(ctx context.Context, sessionID serializer.HexUUID) error {
	// mutex 互斥锁管理器，用于保护共享状态和确保线程安全
	// 提供对配置状态、版本信息等共享资源的读写保护
	// 支持快照机制，避免在状态读取时发生数据竞争
	mutex := newCaptureMutex()
	uploadChan := make(chan time.Time, 100)
	wg, ctx := errgroup.WithContext(ctx)
	wg.Go(func() error {
		return cc.runWithRetry(ctx, cc.cfg.Watch.MaxRetries, func(ctx context.Context) error {
			return cc.watch(ctx, mutex)
		})
	})
	wg.Go(func() error {
		return cc.runWithRetry(ctx, cc.cfg.Feedback.MaxRetries, func(ctx context.Context) error {
			return cc.feedback(ctx, sessionID, mutex, uploadChan)
		})
	})
	wg.Go(func() error {
		return cc.runWithRetry(ctx, cc.cfg.Upload.MaxRetries, func(ctx context.Context) error {
			return cc.upload(ctx, sessionID, mutex, uploadChan)
		})
	})
	return wg.Wait()
}
