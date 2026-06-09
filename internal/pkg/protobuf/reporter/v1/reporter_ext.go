package reporter_v1

import (
	"fmt"
)

type ResourceRaw struct {
	Namespace string `mapstructure:"namespace"`
	Name      string `mapstructure:"name"`
	Version   string `mapstructure:"version"`
}

func (r *ResourceRaw) String() string {
	return fmt.Sprintf("%s/%s:%s", r.Namespace, r.Name, r.Version)
}

func (r *Resource) ToRaw() *ResourceRaw {
	return &ResourceRaw{
		Namespace: r.Namespace,
		Name:      r.Name,
		Version:   r.Version,
	}
}

func (r *ResourceRaw) ToGRPC() *Resource {
	return &Resource{
		Namespace: r.Namespace,
		Name:      r.Name,
		Version:   r.Version,
	}
}
