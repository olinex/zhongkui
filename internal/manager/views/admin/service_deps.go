package admin

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/samber/do/v2"
	clientv3 "go.etcd.io/etcd/client/v3"
	"zhongkui.com/zhongkui/internal/manager/response"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/etcd"
	"zhongkui.com/zhongkui/internal/pkg/health"
	"zhongkui.com/zhongkui/internal/pkg/validator"
	"zhongkui.com/zhongkui/internal/provider"
)

// ServiceItemOut 单个服务实例运行状态输出项。
// 字段为基本类型，逐字段映射自 health.ServiceInfo，不引用模型层结构体。
// ServiceItemOut is an output item for a single service instance running state. Its
// fields are all primitive types mapped from health.ServiceInfo; it does not reference
// any model-layer struct.
type ServiceItemOut struct {
	// ServiceName 服务名（web / grpc / worker / capturer）
	// ServiceName the service name (web / grpc / worker / capturer)
	ServiceName string `json:"serviceName"`
	// InstanceID 实例 ID（host-pid）
	// InstanceID the instance identifier (host-pid)
	InstanceID string `json:"instanceId"`
	// Host 所在主机名
	// Host the host name
	Host string `json:"host"`
	// Version 服务版本
	// Version the service version
	Version string `json:"version"`
	// PID 进程 ID
	// PID the process id
	PID int `json:"pid"`
	// StartedAt 启动时刻
	// StartedAt the start time
	StartedAt time.Time `json:"startedAt"`
	// Live 存活状态
	// Live the live state
	Live bool `json:"live"`
	// Ready 就绪状态
	// Ready the ready state
	Ready bool `json:"ready"`
	// Started 启动完成状态
	// Started the startup-finished state
	Started bool `json:"started"`
}

// ServiceListOut 服务实例状态列表输出。
// ServiceListOut is the service instance status list output.
type ServiceListOut struct {
	// Items 运行中服务实例状态项
	// Items the running service instance status items
	Items []ServiceItemOut `json:"items"`
}

// newServiceItemOut 将 health.ServiceInfo 逐字段转换为视图层独立输出项。
// newServiceItemOut maps a health.ServiceInfo field-by-field to a standalone view-layer
// output item.
func newServiceItemOut(info health.ServiceInfo) ServiceItemOut {
	return ServiceItemOut{
		ServiceName: info.ServiceName,
		InstanceID:  info.InstanceID,
		Host:        info.Host,
		Version:     info.Version,
		PID:         info.PID,
		StartedAt:   info.StartedAt,
		Live:        info.Live,
		Ready:       info.Ready,
		Started:     info.Started,
	}
}

// ServiceListGet 处理服务状态列表请求：读取 etcd 服务目录，返回运行中服务实例列表。
// 由于需从注入器取 viper 与 etcd 客户端，返回 gin.HandlerFunc 闭包。
// ServiceListGet handles the service status list request, reading the etcd service
// directory and returning the running service instance list. Because it needs the viper
// and etcd client from the injector, it returns a gin.HandlerFunc closure.
func ServiceListGet(i do.Injector) gin.HandlerFunc {
	return func(c *gin.Context) {
		viper, err := provider.InvokeViper(i)
		if err != nil {
			response.CreateAccidentError(err).AbortWithGinH(c)
			return
		}
		client := provider.MustInvokeEtcdClient(i)
		cfg, err := health.ExtractConfig(viper, validator.Validate)
		if err != nil {
			response.CreateAccidentError(err).AbortWithGinH(c)
			return
		}

		ctx, cancel := context.WithTimeout(c, 5*time.Second)
		defer cancel()
		prefix := etcd.WithPrefix(cfg.Registry.KeyPrefix)
		resp, err := client.Get(ctx, prefix, clientv3.WithPrefix())
		if err != nil {
			response.CreateAccidentError(errors.WithStack(err)).AbortWithGinH(c)
			return
		}
		items := make([]ServiceItemOut, 0, len(resp.Kvs))
		for _, kv := range resp.Kvs {
			var info health.ServiceInfo
			if err := json.Unmarshal(kv.Value, &info); err != nil {
				continue
			}
			items = append(items, newServiceItemOut(info))
		}
		response.SimpleOK.AbortWithGinW(c, ServiceListOut{Items: items})
	}
}
