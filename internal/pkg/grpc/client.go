// Package grpc 提供gRPC客户端连接创建和管理的功能。
// 本文件包含创建gRPC客户端连接的核心函数，通过ClientConfig配置生成gRPC连接。
// 主要功能是将配置转换为gRPC连接选项，并建立与服务器的连接。
package grpc

import (
	g "google.golang.org/grpc"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// Client 是gRPC客户端连接的类型别名。
// 它是对google.golang.org/grpc.ClientConn的包装，提供与gRPC服务器的连接。
// 通过该连接可以调用远程服务的方法。
type Client = g.ClientConn

// NewClient 根据给定的配置创建一个新的gRPC客户端连接。
// 这是创建gRPC客户端的主要入口点，它将ClientConfig配置转换为gRPC连接选项，
// 然后建立与指定服务器的连接。
//
// 函数执行流程：
// 1. 调用cfg.ToOptions()将配置转换为监听地址和连接选项
// 2. 使用gRPC库的NewClient函数创建客户端连接
// 3. 错误处理：将底层错误包装为带堆栈信息的错误
//
// 参数：
//   - cfg: *ClientConfig - gRPC客户端配置，包含服务器地址、TLS设置、身份验证等信息
//     如果为nil，函数将使用默认的空配置（不建立实际连接）
//
// 返回：
//   - *Client: 成功创建的gRPC客户端连接，可用于调用远程服务
//   - error: 创建连接过程中发生的错误，包括：
//   - 配置转换错误（如无效的TLS证书）
//   - 连接建立错误（如网络不可达、服务器无响应）
//   - 参数错误（如无效的服务器地址格式）
//
// 使用示例：
//
//	config := &ClientConfig{
//	    BaseConfig: BaseConfig{
//	        Host: "localhost",
//	        Port: 8080,
//	    },
//	    TLS: TlsClientConfig{
//	        Enabled: true,
//	        Domain: "localhost",
//	        Cert: "PEM encoded certificate",
//	    },
//	}
//	client, err := NewClient(config)
//	if err != nil {
//	    log.Fatal("Failed to create gRPC client:", err)
//	}
//	defer client.Close()
func NewClient(cfg *ClientConfig) (*Client, error) {
	// 将ClientConfig转换为gRPC连接选项
	lis, options, err := cfg.ToOptions()
	if err != nil {
		// 包装配置转换错误，保留堆栈信息
		return nil, errors.WithStack(err)
	}

	// 使用转换后的地址和选项创建gRPC客户端连接
	conn, err := g.NewClient(lis, options...)
	if err != nil {
		// 包装连接建立错误，保留堆栈信息
		return nil, errors.WithStack(err)
	}

	// 返回成功创建的客户端连接
	return conn, nil
}
