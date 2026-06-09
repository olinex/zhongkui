package flow

import "github.com/hibiken/asynq"

const (
	// PERIODIC_IPV4_REASSEMBLE IPv4 周期性 TCP 流重组任务的类型名称
	PERIODIC_IPV4_REASSEMBLE = "network:periodic:ipv4reassemble"
	// PERIODIC_IPV6_REASSEMBLE IPv6 周期性 TCP 流重组任务的类型名称
	PERIODIC_IPV6_REASSEMBLE = "network:periodic:ipv6reassemble"
)

func NewIPv4ReassembleTask() *asynq.Task {
	return asynq.NewTask(PERIODIC_IPV4_REASSEMBLE, nil, asynq.MaxRetry(0))
}

func NewIPv6ReassembleTask() *asynq.Task {
	return asynq.NewTask(PERIODIC_IPV6_REASSEMBLE, nil, asynq.MaxRetry(0))
}
