package grpc

import (
	"net"
	"net/http"

	g "google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// Server 是gRPC服务器的包装器，提供统一的服务器管理接口
// 它封装了gRPC服务器的生命周期管理、健康检查和服务注册功能
type Server struct {
	// srv 是底层的gRPC服务器实例
	srv *g.Server
	// lis 是服务器监听的网络监听器
	lis net.Listener
	// hthName 是健康检查服务的名称
	hthName string
	// hthSrv 是gRPC健康检查服务器实例
	hthSrv *health.Server
	// TODO: logger
	// 未来可以添加日志记录器，用于记录服务器操作和错误
}

// GetServiceInfo 返回服务器上注册的所有服务的信息
// 该方法包装了底层gRPC服务器的GetServiceInfo方法
// 返回：
//   - map[string]g.ServiceInfo: 服务名称到服务信息的映射
func (s *Server) GetServiceInfo() map[string]g.ServiceInfo {
	return s.srv.GetServiceInfo()
}

// GracefulStop 优雅地停止gRPC服务器
// 服务器将停止接受新连接，并等待现有请求完成
// 该方法包装了底层gRPC服务器的GracefulStop方法
func (s *Server) GracefulStop() {
	s.srv.GracefulStop()
}

// RegisterService 在gRPC服务器上注册一个服务
// 该方法用于将gRPC服务描述符和实现注册到服务器
// 参数：
//   - desc: gRPC服务描述符，定义服务的接口和方法
//   - impl: 服务实现实例，必须实现描述符中定义的所有方法
func (s *Server) RegisterService(desc *g.ServiceDesc, impl any) {
	s.srv.RegisterService(desc, impl)
}

// Serve 启动gRPC服务器并开始接受连接
// 该方法会阻塞，直到服务器停止
// 返回：
//   - error: 如果服务器启动失败，返回相应的错误
func (s *Server) Serve() error {
	return s.srv.Serve(s.lis)
}

// ServeHTTP 允许gRPC服务器处理HTTP请求
// 这支持gRPC-Web或其他需要HTTP兼容性的场景
// 参数：
//   - w: HTTP响应写入器
//   - r: HTTP请求对象
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.srv.ServeHTTP(w, r)
}

// Stop 立即停止gRPC服务器
// 与GracefulStop不同，Stop会立即中断所有活动连接
// 该方法包装了底层gRPC服务器的Stop方法
func (s *Server) Stop() {
	s.srv.Stop()
}

// SetServingStatus 设置健康检查服务的状态
// 该方法用于更新服务器的健康状态，可用于标记服务器为维护中或不健康
// 参数：
//   - status: 健康状态枚举值，如SERVING、NOT_SERVING等
func (s *Server) SetServingStatus(status healthpb.HealthCheckResponse_ServingStatus) {
	if s.hthSrv == nil {
		return
	}
	s.hthSrv.SetServingStatus(s.hthName, status)
}

// NewServer 创建一个新的gRPC服务器实例
// 该方法根据配置初始化gRPC服务器，支持TLS、身份验证、健康检查等功能
// 参数：
//   - authenticators: 身份验证器映射，键为验证器名称，值为Authenticator接口实现
//   - cfg: 服务器配置，包含网络、TLS、身份验证、保活和健康检查等设置
//
// 返回：
//   - *Server: 初始化后的gRPC服务器实例
//   - error: 如果初始化失败，返回相应的错误
func NewServer(authenticators map[string]Authenticator, cfg *ServerConfig, extraOpts ...g.ServerOption) (*Server, error) {
	// 根据配置创建监听器和服务器选项
	lis, options, err := cfg.ToOptions(authenticators)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	options = append(options, extraOpts...)

	// 创建gRPC服务器实例
	srv := g.NewServer(options...)

	// 启用gRPC反射服务，便于客户端发现服务接口
	reflection.Register(srv)

	var (
		hthName string
		hthSrv  *health.Server
	)

	// 如果启用了健康检查，注册健康检查服务
	if cfg != nil && cfg.Health.Enabled {
		hthName = cfg.Health.ServiceName
		hthSrv = health.NewServer()
		healthpb.RegisterHealthServer(srv, hthSrv)
		// 设置初始健康状态为SERVING
		hthSrv.SetServingStatus(cfg.Health.ServiceName, healthpb.HealthCheckResponse_SERVING)
	}

	// 返回配置好的服务器实例
	return &Server{
		srv:     srv,
		lis:     lis,
		hthName: hthName,
		hthSrv:  hthSrv,
	}, nil
}
