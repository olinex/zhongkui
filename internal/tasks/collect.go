package tasks

import (
	"zhongkui.com/zhongkui/internal/tasks/pcap"
	"zhongkui.com/zhongkui/internal/tasks/workers"
)

func init() {
	// 注册采集任务的 worker 到全局 worker 注册表中
	workers.WorkerRegister(pcap.PCAP_IMPORT, pcap.NewPcapBatchTaskWorker())
}
