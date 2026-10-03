package admin

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/samber/do/v2"
	clientv3 "go.etcd.io/etcd/client/v3"
	"zhongkuisec.com/zhongkui/internal/manager/response"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/etcd"
	"zhongkuisec.com/zhongkui/internal/pkg/health"
	"zhongkuisec.com/zhongkui/internal/pkg/validator"
	"zhongkuisec.com/zhongkui/internal/provider"
)

// ServiceItemOut 单个服务实例运行状态输出项。
// 字段为基本类型，逐字段映射自 health.ServiceInfo，不引用模型层结构体。
type ServiceItemOut struct {
	// ServiceName 服务名（web / grpc / worker / capturer）
	ServiceName string `json:"serviceName"`
	// InstanceID 实例 ID（host-pid）
	InstanceID string `json:"instanceId"`
	// Host 所在主机名
	Host string `json:"host"`
	// Version 服务版本
	Version string `json:"version"`
	// PID 进程 ID
	PID int `json:"pid"`
	// StartedAt 启动时刻
	StartedAt time.Time `json:"startedAt"`
	// Live 存活状态
	Live bool `json:"live"`
	// Ready 就绪状态
	Ready bool `json:"ready"`
	// Started 启动完成状态
	Started bool `json:"started"`
}

// ServiceListOut 服务实例状态列表输出。
type ServiceListOut struct {
	// Items 运行中服务实例状态项
	Items []ServiceItemOut `json:"items"`
}

// newServiceItemOut 将 health.ServiceInfo 逐字段转换为视图层独立输出项。
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
