// Package reporter_v1 提供抓包上报服务 protobuf 生成代码的扩展：以 mapstructure 友好的
// ResourceRaw 承载资源标识（命名空间/名称/版本），并在 ResourceRaw 与生成的 Resource
// 之间提供双向转换与字符串格式化。
package reporter_v1

import (
	"fmt"
)

// ResourceRaw 是资源标识的 mapstructure 友好表示，用于从配置中解析资源信息。
type ResourceRaw struct {
	// Namespace 资源所属的命名空间
	Namespace string `mapstructure:"namespace"`
	// Name 资源名称
	Name string `mapstructure:"name"`
	// Version 资源版本
	Version string `mapstructure:"version"`
}

// String 返回资源标识的 "namespace/name:version" 字符串表示。
func (r *ResourceRaw) String() string {
	return fmt.Sprintf("%s/%s:%s", r.Namespace, r.Name, r.Version)
}

// ToRaw 将生成的 Resource 转换为 mapstructure 友好的 ResourceRaw。
func (r *Resource) ToRaw() *ResourceRaw {
	return &ResourceRaw{
		Namespace: r.Namespace,
		Name:      r.Name,
		Version:   r.Version,
	}
}

// ToGRPC 将 ResourceRaw 转换为生成的 Resource。
func (r *ResourceRaw) ToGRPC() *Resource {
	return &Resource{
		Namespace: r.Namespace,
		Name:      r.Name,
		Version:   r.Version,
	}
}
